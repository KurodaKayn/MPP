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
  end
end
