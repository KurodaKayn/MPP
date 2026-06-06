# frozen_string_literal: true

module KubernetesValidation
  module Observability
    module_function

    def validate(context)
      validate_namespace(context)
      validate_alloy(context)
      validate_pod_monitors(context)
      validate_alerts(context)
      validate_browser_worker_metrics_policy(context)
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

    def validate_browser_worker_metrics_policy(context)
      policy = context.require_document("NetworkPolicy", "browser-worker-observability-metrics", "mpp-system")
      return unless policy

      namespace_labels = Array(policy.spec["ingress"]).flat_map do |rule|
        Array(rule["from"]).map { |entry| entry.dig("namespaceSelector", "matchLabels") }
      end.compact
      unless namespace_labels.any? { |labels| labels["mpp.kurodakayn.dev/metrics-scraper"] == "true" }
        context.add_error("browser-worker-observability-metrics must allow metrics-scraper namespaces")
      end

      ports = Array(policy.spec["ingress"]).flat_map { |rule| Array(rule["ports"]) }.map { |port| port["port"] }
      context.add_error("browser-worker-observability-metrics must target browser-worker port 8081") unless ports.include?(8081)
    end
  end
end
