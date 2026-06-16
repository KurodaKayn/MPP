# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "open3"
require "rbconfig"
require "fileutils"
require "tmpdir"

require_relative "keyspace_inventory"

module RedisKeyspaceInventory
  class KeyspaceInventoryTest < Minitest::Test
    FIXTURE = File.expand_path("fixtures/keyspace_inventory_sample.json", __dir__)

    def test_groups_declared_patterns_with_stable_report_fields
      samples = RedisKeyspaceInventory.load_fixture(FIXTURE)
      report = RedisKeyspaceInventory.report(
        samples,
        generated_at: "2026-06-16T00:00:00Z",
        source: "fixture",
        max_keys: 100,
        sample_limit: 1,
      )

      assert_equal 1, report.fetch("version")
      assert_equal "fixture", report.fetch("source")
      assert_equal 8, report.dig("summary", "keys_observed")
      assert_equal 7, report.dig("summary", "patterns_observed")
      assert_empty report.fetch("warnings").grep(/scan stopped/)

      project_cache = pattern(report, "mpp:dashboard:projects:list:v2:{params_hash}")
      assert_equal "backend project service", project_cache.fetch("owner")
      assert_equal "declared", project_cache.fetch("owner_source")
      assert_equal 2, project_cache.fetch("key_count")
      assert_equal({"string" => 2}, project_cache.fetch("redis_types"))
      assert_equal 10_000, project_cache.dig("observed_ttl_ms", "min")
      assert_equal 14_000, project_cache.dig("observed_ttl_ms", "max")
      assert_equal 2_048, project_cache.dig("memory_bytes", "total")
      assert_equal 1, project_cache.fetch("samples").length

      cleanup = pattern(report, "mpp:browser:cleanup")
      assert_equal "zset", cleanup.fetch("redis_types").keys.fetch(0)
      assert_equal 1, cleanup.dig("observed_ttl_ms", "without_expire_count")
    end

    def test_infers_unknown_patterns_without_hiding_them
      report = RedisKeyspaceInventory.report([
        {"key" => "custom:tenant:11111111-1111-4111-8111-111111111111:job:42", "type" => "hash", "ttl_ms" => -1, "memory_bytes" => 512},
      ], generated_at: "2026-06-16T00:00:00Z", max_keys: 100)

      unknown = pattern(report, "custom:tenant:{uuid}:job:{number}")
      assert_equal "unknown", unknown.fetch("owner")
      assert_equal "inferred", unknown.fetch("owner_source")
      assert_equal "unknown", unknown.fetch("ttl_policy")
      assert_includes report.fetch("warnings"), "1 inferred key patterns need owner review"
    end

    def test_declares_asynq_global_process_metadata
      report = RedisKeyspaceInventory.report([
        {"key" => "asynq:queues", "type" => "set", "ttl_ms" => -1, "memory_bytes" => 80},
        {"key" => "asynq:servers:worker-1:123", "type" => "hash", "ttl_ms" => -1, "memory_bytes" => 256},
        {"key" => "asynq:workers:worker-1:123", "type" => "hash", "ttl_ms" => -1, "memory_bytes" => 256},
      ], generated_at: "2026-06-16T00:00:00Z", max_keys: 100)

      global = pattern(report, "asynq:*")
      assert_equal "asynq task queues used by backend workers", global.fetch("owner")
      assert_equal "declared", global.fetch("owner_source")
      assert_equal 3, global.fetch("key_count")
      assert_empty report.fetch("warnings")
    end

    def test_cli_renders_fixture_report
      stdout, stderr, status = Open3.capture3(
        RbConfig.ruby,
        "script/redis/keyspace_inventory.rb",
        "--fixture",
        FIXTURE,
        "--sample-limit",
        "1",
      )

      assert status.success?, stderr
      assert_empty stderr

      report = JSON.parse(stdout)
      assert_equal "fixture:#{FIXTURE}", report.fetch("source")
      assert_equal 8, report.dig("summary", "keys_observed")
      assert pattern(report, "auth:code:{scene}:{email_hash}")
    end

    def test_live_scanner_uses_read_only_redis_metadata_commands
      Dir.mktmpdir("redis-keyspace-inventory-test") do |dir|
        command_log = File.join(dir, "commands.log")
        redis_cli = File.join(dir, "redis-cli")
        File.write(redis_cli, fake_redis_cli(command_log))
        FileUtils.chmod("+x", redis_cli)

        original_path = ENV["PATH"]
        ENV["PATH"] = "#{dir}:#{original_path}"
        scanner = RedisCliScanner.new(
          addr: "redis.example.invalid:6379",
          password: "",
          db: "0",
          tls: false,
          scan_match: "mpp:*",
          scan_count: 2,
          max_keys: 10,
          command_timeout_seconds: 5,
        )
        samples = scanner.scan

        assert_equal ["mpp:dashboard:projects:list:v2:#{'a' * 64}", "mpp:browser:cleanup"], samples.map(&:key)
        assert_equal ["string", "zset"], samples.map(&:type)
        assert_equal [12_000, -1], samples.map(&:ttl_ms)
        assert_equal [128, 256], samples.map(&:memory_bytes)

        commands = File.readlines(command_log, chomp: true)
        assert_includes commands, "SCAN 0 MATCH mpp:* COUNT 2"
        assert_includes commands, "SCAN 1 MATCH mpp:* COUNT 2"
        assert commands.all? { |line| line.match?(/\A(SCAN|TYPE|PTTL|MEMORY USAGE)\b/) }, commands.join("\n")
      ensure
        ENV["PATH"] = original_path
      end
    end

    private

    def pattern(report, name)
      report.fetch("patterns").find { |entry| entry.fetch("pattern") == name } ||
        flunk("missing pattern #{name}")
    end

    def fake_redis_cli(command_log)
      <<~RUBY
        #!/usr/bin/env ruby
        command = ARGV.reject { |arg| ["-u", "redis://redis.example.invalid:6379/0", "--raw"].include?(arg) }
        File.open(#{command_log.inspect}, "a") { |file| file.puts(command.join(" ")) }
        case command
        when ["SCAN", "0", "MATCH", "mpp:*", "COUNT", "2"]
          puts "1"
          puts "mpp:dashboard:projects:list:v2:#{"a" * 64}"
        when ["SCAN", "1", "MATCH", "mpp:*", "COUNT", "2"]
          puts "0"
          puts "mpp:browser:cleanup"
        when ["TYPE", "mpp:dashboard:projects:list:v2:#{"a" * 64}"]
          puts "string"
        when ["PTTL", "mpp:dashboard:projects:list:v2:#{"a" * 64}"]
          puts "12000"
        when ["MEMORY", "USAGE", "mpp:dashboard:projects:list:v2:#{"a" * 64}"]
          puts "128"
        when ["TYPE", "mpp:browser:cleanup"]
          puts "zset"
        when ["PTTL", "mpp:browser:cleanup"]
          puts "-1"
        when ["MEMORY", "USAGE", "mpp:browser:cleanup"]
          puts "256"
        else
          warn "unexpected command: \#{command.join(" ")}"
          exit 1
        end
      RUBY
    end
  end
end
