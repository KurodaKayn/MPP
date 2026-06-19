# frozen_string_literal: true

require_relative "../../env/env_contract"

module KubernetesValidation
  module AppBaseline
    EXPECTED_DEPLOYMENTS = [
      "frontend",
      "backend",
      "publish-worker",
      "browser-worker",
      "ai-service",
      "content-pipeline-service",
      "collab-service",
    ].freeze

    EXPECTED_SERVICES = [
      ["frontend", "http", 3000],
      ["backend", "http", 8080],
      ["browser-worker", "http", 8081],
      ["ai-service", "http", 8000],
      ["content-pipeline-service", "grpc", 50051],
      ["collab-service", "http", 8090],
    ].freeze

    CONFIG_KEYS = [
      "BACKEND_API_BASE_URL",
      "BROWSER_WORKER_URL",
      "AI_SERVICE_URL",
      "CONTENT_PIPELINE_HOST",
      "CONTENT_PIPELINE_PORT",
      "CONTENT_PIPELINE_MEDIA_RESOLVER_URL",
      "CONTENT_PIPELINE_MEDIA_OBJECT_STORE",
      "CONTENT_PIPELINE_MEDIA_OBJECT_PREFIX",
      "CONTENT_PIPELINE_MEDIA_OBJECT_REF_PREFIX",
      "CONTENT_PIPELINE_MEDIA_OBJECT_RETENTION_DAYS",
      "COLLAB_INTERNAL_URL",
      "COLLAB_WEBSOCKET_URL_BASE",
      "DB_HOST",
      "DB_SSLMODE",
      "DB_READER_HOST",
      "DB_READER_MAX_REPLICA_LAG",
      "DB_READER_LAG_CHECK_INTERVAL",
      "REDIS_ENDPOINT_MODE",
      "REDIS_ADDR",
      "REDIS_DB",
      "REDIS_TLS",
      "REDIS_TLS_CA_CERT",
      "REDIS_TLS_CA_FILE",
      "REDIS_TLS_SERVER_NAME",
      "REDIS_SENTINEL_ADDRS",
      "REDIS_SENTINEL_MASTER_NAME",
      "REDIS_POOL_SIZE",
      "REDIS_MIN_IDLE_CONNS",
      "REDIS_MAX_IDLE_CONNS",
      "REDIS_CONN_MAX_IDLE_TIME",
      "REDIS_CONN_MAX_LIFETIME",
      "OBJECT_STORAGE_PROVIDER",
      "R2_ACCOUNT_ID",
      "R2_BUCKET",
      "R2_ENDPOINT",
      "R2_REGION",
      "X_OAUTH2_CLIENT_ID",
      "X_OAUTH2_REDIRECT_URL",
      "X_OAUTH2_AUTHORIZE_URL",
      "X_OAUTH2_TOKEN_URL",
    ].freeze

    SECRET_KEYS = [
      "JWT_SECRET",
      "DB_PASSWORD",
      "COLLAB_TOKEN_SECRET",
      "COOKIE_ENCRYPTION_KEY",
      "LLM_PROVIDER_KEY",
      "AI_SERVICE_INTERNAL_TOKEN",
      "BROWSER_WORKER_INTERNAL_TOKEN",
      "CONTENT_PIPELINE_INTERNAL_TOKEN",
      "R2_ACCESS_KEY_ID",
      "R2_SECRET_ACCESS_KEY",
      "X_OAUTH2_CLIENT_SECRET",
    ].freeze

    INTERNAL_INGRESS_POLICIES = {
      "frontend-backend-access" => ["backend", 8080, ["frontend", "content-pipeline-service"]],
      "browser-worker-internal-access" => ["browser-worker", 8081, ["backend", "publish-worker"]],
      "ai-service-internal-access" => ["ai-service", 8000, ["backend", "publish-worker"]],
      "content-pipeline-internal-access" => [
        "content-pipeline-service",
        50051,
        ["backend", "publish-worker"],
      ],
      "collab-service-internal-access" => ["collab-service", 8090, ["backend", "publish-worker"]],
    }.freeze

    PUBLIC_INGRESS_POLICIES = {
      "public-frontend-access" => ["frontend", 3000],
      "public-collab-access" => ["collab-service", 8090],
    }.freeze

    APP_NETWORK_POLICIES = [
      "mpp-system-default-deny",
      *PUBLIC_INGRESS_POLICIES.keys,
      *INTERNAL_INGRESS_POLICIES.keys,
    ].freeze

    SELF_HOSTED_DATA_NETWORK_POLICIES = [
      "postgres-app-access",
      "postgres-reader-app-access",
      "pgbouncer-app-access",
      "pgbouncer-reader-app-access",
      "redis-app-access",
      "redis-ha-internal-access",
    ].freeze

    module_function

    def validate_overlay(context)
      placeholders = context.find_lines(/replace-me|replace-with-/)
      unless placeholders.empty?
        context.add_error("validation overlay still contains placeholders: #{placeholders.join('; ')}")
      end

      secret = context.require_document("Secret", "mpp-app-secrets")
      if secret
        SECRET_KEYS.each do |key|
          unless secret.data.key?(key)
            context.add_error("validation overlay mpp-app-secrets is missing #{key}")
          end
        end
      end

      ["postgres", "redis"].each do |service|
        context.require_document("Service", service)
      end

      config = context.require_document("ConfigMap", "mpp-app-config", "mpp-system")
      if config
        {
          "DB_HOST" => "postgres.example.invalid",
          "DB_SSLMODE" => "verify-full",
          "DB_READER_HOST" => "postgres-reader.example.invalid",
          "REDIS_ENDPOINT_MODE" => "direct",
          "REDIS_ADDR" => "redis:6379",
          "REDIS_SENTINEL_ADDRS" => "",
          "REDIS_SENTINEL_MASTER_NAME" => "mpp-redis-ha",
          "COLLAB_WEBSOCKET_URL_BASE" => "wss://mpp.example.invalid",
          "LLM_PROVIDER_URL" => "https://llm.example.invalid/v1",
          "LLM_MODEL" => "validation-model",
        }.each do |key, value|
          unless config.data[key] == value
            context.add_error("validation overlay mpp-app-config #{key} is not overridden")
          end
        end
      end

      ingress = context.require_document("Ingress", "mpp-public-gateway", "mpp-system")
      return unless ingress

      hosts = ingress_hosts(ingress)
      tls_hosts = Array(ingress.spec["tls"]).flat_map { |entry| Array(entry["hosts"]) }
      context.add_error("validation overlay Ingress host is not overridden") unless hosts.include?("mpp.example.invalid")
      context.add_error("validation overlay Ingress TLS host is not overridden") unless tls_hosts.include?("mpp.example.invalid")
    end

    def validate_workloads(context)
      EXPECTED_DEPLOYMENTS.each { |deployment| validate_deployment(context, deployment) }

      EXPECTED_DEPLOYMENTS.each do |deployment|
        document = context.require_document("Deployment", deployment, "mpp-system")
        next unless document

        unless consumes_config_map?(document, "mpp-app-config")
          context.add_error("Deployment #{deployment} must consume mpp-app-config")
        end
      end

      backend_secret_keys = [
        "JWT_SECRET",
        "DB_PASSWORD",
        "COOKIE_ENCRYPTION_KEY",
        "COLLAB_TOKEN_SECRET",
        "BROWSER_WORKER_INTERNAL_TOKEN",
        "AI_SERVICE_INTERNAL_TOKEN",
        "CONTENT_PIPELINE_INTERNAL_TOKEN",
        "R2_ACCESS_KEY_ID",
        "R2_SECRET_ACCESS_KEY",
        "X_OAUTH2_CLIENT_SECRET",
      ]
      require_secret_refs(context, "backend", backend_secret_keys)
      require_secret_refs(context, "publish-worker", backend_secret_keys)
      require_secret_refs(context, "browser-worker", ["REDIS_PASSWORD", "BROWSER_WORKER_INTERNAL_TOKEN"])
      require_secret_refs(context, "ai-service", ["LLM_PROVIDER_KEY", "AI_SERVICE_INTERNAL_TOKEN"])
      require_secret_refs(
        context,
        "content-pipeline-service",
        ["CONTENT_PIPELINE_INTERNAL_TOKEN", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"],
      )
      require_secret_refs(context, "collab-service", ["COLLAB_TOKEN_SECRET", "DB_PASSWORD", "REDIS_PASSWORD"])

      validate_content_pipeline(context)
      validate_services(context)
      validate_config_map(context)
      validate_ingress(context)
      validate_availability(context)
      validate_network_policies(context)
    end

    def validate_deployment(context, deployment)
      document = context.require_document("Deployment", deployment, "mpp-system")
      return unless document

      unless document.labels["app.kubernetes.io/component"] == deployment &&
             document.pod_labels["app.kubernetes.io/component"] == deployment
        context.add_error("Deployment #{deployment} is missing its component label")
      end

      pod_security = document.pod_spec["securityContext"] || {}
      context.add_error("Deployment #{deployment} must run as non-root") unless pod_security["runAsNonRoot"] == true
      context.add_error("Deployment #{deployment} must run as UID 10001") unless pod_security["runAsUser"] == 10_001
      context.add_error("Deployment #{deployment} must run as GID 10001") unless pod_security["runAsGroup"] == 10_001
      unless pod_security.dig("seccompProfile", "type") == "RuntimeDefault"
        context.add_error("Deployment #{deployment} must use RuntimeDefault seccomp")
      end

      if deployment == "browser-worker"
        unless document.pod_spec["serviceAccountName"] == "browser-worker-runtime-manager"
          context.add_error("browser-worker must use the runtime manager ServiceAccount")
        end
      elsif document.pod_spec["automountServiceAccountToken"] != false
        context.add_error("Deployment #{deployment} must not mount service account tokens")
      end

      document.containers.each do |container|
        validate_container_security(context, container, "Deployment #{deployment} container #{container['name']}")
        validate_container_probes(context, container, "Deployment #{deployment} container #{container['name']}")
        validate_resource_requests_and_limits(context, container, "Deployment #{deployment} container #{container['name']}")
      end
    end

    def validate_container_security(context, container, label)
      security = container["securityContext"] || {}
      context.add_error("#{label} must disable privilege escalation") unless security["allowPrivilegeEscalation"] == false
      drops = Array(security.dig("capabilities", "drop"))
      context.add_error("#{label} must drop all capabilities") unless drops.include?("ALL")
    end

    def validate_container_probes(context, container, label)
      context.add_error("#{label} must define a readiness probe") unless container.key?("readinessProbe")
      context.add_error("#{label} must define a liveness probe") unless container.key?("livenessProbe")
    end

    def validate_resource_requests_and_limits(context, container, label)
      requests = container.dig("resources", "requests") || {}
      limits = container.dig("resources", "limits") || {}
      ["cpu", "memory"].each do |resource|
        context.add_error("#{label} must define #{resource} requests") unless requests.key?(resource)
        context.add_error("#{label} must define #{resource} limits") unless limits.key?(resource)
      end
    end

    def consumes_config_map?(document, name)
      document.containers.any? do |container|
        Array(container["envFrom"]).any? { |entry| entry.dig("configMapRef", "name") == name }
      end
    end

    def require_secret_refs(context, deployment, keys)
      document = context.require_document("Deployment", deployment, "mpp-system")
      return unless document

      env = document.containers.flat_map { |container| Array(container["env"]) }
      keys.each do |key|
        found = env.any? { |entry| entry.dig("valueFrom", "secretKeyRef", "key") == key }
        context.add_error("Deployment #{deployment} must reference secret key #{key}") unless found
      end
    end

    def validate_content_pipeline(context)
      deployment = context.require_document("Deployment", "content-pipeline-service", "mpp-system")
      if deployment
        container = deployment.container("content-pipeline-service")
        ports = Array(container && container["ports"])
        port_numbers = ports.map { |port| port["containerPort"] }
        context.add_error("content-pipeline-service must expose gRPC port 50051") unless port_numbers.include?(50051)
        context.add_error("content-pipeline-service must expose metrics port 9090") unless port_numbers.include?(9090)
        readiness_port = container.dig("readinessProbe", "grpc", "port") if container
        liveness_port = container.dig("livenessProbe", "grpc", "port") if container
        unless readiness_port == 50051 && liveness_port == 50051
          context.add_error("content-pipeline-service must use gRPC readiness and liveness probes")
        end
      end

      service = context.require_document("Service", "content-pipeline-service", "mpp-system")
      return unless service

      metrics = Array(service.spec["ports"]).find { |port| port["name"] == "metrics" }
      context.add_error("content-pipeline-service Service must expose metrics port 9090") unless metrics && metrics["port"] == 9090
    end

    def validate_services(context)
      EXPECTED_SERVICES.each do |name, port_name, port_number|
        service = context.require_document("Service", name, "mpp-system")
        next unless service

        unless service.spec.dig("selector", "app.kubernetes.io/component") == name
          context.add_error("Service #{name} selector must target its component")
        end

        port = Array(service.spec["ports"]).find { |entry| entry["name"] == port_name }
        unless port && port["port"] == port_number
          context.add_error("Service #{name} must expose #{port_name} port #{port_number}")
        end
      end
    end

    def validate_config_map(context)
      config = context.require_document("ConfigMap", "mpp-app-config", "mpp-system")
      return unless config

      CONFIG_KEYS.each do |key|
        context.add_error("mpp-app-config is missing #{key}") unless config.data.key?(key)
      end

      validate_env_schema_types(context)
    end

    def validate_env_schema_types(context)
      variables = env_schema_variables
      CONFIG_KEYS.each do |key|
        spec = variables[key]
        if spec.nil?
          context.add_error("mpp-app-config key #{key} is not declared in contracts/env.schema.yaml")
        elsif spec["type"] == "secret"
          context.add_error("mpp-app-config must not contain secret env key #{key}")
        end
      end

      SECRET_KEYS.each do |key|
        spec = variables[key]
        if spec.nil?
          context.add_error("mpp-app-secrets key #{key} is not declared in contracts/env.schema.yaml")
        elsif spec["type"] != "secret"
          context.add_error("mpp-app-secrets key #{key} must be declared as a secret in contracts/env.schema.yaml")
        end
      end
    end

    def env_schema_variables
      @env_schema_variables ||= EnvContract.load_schema("contracts/env.schema.yaml").fetch("variables")
    end

    def validate_ingress(context)
      ingress = context.require_document("Ingress", "mpp-public-gateway", "mpp-system")
      return unless ingress

      context.add_error("mpp-public-gateway must define ingressClassName") unless ingress.spec["ingressClassName"]
      tls_secret_names = Array(ingress.spec["tls"]).map { |entry| entry["secretName"] }
      context.add_error("mpp-public-gateway must reference mpp-public-tls") unless tls_secret_names.include?("mpp-public-tls")

      paths = ingress_paths(ingress)
      collab = paths.find { |path| path["path"] == "/collab" }
      root = paths.find { |path| path["path"] == "/" }
      unless collab && collab.dig("backend", "service", "name") == "collab-service"
        context.add_error("mpp-public-gateway must route /collab to collab-service")
      end
      unless root && root.dig("backend", "service", "name") == "frontend"
        context.add_error("mpp-public-gateway must route / to frontend")
      end
    end

    def validate_availability(context)
      ["frontend", "backend"].each do |workload|
        context.require_document("PodDisruptionBudget", workload, "mpp-system")
        hpa = context.require_document("HorizontalPodAutoscaler", workload, "mpp-system")
        next unless hpa

        utilization = Array(hpa.spec["metrics"]).find { |metric| metric.dig("resource", "name") == "cpu" }
        unless utilization&.dig("resource", "target", "averageUtilization") == 70
          context.add_error("HPA #{workload} must target CPU average utilization 70")
        end
      end
    end

    def validate_network_policies(context)
      validate_expected_network_policy_set(context)
      validate_default_deny_policy(context)

      PUBLIC_INGRESS_POLICIES.each do |name, (target_component, port)|
        validate_public_ingress_policy(context, name, target_component, port)
      end

      INTERNAL_INGRESS_POLICIES.each do |name, (target_component, port, allowed_components)|
        validate_internal_ingress_policy(context, name, target_component, port, allowed_components)
      end
    end

    def validate_expected_network_policy_set(context)
      expected = APP_NETWORK_POLICIES.dup
      if context.path_suffix?("deploy/kubernetes/overlays/staging-self-hosted") ||
         context.path_suffix?("deploy/kubernetes/overlays/production-self-hosted-ha")
        expected.concat(SELF_HOSTED_DATA_NETWORK_POLICIES)
        expected << "redis-cluster-internal-access" if context.path_suffix?("deploy/kubernetes/overlays/staging-self-hosted")
      end

      actual = context.documents
        .select { |document| document.kind == "NetworkPolicy" && document.namespace == "mpp-system" }
        .map(&:name)
      duplicates = actual.select { |name| actual.count(name) > 1 }.uniq
      unless duplicates.empty?
        context.add_error(
          "#{context.package_dir} must not include duplicate mpp-system NetworkPolicies: #{duplicates.sort.join(', ')}",
        )
      end

      unexpected = actual - expected
      return if unexpected.empty?

      context.add_error(
        "#{context.package_dir} must not include unexpected mpp-system NetworkPolicies: #{unexpected.sort.join(', ')}",
      )
    end

    def validate_default_deny_policy(context)
      policy = context.require_document("NetworkPolicy", "mpp-system-default-deny", "mpp-system")
      return unless policy

      context.add_error("mpp-system-default-deny must select all Pods") unless policy.spec["podSelector"] == {}

      types = Array(policy.spec["policyTypes"])
      context.add_error("mpp-system-default-deny must deny ingress by default") unless types.include?("Ingress")
      ingress = Array(policy.spec["ingress"])
      context.add_error("mpp-system-default-deny must not define ingress allow rules") unless ingress.empty?
    end

    def validate_public_ingress_policy(context, name, target_component, port)
      policy = context.require_document("NetworkPolicy", name, "mpp-system")
      return unless policy

      validate_policy_target(context, policy, name, target_component)
      validate_policy_ingress_type(context, policy, name)
      validate_exact_ingress_rule(
        context,
        policy,
        name,
        [{
          "namespaceSelector" => {
            "matchLabels" => { "mpp.kurodakayn.dev/public-ingress" => "true" },
          },
        }],
        port,
        "public-ingress namespaces",
      )
    end

    def validate_internal_ingress_policy(context, name, target_component, port, allowed_components)
      policy = context.require_document("NetworkPolicy", name, "mpp-system")
      return unless policy

      validate_policy_target(context, policy, name, target_component)
      validate_policy_ingress_type(context, policy, name)
      expected_sources = allowed_components.map do |component|
        {
          "podSelector" => {
            "matchLabels" => {
              "app.kubernetes.io/name" => "mpp",
              "app.kubernetes.io/component" => component,
            },
          },
        }
      end
      validate_exact_ingress_rule(
        context,
        policy,
        name,
        expected_sources,
        port,
        "#{allowed_components.join(', ')} Pods",
      )
    end

    def validate_policy_target(context, policy, name, target_component)
      expected = {
        "matchLabels" => {
          "app.kubernetes.io/name" => "mpp",
          "app.kubernetes.io/component" => target_component,
        },
      }
      unless policy.spec["podSelector"] == expected
        context.add_error("#{name} must select #{target_component} Pods")
      end
    end

    def validate_policy_ingress_type(context, policy, name)
      types = Array(policy.spec["policyTypes"])
      context.add_error("#{name} must be an ingress policy") unless types.include?("Ingress")
    end

    def validate_exact_ingress_rule(context, policy, name, expected_sources, port, source_label)
      ingress = Array(policy.spec["ingress"])
      unless ingress.length == 1
        context.add_error("#{name} must define exactly one ingress allow rule")
        return
      end

      rule = ingress.first
      unless same_entries?(Array(rule["from"]), expected_sources)
        context.add_error("#{name} must allow only #{source_label}")
      end

      expected_ports = [{ "protocol" => "TCP", "port" => port }]
      unless same_entries?(Array(rule["ports"]), expected_ports)
        context.add_error("#{name} must target only TCP port #{port}")
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

    def ingress_hosts(ingress)
      Array(ingress.spec["rules"]).map { |rule| rule["host"] }
    end

    def ingress_paths(ingress)
      Array(ingress.spec["rules"]).flat_map do |rule|
        Array(rule.dig("http", "paths"))
      end
    end
  end
end
