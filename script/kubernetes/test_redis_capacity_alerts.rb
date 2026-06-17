#!/usr/bin/env ruby
# frozen_string_literal: true

require "open3"
require "tmpdir"
require "yaml"

PROMTOOL_IMAGE = "prom/prometheus:v3.12.0"
OBSERVABILITY_RULES = File.expand_path("../../deploy/kubernetes/observability/runtime-alerts.yaml", __dir__)

def build_rule_document
  documents = YAML.load_stream(File.read(OBSERVABILITY_RULES))
  rule = documents.find { |document| document["kind"] == "PrometheusRule" && document.dig("metadata", "name") == "mpp-browser-runtime-alerts" }
  raise "missing observability Redis alert rules" unless rule

  { "groups" => rule.dig("spec", "groups") }
end

def repeat_value(value, count)
  Array.new(count, value.to_s).join(" ")
end

def rising_counter(start, count)
  Array.new(count) { |index| start + index }.join(" ")
end

def build_promtool_test(rule_file)
  {
    "rule_files" => [rule_file],
    "evaluation_interval" => "1m",
    "tests" => [
      {
        "interval" => "1m",
        "input_series" => [
          { "series" => 'redis_memory_used_bytes{service="redis"}', "values" => repeat_value(340, 20) },
          { "series" => 'redis_memory_max_bytes{service="redis"}', "values" => repeat_value(384, 20) },
          { "series" => 'redis_evicted_keys_total{service="redis"}', "values" => rising_counter(0, 20) },
          { "series" => 'redis_instance_info{service="redis",maxmemory_policy="noeviction"}', "values" => repeat_value(1, 20) },
          { "series" => 'redis_connected_clients{service="redis"}', "values" => repeat_value(85, 20) },
          { "series" => 'redis_config_maxclients{service="redis"}', "values" => repeat_value(100, 20) },
          { "series" => 'redis_latency_percentiles_usec{service="redis",quantile="0.99"}', "values" => repeat_value(60000, 20) },
        ],
        "alert_rule_test" => [
          {
            "eval_time" => "20m",
            "alertname" => "MPPRedisMemoryHeadroomLow",
            "exp_alerts" => [{
              "exp_labels" => { "severity" => "warning", "owner" => "platform" },
              "exp_annotations" => {
                "summary" => "Redis memory headroom is low",
                "description" => "Redis has less than 15% memory headroom below configured maxmemory for at least 15 minutes.",
              },
            }],
          },
          {
            "eval_time" => "10m",
            "alertname" => "MPPRedisUnexpectedKeyEvictions",
            "exp_alerts" => [{
              "exp_labels" => { "severity" => "warning", "owner" => "platform" },
              "exp_annotations" => {
                "summary" => "Redis is evicting keys while noeviction is configured",
                "description" => "Redis key evictions are occurring even though the configured maxmemory policy is noeviction.",
              },
            }],
          },
          {
            "eval_time" => "15m",
            "alertname" => "MPPRedisConnectionCountHigh",
            "exp_alerts" => [{
              "exp_labels" => { "severity" => "warning", "owner" => "platform" },
              "exp_annotations" => {
                "summary" => "Redis connection usage is high",
                "description" => "Redis connected clients are above 80% of configured maxclients.",
              },
            }],
          },
          {
            "eval_time" => "15m",
            "alertname" => "MPPRedisLatencyP99High",
            "exp_alerts" => [{
              "exp_labels" => { "severity" => "warning", "owner" => "platform" },
              "exp_annotations" => {
                "summary" => "Redis p99 command latency is high",
                "description" => "Redis p99 command latency reported by Redis latency histograms is above 50 ms.",
              },
            }],
          },
        ],
      },
    ],
  }
end

def run_promtool(test_file)
  Open3.capture3("promtool", "test", "rules", File.basename(test_file), chdir: File.dirname(test_file))
rescue Errno::ENOENT
  mount_dir = File.dirname(test_file)
  Open3.capture3(
    "docker",
    "run",
    "--rm",
    "--entrypoint",
    "promtool",
    "-w",
    "/work",
    "-v",
    "#{mount_dir}:/work:ro",
    PROMTOOL_IMAGE,
    "test",
    "rules",
    File.basename(test_file),
  )
end

Dir.mktmpdir("mpp-redis-capacity-alerts-", Dir.pwd) do |dir|
  File.chmod(0o755, dir)

  rule_file = File.join(dir, "redis-capacity-rules.yml")
  test_file = File.join(dir, "redis-capacity-alerts.test.yml")

  File.write(rule_file, build_rule_document.to_yaml)
  File.write(test_file, build_promtool_test(File.basename(rule_file)).to_yaml)
  File.chmod(0o644, rule_file, test_file)

  stdout, stderr, status = run_promtool(test_file)
  unless status.success?
    warn stdout unless stdout.empty?
    warn stderr unless stderr.empty?
    abort "promtool Redis capacity alert test failed"
  end

  puts stdout unless stdout.empty?
  puts stderr unless stderr.empty?
  puts "promtool Redis capacity alert test passed"
end
