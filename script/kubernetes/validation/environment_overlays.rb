# frozen_string_literal: true

require "base64"
require "uri"

module KubernetesValidation
  module EnvironmentOverlays
    DEPLOYABLE_VALIDATION_ENV = "MPP_KUBERNETES_VALIDATE_DEPLOYABLE"
    GHCR_APP_IMAGE_PREFIX = "ghcr.io/kurodakayn/mpp-"
    SHA_IMAGE_PATTERN = /:sha-[0-9a-f]{40}\z/
    ALL_ZERO_SHA_IMAGE_PATTERN = /:sha-0{40}\z/
    EXAMPLE_COOKIE_ENCRYPTION_KEY = "12345678901234567890123456789012"
    EXAMPLE_SECRET_PREFIX = "staging-example-"

    module_function

    def validate_staging_self_hosted(context)
      validate_config(context)
      validate_secret(context)
      validate_ingress(context)
      validate_runtime_image(context)
      validate_app_images(context)
    end

    def validate_config(context)
      config = context.require_document("ConfigMap", "mpp-app-config", "mpp-system")
      return unless config

      {
        "APP_ENV" => "staging",
        "DB_HOST" => "postgres",
        "DB_SSLMODE" => "disable",
        "REDIS_ADDR" => "redis:6379",
        "REDIS_TLS" => "false",
      }.each do |key, value|
        unless config.data[key] == value
          context.add_error("staging-self-hosted mpp-app-config #{key} must be #{value}")
        end
      end

      validate_collab_websocket_url(context, config.data["COLLAB_WEBSOCKET_URL_BASE"].to_s)
      validate_llm_provider_url(context, config.data["LLM_PROVIDER_URL"].to_s)

      llm_model = config.data["LLM_MODEL"].to_s.strip
      context.add_error("staging-self-hosted mpp-app-config LLM_MODEL must be set") if llm_model.empty?
      if deployable_validation? && llm_model == "staging-validation-model"
        context.add_error("staging-self-hosted mpp-app-config LLM_MODEL must not use the example model in deployable validation")
      end
    end

    def validate_secret(context)
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
      ].each do |key|
        value = secret_value(secret, key)
        if value.empty?
          context.add_error("staging-self-hosted mpp-app-secrets is missing #{key}")
        elsif deployable_validation? && example_secret_value?(value)
          context.add_error("staging-self-hosted mpp-app-secrets #{key} must not use an example value in deployable validation")
        end
      end
    end

    def validate_ingress(context)
      ingress = context.require_document("Ingress", "mpp-public-gateway", "mpp-system")
      return unless ingress

      hosts = AppBaseline.ingress_hosts(ingress)
      tls_hosts = Array(ingress.spec["tls"]).flat_map { |entry| Array(entry["hosts"]) }
      context.add_error("staging-self-hosted Ingress must define a host") if hosts.empty?
      hosts.each do |host|
        context.add_error("staging-self-hosted Ingress host must have a TLS entry") unless tls_hosts.include?(host)
        if deployable_validation? && example_host?(host)
          context.add_error("staging-self-hosted Ingress host must not use example.invalid in deployable validation")
        end
      end
    end

    def validate_runtime_image(context)
      deployment = context.require_document("Deployment", "browser-worker", "mpp-system")
      return unless deployment

      env = deployment.containers.flat_map { |container| Array(container["env"]) }
      runtime_image = env.find { |entry| entry["name"] == "BROWSER_RUNTIME_IMAGE" }&.fetch("value", nil).to_s
      validate_sha_image(context, runtime_image, "staging-self-hosted BROWSER_RUNTIME_IMAGE")
    end

    def validate_app_images(context)
      Images.image_lines(context).each do |line|
        image = line[:value]
        next unless image.start_with?(GHCR_APP_IMAGE_PREFIX)

        validate_sha_image(context, image, "staging-self-hosted image at line #{line[:line_number]}")
      end
    end

    def validate_collab_websocket_url(context, value)
      uri = parse_uri(value)
      if uri.nil? || uri.scheme != "wss" || uri.host.to_s.empty?
        context.add_error("staging-self-hosted mpp-app-config COLLAB_WEBSOCKET_URL_BASE must be a wss URL")
        return
      end

      ingress = context.document("Ingress", "mpp-public-gateway", "mpp-system")
      hosts = ingress ? AppBaseline.ingress_hosts(ingress) : []
      unless hosts.include?(uri.host)
        context.add_error("staging-self-hosted COLLAB_WEBSOCKET_URL_BASE host must match an Ingress host")
      end
      if deployable_validation? && example_host?(uri.host)
        context.add_error("staging-self-hosted COLLAB_WEBSOCKET_URL_BASE must not use example.invalid in deployable validation")
      end
    end

    def validate_llm_provider_url(context, value)
      uri = parse_uri(value)
      if uri.nil? || uri.scheme != "https" || uri.host.to_s.empty?
        context.add_error("staging-self-hosted mpp-app-config LLM_PROVIDER_URL must be an https URL")
        return
      end

      if deployable_validation? && example_host?(uri.host)
        context.add_error("staging-self-hosted LLM_PROVIDER_URL must not use example.invalid in deployable validation")
      end
    end

    def validate_sha_image(context, image, label)
      unless image.start_with?(GHCR_APP_IMAGE_PREFIX) && image.match?(SHA_IMAGE_PATTERN)
        context.add_error("#{label} must use a ghcr.io/kurodakayn/mpp-* sha tag")
      end
      if deployable_validation? && image.match?(ALL_ZERO_SHA_IMAGE_PATTERN)
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

    def example_host?(host)
      host.end_with?(".example.invalid") || host == "example.invalid"
    end

    def deployable_validation?
      ["1", "true", "yes"].include?(ENV.fetch(DEPLOYABLE_VALIDATION_ENV, "").downcase)
    end
  end
end
