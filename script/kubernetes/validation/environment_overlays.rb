# frozen_string_literal: true

require "base64"
require "uri"

module KubernetesValidation
  module EnvironmentOverlays
    DEPLOYABLE_VALIDATION_ENV = "MPP_KUBERNETES_VALIDATE_DEPLOYABLE"
    RUNTIME_IMAGE_REPOSITORY = "ghcr.io/kurodakayn/mpp-browser-runtime"
    ALL_ZERO_SHA_TAG = "sha-0000000000000000000000000000000000000000"
    EXAMPLE_COOKIE_ENCRYPTION_KEY = "12345678901234567890123456789012"
    EXAMPLE_SECRET_PREFIX = "staging-example-"
    EXAMPLE_LLM_MODELS = [
      "staging-validation-model",
      "production-provider-model",
      "replace-with-production-model",
    ].freeze
    REQUIRED_EXTERNAL_SECRET_KEYS = [
      "JWT_SECRET",
      "DB_PASSWORD",
      "COLLAB_TOKEN_SECRET",
      "COOKIE_ENCRYPTION_KEY",
      "LLM_PROVIDER_KEY",
      "BROWSER_WORKER_INTERNAL_TOKEN",
      "AI_SERVICE_INTERNAL_TOKEN",
      "CONTENT_PIPELINE_INTERNAL_TOKEN",
      "R2_ACCESS_KEY_ID",
      "R2_SECRET_ACCESS_KEY",
    ].freeze
    OPTIONAL_EXTERNAL_SECRET_KEYS = ["REDIS_PASSWORD"].freeze
    APP_IMAGES = {
      "frontend" => ["frontend", "ghcr.io/kurodakayn/mpp-frontend"],
      "backend" => ["backend", "ghcr.io/kurodakayn/mpp-backend"],
      "publish-worker" => ["publish-worker", "ghcr.io/kurodakayn/mpp-backend"],
      "browser-worker" => ["browser-worker", "ghcr.io/kurodakayn/mpp-browser-worker"],
      "ai-service" => ["ai-service", "ghcr.io/kurodakayn/mpp-ai-service"],
      "content-pipeline-service" => [
        "content-pipeline-service",
        "ghcr.io/kurodakayn/mpp-content-pipeline-service",
      ],
      "collab-service" => ["collab-service", "ghcr.io/kurodakayn/mpp-collab-service"],
    }.freeze

    module_function

    def validate_staging_self_hosted(context)
      overlay = "staging-self-hosted"
      validate_self_hosted_config(context, overlay)
      validate_secret(context, overlay)
      validate_ingress(context, overlay)
      validate_runtime_image(context, overlay)
      validate_app_images(context, overlay)
    end

    def validate_staging_managed(context)
      overlay = "staging-managed"
      validate_managed_config(context, overlay, app_env: "staging")
      validate_secret(context, overlay)
      validate_ingress(context, overlay)
      validate_runtime_image(context, overlay)
      validate_app_images(context, overlay)
      validate_managed_services(context, overlay)
    end

    def validate_production_managed(context)
      overlay = "production-managed"
      validate_managed_config(context, overlay, app_env: "production")
      validate_external_secret_contract(context, overlay)
      validate_ingress(context, overlay)
      validate_runtime_image(context, overlay)
      validate_app_images(context, overlay)
      validate_managed_services(context, overlay)
    end

    def validate_self_hosted_config(context, overlay)
      config = context.require_document("ConfigMap", "mpp-app-config", "mpp-system")
      return unless config

      {
        "APP_ENV" => "staging",
        "DB_HOST" => "pgbouncer",
        "DB_SSLMODE" => "disable",
        "REDIS_ADDR" => "redis:6379",
        "REDIS_TLS" => "false",
      }.each do |key, value|
        unless config.data[key] == value
          context.add_error("#{overlay} mpp-app-config #{key} must be #{value}")
        end
      end

      validate_common_config(context, config, overlay)
    end

    def validate_managed_config(context, overlay, app_env:)
      config = context.require_document("ConfigMap", "mpp-app-config", "mpp-system")
      return unless config

      {
        "APP_ENV" => app_env,
        "DB_SSLMODE" => "verify-full",
        "REDIS_TLS" => "true",
      }.each do |key, value|
        unless config.data[key] == value
          context.add_error("#{overlay} mpp-app-config #{key} must be #{value}")
        end
      end

      postgres = context.document("Service", "postgres", "mpp-system")
      postgres_host = postgres&.spec&.fetch("externalName", nil).to_s
      db_host = config.data["DB_HOST"].to_s
      context.add_error("#{overlay} mpp-app-config DB_HOST must be set") if db_host.empty?
      unless postgres_host.empty? || db_host == postgres_host
        context.add_error("#{overlay} mpp-app-config DB_HOST must match the managed postgres ExternalName")
      end
      if deployable_validation? && example_host?(db_host)
        context.add_error("#{overlay} mpp-app-config DB_HOST must not use example.invalid in deployable validation")
      end

      redis = context.document("Service", "redis", "mpp-system")
      redis_host = redis&.spec&.fetch("externalName", nil).to_s
      redis_addr = config.data["REDIS_ADDR"].to_s
      redis_addr_host = address_host(redis_addr)
      context.add_error("#{overlay} mpp-app-config REDIS_ADDR must be set") if redis_addr.empty?
      unless redis_host.empty? || redis_addr_host == redis_host
        context.add_error("#{overlay} mpp-app-config REDIS_ADDR host must match the managed redis ExternalName")
      end
      if deployable_validation? && example_host?(redis_addr_host)
        context.add_error("#{overlay} mpp-app-config REDIS_ADDR must not use example.invalid in deployable validation")
      end

      validate_common_config(context, config, overlay)
    end

    def validate_common_config(context, config, overlay)
      validate_collab_websocket_url(context, config.data["COLLAB_WEBSOCKET_URL_BASE"].to_s, overlay)
      validate_llm_provider_url(context, config.data["LLM_PROVIDER_URL"].to_s, overlay)

      llm_model = config.data["LLM_MODEL"].to_s.strip
      context.add_error("#{overlay} mpp-app-config LLM_MODEL must be set") if llm_model.empty?
      if deployable_validation? && example_llm_model?(llm_model)
        context.add_error("#{overlay} mpp-app-config LLM_MODEL must not use the example model in deployable validation")
      end
    end

    def validate_secret(context, overlay)
      secret = context.require_document("Secret", "mpp-app-secrets", "mpp-system")
      return unless secret

      [
        "JWT_SECRET",
        "DB_PASSWORD",
        "REDIS_PASSWORD",
        "COLLAB_TOKEN_SECRET",
        "COOKIE_ENCRYPTION_KEY",
        "LLM_PROVIDER_KEY",
        "BROWSER_WORKER_INTERNAL_TOKEN",
        "AI_SERVICE_INTERNAL_TOKEN",
        "CONTENT_PIPELINE_INTERNAL_TOKEN",
        "R2_ACCESS_KEY_ID",
        "R2_SECRET_ACCESS_KEY",
      ].each do |key|
        value = secret_value(secret, key)
        if value.empty?
          context.add_error("#{overlay} mpp-app-secrets is missing #{key}")
        elsif deployable_validation? && example_secret_value?(value)
          context.add_error("#{overlay} mpp-app-secrets #{key} must not use an example value in deployable validation")
        end
      end
    end

    def validate_external_secret_contract(context, overlay)
      if context.document("Secret", "mpp-app-secrets", "mpp-system")
        context.add_error(
          "#{overlay} must not render mpp-app-secrets; materialize it through the production secret workflow",
        )
      end

      refs = secret_refs(context)
      mpp_refs = refs.select { |ref| ref[:secret_name] == "mpp-app-secrets" }
      required_refs = mpp_refs.reject { |ref| ref[:optional] }.map { |ref| ref[:key] }.uniq.sort
      optional_refs = mpp_refs.select { |ref| ref[:optional] }.map { |ref| ref[:key] }.uniq.sort
      missing_required = REQUIRED_EXTERNAL_SECRET_KEYS - required_refs
      unexpected_required = required_refs - REQUIRED_EXTERNAL_SECRET_KEYS
      unexpected_optional = optional_refs - OPTIONAL_EXTERNAL_SECRET_KEYS
      tracked_keys = REQUIRED_EXTERNAL_SECRET_KEYS + OPTIONAL_EXTERNAL_SECRET_KEYS
      wrong_secret_refs = refs.select do |ref|
        tracked_keys.include?(ref[:key]) && ref[:secret_name] != "mpp-app-secrets"
      end

      unless missing_required.empty?
        context.add_error("#{overlay} external secret contract is missing required refs: #{missing_required.join(', ')}")
      end
      unless unexpected_required.empty?
        context.add_error("#{overlay} external secret contract has unexpected required refs: #{unexpected_required.join(', ')}")
      end
      unless unexpected_optional.empty?
        context.add_error("#{overlay} external secret contract has unexpected optional refs: #{unexpected_optional.join(', ')}")
      end
      unless wrong_secret_refs.empty?
        formatted = wrong_secret_refs
          .map { |ref| "#{ref[:env_name]}->#{ref[:secret_name]}/#{ref[:key]}" }
          .uniq
          .sort
        context.add_error("#{overlay} external secret contract must use mpp-app-secrets for refs: #{formatted.join(', ')}")
      end
    end

    def validate_ingress(context, overlay)
      ingress = context.require_document("Ingress", "mpp-public-gateway", "mpp-system")
      return unless ingress

      hosts = AppBaseline.ingress_hosts(ingress)
      tls_hosts = Array(ingress.spec["tls"]).flat_map { |entry| Array(entry["hosts"]) }
      context.add_error("#{overlay} Ingress must define a host") if hosts.empty?
      hosts.each do |host|
        context.add_error("#{overlay} Ingress host must have a TLS entry") unless tls_hosts.include?(host)
        if deployable_validation? && example_host?(host)
          context.add_error("#{overlay} Ingress host must not use example.invalid in deployable validation")
        end
      end
    end

    def validate_runtime_image(context, overlay)
      deployment = context.require_document("Deployment", "browser-worker", "mpp-system")
      return unless deployment

      env = deployment.containers.flat_map { |container| Array(container["env"]) }
      runtime_image = env.find { |entry| entry["name"] == "BROWSER_RUNTIME_IMAGE" }&.fetch("value", nil).to_s
      validate_sha_image(context, runtime_image, "#{overlay} BROWSER_RUNTIME_IMAGE", RUNTIME_IMAGE_REPOSITORY)
    end

    def validate_app_images(context, overlay)
      APP_IMAGES.each do |deployment_name, (container_name, repository)|
        deployment = context.require_document("Deployment", deployment_name, "mpp-system")
        next unless deployment

        container = deployment.container(container_name)
        if container.nil?
          context.add_error("#{overlay} Deployment #{deployment_name} must define container #{container_name}")
          next
        end

        validate_sha_image(
          context,
          container["image"].to_s,
          "#{overlay} Deployment #{deployment_name} image",
          repository,
        )
      end
    end

    def validate_managed_services(context, overlay)
      {
        "postgres" => 5432,
        "redis" => 6379,
      }.each do |name, port|
        service = context.require_document("Service", name, "mpp-system")
        next unless service

        external_name = service.spec["externalName"].to_s
        context.add_error("#{overlay} managed #{name} ExternalName must be set") if external_name.empty?
        if deployable_validation? && example_host?(external_name)
          context.add_error("#{overlay} managed #{name} ExternalName must not use example.invalid in deployable validation")
        end

        service_port = Array(service.spec["ports"]).find { |entry| entry["port"] == port }
        context.add_error("#{overlay} managed #{name} Service must expose port #{port}") unless service_port
      end
    end

    def validate_collab_websocket_url(context, value, overlay)
      uri = parse_uri(value)
      if uri.nil? || uri.scheme != "wss" || uri.host.to_s.empty?
        context.add_error("#{overlay} mpp-app-config COLLAB_WEBSOCKET_URL_BASE must be a wss URL")
        return
      end

      ingress = context.document("Ingress", "mpp-public-gateway", "mpp-system")
      hosts = ingress ? AppBaseline.ingress_hosts(ingress) : []
      unless hosts.include?(uri.host)
        context.add_error("#{overlay} COLLAB_WEBSOCKET_URL_BASE host must match an Ingress host")
      end
      if deployable_validation? && example_host?(uri.host)
        context.add_error("#{overlay} COLLAB_WEBSOCKET_URL_BASE must not use example.invalid in deployable validation")
      end
    end

    def validate_llm_provider_url(context, value, overlay)
      uri = parse_uri(value)
      if uri.nil? || uri.scheme != "https" || uri.host.to_s.empty?
        context.add_error("#{overlay} mpp-app-config LLM_PROVIDER_URL must be an https URL")
        return
      end

      if deployable_validation? && example_host?(uri.host)
        context.add_error("#{overlay} LLM_PROVIDER_URL must not use example.invalid in deployable validation")
      end
    end

    def validate_sha_image(context, image, label, repository)
      unless image.match?(/\A#{Regexp.escape(repository)}:sha-[0-9a-f]{40}\z/)
        context.add_error("#{label} must use #{repository}:sha-<40 hex>")
      end
      if deployable_validation? && image == "#{repository}:#{ALL_ZERO_SHA_TAG}"
        context.add_error("#{label} must not use the all-zero example sha tag in deployable validation")
      end
    end

    def parse_uri(value)
      URI.parse(value)
    rescue URI::InvalidURIError
      nil
    end

    def secret_value(secret, key)
      encoded = secret.data[key].to_s
      Base64.strict_decode64(encoded)
    rescue ArgumentError
      encoded
    end

    def example_secret_value?(value)
      value.start_with?(EXAMPLE_SECRET_PREFIX) || value == EXAMPLE_COOKIE_ENCRYPTION_KEY
    end

    def example_llm_model?(value)
      EXAMPLE_LLM_MODELS.include?(value) || value.start_with?("replace-with-")
    end

    def example_host?(host)
      host.end_with?(".example.invalid") || host == "example.invalid"
    end

    def address_host(value)
      raw = value.to_s.strip
      uri = parse_uri(raw)
      return uri.host.to_s if uri&.scheme && !uri.host.to_s.empty?

      raw.split(":", 2).first.to_s
    end

    def secret_refs(context)
      context.documents.flat_map do |document|
        document.containers.flat_map do |container|
          Array(container["env"]).map do |entry|
            secret_ref = entry.dig("valueFrom", "secretKeyRef")
            next unless secret_ref

            {
              env_name: entry["name"].to_s,
              secret_name: secret_ref["name"].to_s,
              key: secret_ref["key"].to_s,
              optional: secret_ref["optional"] == true,
            }
          end.compact
        end
      end
    end

    def deployable_validation?
      ["1", "true", "yes"].include?(ENV.fetch(DEPLOYABLE_VALIDATION_ENV, "").downcase)
    end
  end
end
