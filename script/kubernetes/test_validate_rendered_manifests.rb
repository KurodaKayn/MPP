# frozen_string_literal: true

require "minitest/autorun"
require "open3"
require "rbconfig"
require "tempfile"
require "yaml"

class ValidateRenderedManifestsTest < Minitest::Test
  STAGING_OVERLAYS = [
    "deploy/kubernetes/overlays/staging-managed",
    "deploy/kubernetes/overlays/staging-self-hosted",
  ].freeze
  EXTERNAL_SECRETS_PACKAGE = "deploy/kubernetes/external-secrets"
  PRODUCTION_MANAGED_OVERLAY = "deploy/kubernetes/overlays/production-managed"

  def test_deployable_validation_rejects_checked_in_staging_examples
    STAGING_OVERLAYS.each do |overlay|
      rendered = render_overlay(overlay)

      _stdout, stderr, status = run_validator(overlay, rendered.path, { "MPP_KUBERNETES_VALIDATE_DEPLOYABLE" => "1" })

      refute status.success?, "#{overlay} unexpectedly passed deployable validation"
      assert_includes stderr, "must not use example.invalid"
      assert_includes stderr, "must not use the all-zero example sha tag"
      assert_includes stderr, "must not use an example value"
    ensure
      rendered&.unlink
    end
  end

  def test_staging_self_hosted_overlay_validates_for_local_render
    overlay = "deploy/kubernetes/overlays/staging-self-hosted"
    rendered = render_overlay(overlay)

    _stdout, stderr, status = run_validator(overlay, rendered.path)

    assert status.success?, "staging self-hosted validation failed: #{stderr}"
  ensure
    rendered&.unlink
  end

  def test_production_managed_overlay_uses_external_secret_contract
    rendered = render_overlay(PRODUCTION_MANAGED_OVERLAY)
    documents = parse_documents(File.read(rendered.path))
    external_secret = document(documents, "ExternalSecret", "mpp-app-secrets", "mpp-system")

    assert_nil documents.find { |document| document["kind"] == "Secret" && document.dig("metadata", "name") == "mpp-app-secrets" }
    assert_equal "external-secrets.io/v1", external_secret["apiVersion"]
    assert_equal "mpp-app-secrets", external_secret.dig("spec", "target", "name")

    _stdout, stderr, status = run_validator(PRODUCTION_MANAGED_OVERLAY, rendered.path)

    assert status.success?, "production-managed validation failed: #{stderr}"
  ensure
    rendered&.unlink
  end

  def test_external_secrets_package_renders_app_secret_contract
    rendered = render_overlay(EXTERNAL_SECRETS_PACKAGE)
    documents = parse_documents(File.read(rendered.path))
    external_secret = document(documents, "ExternalSecret", "mpp-app-secrets", "mpp-system")

    assert_equal "external-secrets.io/v1", external_secret["apiVersion"]
    assert_equal "ClusterSecretStore", external_secret.dig("spec", "secretStoreRef", "kind")
    assert_equal "mpp-production-secrets", external_secret.dig("spec", "secretStoreRef", "name")

    _stdout, stderr, status = run_validator(EXTERNAL_SECRETS_PACKAGE, rendered.path)

    assert status.success?, "external-secrets validation failed: #{stderr}"
  ensure
    rendered&.unlink
  end

  def test_external_secrets_package_rejects_missing_required_remote_ref
    rendered = mutated_render(EXTERNAL_SECRETS_PACKAGE) do |documents|
      external_secret = document(documents, "ExternalSecret", "mpp-app-secrets", "mpp-system")
      external_secret.dig("spec", "data").reject! { |entry| entry["secretKey"] == "JWT_SECRET" }
    end

    _stdout, stderr, status = run_validator(EXTERNAL_SECRETS_PACKAGE, rendered.path)

    refute status.success?, "external-secrets validation unexpectedly accepted a missing app Secret key"
    assert_includes stderr, "external secret contract is missing remote refs: JWT_SECRET"
  ensure
    rendered&.unlink
  end

  def test_production_managed_rejects_rendered_app_secret
    rendered = mutated_render(PRODUCTION_MANAGED_OVERLAY) do |documents|
      documents << {
        "apiVersion" => "v1",
        "kind" => "Secret",
        "metadata" => {
          "name" => "mpp-app-secrets",
          "namespace" => "mpp-system",
        },
        "data" => {},
      }
    end

    _stdout, stderr, status = run_validator(PRODUCTION_MANAGED_OVERLAY, rendered.path)

    refute status.success?, "production-managed validation unexpectedly accepted a rendered app Secret"
    assert_includes stderr, "must not render raw mpp-app-secrets"
  ensure
    rendered&.unlink
  end

  def test_production_managed_rejects_unknown_required_app_secret_ref
    rendered = mutated_render(PRODUCTION_MANAGED_OVERLAY) do |documents|
      deployment = document(documents, "Deployment", "backend", "mpp-system")
      container = deployment.dig("spec", "template", "spec", "containers").find { |entry| entry["name"] == "backend" }
      container["env"] << {
        "name" => "EXTRA_REQUIRED_SECRET",
        "valueFrom" => {
          "secretKeyRef" => {
            "name" => "mpp-app-secrets",
            "key" => "EXTRA_REQUIRED_SECRET",
          },
        },
      }
    end

    _stdout, stderr, status = run_validator(PRODUCTION_MANAGED_OVERLAY, rendered.path)

    refute status.success?, "production-managed validation unexpectedly accepted an unknown app Secret ref"
    assert_includes stderr, "external secret contract has unexpected required refs: EXTRA_REQUIRED_SECRET"
  ensure
    rendered&.unlink
  end

  def test_production_managed_rejects_required_secret_ref_from_other_secret
    rendered = mutated_render(PRODUCTION_MANAGED_OVERLAY) do |documents|
      deployment = document(documents, "Deployment", "backend", "mpp-system")
      container = deployment.dig("spec", "template", "spec", "containers").find { |entry| entry["name"] == "backend" }
      jwt_env = container["env"].find { |entry| entry["name"] == "JWT_SECRET" }
      jwt_env.dig("valueFrom", "secretKeyRef")["name"] = "other-secret"
    end

    _stdout, stderr, status = run_validator(PRODUCTION_MANAGED_OVERLAY, rendered.path)

    refute status.success?, "production-managed validation unexpectedly accepted an app Secret ref from another Secret"
    assert_includes stderr, "external secret contract must use mpp-app-secrets for refs"
    assert_includes stderr, "JWT_SECRET->other-secret/JWT_SECRET"
  ensure
    rendered&.unlink
  end

  def test_deployable_validation_rejects_checked_in_production_managed_examples
    rendered = render_overlay(PRODUCTION_MANAGED_OVERLAY)

    _stdout, stderr, status = run_validator(
      PRODUCTION_MANAGED_OVERLAY,
      rendered.path,
      { "MPP_KUBERNETES_VALIDATE_DEPLOYABLE" => "1" },
    )

    refute status.success?, "production-managed unexpectedly passed deployable validation"
    assert_includes stderr, "must not use example.invalid"
    assert_includes stderr, "must not use the all-zero example sha tag"
    refute_includes stderr, "must not use an example value"
  ensure
    rendered&.unlink
  end

  def test_app_network_policy_rejects_extra_internal_sources
    rendered = mutated_render("deploy/kubernetes/app-baseline") do |documents|
      policy = document(documents, "NetworkPolicy", "ai-service-internal-access", "mpp-system")
      policy.dig("spec", "ingress", 0, "from") << {
        "podSelector" => {
          "matchLabels" => {
            "app.kubernetes.io/name" => "mpp",
            "app.kubernetes.io/component" => "frontend",
          },
        },
      }
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/app-baseline", rendered.path)

    refute status.success?, "app baseline validation unexpectedly accepted an extra NetworkPolicy source"
    assert_includes stderr, "ai-service-internal-access must allow only backend, publish-worker Pods"
  ensure
    rendered&.unlink
  end

  def test_app_network_policy_rejects_unexpected_policies
    rendered = mutated_render("deploy/kubernetes/app-baseline") do |documents|
      documents << broad_network_policy("unexpected-backend-access", "backend", 8080)
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/app-baseline", rendered.path)

    refute status.success?, "app baseline validation unexpectedly accepted an extra NetworkPolicy"
    assert_includes stderr, "unexpected mpp-system NetworkPolicies: unexpected-backend-access"
  ensure
    rendered&.unlink
  end

  def test_app_network_policy_rejects_extra_target_selector_labels
    rendered = mutated_render("deploy/kubernetes/app-baseline") do |documents|
      policy = document(documents, "NetworkPolicy", "public-frontend-access", "mpp-system")
      policy.dig("spec", "podSelector", "matchLabels")["mpp.kurodakayn.dev/extra-selector"] = "true"
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/app-baseline", rendered.path)

    refute status.success?, "app baseline validation unexpectedly accepted an extra target selector label"
    assert_includes stderr, "public-frontend-access must select frontend Pods"
  ensure
    rendered&.unlink
  end

  def test_observability_metrics_policy_rejects_split_source_and_port_rules
    rendered = mutated_render("deploy/kubernetes/observability") do |documents|
      policy = document(documents, "NetworkPolicy", "browser-worker-observability-metrics", "mpp-system")
      policy["spec"]["ingress"] = [
        {
          "from" => [{
            "namespaceSelector" => {
              "matchLabels" => { "mpp.kurodakayn.dev/metrics-scraper" => "true" },
            },
          }],
          "ports" => [{ "protocol" => "TCP", "port" => 9999 }],
        },
        {
          "from" => [{
            "namespaceSelector" => {
              "matchLabels" => { "mpp.kurodakayn.dev/public-ingress" => "true" },
            },
          }],
          "ports" => [{ "protocol" => "TCP", "port" => 8081 }],
        },
      ]
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/observability", rendered.path)

    refute status.success?, "observability validation unexpectedly accepted split NetworkPolicy rules"
    assert_includes stderr, "browser-worker-observability-metrics must define exactly one metrics ingress rule"
  ensure
    rendered&.unlink
  end

  def test_observability_metrics_policy_rejects_extra_target_selector_labels
    rendered = mutated_render("deploy/kubernetes/observability") do |documents|
      policy = document(documents, "NetworkPolicy", "browser-worker-observability-metrics", "mpp-system")
      policy.dig("spec", "podSelector", "matchLabels")["mpp.kurodakayn.dev/extra-selector"] = "true"
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/observability", rendered.path)

    refute status.success?, "observability validation unexpectedly accepted an extra target selector label"
    assert_includes stderr, "browser-worker-observability-metrics must select only browser-worker Pods"
  ensure
    rendered&.unlink
  end

  def test_observability_metrics_policy_rejects_unexpected_policies
    rendered = mutated_render("deploy/kubernetes/observability") do |documents|
      documents << broad_network_policy("unexpected-metrics-access", "backend", 8080)
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/observability", rendered.path)

    refute status.success?, "observability validation unexpectedly accepted an extra NetworkPolicy"
    assert_includes stderr, "unexpected mpp-system metrics NetworkPolicies: unexpected-metrics-access"
  ensure
    rendered&.unlink
  end

  def test_observability_metrics_policy_requires_trust_boundary_annotations
    rendered = mutated_render("deploy/kubernetes/observability") do |documents|
      policy = document(documents, "NetworkPolicy", "backend-worker-observability-metrics", "mpp-system")
      policy.dig("metadata", "annotations").delete("mpp.kurodakayn.dev/metrics-port-scope")
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/observability", rendered.path)

    refute status.success?, "observability validation unexpectedly accepted missing trust-boundary annotations"
    assert_includes stderr, "backend-worker-observability-metrics must annotate"
  ensure
    rendered&.unlink
  end

  def test_observability_redis_alerts_require_owner_and_severity
    rendered = mutated_render("deploy/kubernetes/observability") do |documents|
      rule = document(documents, "PrometheusRule", "mpp-browser-runtime-alerts", "mpp-observability")
      alerts = rule.dig("spec", "groups").flat_map { |group| Array(group["rules"]) }
      redis_alert = alerts.find { |alert| alert["alert"] == "MPPRedisUnavailable" }
      redis_alert["labels"].delete("owner")
      redis_alert["labels"]["severity"] = "warning"
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/observability", rendered.path)

    refute status.success?, "observability validation unexpectedly accepted Redis alert metadata drift"
    assert_includes stderr, "MPPRedisUnavailable must label severity=critical"
    assert_includes stderr, "MPPRedisUnavailable must label owner=platform"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_reject_missing_backup_cronjob
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      documents.reject! do |entry|
        entry["kind"] == "CronJob" &&
          entry.dig("metadata", "name") == "postgres-backup" &&
          entry.dig("metadata", "namespace") == "mpp-system"
      end
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted missing backup CronJob"
    assert_includes stderr, "rendered manifests are missing CronJob/mpp-system/postgres-backup"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_redis_backup_snapshot_script
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      config = document(documents, "ConfigMap", "mpp-data-backup-scripts", "mpp-system")
      config["data"]["redis-backup.sh"] = config.dig("data", "redis-backup.sh")
        .gsub("--rdb \"$tmp_file\"", "SAVE")
        .gsub("mv \"$tmp_file\" \"$target_file\"", "cp \"$tmp_file\" \"$target_file\"")
        .gsub("find \"$backup_root\" -type f -name \"redis-*.rdb\" -mtime +\"$retention_days\" -delete", "")
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted a weak redis backup script"
    assert_includes stderr, "self-hosted redis backup script must stream RDB snapshots with redis-cli --rdb"
    assert_includes stderr, "self-hosted redis backup script must atomically publish complete snapshots"
    assert_includes stderr, "self-hosted redis backup script must prune retained Redis snapshots"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_redis_backup_schedule
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      cronjob = document(documents, "CronJob", "redis-backup", "mpp-system")
      cronjob["spec"].delete("schedule")
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted redis backup without a schedule"
    assert_includes stderr, "self-hosted redis-backup CronJob must define a schedule"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_backup_network_policy_sources
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      policy = document(documents, "NetworkPolicy", "postgres-app-access", "mpp-system")
      policy.dig("spec", "ingress", 0, "from").reject! do |entry|
        entry.dig("podSelector", "matchLabels", "app.kubernetes.io/component") == "postgres-backup"
      end
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted missing backup NetworkPolicy source"
    assert_includes stderr, "self-hosted postgres NetworkPolicy must allow postgres-backup ingress"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_redis_persistence_config
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      documents.reject! do |entry|
        entry["kind"] == "ConfigMap" &&
          entry.dig("metadata", "name") == "redis-persistence-config" &&
          entry.dig("metadata", "namespace") == "mpp-system"
      end
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted missing redis persistence config"
    assert_includes stderr, "rendered manifests are missing ConfigMap/mpp-system/redis-persistence-config"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_redis_aof_policy
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      config = document(documents, "ConfigMap", "redis-persistence-config", "mpp-system")
      config["data"]["redis.conf"] = config.dig("data", "redis.conf").gsub("appendonly yes", "appendonly no")
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted disabled redis AOF"
    assert_includes stderr, "self-hosted redis persistence config must enable AOF"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_redis_memory_pressure_policy
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      config = document(documents, "ConfigMap", "redis-persistence-config", "mpp-system")
      config["data"]["redis.conf"] = config.dig("data", "redis.conf")
        .gsub("maxmemory 384mb", "maxmemory 0")
        .gsub("maxmemory-policy noeviction", "maxmemory-policy allkeys-lru")
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted weak redis memory policy"
    assert_includes stderr, "self-hosted redis runtime config maxmemory must be greater than zero"
    assert_includes stderr, "self-hosted redis runtime config must keep maxmemory at 384mb"
    assert_includes stderr, "self-hosted redis runtime config must use noeviction"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_redis_connection_runtime_settings
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      config = document(documents, "ConfigMap", "redis-persistence-config", "mpp-system")
      config["data"]["redis.conf"] = config.dig("data", "redis.conf")
        .gsub("timeout 0", "timeout -1")
        .gsub("tcp-keepalive 300", "tcp-keepalive 0")
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted weak redis connection settings"
    assert_includes stderr, "self-hosted redis runtime config must set non-negative timeout"
    assert_includes stderr, "self-hosted redis runtime config must enable tcp-keepalive"
    assert_includes stderr, "self-hosted redis runtime config must keep idle timeout disabled"
    assert_includes stderr, "self-hosted redis runtime config must keep tcp-keepalive at 300 seconds"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_redis_slowlog_settings
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      config = document(documents, "ConfigMap", "redis-persistence-config", "mpp-system")
      config["data"]["redis.conf"] = config.dig("data", "redis.conf")
        .gsub("slowlog-log-slower-than 10000", "slowlog-log-slower-than -1")
        .gsub("slowlog-max-len 256", "slowlog-max-len 0")
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted weak redis slowlog settings"
    assert_includes stderr, "self-hosted redis runtime config must set non-negative slowlog-log-slower-than"
    assert_includes stderr, "self-hosted redis runtime config must retain slowlog entries"
    assert_includes stderr, "self-hosted redis runtime config must log commands slower than 10ms"
    assert_includes stderr, "self-hosted redis runtime config must retain 256 slowlog entries"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_reject_redis_runtime_duplicate_overrides
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      config = document(documents, "ConfigMap", "redis-persistence-config", "mpp-system")
      config["data"]["redis.conf"] = "#{config.dig('data', 'redis.conf')}\n" \
        "maxmemory 1gb\n" \
        "maxmemory-policy allkeys-lru\n" \
        "timeout 30\n" \
        "tcp-keepalive 60\n" \
        "slowlog-log-slower-than 50000\n" \
        "slowlog-max-len 512\n"
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted duplicate redis runtime overrides"
    assert_includes stderr, "self-hosted redis runtime config must keep maxmemory at 384mb"
    assert_includes stderr, "self-hosted redis runtime config must use noeviction"
    assert_includes stderr, "self-hosted redis runtime config must keep idle timeout disabled"
    assert_includes stderr, "self-hosted redis runtime config must keep tcp-keepalive at 300 seconds"
    assert_includes stderr, "self-hosted redis runtime config must log commands slower than 10ms"
    assert_includes stderr, "self-hosted redis runtime config must retain 256 slowlog entries"
  ensure
    rendered&.unlink
  end

  def test_staging_self_hosted_overlay_allows_documented_redis_policy_override
    rendered = mutated_render("deploy/kubernetes/overlays/staging-self-hosted") do |documents|
      config = document(documents, "ConfigMap", "redis-persistence-config", "mpp-system")
      config["data"]["redis.conf"] = config.dig("data", "redis.conf")
        .gsub("appendfsync everysec", "appendfsync always")
        .gsub("save 900 1\nsave 300 10\nsave 60 10000", "save 600 1")
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/overlays/staging-self-hosted", rendered.path)

    assert status.success?, "staging self-hosted validation rejected a documented Redis policy override: #{stderr}"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_redis_persistence_config_mount
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      stateful_set = document(documents, "StatefulSet", "redis", "mpp-system")
      container = stateful_set.dig("spec", "template", "spec", "containers").find { |entry| entry["name"] == "redis" }
      container["volumeMounts"].reject! { |entry| entry["name"] == "redis-persistence-config" }
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted missing redis config mount"
    assert_includes stderr, "self-hosted redis container must mount persistence config read-only"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_redis_ping_readiness
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      stateful_set = document(documents, "StatefulSet", "redis", "mpp-system")
      container = stateful_set.dig("spec", "template", "spec", "containers").find { |entry| entry["name"] == "redis" }
      container["readinessProbe"] = {
        "tcpSocket" => { "port" => "redis" },
        "initialDelaySeconds" => 5,
        "periodSeconds" => 10,
        "timeoutSeconds" => 3,
        "failureThreshold" => 6,
      }
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted TCP-only redis readiness"
    assert_includes stderr, "self-hosted redis container readinessProbe must run redis-cli PING"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_redis_graceful_shutdown
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      stateful_set = document(documents, "StatefulSet", "redis", "mpp-system")
      pod_spec = stateful_set.dig("spec", "template", "spec")
      pod_spec["terminationGracePeriodSeconds"] = 10
      container = pod_spec["containers"].find { |entry| entry["name"] == "redis" }
      container.delete("lifecycle")
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted missing redis graceful shutdown"
    assert_includes stderr, "self-hosted redis StatefulSet must allow at least 60 seconds for graceful termination"
    assert_includes stderr, "self-hosted redis container must run SHUTDOWN SAVE before termination"
  ensure
    rendered&.unlink
  end

  def test_self_hosted_data_services_require_redis_exporter_network_policy_source
    rendered = mutated_render("deploy/kubernetes/data-services/self-hosted") do |documents|
      policy = document(documents, "NetworkPolicy", "redis-app-access", "mpp-system")
      policy.dig("spec", "ingress", 0, "from").reject! do |entry|
        entry.dig("podSelector", "matchLabels", "app.kubernetes.io/component") == "redis-exporter"
      end
    end

    _stdout, stderr, status = run_validator("deploy/kubernetes/data-services/self-hosted", rendered.path)

    refute status.success?, "self-hosted validation unexpectedly accepted missing redis-exporter NetworkPolicy source"
    assert_includes stderr, "self-hosted redis NetworkPolicy must allow redis-exporter ingress"
  ensure
    rendered&.unlink
  end

  private

  def run_validator(overlay, rendered_path, env = {})
    Open3.capture3(
      env,
      RbConfig.ruby,
      "script/kubernetes/validate-rendered-manifests.rb",
      overlay,
      rendered_path,
    )
  end

  def render_overlay(overlay)
    rendered = Tempfile.new(["mpp-rendered-manifests", ".yaml"])
    stdout, stderr, status = Open3.capture3("kubectl", "kustomize", overlay)
    raise "#{overlay} failed to render: #{stderr}" unless status.success?

    rendered.write(stdout)
    rendered.close
    rendered
  end

  def mutated_render(overlay)
    original = render_overlay(overlay)
    documents = parse_documents(File.read(original.path))
    yield documents

    mutated = Tempfile.new(["mpp-mutated-manifests", ".yaml"])
    documents.each { |document| mutated.write(document.to_yaml) }
    mutated.close
    mutated
  ensure
    original&.unlink
  end

  def document(documents, kind, name, namespace)
    found = documents.find do |document|
      document["kind"] == kind &&
        document.dig("metadata", "name") == name &&
        document.dig("metadata", "namespace") == namespace
    end
    raise "missing #{kind}/#{namespace}/#{name}" unless found

    found
  end

  def broad_network_policy(name, component, port)
    {
      "apiVersion" => "networking.k8s.io/v1",
      "kind" => "NetworkPolicy",
      "metadata" => {
        "name" => name,
        "namespace" => "mpp-system",
      },
      "spec" => {
        "podSelector" => {
          "matchLabels" => {
            "app.kubernetes.io/name" => "mpp",
            "app.kubernetes.io/component" => component,
          },
        },
        "policyTypes" => ["Ingress"],
        "ingress" => [{
          "from" => [{
            "podSelector" => {
              "matchLabels" => { "app.kubernetes.io/name" => "mpp" },
            },
          }],
          "ports" => [{ "protocol" => "TCP", "port" => port }],
        }],
      },
    }
  end

  def parse_documents(rendered)
    rendered
      .split(/^---\s*$/)
      .map(&:strip)
      .reject(&:empty?)
      .map do |document|
        YAML.safe_load(
          document,
          permitted_classes: [],
          aliases: true,
        )
      end
      .compact
  end
end
