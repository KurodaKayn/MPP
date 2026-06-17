# frozen_string_literal: true

require "minitest/autorun"
require "open3"

class RedisHAFailoverDrillScriptTest < Minitest::Test
  SCRIPT = File.expand_path("redis-ha-failover-drill.sh", __dir__)

  def test_script_is_executable
    assert File.executable?(SCRIPT), "#{SCRIPT} must be executable"
  end

  def test_script_has_valid_shell_syntax
    stdout, stderr, status = Open3.capture3("bash", "-n", SCRIPT)

    assert status.success?, stderr
    assert_empty stdout
  end

  def test_help_mentions_drill_purpose
    stdout, stderr, status = Open3.capture3("bash", SCRIPT, "--help")

    assert status.success?, stderr
    assert_includes stdout, "non-production HA Redis client failover drill"
    assert_includes stdout, "MPP_REDIS_FAILOVER_TARGET_SECONDS"
    assert_includes stdout, "MPP_REDIS_FAILOVER_ALLOW_PRODUCTION"
  end

  def test_script_requires_explicit_production_allowance
    script = File.read(SCRIPT)

    assert_includes script, "refusing to run against APP_ENV=$app_env"
    assert_includes script, "MPP_REDIS_FAILOVER_ALLOW_PRODUCTION=1"
    assert_includes script, "env_flag_enabled \"$ALLOW_PRODUCTION\""
  end
end
