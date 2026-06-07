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
