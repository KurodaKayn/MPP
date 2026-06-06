# frozen_string_literal: true

require "optparse"
require "uri"

module KubernetesSmoke
  class Config
    DEFAULT_DEPLOYMENTS = [
      "frontend",
      "backend",
      "publish-worker",
      "browser-worker",
      "ai-service",
      "content-pipeline-service",
      "collab-service",
    ].freeze

    DEFAULT_SERVICES = [
      "frontend",
      "backend",
      "browser-worker",
      "ai-service",
      "content-pipeline-service",
      "collab-service",
    ].freeze

    REQUIRED_CONFIG_KEYS = [
      "BACKEND_API_BASE_URL",
      "BROWSER_WORKER_URL",
      "AI_SERVICE_URL",
      "CONTENT_PIPELINE_HOST",
      "CONTENT_PIPELINE_PORT",
      "COLLAB_INTERNAL_URL",
      "COLLAB_WEBSOCKET_URL_BASE",
      "DB_HOST",
      "DB_SSLMODE",
      "REDIS_ADDR",
      "REDIS_TLS",
    ].freeze

    REQUIRED_SECRET_KEYS = [
      "JWT_SECRET",
      "DB_PASSWORD",
      "COLLAB_TOKEN_SECRET",
      "COOKIE_ENCRYPTION_KEY",
      "LLM_PROVIDER_KEY",
      "BROWSER_WORKER_INTERNAL_TOKEN",
      "AI_SERVICE_INTERNAL_TOKEN",
    ].freeze

    attr_accessor :app_namespace,
                  :runtime_namespace,
                  :observability_namespace,
                  :rollout_timeout,
                  :request_timeout,
                  :curl_image,
                  :public_url,
                  :api_base_url,
                  :auth_token,
                  :project_id,
                  :browser_platform,
                  :run_user_flow_probes,
                  :run_browser_session_probe,
                  :require_user_flows,
                  :skip_public,
                  :skip_internal_http,
                  :skip_runtime_rbac,
                  :skip_runtime_cleanup,
                  :dry_run,
                  :verbose

    def self.parse(argv, env)
      config = new(env)
      parser = option_parser(config)
      parser.parse!(argv)
      config.normalize!
      config
    rescue OptionParser::ParseError => error
      warn error.message
      warn parser
      exit 2
    end

    def self.option_parser(config)
      OptionParser.new do |opts|
        opts.banner = "Usage: smoke-test.rb [options]"

        opts.separator ""
        opts.separator "Cluster scope:"
        opts.on("--app-namespace NAME", "Application namespace. Default: #{config.app_namespace}") do |value|
          config.app_namespace = value
        end
        opts.on("--runtime-namespace NAME", "Browser runtime namespace. Default: #{config.runtime_namespace}") do |value|
          config.runtime_namespace = value
        end
        opts.on("--observability-namespace NAME", "Observability namespace. Default: #{config.observability_namespace}") do |value|
          config.observability_namespace = value
        end
        opts.on("--rollout-timeout SECONDS", Integer, "Rollout timeout per Deployment. Default: #{config.rollout_timeout}") do |value|
          config.rollout_timeout = value
        end
        opts.on("--request-timeout SECONDS", Integer, "HTTP request timeout. Default: #{config.request_timeout}") do |value|
          config.request_timeout = value
        end
        opts.on("--curl-image IMAGE", "Image used for in-cluster HTTP probes. Default: #{config.curl_image}") do |value|
          config.curl_image = value
        end

        opts.separator ""
        opts.separator "Public and user-flow probes:"
        opts.on("--public-url URL", "Public frontend base URL. Env: MPP_PUBLIC_URL") do |value|
          config.public_url = value
        end
        opts.on("--api-base-url URL", "API base URL. Defaults to --public-url. Env: MPP_API_BASE_URL") do |value|
          config.api_base_url = value
        end
        opts.on("--auth-token TOKEN", "Bearer token for authenticated smoke probes. Env: MPP_SMOKE_AUTH_TOKEN") do |value|
          config.auth_token = value
        end
        opts.on("--project-id ID", "Existing project ID for collaboration and publishing dependency probes. Env: MPP_SMOKE_PROJECT_ID") do |value|
          config.project_id = value
        end
        opts.on("--browser-platform NAME", "Browser session platform. Default: #{config.browser_platform}") do |value|
          config.browser_platform = value
        end
        opts.on("--run-user-flow-probes", "Run authenticated read and project-scoped probes.") do
          config.run_user_flow_probes = true
        end
        opts.on("--run-browser-session-probe", "Start and cancel a remote browser session through the backend API.") do
          config.run_browser_session_probe = true
          config.run_user_flow_probes = true
        end
        opts.on("--require-user-flows", "Fail instead of skipping when user-flow inputs are missing.") do
          config.require_user_flows = true
        end

        opts.separator ""
        opts.separator "Skips:"
        opts.on("--skip-public", "Skip public URL probes.") do
          config.skip_public = true
        end
        opts.on("--skip-internal-http", "Skip in-cluster HTTP probes.") do
          config.skip_internal_http = true
        end
        opts.on("--skip-runtime-rbac", "Skip browser runtime RBAC can-i probes.") do
          config.skip_runtime_rbac = true
        end
        opts.on("--skip-runtime-cleanup", "Skip runtime Pod cleanup-state probes.") do
          config.skip_runtime_cleanup = true
        end

        opts.separator ""
        opts.separator "Execution:"
        opts.on("--dry-run", "Print command intent without calling kubectl.") do
          config.dry_run = true
        end
        opts.on("-v", "--verbose", "Print command details.") do
          config.verbose = true
        end
        opts.on("-h", "--help", "Show this help.") do
          puts opts
          exit
        end

        opts.separator ""
        opts.separator "Examples:"
        opts.separator "  ruby script/kubernetes/smoke-test.rb --public-url https://mpp.example.com"
        opts.separator "  MPP_SMOKE_AUTH_TOKEN=... ruby script/kubernetes/smoke-test.rb --public-url https://mpp.example.com --run-user-flow-probes"
        opts.separator "  ruby script/kubernetes/smoke-test.rb --run-browser-session-probe --auth-token ... --public-url https://mpp.example.com"
      end
    end

    def initialize(env)
      @app_namespace = env.fetch("MPP_APP_NS", "mpp-system")
      @runtime_namespace = env.fetch("MPP_RUNTIME_NS", "mpp-browser-runtime")
      @observability_namespace = env.fetch("MPP_OBSERVABILITY_NS", "mpp-observability")
      @rollout_timeout = integer_env(env, "MPP_SMOKE_ROLLOUT_TIMEOUT", 300)
      @request_timeout = integer_env(env, "MPP_SMOKE_REQUEST_TIMEOUT", 10)
      @curl_image = env.fetch("MPP_SMOKE_CURL_IMAGE", "curlimages/curl:8.11.1")
      @public_url = env["MPP_PUBLIC_URL"]
      @api_base_url = env["MPP_API_BASE_URL"]
      @auth_token = env["MPP_SMOKE_AUTH_TOKEN"]
      @project_id = env["MPP_SMOKE_PROJECT_ID"]
      @browser_platform = env.fetch("MPP_SMOKE_BROWSER_PLATFORM", "douyin")
      @run_user_flow_probes = truthy?(env["MPP_SMOKE_RUN_USER_FLOW_PROBES"])
      @run_browser_session_probe = truthy?(env["MPP_SMOKE_RUN_BROWSER_SESSION_PROBE"])
      @require_user_flows = truthy?(env["MPP_SMOKE_REQUIRE_USER_FLOWS"])
      @skip_public = truthy?(env["MPP_SMOKE_SKIP_PUBLIC"])
      @skip_internal_http = truthy?(env["MPP_SMOKE_SKIP_INTERNAL_HTTP"])
      @skip_runtime_rbac = truthy?(env["MPP_SMOKE_SKIP_RUNTIME_RBAC"])
      @skip_runtime_cleanup = truthy?(env["MPP_SMOKE_SKIP_RUNTIME_CLEANUP"])
      @dry_run = false
      @verbose = truthy?(env["MPP_SMOKE_VERBOSE"])
    end

    def normalize!
      @app_namespace = clean_required(@app_namespace, "app namespace")
      @runtime_namespace = clean_required(@runtime_namespace, "runtime namespace")
      @observability_namespace = clean_required(@observability_namespace, "observability namespace")
      @curl_image = clean_required(@curl_image, "curl image")
      @browser_platform = clean_required(@browser_platform, "browser platform")
      @rollout_timeout = positive_integer(@rollout_timeout, "rollout timeout")
      @request_timeout = positive_integer(@request_timeout, "request timeout")
      @public_url = normalize_url(@public_url, "public URL")
      @api_base_url = normalize_url(@api_base_url, "API base URL") || @public_url
      @auth_token = blank_to_nil(@auth_token)
      @project_id = blank_to_nil(@project_id)
      @run_user_flow_probes = true if @run_browser_session_probe
    end

    def public_url_configured?
      !public_url.nil?
    end

    def api_base_url_configured?
      !api_base_url.nil?
    end

    def auth_configured?
      !auth_token.nil?
    end

    def project_configured?
      !project_id.nil?
    end

    def user_flow_inputs_configured?
      api_base_url_configured? && auth_configured?
    end

    private

    def integer_env(env, key, fallback)
      value = env[key]
      return fallback if value.nil? || value.strip.empty?

      Integer(value, 10)
    rescue ArgumentError
      fallback
    end

    def truthy?(value)
      return false if value.nil?

      ["1", "true", "yes", "on"].include?(value.to_s.strip.downcase)
    end

    def blank_to_nil(value)
      text = value.to_s.strip
      text.empty? ? nil : text
    end

    def clean_required(value, label)
      text = blank_to_nil(value)
      raise OptionParser::InvalidArgument, "#{label} must not be empty" if text.nil?

      text
    end

    def positive_integer(value, label)
      number = value.is_a?(Integer) ? value : Integer(value, 10)
      raise OptionParser::InvalidArgument, "#{label} must be positive" unless number.positive?

      number
    rescue ArgumentError
      raise OptionParser::InvalidArgument, "#{label} must be an integer"
    end

    def normalize_url(value, label)
      text = blank_to_nil(value)
      return nil if text.nil?

      uri = URI.parse(text)
      unless uri.is_a?(URI::HTTP) && uri.host
        raise OptionParser::InvalidArgument, "#{label} must be an http or https URL"
      end
      text.sub(%r{/+\z}, "")
    rescue URI::InvalidURIError
      raise OptionParser::InvalidArgument, "#{label} must be a valid URL"
    end
  end
end
