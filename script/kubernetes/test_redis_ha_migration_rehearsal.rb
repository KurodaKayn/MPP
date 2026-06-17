# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "open3"
require "rbconfig"
require "fileutils"
require "tmpdir"

class RedisHAMigrationRehearsalScriptTest < Minitest::Test
  SCRIPT = File.expand_path("redis-ha-migration-rehearsal.rb", __dir__)

  def test_script_is_executable
    assert File.executable?(SCRIPT), "#{SCRIPT} must be executable"
  end

  def test_script_has_valid_ruby_syntax
    stdout, stderr, status = Open3.capture3(RbConfig.ruby, "-c", SCRIPT)

    assert status.success?, stderr
    assert_includes stdout, "Syntax OK"
  end

  def test_help_mentions_rehearsal_report
    stdout, stderr, status = Open3.capture3(RbConfig.ruby, SCRIPT, "--help")

    assert status.success?, stderr
    assert_includes stdout, "non-production Redis HA migration rehearsal"
    assert_includes stdout, "--allow-target-flush"
  end

  def test_fake_kubectl_rehearsal_imports_and_reports_ttl_diff
    Dir.mktmpdir("redis-ha-migration-rehearsal-test") do |dir|
      command_log = File.join(dir, "commands.log")
      kubectl = File.join(dir, "kubectl")
      report_path = File.join(dir, "report.json")
      File.write(kubectl, fake_kubectl(command_log, redis_password: ""))
      FileUtils.chmod("+x", kubectl)

      original_path = ENV["PATH"]
      ENV["PATH"] = "#{dir}:#{original_path}"
      stdout, stderr, status = Open3.capture3(
        {"PATH" => ENV.fetch("PATH")},
        RbConfig.ruby,
        SCRIPT,
        "--allow-target-flush",
        "--sample-limit",
        "2",
        "--report",
        report_path,
      )

      assert status.success?, stderr
      assert_empty stderr
      report = JSON.parse(stdout)
      assert_equal "pass", report.fetch("status")
      assert_equal 2, report.dig("summary", "keys_imported")
      assert_equal 2, report.dig("summary", "sample_value_matches")
      assert_equal 0, report.dig("summary", "sample_ttl_mismatches")
      assert_equal 0, report.dig("summary", "source_vs_target_key_count_diff")
      assert_equal report, JSON.parse(File.read(report_path))

      commands = File.readlines(command_log, chomp: true)
      assert_includes commands, "source SCAN 0 MATCH * COUNT 100"
      assert_includes commands, "source DUMP mpp:cache:one"
      assert_includes commands, "source MIGRATE redis-ha-primary 6379  0 5000 COPY REPLACE KEYS mpp:cache:one"
      assert_includes commands, "target PTTL mpp:cache:one"
      assert_includes commands, "target DUMP mpp:cache:one"
    ensure
      ENV["PATH"] = original_path
    end
  end

  def test_fake_kubectl_rehearsal_passes_migrate_auth_when_redis_has_password
    Dir.mktmpdir("redis-ha-migration-rehearsal-auth-test") do |dir|
      command_log = File.join(dir, "commands.log")
      kubectl = File.join(dir, "kubectl")
      File.write(kubectl, fake_kubectl(command_log, redis_password: "secret"))
      FileUtils.chmod("+x", kubectl)

      original_path = ENV["PATH"]
      ENV["PATH"] = "#{dir}:#{original_path}"
      _stdout, stderr, status = Open3.capture3(
        {"PATH" => ENV.fetch("PATH")},
        RbConfig.ruby,
        SCRIPT,
        "--allow-target-flush",
        "--sample-limit",
        "1",
      )

      assert status.success?, stderr
      commands = File.readlines(command_log, chomp: true)
      assert_includes commands, "source MIGRATE redis-ha-primary 6379  0 5000 AUTH secret COPY REPLACE KEYS mpp:cache:one"
    ensure
      ENV["PATH"] = original_path
    end
  end

  private

  def fake_kubectl(command_log, redis_password:)
    <<~RUBY
      #!/usr/bin/env ruby
      command_log = #{command_log.inspect}
      redis_password = #{redis_password.inspect}
      args = ARGV.dup

      def endpoint(args)
        return "source" if args.include?("statefulset/redis")
        return "target" if args.include?("statefulset/redis-ha-primary")

        "kubectl"
      end

      if args == ["version", "--client=true"]
        puts "Client Version: fake"
        exit 0
      end

      if args[0, 3] == ["auth", "can-i", "create"]
        puts "yes"
        exit 0
      end

      if args[0, 2] == ["rollout", "status"]
        puts "ready"
        exit 0
      end

      if args[0, 3] == ["get", "configmap", "mpp-app-config"]
        jsonpath = args.last
        case jsonpath
        when /APP_ENV/
          print "staging"
        when /REDIS_ENDPOINT_MODE/
          print "direct"
        when /REDIS_ADDR/
          print "redis:6379"
        else
          print ""
        end
        exit 0
      end

      if args.first == "exec"
        sep = args.index("--")
        redis_cli = args.rindex("redis-cli")
        if redis_cli.nil? && args[(sep + 1), 2] == ["sh", "-ec"] && args[(sep + 3)].to_s.include?("REDIS_PASSWORD")
          print redis_password
          exit 0
        end
        redis_args = args[(redis_cli + 1)..] || []
        ep = endpoint(args)
        File.open(command_log, "a") { |file| file.puts(([ep] + redis_args).join(" ")) }

        case [ep, *redis_args]
        when ["source", "PING"], ["target", "PING"]
          puts "PONG"
        when ["target", "INFO", "replication"]
          puts "role:master"
        when ["source", "CLIENT", "LIST"]
          puts "id=1 addr=127.0.0.1:6379"
        when ["source", "DBSIZE"]
          puts "2"
        when ["target", "DBSIZE"]
          state = File.exist?(command_log) && File.read(command_log).match?(/source MIGRATE redis-ha-primary 6379  0 5000 (AUTH secret )?COPY REPLACE KEYS mpp:cache:two/)
          puts(state ? "2" : "1")
        when ["target", "FLUSHDB"]
          puts "OK"
        when ["source", "SCAN", "0", "MATCH", "*", "COUNT", "100"]
          puts "0"
          puts "mpp:cache:one"
          puts "mpp:cache:two"
        when ["source", "PTTL", "mpp:cache:one"]
          puts "60000"
        when ["source", "PTTL", "mpp:cache:two"]
          puts "-1"
        when ["source", "DUMP", "mpp:cache:one"], ["target", "DUMP", "mpp:cache:one"]
          print "dump-one"
        when ["source", "DUMP", "mpp:cache:two"], ["target", "DUMP", "mpp:cache:two"]
          print "dump-two"
        when ["source", "MIGRATE", "redis-ha-primary", "6379", "", "0", "5000", "COPY", "REPLACE", "KEYS", "mpp:cache:one"],
             ["source", "MIGRATE", "redis-ha-primary", "6379", "", "0", "5000", "COPY", "REPLACE", "KEYS", "mpp:cache:two"],
             ["source", "MIGRATE", "redis-ha-primary", "6379", "", "0", "5000", "AUTH", "secret", "COPY", "REPLACE", "KEYS", "mpp:cache:one"],
             ["source", "MIGRATE", "redis-ha-primary", "6379", "", "0", "5000", "AUTH", "secret", "COPY", "REPLACE", "KEYS", "mpp:cache:two"]
          puts "OK"
        when ["source", "TYPE", "mpp:cache:one"], ["source", "TYPE", "mpp:cache:two"]
          puts "string"
        when ["target", "EXISTS", "mpp:cache:one"], ["target", "EXISTS", "mpp:cache:two"]
          puts "1"
        when ["target", "PTTL", "mpp:cache:one"]
          puts "59000"
        when ["target", "PTTL", "mpp:cache:two"]
          puts "-1"
        else
          warn "unexpected command: \#{([ep] + redis_args).join(" ")}"
          exit 1
        end
        exit 0
      end

      warn "unexpected kubectl: \#{args.join(" ")}"
      exit 1
    RUBY
  end
end
