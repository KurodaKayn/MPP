# frozen_string_literal: true

require "minitest/autorun"
require "open3"
require "rbconfig"
require "tempfile"
require "yaml"

require_relative "app_secret_materializer"

module KubernetesAppSecret
  class MaterializerTest < Minitest::Test
    def test_renders_opaque_secret_with_required_string_data
      materializer = build_materializer(env: complete_env)

      rendered = materializer.render
      manifest = YAML.safe_load(rendered, permitted_classes: [], permitted_symbols: [], aliases: false)

      assert_empty materializer.errors
      assert_equal "v1", manifest["apiVersion"]
      assert_equal "Secret", manifest["kind"]
      assert_equal "mpp-app-secrets", manifest.dig("metadata", "name")
      assert_equal "mpp-system", manifest.dig("metadata", "namespace")
      assert_equal "app-secrets", manifest.dig("metadata", "labels", "app.kubernetes.io/component")
      assert_equal "Opaque", manifest["type"]
      assert_equal complete_env, manifest["stringData"]
      refute_includes manifest.fetch("stringData"), "REDIS_PASSWORD"
    end

    def test_custom_secret_name_and_namespace_are_rendered
      materializer = build_materializer(
        env: complete_env,
        name: "custom-app-secrets",
        namespace: "custom-system",
      )

      manifest = YAML.safe_load(materializer.render, permitted_classes: [], permitted_symbols: [], aliases: false)

      assert_equal "custom-app-secrets", manifest.dig("metadata", "name")
      assert_equal "custom-system", manifest.dig("metadata", "namespace")
    end

    def test_redis_password_is_optional_by_default
      materializer = build_materializer(env: complete_env)

      assert materializer.valid?, materializer.errors.join("\n")
    end

    def test_redis_password_can_be_required_for_authenticated_redis
      materializer = build_materializer(env: complete_env, require_redis_password: true)

      refute materializer.valid?
      assert_includes materializer.errors.join("\n"), "REDIS_PASSWORD is required"
    end

    def test_redis_password_is_rendered_when_supplied
      env = complete_env.merge("REDIS_PASSWORD" => "redis-secret")
      materializer = build_materializer(env: env, require_redis_password: true)
      manifest = YAML.safe_load(materializer.render, permitted_classes: [], permitted_symbols: [], aliases: false)

      assert_equal "redis-secret", manifest.dig("stringData", "REDIS_PASSWORD")
    end

    def test_rejects_placeholder_values_by_default
      env = complete_env.merge("JWT_SECRET" => "replace-with-a-strong-random-secret")
      materializer = build_materializer(env: env)

      refute materializer.valid?
      assert_includes materializer.errors.join("\n"), "JWT_SECRET still looks like a placeholder"
    end

    def test_allows_placeholder_values_for_example_rendering
      env = complete_env.merge("JWT_SECRET" => "replace-with-a-strong-random-secret")
      materializer = build_materializer(env: env, allow_placeholders: true)

      assert materializer.valid?, materializer.errors.join("\n")
    end

    def test_allow_placeholders_skips_placeholder_length_checks
      env = complete_env.merge("COOKIE_ENCRYPTION_KEY" => "replace-with-exactly-32-byte-secret")
      materializer = build_materializer(env: env, allow_placeholders: true)

      assert materializer.valid?, materializer.errors.join("\n")
    end

    def test_rejects_cookie_key_with_wrong_length
      env = complete_env.merge("COOKIE_ENCRYPTION_KEY" => "too-short")
      materializer = build_materializer(env: env)

      refute materializer.valid?
      assert_includes materializer.errors.join("\n"), "COOKIE_ENCRYPTION_KEY must be exactly 32 bytes"
    end

    def test_rejects_short_internal_tokens
      env = complete_env.merge("AI_SERVICE_INTERNAL_TOKEN" => "short")
      materializer = build_materializer(env: env)

      refute materializer.valid?
      assert_includes materializer.errors.join("\n"), "AI_SERVICE_INTERNAL_TOKEN must be at least 16 characters"
    end

    def test_warns_for_unknown_keys_without_rendering_them
      env = complete_env.merge("GRAFANA_ADMIN_PASSWORD" => "grafana-secret")
      materializer = build_materializer(env: env)
      manifest = YAML.safe_load(materializer.render, permitted_classes: [], permitted_symbols: [], aliases: false)

      assert_includes materializer.warnings.join("\n"), "GRAFANA_ADMIN_PASSWORD"
      refute_includes manifest["stringData"], "GRAFANA_ADMIN_PASSWORD"
    end

    def test_parse_env_accepts_exports_quotes_comments_and_reports_duplicates
      result = KubernetesAppSecret.parse_env(<<~ENV)
        # comment
        export JWT_SECRET="jwt-secret"
        JWT_SECRET='jwt-secret-2'
        BAD LINE
      ENV

      assert_equal "jwt-secret-2", result.env["JWT_SECRET"]
      assert_equal ["JWT_SECRET"], result.duplicate_keys
      assert_includes result.parse_errors, "line 4 is not KEY=VALUE"
    end

    def test_cli_renders_from_env_file
      with_env_file(renderable_env_text(complete_env.merge("REDIS_PASSWORD" => "redis-secret"))) do |file|
        stdout, stderr, status = Open3.capture3(
          RbConfig.ruby,
          "script/kubernetes/render-app-secret.rb",
          "--env-file",
          file.path,
          "--require-redis-password",
        )

        assert status.success?, stderr
        manifest = YAML.safe_load(stdout, permitted_classes: [], permitted_symbols: [], aliases: false)
        assert_equal "redis-secret", manifest.dig("stringData", "REDIS_PASSWORD")
        assert_empty stderr
      end
    end

    def test_cli_renders_from_stdin
      stdout, stderr, status = Open3.capture3(
        RbConfig.ruby,
        "script/kubernetes/render-app-secret.rb",
        stdin_data: renderable_env_text,
      )

      assert status.success?, stderr
      manifest = YAML.safe_load(stdout, permitted_classes: [], permitted_symbols: [], aliases: false)
      assert_equal "mpp-app-secrets", manifest.dig("metadata", "name")
    end

    def test_cli_fails_for_missing_required_key
      env = complete_env.except("JWT_SECRET")

      stdout, stderr, status = Open3.capture3(
        RbConfig.ruby,
        "script/kubernetes/render-app-secret.rb",
        stdin_data: renderable_env_text(env),
      )

      refute status.success?
      assert_empty stdout
      assert_includes stderr, "JWT_SECRET is required"
    end

    private

    def build_materializer(env:, source: "test.env", schema_path: KubernetesAppSecret::DEFAULT_SCHEMA_PATH,
                           name: KubernetesAppSecret::DEFAULT_SECRET_NAME,
                           namespace: KubernetesAppSecret::DEFAULT_NAMESPACE,
                           require_redis_password: false, allow_placeholders: false)
      KubernetesAppSecret::Materializer.new(
        env: env,
        source: source,
        schema_path: schema_path,
        name: name,
        namespace: namespace,
        require_redis_password: require_redis_password,
        allow_placeholders: allow_placeholders,
      )
    end

    def complete_env
      {
        "JWT_SECRET" => "jwt-secret-with-32-characters!!",
        "DB_PASSWORD" => "db-secret",
        "COLLAB_TOKEN_SECRET" => "collab-token-secret",
        "COOKIE_ENCRYPTION_KEY" => "12345678901234567890123456789012",
        "LLM_PROVIDER_KEY" => "llm-provider-secret",
        "AI_SERVICE_INTERNAL_TOKEN" => "ai-service-token",
        "BROWSER_WORKER_INTERNAL_TOKEN" => "browser-worker-token",
        "CONTENT_PIPELINE_INTERNAL_TOKEN" => "content-pipeline-token",
      }
    end

    def renderable_env_text(env = complete_env)
      env.map { |key, value| "#{key}=#{value}" }.join("\n") + "\n"
    end

    def with_env_file(text)
      file = Tempfile.new("mpp-app-secrets")
      file.write(text)
      file.close
      yield file
    ensure
      file&.unlink
    end
  end
end
