# frozen_string_literal: true

module KubernetesValidation
  module ExternalSecrets
    APP_SECRET_NAME = "mpp-app-secrets"
    APP_SECRET_NAMESPACE = "mpp-system"
    APP_SECRET_API_VERSION = "external-secrets.io/v1"
    APP_SECRET_STORE_KINDS = ["SecretStore", "ClusterSecretStore"].freeze
    REQUIRED_APP_SECRET_KEYS = [
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
    OPTIONAL_APP_SECRET_KEYS = ["REDIS_PASSWORD"].freeze

    module_function

    def validate_app_secret_contract(context, overlay)
      if context.document("Secret", APP_SECRET_NAME, APP_SECRET_NAMESPACE)
        context.add_error("#{overlay} must not render raw #{APP_SECRET_NAME}; use an ExternalSecret")
      end

      external_secret = context.require_document("ExternalSecret", APP_SECRET_NAME, APP_SECRET_NAMESPACE)
      return unless external_secret

      validate_api_version(context, external_secret, overlay)
      validate_refresh_interval(context, external_secret, overlay)
      validate_secret_store_ref(context, external_secret, overlay)
      validate_target(context, external_secret, overlay)
      validate_data_refs(context, external_secret, overlay)
    end

    def validate_api_version(context, external_secret, overlay)
      return if external_secret["apiVersion"].to_s == APP_SECRET_API_VERSION

      context.add_error("#{overlay} #{APP_SECRET_NAME} ExternalSecret must use #{APP_SECRET_API_VERSION}")
    end

    def validate_refresh_interval(context, external_secret, overlay)
      return unless external_secret.spec["refreshInterval"].to_s.strip.empty?

      context.add_error("#{overlay} #{APP_SECRET_NAME} ExternalSecret must set spec.refreshInterval")
    end

    def validate_secret_store_ref(context, external_secret, overlay)
      store_ref = hash(external_secret.spec["secretStoreRef"])
      store_name = store_ref["name"].to_s.strip
      store_kind = store_ref["kind"].to_s.strip
      context.add_error("#{overlay} #{APP_SECRET_NAME} ExternalSecret must set spec.secretStoreRef.name") if store_name.empty?
      unless APP_SECRET_STORE_KINDS.include?(store_kind)
        context.add_error(
          "#{overlay} #{APP_SECRET_NAME} ExternalSecret secretStoreRef.kind must be SecretStore or ClusterSecretStore",
        )
      end
    end

    def validate_target(context, external_secret, overlay)
      target = hash(external_secret.spec["target"])
      unless target["name"].to_s == APP_SECRET_NAME
        context.add_error("#{overlay} #{APP_SECRET_NAME} ExternalSecret target.name must be #{APP_SECRET_NAME}")
      end
      unless target["creationPolicy"].to_s == "Owner"
        context.add_error("#{overlay} #{APP_SECRET_NAME} ExternalSecret target.creationPolicy must be Owner")
      end
      unless target["deletionPolicy"].to_s == "Retain"
        context.add_error("#{overlay} #{APP_SECRET_NAME} ExternalSecret target.deletionPolicy must be Retain")
      end
    end

    def validate_data_refs(context, external_secret, overlay)
      data = array(external_secret.spec["data"])
      mapped_keys = data.map { |entry| entry["secretKey"].to_s }.reject(&:empty?)
      missing_required = REQUIRED_APP_SECRET_KEYS - mapped_keys
      unexpected_keys = mapped_keys - REQUIRED_APP_SECRET_KEYS - OPTIONAL_APP_SECRET_KEYS
      duplicate_keys = duplicate_values(mapped_keys)

      unless missing_required.empty?
        context.add_error("#{overlay} external secret contract is missing remote refs: #{missing_required.join(', ')}")
      end
      unless unexpected_keys.empty?
        context.add_error("#{overlay} external secret contract has unexpected remote refs: #{unexpected_keys.sort.join(', ')}")
      end
      unless duplicate_keys.empty?
        context.add_error("#{overlay} external secret contract has duplicate remote refs: #{duplicate_keys.join(', ')}")
      end

      data.each do |entry|
        secret_key = entry["secretKey"].to_s
        remote_key = hash(entry["remoteRef"])["key"].to_s.strip
        next unless remote_key.empty?

        label = secret_key.empty? ? "unnamed key" : secret_key
        context.add_error("#{overlay} external secret contract #{label} must set remoteRef.key")
      end
    end

    def hash(value)
      value.is_a?(Hash) ? value : {}
    end

    def array(value)
      value.is_a?(Array) ? value : []
    end

    def duplicate_values(values)
      counts = Hash.new(0)
      values.each { |value| counts[value] += 1 }
      counts.select { |_value, count| count > 1 }.keys.sort
    end
  end
end
