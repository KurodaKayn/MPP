# frozen_string_literal: true

module KubernetesValidation
  module BrowserRuntimeControl
    module_function

    def validate(context)
      validate_namespaces(context)
      validate_rbac(context)
      validate_network_policies(context)
      validate_admission_policy(context)
    end

    def validate_namespaces(context)
      ["mpp-system", "mpp-browser-runtime"].each do |namespace|
        document = context.require_document("Namespace", namespace)
        next unless document

        labels = document.labels
        ["enforce", "audit", "warn"].each do |mode|
          key = "pod-security.kubernetes.io/#{mode}"
          context.add_error("Namespace #{namespace} must #{mode} restricted Pod security") unless labels[key] == "restricted"
        end
      end
    end

    def validate_rbac(context)
      context.require_document("ServiceAccount", "browser-worker-runtime-manager", "mpp-system")

      role = context.require_document("Role", "browser-runtime-manager", "mpp-browser-runtime")
      if role
        verbs = Array(role["rules"]).flat_map { |rule| Array(rule["verbs"]) }
        ["create", "get", "list", "watch", "delete"].each do |verb|
          context.add_error("browser-runtime-manager Role is missing #{verb} permission") unless verbs.include?(verb)
        end
      end

      binding = context.require_document("RoleBinding", "browser-runtime-manager", "mpp-browser-runtime")
      return unless binding

      subjects = Array(binding["subjects"])
      matched = subjects.any? do |subject|
        subject["kind"] == "ServiceAccount" &&
          subject["name"] == "browser-worker-runtime-manager" &&
          subject["namespace"] == "mpp-system"
      end
      context.add_error("browser-runtime-manager RoleBinding must bind the browser-worker ServiceAccount") unless matched
    end

    def validate_network_policies(context)
      default_deny = context.require_document("NetworkPolicy", "browser-runtime-default-deny", "mpp-browser-runtime")
      if default_deny
        context.add_error("browser-runtime-default-deny must select all Pods") unless default_deny.spec["podSelector"] == {}
        types = Array(default_deny.spec["policyTypes"])
        unless types.include?("Ingress") && types.include?("Egress")
          context.add_error("browser-runtime-default-deny must deny ingress and egress by default")
        end
      end

      private_access = context.require_document("NetworkPolicy", "browser-runtime-private-access", "mpp-browser-runtime")
      return unless private_access

      ports = Array(private_access.spec["ingress"]).flat_map { |rule| Array(rule["ports"]) }
      ports += Array(private_access.spec["egress"]).flat_map { |rule| Array(rule["ports"]) }
      port_numbers = ports.map { |port| port["port"] }
      [9222, 6080, 53, 80, 443].each do |port|
        context.add_error("browser-runtime-private-access is missing port #{port}") unless port_numbers.include?(port)
      end

      namespace_labels = Array(private_access.spec["ingress"]).flat_map do |rule|
        Array(rule["from"]).map { |entry| entry.dig("namespaceSelector", "matchLabels") }
      end.compact
      unless namespace_labels.any? { |labels| labels["mpp.kurodakayn.dev/browser-worker-namespace"] == "true" }
        context.add_error("browser-runtime-private-access must select the browser-worker namespace")
      end

      egress_blocks = Array(private_access.spec["egress"]).flat_map { |rule| Array(rule["to"]) }
      cidrs = egress_blocks.map { |entry| entry.dig("ipBlock", "cidr") }.compact
      context.add_error("browser-runtime-private-access must define constrained web egress") unless cidrs.include?("0.0.0.0/0")
    end

    def validate_admission_policy(context)
      policy = context.require_document("ValidatingAdmissionPolicy", "mpp-browser-runtime-pods")
      if policy
        expressions = Array(policy.spec["validations"]).map { |validation| validation["expression"].to_s }.join(" ")
        [
          "startsWith('mpp-browser-')",
          "restartPolicy == 'Never'",
          "activeDeadlineSeconds",
          "automountServiceAccountToken",
          "object.spec.securityContext",
          "runAsUser == 1000",
          "runAsGroup == 1000",
          "allowPrivilegeEscalation",
          "RuntimeDefault",
        ].each do |fragment|
          context.add_error("mpp-browser-runtime-pods admission policy is missing #{fragment}") unless expressions.include?(fragment)
        end

        [9222, 6080].each do |port|
          unless expressions.match?(/containerPort\s*==\s*#{port}\b/)
            context.add_error("mpp-browser-runtime-pods admission policy is missing runtime port #{port}")
          end
        end
      end

      context.require_document("ValidatingAdmissionPolicyBinding", "mpp-browser-runtime-pods")
    end
  end
end
