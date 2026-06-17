#!/usr/bin/env ruby
# frozen_string_literal: true

require "digest"
require "json"
require "open3"
require "optparse"
require "time"

module RedisHAMigrationRehearsal
  VERSION = 1
  DEFAULT_NAMESPACE = "mpp-system"
  DEFAULT_SOURCE_RESOURCE = "statefulset/redis"
  DEFAULT_SOURCE_CONTAINER = "redis"
  DEFAULT_TARGET_RESOURCE = "statefulset/redis-ha-primary"
  DEFAULT_TARGET_CONTAINER = "redis"
  DEFAULT_TARGET_HOST = "redis-ha-primary"
  DEFAULT_TARGET_PORT = 6379
  DEFAULT_TARGET_DB = 0
  DEFAULT_MIGRATE_TIMEOUT_MS = 5_000
  DEFAULT_SCAN_MATCH = "*"
  DEFAULT_SCAN_COUNT = 100
  DEFAULT_MAX_KEYS = 10_000
  DEFAULT_SAMPLE_LIMIT = 20
  DEFAULT_TTL_TOLERANCE_MS = 5_000
  PRODUCTION_ENVS = %w[prod production].freeze

  class CommandError < StandardError; end

  module_function

  def run(argv)
    CLI.new(argv).run
  end

  def build_report(config:, app_config:, started_at:, completed_at:, source:, target:, imported:, skipped_counts:, samples:, warnings:)
    value_matches = samples.count { |sample| sample.fetch("value_match") == true }
    ttl_mismatches = samples.count { |sample| sample.fetch("ttl_status") == "mismatch" }
    key_count_diff = target.fetch(:dbsize_after) - source.fetch(:keys_observed)
    status = key_count_diff.zero? && value_matches == samples.length && ttl_mismatches.zero? ? "pass" : "review"

    {
      "version" => VERSION,
      "generated_at" => completed_at.iso8601,
      "status" => status,
      "namespace" => config.fetch(:namespace),
      "safety" => {
        "app_env" => app_config.fetch(:app_env),
        "redis_endpoint_mode" => app_config.fetch(:redis_endpoint_mode),
        "redis_addr" => app_config.fetch(:redis_addr),
        "no_endpoint_cutover" => true,
        "production_refused" => true,
        "write_operations" => target.fetch(:flushed) ? ["target FLUSHDB", "source MIGRATE COPY REPLACE"] : ["source MIGRATE COPY REPLACE"],
      },
      "source" => {
        "resource" => config.fetch(:source_resource),
        "container" => config.fetch(:source_container),
        "dbsize_before" => source.fetch(:dbsize_before),
        "scan_match" => config.fetch(:scan_match),
        "scan_count" => config.fetch(:scan_count),
        "max_keys" => config.fetch(:max_keys),
        "keys_observed" => source.fetch(:keys_observed),
      },
      "target" => {
        "resource" => config.fetch(:target_resource),
        "container" => config.fetch(:target_container),
        "dbsize_before" => target.fetch(:dbsize_before),
        "flushed_before_import" => target.fetch(:flushed),
        "dbsize_after" => target.fetch(:dbsize_after),
      },
      "summary" => {
        "elapsed_seconds" => (completed_at - started_at).round(3),
        "keys_imported" => imported.length,
        "keys_skipped" => skipped_counts.values.sum,
        "sampled_keys" => samples.length,
        "sample_value_matches" => value_matches,
        "sample_ttl_mismatches" => ttl_mismatches,
        "source_vs_target_key_count_diff" => key_count_diff,
        "ttl_tolerance_ms" => config.fetch(:ttl_tolerance_ms),
      },
      "skipped_counts" => skipped_counts,
      "samples" => samples,
      "warnings" => warnings,
    }
  end

  class CLI
    def initialize(argv)
      @argv = argv
      @config = {
        namespace: ENV.fetch("MPP_APP_NS", DEFAULT_NAMESPACE),
        source_resource: ENV.fetch("MPP_REDIS_MIGRATION_SOURCE_RESOURCE", DEFAULT_SOURCE_RESOURCE),
        source_container: ENV.fetch("MPP_REDIS_MIGRATION_SOURCE_CONTAINER", DEFAULT_SOURCE_CONTAINER),
        target_resource: ENV.fetch("MPP_REDIS_MIGRATION_TARGET_RESOURCE", DEFAULT_TARGET_RESOURCE),
        target_container: ENV.fetch("MPP_REDIS_MIGRATION_TARGET_CONTAINER", DEFAULT_TARGET_CONTAINER),
        target_host: ENV.fetch("MPP_REDIS_MIGRATION_TARGET_HOST", DEFAULT_TARGET_HOST),
        target_port: positive_integer(ENV.fetch("MPP_REDIS_MIGRATION_TARGET_PORT", DEFAULT_TARGET_PORT).to_i, "MPP_REDIS_MIGRATION_TARGET_PORT"),
        target_db: non_negative_integer(ENV.fetch("MPP_REDIS_MIGRATION_TARGET_DB", DEFAULT_TARGET_DB).to_i, "MPP_REDIS_MIGRATION_TARGET_DB"),
        migrate_timeout_ms: positive_integer(ENV.fetch("MPP_REDIS_MIGRATION_TIMEOUT_MS", DEFAULT_MIGRATE_TIMEOUT_MS).to_i, "MPP_REDIS_MIGRATION_TIMEOUT_MS"),
        scan_match: ENV.fetch("MPP_REDIS_MIGRATION_MATCH", DEFAULT_SCAN_MATCH),
        scan_count: positive_integer(ENV.fetch("MPP_REDIS_MIGRATION_SCAN_COUNT", DEFAULT_SCAN_COUNT).to_i, "MPP_REDIS_MIGRATION_SCAN_COUNT"),
        max_keys: positive_integer(ENV.fetch("MPP_REDIS_MIGRATION_MAX_KEYS", DEFAULT_MAX_KEYS).to_i, "MPP_REDIS_MIGRATION_MAX_KEYS"),
        sample_limit: non_negative_integer(ENV.fetch("MPP_REDIS_MIGRATION_SAMPLE_LIMIT", DEFAULT_SAMPLE_LIMIT).to_i, "MPP_REDIS_MIGRATION_SAMPLE_LIMIT"),
        ttl_tolerance_ms: non_negative_integer(ENV.fetch("MPP_REDIS_MIGRATION_TTL_TOLERANCE_MS", DEFAULT_TTL_TOLERANCE_MS).to_i, "MPP_REDIS_MIGRATION_TTL_TOLERANCE_MS"),
        allow_target_flush: env_flag?("MPP_REDIS_MIGRATION_ALLOW_TARGET_FLUSH"),
        report_path: ENV["MPP_REDIS_MIGRATION_REPORT"],
      }
    end

    def run
      parser.parse!(@argv)
      started_at = Time.now.utc
      preflight

      app_config = app_config_values
      source_dbsize_before = redis_integer(:source, "DBSIZE")
      target_dbsize_before = redis_integer(:target, "DBSIZE")
      target_flushed = flush_target_if_needed(target_dbsize_before)
      keys = scan_source_keys
      imported = []
      skipped_counts = Hash.new(0)

      keys.each do |key|
        result = import_key(key)
        if result
          imported << result
        else
          skipped_counts["expired_during_export"] += 1
        end
      rescue CommandError => e
        skipped_counts["restore_error"] += 1
        warn "skip #{key.inspect}: #{e.message}"
      end

      target_dbsize_after = redis_integer(:target, "DBSIZE")
      samples = validate_samples(imported.first(@config.fetch(:sample_limit)))
      completed_at = Time.now.utc
      warnings = []
      warnings << "scan stopped at max_keys=#{@config.fetch(:max_keys)}; rerun with a higher limit if this was not intentional" if keys.length >= @config.fetch(:max_keys)
      warnings << "target key count differs from source key count" unless target_dbsize_after == source_dbsize_before

      report = RedisHAMigrationRehearsal.build_report(
        config: @config,
        app_config: app_config,
        started_at: started_at,
        completed_at: completed_at,
        source: {dbsize_before: source_dbsize_before, keys_observed: keys.length},
        target: {dbsize_before: target_dbsize_before, flushed: target_flushed, dbsize_after: target_dbsize_after},
        imported: imported,
        skipped_counts: skipped_counts,
        samples: samples,
        warnings: warnings,
      )
      output = JSON.pretty_generate(report)
      write_report(output)
      puts output
      report.fetch("status") == "pass" ? 0 : 1
    rescue OptionParser::ParseError, CommandError, KeyError => e
      warn e.message
      1
    end

    private

    def parser
      OptionParser.new do |opts|
        opts.banner = "Usage: ruby script/kubernetes/redis-ha-migration-rehearsal.rb [options]"
        opts.separator ""
        opts.separator "Runs the non-production Redis HA migration rehearsal with MIGRATE and emits a count and TTL diff report."
        opts.separator ""
        opts.on("--namespace NAMESPACE", "Kubernetes namespace. Defaults to MPP_APP_NS or #{DEFAULT_NAMESPACE}.") { |value| @config[:namespace] = value }
        opts.on("--source-resource RESOURCE", "Existing Redis resource. Defaults to #{DEFAULT_SOURCE_RESOURCE}.") { |value| @config[:source_resource] = value }
        opts.on("--target-resource RESOURCE", "HA Redis target resource. Defaults to #{DEFAULT_TARGET_RESOURCE}.") { |value| @config[:target_resource] = value }
        opts.on("--target-host HOST", "Target Redis Service used by MIGRATE. Defaults to #{DEFAULT_TARGET_HOST}.") { |value| @config[:target_host] = value }
        opts.on("--target-port PORT", Integer, "Target Redis port. Defaults to #{DEFAULT_TARGET_PORT}.") { |value| @config[:target_port] = positive_integer(value, "target-port") }
        opts.on("--target-db DB", Integer, "Target Redis database. Defaults to #{DEFAULT_TARGET_DB}.") { |value| @config[:target_db] = non_negative_integer(value, "target-db") }
        opts.on("--migrate-timeout-ms MS", Integer, "Per-key MIGRATE timeout. Defaults to #{DEFAULT_MIGRATE_TIMEOUT_MS}.") { |value| @config[:migrate_timeout_ms] = positive_integer(value, "migrate-timeout-ms") }
        opts.on("--match PATTERN", "Source SCAN pattern. Defaults to #{DEFAULT_SCAN_MATCH.inspect}.") { |value| @config[:scan_match] = value }
        opts.on("--scan-count COUNT", Integer, "Source SCAN COUNT hint. Defaults to #{DEFAULT_SCAN_COUNT}.") { |value| @config[:scan_count] = positive_integer(value, "scan-count") }
        opts.on("--max-keys COUNT", Integer, "Stop after this many source keys. Defaults to #{DEFAULT_MAX_KEYS}.") { |value| @config[:max_keys] = positive_integer(value, "max-keys") }
        opts.on("--sample-limit COUNT", Integer, "Validate this many restored values and TTLs. Defaults to #{DEFAULT_SAMPLE_LIMIT}.") { |value| @config[:sample_limit] = non_negative_integer(value, "sample-limit") }
        opts.on("--ttl-tolerance-ms MS", Integer, "Accepted TTL drift for sampled keys. Defaults to #{DEFAULT_TTL_TOLERANCE_MS}.") { |value| @config[:ttl_tolerance_ms] = non_negative_integer(value, "ttl-tolerance-ms") }
        opts.on("--allow-target-flush", "Discard existing HA Redis rehearsal data before import. Same as MPP_REDIS_MIGRATION_ALLOW_TARGET_FLUSH=1.") { @config[:allow_target_flush] = true }
        opts.on("--report PATH", "Also write the JSON report to PATH.") { |value| @config[:report_path] = value }
        opts.on("-h", "--help", "Show help.") do
          puts opts
          exit 0
        end
      end
    end

    def preflight
      require_kubectl
      app_config = app_config_values
      app_env = app_config.fetch(:app_env)
      raise CommandError, "refusing to run against APP_ENV=#{app_env}" if PRODUCTION_ENVS.include?(app_env.downcase)
      raise CommandError, "mpp-app-config APP_ENV must be set before running this rehearsal" if app_env.empty?
      unless app_config.fetch(:redis_endpoint_mode) == "direct" && app_config.fetch(:redis_addr) == "redis:6379"
        raise CommandError, "rehearsal must run before endpoint cutover with REDIS_ENDPOINT_MODE=direct and REDIS_ADDR=redis:6379"
      end

      require_can_i("create", "pods/exec")
      rollout_status("statefulset/redis")
      rollout_status("statefulset/redis-ha-primary")
      rollout_status("statefulset/redis-ha-replica")
      rollout_status("statefulset/redis-ha-sentinel")
      expect_redis_ping(:source)
      expect_redis_ping(:target)
      @config[:redis_password] = redis_password
      target_replication = redis_text(:target, "INFO", "replication")
      raise CommandError, "target #{@config.fetch(:target_resource)} must be the current HA Redis master" unless target_replication.include?("role:master")
      redis_text(:source, "CLIENT", "LIST")
    end

    def require_kubectl
      _stdout, _stderr, status = Open3.capture3("kubectl", "version", "--client=true")
      raise CommandError, "kubectl is required" unless status.success?
    rescue Errno::ENOENT
      raise CommandError, "kubectl is required"
    end

    def require_can_i(verb, resource)
      stdout = kubectl("auth", "can-i", verb, resource, "-n", @config.fetch(:namespace)).strip
      raise CommandError, "kubectl user cannot #{verb} #{resource} in namespace #{@config.fetch(:namespace)}" unless stdout == "yes"
    end

    def rollout_status(resource)
      kubectl("rollout", "status", resource, "-n", @config.fetch(:namespace), "--timeout=300s")
    end

    def app_config_values
      {
        app_env: configmap_value("APP_ENV"),
        redis_endpoint_mode: configmap_value("REDIS_ENDPOINT_MODE"),
        redis_addr: configmap_value("REDIS_ADDR"),
      }
    end

    def configmap_value(key)
      kubectl("get", "configmap", "mpp-app-config", "-n", @config.fetch(:namespace), "-o", "jsonpath={.data.#{key}}").strip
    rescue CommandError
      ""
    end

    def expect_redis_ping(endpoint)
      pong = redis_text(endpoint, "PING").strip
      raise CommandError, "#{endpoint} Redis did not answer PONG" unless pong == "PONG"
    end

    def flush_target_if_needed(target_dbsize_before)
      return false if target_dbsize_before.zero?

      unless @config.fetch(:allow_target_flush)
        raise CommandError, "target HA Redis has #{target_dbsize_before} keys; set MPP_REDIS_MIGRATION_ALLOW_TARGET_FLUSH=1 or pass --allow-target-flush to discard rehearsal data"
      end

      redis_text(:target, "FLUSHDB")
      true
    end

    def redis_password
      source_password = pod_env_value(:source, "REDIS_PASSWORD")
      target_password = pod_env_value(:target, "REDIS_PASSWORD")

      if !source_password.empty? && !target_password.empty? && source_password != target_password
        raise CommandError, "source and target Redis passwords differ; rehearsal expects the same secret on both ends"
      end

      target_password.empty? ? source_password : target_password
    end

    def pod_env_value(endpoint, name)
      shell_exec(endpoint, "printf '%s' \"\${#{name}:-}\"")
    end

    def scan_source_keys
      keys = []
      cursor = "0"
      loop do
        raw = redis_text(:source, "SCAN", cursor, "MATCH", @config.fetch(:scan_match), "COUNT", @config.fetch(:scan_count).to_s)
        lines = raw.lines.map(&:chomp)
        raise CommandError, "source Redis SCAN returned no cursor" if lines.empty?

        cursor = lines.fetch(0)
        lines.drop(1).reject(&:empty?).each do |key|
          keys << key
          return keys.sort if keys.length >= @config.fetch(:max_keys)
        end
        break if cursor == "0"
      end
      keys.sort
    end

    def import_key(key)
      ttl_before = redis_integer(:source, "PTTL", key)
      return nil if ttl_before == -2 || ttl_before.zero?

      dump = redis_binary(:source, "DUMP", key)
      return nil if dump.empty?

      migrate_key(key)
      {
        "key" => key,
        "type" => redis_text(:source, "TYPE", key).strip,
        "ttl_ms_source_before" => ttl_before,
        "source_dump_sha256" => Digest::SHA256.hexdigest(dump),
      }
    end

    def migrate_key(key)
      redis_text(
        :source,
        "MIGRATE",
        @config.fetch(:target_host),
        @config.fetch(:target_port).to_s,
        "",
        @config.fetch(:target_db).to_s,
        @config.fetch(:migrate_timeout_ms).to_s,
        *migration_auth_args,
        "COPY",
        "REPLACE",
        "KEYS",
        key,
      )
    end

    def migration_auth_args
      password = @config.fetch(:redis_password, "").to_s
      return [] if password.empty?

      ["AUTH", password]
    end

    def validate_samples(imported_samples)
      imported_samples.map do |entry|
        key = entry.fetch("key")
        target_exists = redis_integer(:target, "EXISTS", key) == 1
        target_ttl = target_exists ? redis_integer(:target, "PTTL", key) : -2
        target_dump = target_exists ? redis_binary(:target, "DUMP", key) : +""
        target_hash = target_dump.empty? ? nil : Digest::SHA256.hexdigest(target_dump)
        source_ttl_before = entry.fetch("ttl_ms_source_before")

        entry.merge(
          "target_value_present" => target_exists,
          "target_dump_sha256" => target_hash,
          "value_match" => target_hash == entry.fetch("source_dump_sha256"),
          "ttl_ms_target_after" => target_ttl,
          "ttl_delta_ms" => ttl_delta(source_ttl_before, target_ttl),
          "ttl_status" => ttl_status(source_ttl_before, target_ttl),
        )
      rescue CommandError => e
        entry.merge(
          "target_value_present" => false,
          "target_dump_sha256" => nil,
          "value_match" => false,
          "ttl_ms_target_after" => nil,
          "ttl_delta_ms" => nil,
          "ttl_status" => "error",
          "error" => e.message,
        )
      end
    end

    def ttl_delta(source_ttl_before, target_ttl_after)
      return 0 if source_ttl_before == -1 && target_ttl_after == -1
      return nil if source_ttl_before.nil? || target_ttl_after.nil? || target_ttl_after.negative?

      source_ttl_before - target_ttl_after
    end

    def ttl_status(source_ttl_before, target_ttl_after)
      return "preserved_no_expire" if source_ttl_before == -1 && target_ttl_after == -1
      return "missing" if target_ttl_after == -2
      return "mismatch" if source_ttl_before == -1 || target_ttl_after == -1

      delta = ttl_delta(source_ttl_before, target_ttl_after)
      return "mismatch" if delta.nil? || delta.negative?
      return "mismatch" if delta > @config.fetch(:ttl_tolerance_ms)

      "within_tolerance"
    end

    def redis_integer(endpoint, *command)
      Integer(redis_text(endpoint, *command).strip)
    rescue ArgumentError
      raise CommandError, "#{endpoint} Redis #{command.first} returned a non-integer value"
    end

    def redis_binary(endpoint, *command)
      redis_exec(endpoint, command, binmode: true)
    end

    def redis_text(endpoint, *command)
      redis_exec(endpoint, command, binmode: false)
    end

    def redis_exec(endpoint, command, binmode:)
      resource, container = endpoint_config(endpoint)
      script = <<~'SH'
        if [ -n "${REDIS_PASSWORD:-}" ]; then
          export REDISCLI_AUTH="$REDIS_PASSWORD"
        fi
        exec redis-cli --no-auth-warning --raw "$@"
      SH
      args = [
        "exec",
        "-n",
        @config.fetch(:namespace),
        resource,
        "-c",
        container,
        "--",
      ]
      args.concat(["sh", "-ec", script, "redis-cli", *command])
      kubectl(*args, binmode: binmode)
    end

    def shell_exec(endpoint, script)
      resource, container = endpoint_config(endpoint)
      kubectl(
        "exec",
        "-n",
        @config.fetch(:namespace),
        resource,
        "-c",
        container,
        "--",
        "sh",
        "-ec",
        script,
      )
    end

    def endpoint_config(endpoint)
      case endpoint
      when :source
        [@config.fetch(:source_resource), @config.fetch(:source_container)]
      when :target
        [@config.fetch(:target_resource), @config.fetch(:target_container)]
      else
        raise KeyError, "unknown Redis endpoint #{endpoint.inspect}"
      end
    end

    def kubectl(*args, binmode: false)
      stdout, stderr, status = Open3.capture3(
        "kubectl",
        *args,
        stdin_data: "",
        binmode: binmode,
      )
      raise CommandError, "kubectl #{args.join(' ')} failed: #{stderr.strip.empty? ? stdout.strip : stderr.strip}" unless status.success?

      stdout
    end

    def write_report(output)
      path = @config[:report_path].to_s
      return if path.empty?

      File.write(path, "#{output}\n")
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
  exit RedisHAMigrationRehearsal.run(ARGV)
end
