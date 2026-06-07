# frozen_string_literal: true

module KubernetesValidation
  module Observability
    module_function

    METRICS_POLICIES = {
      "backend-worker-observability-metrics" => [["backend", "publish-worker"], 8080, "shared-http-listener"],
      "browser-worker-observability-metrics" => [["browser-worker"], 8081, "shared-http-listener"],
      "ai-service-observability-metrics" => [["ai-service"], 8000, "shared-http-listener"],
      "collab-service-observability-metrics" => [["collab-service"], 8090, "shared-http-listener"],
      "content-pipeline-observability-metrics" => [["content-pipeline-service"], 9090, "dedicated-metrics-listener"],
    }.freeze

    def validate(context)
      validate_namespace(context)
      validate_alloy(context)
      validate_pod_monitors(context)
      validate_alerts(context)
      validate_metrics_policies(context)
    end

    def validate_namespace(context)
      namespace = context.require_document("Namespace", "mpp-observability")
      return unless namespace

      unless namespace.labels["mpp.kurodakayn.dev/metrics-scraper"] == "true"
        context.add_error("mpp-observability namespace must be labeled as a metrics scraper")
      end
    end

    def validate_alloy(context)
      deployment = context.require_document("Deployment", "mpp-alloy", "mpp-observability")
      return unless deployment

      pod_spec = deployment.pod_spec
      pod_security = pod_spec["securityContext"] || {}
      {
        "runAsUser" => 473,
        "runAsGroup" => 473,
        "fsGroup" => 473,
        "fsGroupChangePolicy" => "OnRootMismatch",
      }.each do |key, value|
        context.add_error("mpp-alloy Deployment is missing #{key}=#{value}") unless pod_security[key] == value
      end
      context.add_error("mpp-alloy must use mpp-alloy ServiceAccount") unless pod_spec["serviceAccountName"] == "mpp-alloy"

      container = deployment.container("alloy")
      return context.add_error("mpp-alloy Deployment is missing alloy container") unless container

      security = container["securityContext"] || {}
      drops = Array(security.dig("capabilities", "drop"))
      context.add_error("mpp-alloy container must disable privilege escalation") unless security["allowPrivilegeEscalation"] == false
      context.add_error("mpp-alloy container must drop all capabilities") unless drops.include?("ALL")
      args = Array(container["args"])
      unless args.include?("--storage.path=/var/lib/alloy/data")
        context.add_error("mpp-alloy must set writable Alloy storage path")
      end
      env = Array(container["env"])
      unless env.any? { |entry| entry["name"] == "LOKI_WRITE_URL" }
        context.add_error("mpp-alloy must define LOKI_WRITE_URL")
      end
    end

    def validate_pod_monitors(context)
      ["mpp-http-services", "mpp-content-pipeline-service", "mpp-alloy"].each do |monitor|
        context.require_document("PodMonitor", monitor, "mpp-observability")
      end
    end

    def validate_alerts(context)
      rule = context.require_document("PrometheusRule", "mpp-browser-runtime-alerts", "mpp-observability")
      return unless rule

      alerts = Array(rule.spec["groups"]).flat_map do |group|
        Array(group["rules"]).map { |alert| alert["alert"] }
      end
      [
        "MPPBrowserRuntimeStartupFailures",
        "MPPBrowserRuntimeCleanupFailures",
        "MPPBrowserRuntimeCleanupLagHigh",
        "MPPServiceReadinessFailures",
        "MPPRedisDependentServiceReadinessFailures",
        "MPPPublishWorkerJobFailures",
      ].each do |alert|
        context.add_error("PrometheusRule is missing #{alert}") unless alerts.include?(alert)
      end
    end

    def validate_metrics_policies(context)
      validate_expected_metrics_policy_set(context)

      METRICS_POLICIES.each do |name, (components, port, port_scope)|
        validate_metrics_policy(context, name, components, port, port_scope)
      end
    end

    def validate_expected_metrics_policy_set(context)
      actual = context.documents
        .select { |document| document.kind == "NetworkPolicy" && document.namespace == "mpp-system" }
        .map(&:name)
      duplicates = actual.select { |name| actual.count(name) > 1 }.uniq
      unless duplicates.empty?
        context.add_error(
          "#{context.package_dir} must not include duplicate mpp-system metrics NetworkPolicies: #{duplicates.sort.join(', ')}",
        )
      end

      unexpected = actual - METRICS_POLICIES.keys
      return if unexpected.empty?

      context.add_error(
        "#{context.package_dir} must not include unexpected mpp-system metrics NetworkPolicies: #{unexpected.sort.join(', ')}",
      )
    end

    def validate_metrics_policy(context, name, components, port, port_scope)
      policy = context.require_document("NetworkPolicy", name, "mpp-system")
      return unless policy

      validate_metrics_policy_annotations(context, policy, name, port_scope)
      validate_metrics_policy_target(context, policy, name, components)
      validate_metrics_ingress_rule(context, policy, name, port)
    end

    def validate_metrics_policy_annotations(context, policy, name, port_scope)
      annotations = policy.metadata["annotations"] || {}
      expected = {
        "mpp.kurodakayn.dev/networkpolicy-layer" => "l4-port-allowlist",
        "mpp.kurodakayn.dev/metrics-scraper-trust" => "trusted-namespace",
        "mpp.kurodakayn.dev/metrics-port-scope" => port_scope,
      }

      expected.each do |key, value|
        next if annotations[key] == value

        context.add_error("#{name} must annotate #{key}=#{value}")
      end
    end

    def validate_metrics_policy_target(context, policy, name, components)
      expected = if components.length == 1
        {
          "matchLabels" => {
            "app.kubernetes.io/name" => "mpp",
            "app.kubernetes.io/component" => components.first,
          },
        }
      else
        {
          "matchLabels" => {
            "app.kubernetes.io/name" => "mpp",
          },
          "matchExpressions" => [{
            "key" => "app.kubernetes.io/component",
            "operator" => "In",
            "values" => components,
          }],
        }
      end

      unless policy.spec["podSelector"] == expected
        context.add_error("#{name} must select only #{components.join(', ')} Pods")
      end
    end

    def validate_metrics_ingress_rule(context, policy, name, port)
      ingress = Array(policy.spec["ingress"])
      unless ingress.length == 1
        context.add_error("#{name} must define exactly one metrics ingress rule")
        return
      end

      rule = ingress.first
      expected_sources = [{
        "namespaceSelector" => {
          "matchLabels" => { "mpp.kurodakayn.dev/metrics-scraper" => "true" },
        },
      }]
      unless same_entries?(Array(rule["from"]), expected_sources)
        context.add_error("#{name} must allow only metrics-scraper namespaces")
      end

      expected_ports = [{ "protocol" => "TCP", "port" => port }]
      unless same_entries?(Array(rule["ports"]), expected_ports)
        context.add_error("#{name} must target only TCP metrics port #{port}")
      end
    end

    def same_entries?(actual, expected)
      return false unless actual.length == expected.length

      remaining = expected.dup
      actual.each do |entry|
        index = remaining.index(entry)
        return false unless index

        remaining.delete_at(index)
      end
      remaining.empty?
    end
  end
end
