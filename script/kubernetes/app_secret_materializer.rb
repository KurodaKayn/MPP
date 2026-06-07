# frozen_string_literal: true

require "yaml"

require_relative "../env/env_contract"
require_relative "../env/env_file"

module KubernetesAppSecret
  DEFAULT_SCHEMA_PATH = "contracts/env.schema.yaml"
  DEFAULT_SECRET_NAME = "mpp-app-secrets"
  DEFAULT_NAMESPACE = "mpp-system"
  REQUIRED_KEYS = [
    "JWT_SECRET",
    "DB_PASSWORD",
    "COLLAB_TOKEN_SECRET",
    "COOKIE_ENCRYPTION_KEY",
    "LLM_PROVIDER_KEY",
    "AI_SERVICE_INTERNAL_TOKEN",
    "BROWSER_WORKER_INTERNAL_TOKEN",
    "CONTENT_PIPELINE_INTERNAL_TOKEN",
  ].freeze
  OPTIONAL_KEYS = [
    "REDIS_PASSWORD",
  ].freeze
  OUTPUT_KEYS = (REQUIRED_KEYS + OPTIONAL_KEYS).freeze
  PLACEHOLDER_PATTERNS = [
    /replace-with/i,
    /change-me/i,
    /your[-_]/i,
    /example\.invalid/i,
    /your-domain\.example/i,
    /\Astaging-example-/i,
  ].freeze

  module_function

  def parse_env(text)
    EnvFile.parse_string(text)
  end

  class Materializer
    attr_reader :errors, :warnings

    def initialize(env:, source:, schema_path: DEFAULT_SCHEMA_PATH, name: DEFAULT_SECRET_NAME,
                   namespace: DEFAULT_NAMESPACE, require_redis_password: false,
                   allow_placeholders: false)
      @env = env
      @source = source
      @schema_path = schema_path
      @name = name
      @namespace = namespace
      @require_redis_password = require_redis_password
      @allow_placeholders = allow_placeholders
      @errors = []
      @warnings = []
    end

    def valid?
      validate
      errors.empty?
    end

    def render
      return nil unless valid?

      YAML.dump(manifest)
    end

    private

    attr_reader :env, :source, :schema_path, :name, :namespace

    def validate
      return if @validated

      @validated = true
      validate_metadata
      validate_schema
      validate_required_keys
      validate_secret_values
      validate_unknown_keys
    end

    def validate_metadata
      add_error("Secret name must be set") if name.to_s.strip.empty?
      add_error("Secret namespace must be set") if namespace.to_s.strip.empty?
    end

    def validate_schema
      OUTPUT_KEYS.each do |key|
        spec = schema_variables[key]
        if spec.nil?
          add_error("#{key} is not declared in #{schema_path}")
        elsif spec.fetch("type", nil) != "secret"
          add_error("#{key} must be declared as a secret in #{schema_path}")
        end
      end
    end

    def validate_required_keys
      required_keys.each do |key|
        next if present?(env[key])

        add_error("#{source}: #{key} is required for #{name}")
      end
    end

    def validate_secret_values
      materialized_env.each do |key, value|
        value_is_placeholder = placeholder?(value)
        if value_is_placeholder
          add_error("#{source}: #{key} still looks like a placeholder") unless @allow_placeholders
          next
        end

        spec = schema_variables[key] || {}
        exact_bytes = spec["exact_bytes"]
        min_length = spec["min_length"]
        add_error("#{source}: #{key} must be exactly #{exact_bytes} bytes") if exact_bytes && value.bytesize != exact_bytes
        add_error("#{source}: #{key} must be at least #{min_length} characters") if min_length && value.length < min_length
      end
    end

    def validate_unknown_keys
      unknown = env.keys.reject { |key| OUTPUT_KEYS.include?(key) }.sort
      return if unknown.empty?

      warnings << "#{source}: ignoring keys that do not belong in #{name}: #{unknown.join(', ')}"
    end

    def manifest
      {
        "apiVersion" => "v1",
        "kind" => "Secret",
        "metadata" => {
          "name" => name,
          "namespace" => namespace,
          "labels" => {
            "app.kubernetes.io/name" => "mpp",
            "app.kubernetes.io/component" => "app-secrets",
            "app.kubernetes.io/part-of" => "mpp",
          },
        },
        "type" => "Opaque",
        "stringData" => materialized_env,
      }
    end

    def materialized_env
      OUTPUT_KEYS.each_with_object({}) do |key, values|
        value = env[key]
        values[key] = value if present?(value)
      end
    end

    def required_keys
      return REQUIRED_KEYS + OPTIONAL_KEYS if @require_redis_password

      REQUIRED_KEYS
    end

    def schema_variables
      @schema_variables ||= EnvContract.load_schema(schema_path).fetch("variables")
    end

    def placeholder?(value)
      PLACEHOLDER_PATTERNS.any? { |pattern| value.match?(pattern) }
    end

    def present?(value)
      !value.nil? && !value.to_s.strip.empty?
    end

    def add_error(message)
      errors << message
    end
  end
end
