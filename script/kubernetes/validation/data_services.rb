# frozen_string_literal: true

module KubernetesValidation
  module DataServices
    module_function

    def validate_managed(context)
      [["postgres", 5432], ["redis", 6379]].each do |name, port|
        service = context.require_document("Service", name, "mpp-system")
        next unless service

        context.add_error("managed #{name} Service must be ExternalName") unless service.spec["type"] == "ExternalName"
        service_port = Array(service.spec["ports"]).find { |entry| entry["port"] == port }
        context.add_error("managed #{name} Service must expose port #{port}") unless service_port
        unless service.labels["app.kubernetes.io/managed-by"] == "external-provider"
          context.add_error("managed #{name} Service must be labeled as external-provider managed")
        end
      end
    end

    def validate_self_hosted(context)
      [["postgres", 5432], ["redis", 6379]].each do |name, port|
        service = context.require_document("Service", name, "mpp-system")
        if service
          service_port = Array(service.spec["ports"]).find { |entry| entry["port"] == port }
          context.add_error("self-hosted #{name} Service must expose port #{port}") unless service_port
        end

        stateful_set = context.require_document("StatefulSet", name, "mpp-system")
        next unless stateful_set

        pod_spec = stateful_set.pod_spec
        pod_security = pod_spec["securityContext"] || {}
        context.add_error("self-hosted #{name} StatefulSet must not mount service account tokens") unless pod_spec["automountServiceAccountToken"] == false
        context.add_error("self-hosted #{name} StatefulSet must run as non-root") unless pod_security["runAsNonRoot"] == true
        unless pod_security["fsGroupChangePolicy"] == "OnRootMismatch"
          context.add_error("self-hosted #{name} StatefulSet must use OnRootMismatch fsGroup changes")
        end
        unless Array(stateful_set.spec["volumeClaimTemplates"]).any?
          context.add_error("self-hosted #{name} StatefulSet must define persistent storage")
        end

        stateful_set.containers.each do |container|
          validate_container(context, container, "self-hosted #{name} container #{container['name']}")
        end
      end

      validate_self_hosted_network_policy(context, "postgres", 5432, ["backend", "publish-worker", "collab-service"])
      validate_self_hosted_network_policy(context, "redis", 6379, ["backend", "publish-worker", "browser-worker", "collab-service"])
    end

    def validate_container(context, container, label)
      context.add_error("#{label} must define readinessProbe") unless container.key?("readinessProbe")
      context.add_error("#{label} must define livenessProbe") unless container.key?("livenessProbe")

      requests = container.dig("resources", "requests") || {}
      limits = container.dig("resources", "limits") || {}
      ["cpu", "memory"].each do |resource|
        context.add_error("#{label} must define #{resource} requests") unless requests.key?(resource)
        context.add_error("#{label} must define #{resource} limits") unless limits.key?(resource)
      end
    end

    def validate_self_hosted_network_policy(context, name, port, allowed_components)
      policy = context.require_document("NetworkPolicy", "#{name}-app-access", "mpp-system")
      return unless policy

      selector = policy.spec.dig("podSelector", "matchLabels") || {}
      unless selector["app.kubernetes.io/component"] == name
        context.add_error("self-hosted #{name} NetworkPolicy must select #{name} Pods")
      end

      types = Array(policy.spec["policyTypes"])
      context.add_error("self-hosted #{name} NetworkPolicy must be an ingress policy") unless types.include?("Ingress")

      from_entries = Array(policy.spec["ingress"]).flat_map { |rule| Array(rule["from"]) }
      components = from_entries.map { |entry| entry.dig("podSelector", "matchLabels", "app.kubernetes.io/component") }.compact
      allowed_components.each do |component|
        unless components.include?(component)
          context.add_error("self-hosted #{name} NetworkPolicy must allow #{component} ingress")
        end
      end

      ports = Array(policy.spec["ingress"]).flat_map { |rule| Array(rule["ports"]) }.map { |entry| entry["port"] }
      context.add_error("self-hosted #{name} NetworkPolicy must target port #{port}") unless ports.include?(port)
    end
  end
end
