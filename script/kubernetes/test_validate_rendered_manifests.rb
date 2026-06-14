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
