#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "open3"
require "optparse"
require "shellwords"
require "time"
require "timeout"

module RedisKeyspaceInventory
  DEFAULT_REDIS_ADDR = "redis:6379"
  DEFAULT_BATCH_SIZE = 100
  DEFAULT_MAX_KEYS = 10_000
  DEFAULT_SAMPLE_LIMIT = 3
  DEFAULT_SCAN_MATCH = "*"
  DEFAULT_COMMAND_TIMEOUT_SECONDS = 5
  VERSION = 1

  KeySample = Struct.new(:key, :type, :ttl_ms, :memory_bytes, keyword_init: true)

  RESPONSIBILITY_TIERS = {
    "R0" => "critical coordination",
    "R1" => "user continuity",
    "R2" => "performance cache",
    "R3" => "ephemeral signal",
    "R4" => "queue-like usage",
  }.freeze

  AUTH_VERIFICATION_RESPONSIBILITY = {
    responsibility_tier: "R1",
    loss_tolerance: "Loss is tolerable only by restarting the pending verification step.",
    recovery_expectation: "Prompt the user to request a fresh code or retry the auth flow; PostgreSQL remains authoritative for durable account state.",
  }.freeze

  RATE_LIMIT_RESPONSIBILITY = {
    responsibility_tier: "R3",
    loss_tolerance: "Loss may temporarily reset throttling counters and allow extra requests.",
    recovery_expectation: "Allow counters to rebuild from new traffic and watch abuse or cost alerts during the affected window.",
  }.freeze

  STREAM_GATE_RESPONSIBILITY = {
    responsibility_tier: "R0",
    loss_tolerance: "Loss can admit more concurrent streams than configured until leases rebuild.",
    recovery_expectation: "Keep existing streams alive where possible; new admissions recreate leases while expired members are pruned by score.",
  }.freeze

  BROWSER_COORDINATION_RESPONSIBILITY = {
    responsibility_tier: "R0",
    loss_tolerance: "Loss can permit duplicate browser sessions or quota over-admission for a user or tenant.",
    recovery_expectation: "Use session rows, worker lookups, and heartbeat checks to expire stale sessions before admitting replacements.",
  }.freeze

  BROWSER_SESSION_RESPONSIBILITY = {
    responsibility_tier: "R1",
    loss_tolerance: "Loss interrupts live remote-browser continuity but does not delete durable account or project data.",
    recovery_expectation: "Mark missing or stale live sessions expired and have the user start a new browser session.",
  }.freeze

  BROWSER_STREAM_TOKEN_RESPONSIBILITY = {
    responsibility_tier: "R1",
    loss_tolerance: "Loss invalidates the pending browser stream handoff for that session.",
    recovery_expectation: "Issue a new short-lived stream token from the durable browser session row when the session is still active.",
  }.freeze

  BROWSER_CLEANUP_RESPONSIBILITY = {
    responsibility_tier: "R3",
    loss_tolerance: "Loss delays Redis cleanup indexing for expired browser sessions.",
    recovery_expectation: "Rely on per-session TTLs and database expiry checks; recreate cleanup members as sessions are refreshed or replaced.",
  }.freeze

  BROWSER_HEARTBEAT_RESPONSIBILITY = {
    responsibility_tier: "R3",
    loss_tolerance: "Loss can make a live worker look stale until the next heartbeat refresh.",
    recovery_expectation: "Treat the heartbeat as a liveness hint and verify the worker directly before expiring a session.",
  }.freeze

  PUBLISH_LOCK_RESPONSIBILITY = {
    responsibility_tier: "R0",
    loss_tolerance: "Loss can allow duplicate publish execution for the same project and platform.",
    recovery_expectation: "Re-check publication state and PostgreSQL outbox records before running work; workers reacquire the lock before processing.",
  }.freeze

  DASHBOARD_CACHE_RESPONSIBILITY = {
    responsibility_tier: "R2",
    loss_tolerance: "Loss causes a cold cache or short-lived stale read protection fallback.",
    recovery_expectation: "Recompute from PostgreSQL on cache miss; generation counters and short TTLs bound stale cache exposure.",
  }.freeze

  OAUTH2_STATE_RESPONSIBILITY = {
    responsibility_tier: "R1",
    loss_tolerance: "Loss invalidates a pending X OAuth2 authorization callback.",
    recovery_expectation: "Ask the user to restart account connection; no durable platform account credential is committed before callback success.",
  }.freeze

  ASYNQ_QUEUE_RESPONSIBILITY = {
    responsibility_tier: "R4",
    loss_tolerance: "Queued task loss is not tolerated unless the durable domain record can replay the work.",
    recovery_expectation: "Replay publish work from PostgreSQL outbox or scheduled publication state; manually assess email and read-model tasks if queue data is lost.",
  }.freeze

  DECLARED_PATTERNS = [
    {
      pattern: "auth:code:{scene}:{email_hash}",
      regex: /\Aauth:code:[^:]+:[0-9a-f]{64}\z/,
      owner: "backend auth handler",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "10 minutes",
      notes: "Email verification and password reset code.",
    }.merge(AUTH_VERIFICATION_RESPONSIBILITY),
    {
      pattern: "auth:code_attempts:{scene}:{email_hash}",
      regex: /\Aauth:code_attempts:[^:]+:[0-9a-f]{64}\z/,
      owner: "backend auth handler",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "10 minutes after first failed attempt",
      notes: "Failed verification attempt counter.",
    }.merge(AUTH_VERIFICATION_RESPONSIBILITY),
    {
      pattern: "auth:last_send:{scene}:{email_hash}",
      regex: /\Aauth:last_send:[^:]+:[0-9a-f]{64}\z/,
      owner: "backend auth handler",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "60 seconds",
      notes: "Verification email resend throttle.",
    }.merge(AUTH_VERIFICATION_RESPONSIBILITY),
    {
      pattern: "mpp:ratelimit:{scope}:{identifier}:{category}:{bucket}",
      regex: /\Ampp:ratelimit:.+\z/,
      owner: "backend rate-limit middleware",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "request bucket window, usually 1 minute or 24 hours",
      notes: "Application API rate limit counters.",
    }.merge(RATE_LIMIT_RESPONSIBILITY),
    {
      pattern: "mpp:stream:conn:{connection_id}",
      regex: /\Ampp:stream:conn:[0-9a-f]{32}\z/,
      owner: "backend stream gate",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "stream lease TTL, defaults to 10 minutes for AI and 16 minutes for browser streams",
      notes: "Individual stream lease payload.",
    }.merge(STREAM_GATE_RESPONSIBILITY),
    {
      pattern: "mpp:stream:{kind}:user:{user_id}",
      regex: /\Ampp:stream:[^:]+:user:[^:]+\z/,
      owner: "backend stream gate",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "members expire by score; key may have no Redis TTL",
      notes: "Per-user stream concurrency zset.",
    }.merge(STREAM_GATE_RESPONSIBILITY),
    {
      pattern: "mpp:stream:{kind}:tenant:{tenant_id}",
      regex: /\Ampp:stream:[^:]+:tenant:[^:]+\z/,
      owner: "backend stream gate",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "members expire by score; key may have no Redis TTL",
      notes: "Per-tenant stream concurrency zset.",
    }.merge(STREAM_GATE_RESPONSIBILITY),
    {
      pattern: "mpp:stream:{kind}:ip:{ip_hash}",
      regex: /\Ampp:stream:[^:]+:ip:[0-9a-f]{64}\z/,
      owner: "backend stream gate",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "members expire by score; key may have no Redis TTL",
      notes: "Per-IP stream concurrency zset.",
    }.merge(STREAM_GATE_RESPONSIBILITY),
    {
      pattern: "mpp:stream:{kind}:global",
      regex: /\Ampp:stream:[^:]+:global\z/,
      owner: "backend stream gate",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "members expire by score; key may have no Redis TTL",
      notes: "Global stream concurrency zset.",
    }.merge(STREAM_GATE_RESPONSIBILITY),
    {
      pattern: "mpp:browser:active:{user_id}:{platform}",
      regex: /\Ampp:browser:active:[0-9a-f-]{36}:[^:]+\z/,
      owner: "backend browser session service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "browser session TTL plus 1 minute grace",
      notes: "One active remote browser session per user and platform.",
    }.merge(BROWSER_COORDINATION_RESPONSIBILITY),
    {
      pattern: "mpp:browser:session:{session_id}",
      regex: /\Ampp:browser:session:[0-9a-f-]{36}\z/,
      owner: "backend/browser-worker browser session state",
      reads: ["backend", "browser-worker"],
      writes: ["backend", "browser-worker"],
      ttl_policy: "browser session TTL plus 1 minute grace",
      notes: "Remote browser live session JSON state.",
    }.merge(BROWSER_SESSION_RESPONSIBILITY),
    {
      pattern: "mpp:browser:stream-token:{session_id}:{token_hash}",
      regex: /\Ampp:browser:stream-token:[0-9a-f-]{36}:[^:]+\z/,
      owner: "backend browser session service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "min(5 minutes, remaining session TTL)",
      notes: "Single-use browser stream token metadata.",
    }.merge(BROWSER_STREAM_TOKEN_RESPONSIBILITY),
    {
      pattern: "mpp:browser:stream-current:{session_id}",
      regex: /\Ampp:browser:stream-current:[0-9a-f-]{36}\z/,
      owner: "backend browser session service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "min(5 minutes, remaining session TTL)",
      notes: "Pointer to current browser stream token hash.",
    }.merge(BROWSER_STREAM_TOKEN_RESPONSIBILITY),
    {
      pattern: "mpp:browser:cleanup",
      regex: /\Ampp:browser:cleanup\z/,
      owner: "backend browser session service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "no Redis TTL; members scored by session expiration",
      notes: "Sorted-set cleanup index for expired browser sessions.",
    }.merge(BROWSER_CLEANUP_RESPONSIBILITY),
    {
      pattern: "mpp:browser:worker-heartbeat:{worker_session_ref}",
      regex: /\Ampp:browser:worker-heartbeat:.+\z/,
      owner: "browser-worker session state",
      reads: ["backend"],
      writes: ["browser-worker"],
      ttl_policy: "45 seconds",
      notes: "Browser worker liveness heartbeat.",
    }.merge(BROWSER_HEARTBEAT_RESPONSIBILITY),
    {
      pattern: "mpp:browser:quota:user:{user_id}",
      regex: /\Ampp:browser:quota:user:[0-9a-f-]{36}\z/,
      owner: "backend browser session service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "browser session TTL plus 1 minute grace",
      notes: "Per-user remote browser session concurrency zset.",
    }.merge(BROWSER_COORDINATION_RESPONSIBILITY),
    {
      pattern: "mpp:browser:quota:tenant:{tenant_id}",
      regex: /\Ampp:browser:quota:tenant:[^:]+\z/,
      owner: "backend browser session service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "browser session TTL plus 1 minute grace",
      notes: "Per-tenant remote browser session concurrency zset.",
    }.merge(BROWSER_COORDINATION_RESPONSIBILITY),
    {
      pattern: "mpp:publish:lock:{project_id}:{platform}",
      regex: /\Ampp:publish:lock:[0-9a-f-]{36}:[^:]+\z/,
      owner: "backend publish service",
      reads: ["backend", "publish-worker"],
      writes: ["backend", "publish-worker"],
      ttl_policy: "30 minutes, refreshed while publishing",
      notes: "Publish job idempotency and mutual-exclusion lock.",
    }.merge(PUBLISH_LOCK_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:projects:list:v2:{params_hash}",
      regex: /\Ampp:dashboard:projects:list:v2:[0-9a-f]{64}\z/,
      owner: "backend project service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "15 seconds",
      notes: "Dashboard project list cache.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:projects:list-generation:v2",
      regex: /\Ampp:dashboard:projects:list-generation:v2\z/,
      owner: "backend project service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "no Redis TTL",
      notes: "Project list cache generation counter.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:content-setup:v1:{resource}:user:{user_id}:workspace:{workspace_id}:generation:{generation}",
      regex: /\Ampp:dashboard:content-setup:v1:(content-templates|brand-profiles):user:[0-9a-f-]{36}:workspace:[0-9a-f-]{36}:generation:.+\z/,
      owner: "backend project service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "15 seconds",
      notes: "Content setup options cache.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:content-setup:v1:generation:{resource}:user:{user_id}",
      regex: /\Ampp:dashboard:content-setup:v1:generation:[^:]+:user:[0-9a-f-]{36}\z/,
      owner: "backend project service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "no Redis TTL",
      notes: "Per-user content setup cache generation counter.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:content-setup:v1:generation:{resource}:workspace:{workspace_id}",
      regex: /\Ampp:dashboard:content-setup:v1:generation:[^:]+:workspace:[0-9a-f-]{36}\z/,
      owner: "backend project service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "no Redis TTL",
      notes: "Per-workspace content setup cache generation counter.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:stats:global:v1:{generation}",
      regex: /\Ampp:dashboard:stats:global:v1:[^:]+\z/,
      owner: "backend stats service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "15 seconds",
      notes: "Global dashboard stats cache.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:stats:user:v1:{user_id}:{generation}",
      regex: /\Ampp:dashboard:stats:user:v1:[0-9a-f-]{36}:[^:]+\z/,
      owner: "backend stats service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "15 seconds",
      notes: "User-scoped dashboard stats cache.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:stats:workspace:v1:{workspace_id}:{generation}",
      regex: /\Ampp:dashboard:stats:workspace:v1:[0-9a-f-]{36}:[^:]+\z/,
      owner: "backend stats service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "15 seconds",
      notes: "Workspace-scoped dashboard stats cache.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:stats-generation:v1",
      regex: /\Ampp:dashboard:stats-generation:v1\z/,
      owner: "backend stats service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "no Redis TTL",
      notes: "Global stats cache generation counter.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:stats-generation:scoped:v1",
      regex: /\Ampp:dashboard:stats-generation:scoped:v1\z/,
      owner: "backend stats service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "no Redis TTL",
      notes: "Scoped stats cache generation counter.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:accounts:v1:{workspace_id}:{platform}",
      regex: /\Ampp:dashboard:accounts:v1:[0-9a-f-]{36}:[^:]+\z/,
      owner: "backend platform account service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "15 seconds",
      notes: "Dashboard platform account cache.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "mpp:x_oauth2_state:{state}",
      regex: /\Ampp:x_oauth2_state:.+\z/,
      owner: "backend platform account service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "10 minutes",
      notes: "Pending X OAuth2 state and PKCE verifier.",
    }.merge(OAUTH2_STATE_RESPONSIBILITY),
    {
      pattern: "mpp:dashboard:media-assets:resolve:v1:{asset_id}:actor:{user_id}",
      regex: /\Ampp:dashboard:media-assets:resolve:v1:[0-9a-f-]{36}:actor:[0-9a-f-]{36}\z/,
      owner: "backend media asset service",
      reads: ["backend"],
      writes: ["backend"],
      ttl_policy: "15 seconds or shorter than signed URL expiry",
      notes: "Resolved media asset URL cache.",
    }.merge(DASHBOARD_CACHE_RESPONSIBILITY),
    {
      pattern: "asynq:{queue}:*",
      regex: /\Aasynq:\{[^}]+\}:.+\z/,
      owner: "asynq task queues used by backend workers",
      reads: ["backend", "publish-worker"],
      writes: ["backend", "publish-worker"],
      ttl_policy: "asynq-managed; task retention and uniqueness TTLs vary by queue",
      notes: "Email, publish, and dashboard read-model queue internals.",
    }.merge(ASYNQ_QUEUE_RESPONSIBILITY),
    {
      pattern: "asynq:*",
      regex: /\Aasynq:(?!\{).+\z/,
      owner: "asynq task queues used by backend workers",
      reads: ["backend", "publish-worker"],
      writes: ["backend", "publish-worker"],
      ttl_policy: "asynq-managed; process, worker, scheduler, and queue metadata may not use Redis TTLs",
      notes: "Global Asynq queue registry plus server and worker process metadata.",
    }.merge(ASYNQ_QUEUE_RESPONSIBILITY),
  ].freeze

  module_function

  def report(samples, options = {})
    normalized = samples.map { |sample| normalize_sample(sample) }.sort_by(&:key)
    inventory_options = {
      source: options.fetch(:source, "fixture"),
      generated_at: options[:generated_at] || Time.now.utc.iso8601,
      scan_match: options.fetch(:scan_match, DEFAULT_SCAN_MATCH),
      scan_count: options.fetch(:scan_count, DEFAULT_BATCH_SIZE),
      max_keys: options.fetch(:max_keys, DEFAULT_MAX_KEYS),
      sample_limit: options.fetch(:sample_limit, DEFAULT_SAMPLE_LIMIT),
    }

    grouped = group_samples(normalized, inventory_options.fetch(:sample_limit))
    {
      "version" => VERSION,
      "generated_at" => inventory_options.fetch(:generated_at),
      "source" => inventory_options.fetch(:source),
      "safety" => {
        "redis_operations" => ["SCAN", "TYPE", "PTTL", "MEMORY USAGE"],
        "scan_match" => inventory_options.fetch(:scan_match),
        "scan_count" => inventory_options.fetch(:scan_count),
        "max_keys" => inventory_options.fetch(:max_keys),
        "sample_limit_per_pattern" => inventory_options.fetch(:sample_limit),
        "write_operations" => [],
      },
      "responsibility_tiers" => RESPONSIBILITY_TIERS,
      "summary" => {
        "keys_observed" => normalized.length,
        "patterns_observed" => grouped.length,
        "memory_bytes_observed" => normalized.sum { |sample| sample.memory_bytes || 0 },
      },
      "patterns" => grouped,
      "warnings" => warnings_for(normalized, inventory_options.fetch(:max_keys)),
    }
  end

  def load_fixture(path)
    raw = JSON.parse(File.read(path))
    entries = raw.is_a?(Hash) ? raw.fetch("keys") : raw
    entries.map { |entry| normalize_sample(entry) }
  end

  def normalize_sample(entry)
    return entry if entry.is_a?(KeySample)

    entry = stringify_keys(entry)
    KeySample.new(
      key: entry.fetch("key").to_s,
      type: entry.fetch("type", "unknown").to_s,
      ttl_ms: integer_or_nil(entry["ttl_ms"]),
      memory_bytes: integer_or_nil(entry["memory_bytes"]),
    )
  end

  def group_samples(samples, sample_limit)
    samples.each_with_object({}) do |sample, groups|
      metadata = metadata_for(sample.key)
      pattern = metadata.fetch(:pattern)
      groups[pattern] ||= new_group(pattern, metadata)
      add_sample(groups[pattern], sample, sample_limit)
    end.values.sort_by { |group| group.fetch("pattern") }
  end

  def new_group(pattern, metadata)
    {
      "pattern" => pattern,
      "owner" => metadata.fetch(:owner),
      "owner_source" => metadata.fetch(:owner_source),
      "reads" => metadata.fetch(:reads),
      "writes" => metadata.fetch(:writes),
      "responsibility_tier" => metadata.fetch(:responsibility_tier),
      "responsibility_label" => responsibility_label(metadata.fetch(:responsibility_tier)),
      "loss_tolerance" => metadata.fetch(:loss_tolerance),
      "recovery_expectation" => metadata.fetch(:recovery_expectation),
      "ttl_policy" => metadata.fetch(:ttl_policy),
      "redis_types" => Hash.new(0),
      "key_count" => 0,
      "observed_ttl_ms" => {
        "min" => nil,
        "max" => nil,
        "without_expire_count" => 0,
        "unknown_count" => 0,
      },
      "memory_bytes" => {
        "total" => 0,
        "known_count" => 0,
        "unknown_count" => 0,
        "min" => nil,
        "max" => nil,
      },
      "samples" => [],
      "notes" => metadata.fetch(:notes),
    }
  end

  def add_sample(group, sample, sample_limit)
    group["key_count"] += 1
    group["redis_types"][sample.type] += 1
    add_ttl(group.fetch("observed_ttl_ms"), sample.ttl_ms)
    add_memory(group.fetch("memory_bytes"), sample.memory_bytes)

    return unless group.fetch("samples").length < sample_limit

    group.fetch("samples") << {
      "key" => sample.key,
      "type" => sample.type,
      "ttl_ms" => sample.ttl_ms,
      "memory_bytes" => sample.memory_bytes,
    }
  end

  def add_ttl(stats, ttl_ms)
    case ttl_ms
    when nil
      stats["unknown_count"] += 1
    when -1
      stats["without_expire_count"] += 1
    when -2
      stats["unknown_count"] += 1
    else
      stats["min"] = ttl_ms if stats["min"].nil? || ttl_ms < stats["min"]
      stats["max"] = ttl_ms if stats["max"].nil? || ttl_ms > stats["max"]
    end
  end

  def add_memory(stats, memory_bytes)
    if memory_bytes.nil?
      stats["unknown_count"] += 1
      return
    end

    stats["total"] += memory_bytes
    stats["known_count"] += 1
    stats["min"] = memory_bytes if stats["min"].nil? || memory_bytes < stats["min"]
    stats["max"] = memory_bytes if stats["max"].nil? || memory_bytes > stats["max"]
  end

  def metadata_for(key)
    declared = DECLARED_PATTERNS.find { |pattern| key.match?(pattern.fetch(:regex)) }
    return declared.merge(owner_source: "declared") if declared

    {
      pattern: inferred_pattern(key),
      owner: "unknown",
      owner_source: "inferred",
      reads: [],
      writes: [],
      responsibility_tier: "unclassified",
      loss_tolerance: "unknown",
      recovery_expectation: "Review the observed pattern before assigning a Redis responsibility tier.",
      ttl_policy: "unknown",
      notes: "Pattern inferred from observed key shape; review owner and TTL policy.",
    }
  end

  def responsibility_label(tier)
    RESPONSIBILITY_TIERS.fetch(tier, "unclassified")
  end

  def inferred_pattern(key)
    key.split(":").map { |part| generalized_token(part) }.join(":")
  end

  def generalized_token(part)
    case part
    when /\A[0-9a-f]{64}\z/i
      "{sha256}"
    when /\A[0-9a-f]{32}\z/i
      "{hex32}"
    when /\A[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\z/i
      "{uuid}"
    when /\A\d+\z/
      "{number}"
    else
      part
    end
  end

  def warnings_for(samples, max_keys)
    warnings = []
    warnings << "scan stopped at max_keys=#{max_keys}; rerun with a higher limit if this was not intentional" if samples.length >= max_keys
    warnings << "no keys observed" if samples.empty?
    unknown_count = group_samples(samples, 0).count { |group| group.fetch("owner_source") == "inferred" }
    warnings << "#{unknown_count} inferred key patterns need owner review" if unknown_count.positive?
    warnings
  end

  def stringify_keys(value)
    value.each_with_object({}) { |(key, entry_value), hash| hash[key.to_s] = entry_value }
  end

  def integer_or_nil(value)
    return nil if value.nil? || value == ""

    Integer(value)
  rescue ArgumentError, TypeError
    nil
  end

  class RedisCliScanner
    def initialize(addr:, password:, db:, tls:, scan_match:, scan_count:, max_keys:, command_timeout_seconds:)
      @addr = addr
      @password = password
      @db = db
      @tls = tls
      @scan_match = scan_match
      @scan_count = scan_count
      @max_keys = max_keys
      @command_timeout_seconds = command_timeout_seconds
    end

    def scan
      samples = []
      base_args = redis_cli_base_args
      cursor = "0"
      loop do
        cursor, keys = scan_page(base_args, cursor)
        keys.each do |key|
          samples << key_sample(base_args, key)
          return samples if samples.length >= @max_keys
        end
        break if cursor == "0"
      end
      samples
    end

    private

    def redis_cli_base_args
      args = ["redis-cli", "-u", redis_url, "--raw"]
      args << "--tls" if @tls && !redis_url.start_with?("rediss://")
      args
    end

    def scan_page(base_args, cursor)
      raw = redis_value(base_args, "SCAN", cursor, "MATCH", @scan_match, "COUNT", @scan_count.to_s)
      raise "redis-cli SCAN returned no output" if raw.nil? || raw.empty?

      lines = raw.lines.map(&:chomp)
      [lines.fetch(0), lines.drop(1).reject(&:empty?)]
    end

    def redis_url
      raw = @addr.to_s.strip
      scheme = @tls ? "rediss" : "redis"
      return with_db(raw) if raw.start_with?("redis://") || raw.start_with?("rediss://")

      with_db("#{scheme}://#{raw}")
    end

    def with_db(url)
      return url if @db.nil? || @db.to_s.empty?

      uri = url.dup
      uri = "#{uri}/" unless uri.match?(%r{/\d+\z})
      uri.sub(%r{/\z}, "/#{@db}")
    end

    def key_sample(base_args, key)
      KeySample.new(
        key: key,
        type: redis_value(base_args, "TYPE", key) || "unknown",
        ttl_ms: RedisKeyspaceInventory.integer_or_nil(redis_value(base_args, "PTTL", key)),
        memory_bytes: RedisKeyspaceInventory.integer_or_nil(redis_value(base_args, "MEMORY", "USAGE", key)),
      )
    end

    def redis_value(base_args, *command)
      stdout, stderr, status = Timeout.timeout(@command_timeout_seconds) do
        Open3.capture3(
          {"REDISCLI_AUTH" => @password.to_s},
          *base_args,
          *command,
          binmode: true,
          stdin_data: "",
        )
      end
      return nil unless status.success?

      stdout.to_s.strip
    rescue StandardError => e
      warn "redis-cli #{command.first} failed for #{key_for_warning(command)}: #{e.message}"
      nil
    end

    def key_for_warning(command)
      command.last.to_s.shellescape
    end
  end

  class CLI
    def self.run(argv)
      new(argv).run
    end

    def initialize(argv)
      @options = {
        addr: ENV.fetch("REDIS_ADDR", DEFAULT_REDIS_ADDR),
        password: ENV.fetch("REDIS_PASSWORD", ""),
        db: ENV.fetch("REDIS_DB", "0"),
        tls: env_flag?("REDIS_TLS"),
        scan_match: DEFAULT_SCAN_MATCH,
        scan_count: DEFAULT_BATCH_SIZE,
        max_keys: DEFAULT_MAX_KEYS,
        sample_limit: DEFAULT_SAMPLE_LIMIT,
        command_timeout_seconds: DEFAULT_COMMAND_TIMEOUT_SECONDS,
      }
      @argv = argv
    end

    def run
      parser.parse!(@argv)
      samples = if @options[:fixture]
                  RedisKeyspaceInventory.load_fixture(@options.fetch(:fixture))
                else
                  RedisCliScanner.new(**scanner_options).scan
                end

      generated_report = RedisKeyspaceInventory.report(samples, report_options)
      puts JSON.pretty_generate(generated_report)
      0
    rescue OptionParser::ParseError, KeyError, JSON::ParserError, RuntimeError => e
      warn e.message
      1
    end

    private

    def parser
      OptionParser.new do |opts|
        opts.banner = "Usage: ruby script/redis/keyspace_inventory.rb [options]"
        opts.on("--redis-addr ADDR", "Redis host:port or redis:// URL. Defaults to REDIS_ADDR or #{DEFAULT_REDIS_ADDR}.") do |value|
          @options[:addr] = value
        end
        opts.on("--redis-password PASSWORD", "Redis password. Defaults to REDIS_PASSWORD.") do |value|
          @options[:password] = value
        end
        opts.on("--redis-db DB", "Redis database number. Defaults to REDIS_DB or 0.") do |value|
          @options[:db] = value
        end
        opts.on("--tls", "Use TLS when REDIS_ADDR is not already a rediss:// URL.") do
          @options[:tls] = true
        end
        opts.on("--match PATTERN", "SCAN match pattern. Defaults to #{DEFAULT_SCAN_MATCH.inspect}.") do |value|
          @options[:scan_match] = value
        end
        opts.on("--scan-count COUNT", Integer, "SCAN COUNT hint. Defaults to #{DEFAULT_BATCH_SIZE}.") do |value|
          @options[:scan_count] = positive_integer(value, "scan-count")
        end
        opts.on("--max-keys COUNT", Integer, "Stop after this many observed keys. Defaults to #{DEFAULT_MAX_KEYS}.") do |value|
          @options[:max_keys] = positive_integer(value, "max-keys")
        end
        opts.on("--sample-limit COUNT", Integer, "Keep this many sample keys per pattern. Defaults to #{DEFAULT_SAMPLE_LIMIT}.") do |value|
          @options[:sample_limit] = non_negative_integer(value, "sample-limit")
        end
        opts.on("--command-timeout SECONDS", Integer, "Per-command timeout for TYPE/PTTL/MEMORY. Defaults to #{DEFAULT_COMMAND_TIMEOUT_SECONDS}.") do |value|
          @options[:command_timeout_seconds] = positive_integer(value, "command-timeout")
        end
        opts.on("--fixture PATH", "Read sampled key metadata from JSON instead of Redis.") do |value|
          @options[:fixture] = value
        end
        opts.on("-h", "--help", "Show help.") do
          puts opts
          exit 0
        end
      end
    end

    def scanner_options
      {
        addr: @options.fetch(:addr),
        password: @options.fetch(:password),
        db: @options.fetch(:db),
        tls: @options.fetch(:tls),
        scan_match: @options.fetch(:scan_match),
        scan_count: @options.fetch(:scan_count),
        max_keys: @options.fetch(:max_keys),
        command_timeout_seconds: @options.fetch(:command_timeout_seconds),
      }
    end

    def report_options
      {
        source: @options[:fixture] ? "fixture:#{@options.fetch(:fixture)}" : "redis:#{redacted_addr}",
        scan_match: @options.fetch(:scan_match),
        scan_count: @options.fetch(:scan_count),
        max_keys: @options.fetch(:max_keys),
        sample_limit: @options.fetch(:sample_limit),
      }
    end

    def redacted_addr
      @options.fetch(:addr).to_s.sub(%r{://[^/@]+@}, "://redacted@")
    end

    def positive_integer(value, name)
      raise OptionParser::InvalidArgument, "#{name} must be positive" unless value.positive?

      value
    end

    def non_negative_integer(value, name)
      raise OptionParser::InvalidArgument, "#{name} must be non-negative" if value.negative?

      value
    end

    def env_flag?(name)
      %w[1 true yes y on].include?(ENV.fetch(name, "").strip.downcase)
    end
  end
end

if $PROGRAM_NAME == __FILE__
  exit RedisKeyspaceInventory::CLI.run(ARGV)
end
