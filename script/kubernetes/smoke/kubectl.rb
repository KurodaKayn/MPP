# frozen_string_literal: true

require "json"
require "open3"
require "securerandom"

require_relative "config"

module KubernetesSmoke
  class Kubectl
    class CommandError < StandardError
      attr_reader :command, :stdout, :stderr, :status

      def initialize(command, stdout, stderr, status)
        super("kubectl command failed")
        @command = command
        @stdout = stdout
        @stderr = stderr
        @status = status
      end
    end

    def initialize(reporter:, dry_run: false)
      @reporter = reporter
      @dry_run = dry_run
    end

    def run(*args, input: nil, allow_failure: false)
      command = ["kubectl", *args.flatten.compact.map(&:to_s)]
      @reporter.command(command, dry_run: @dry_run)
      return dry_run_stdout(command) if @dry_run

      stdout, stderr, status = Open3.capture3(*command, stdin_data: input)
      if !status.success? && !allow_failure
        raise CommandError.new(command, stdout, stderr, status)
      end
      stdout
    end

    def json(*args)
      raw = run(*args, "-o", "json")
      return {} if raw.strip.empty?

      JSON.parse(raw)
    rescue JSON::ParserError => error
      raise CheckFailure, "kubectl returned invalid JSON: #{error.message}"
    end

    def current_context
      run("config", "current-context").strip
    end

    def client_version
      json("version", "--client")
    rescue CommandError
      run("version", "--client").strip
    end

    def namespace(name)
      json("get", "namespace", name)
    end

    def resource(kind, name, namespace: nil)
      args = ["get", kind, name]
      args += ["-n", namespace] if namespace
      json(*args)
    end

    def resource_list(kind, namespace: nil, selector: nil)
      args = ["get", kind]
      args += ["-n", namespace] if namespace
      args += ["-l", selector] if selector
      json(*args).fetch("items", [])
    end

    def rollout_status(resource, namespace:, timeout:)
      run(
        "rollout",
        "status",
        resource,
        "-n",
        namespace,
        "--timeout=#{timeout}s",
      ).strip
    end

    def auth_can_i(verb, resource, as:, namespace:)
      run(
        "auth",
        "can-i",
        verb,
        resource,
        "--as=#{as}",
        "-n",
        namespace,
      ).strip
    end

    def exec(resource, command, namespace:, container: nil)
      args = ["exec", resource, "-n", namespace]
      args += ["-c", container] if container
      args += ["--", *command]
      run(*args)
    end

    def curl_from_ephemeral_pod(namespace:, image:, url:, timeout:, headers: {}, method: "GET", body: nil)
      pod = "mpp-smoke-curl-#{SecureRandom.hex(4)}"
      args = [
        "run",
        pod,
        "-n",
        namespace,
        "--image",
        image,
        "--restart=Never",
        "--attach",
        "--rm",
        "--quiet",
        "--labels",
        "app.kubernetes.io/name=mpp,app.kubernetes.io/component=smoke-test",
        "--command",
        "--",
        "curl",
        "-fsS",
        "--max-time",
        timeout.to_s,
        "-X",
        method,
      ]
      headers.each do |key, value|
        args += ["-H", "#{key}: #{value}"]
      end
      if body
        args += ["-H", "Content-Type: application/json", "--data", body]
      end
      args << url
      run(*args)
    ensure
      delete_pod(namespace, pod) if pod && !@dry_run
    end

    def delete_pod(namespace, pod)
      run(
        "delete",
        "pod",
        pod,
        "-n",
        namespace,
        "--ignore-not-found=true",
        "--wait=false",
        allow_failure: true,
      )
    end

    private

    def dry_run_stdout(command)
      args = command.drop(1)
      case args.first
      when "config"
        "dry-run-context\n"
      when "version"
        json_response("clientVersion" => { "gitVersion" => "dry-run" })
      when "get"
        dry_run_get(args)
      when "rollout"
        "dry-run rollout ok\n"
      when "auth"
        "yes\n"
      when "exec", "run"
        '{"status":"ready"}'
      else
        ""
      end
    end

    def dry_run_get(args)
      kind = args[1]
      name = args[2] unless args[2].to_s.start_with?("-")
      selector = option_value(args, "-l")

      case kind
      when "namespace", "namespaces"
        json_response("metadata" => { "name" => name, "labels" => {} })
      when "serviceaccount", "serviceaccounts"
        json_response("metadata" => { "name" => name, "labels" => {} })
      when "deployments", "deployment"
        json_response("items" => dry_run_deployments)
      when "pods", "pod"
        json_response("items" => dry_run_pods(selector))
      when "endpoints", "endpoint"
        json_response(
          "metadata" => { "name" => name },
          "subsets" => [{ "addresses" => [{ "ip" => "10.0.0.10" }] }],
        )
      when "configmap", "configmaps"
        json_response("metadata" => { "name" => name }, "data" => dry_run_config_map)
      when "secret", "secrets"
        json_response("metadata" => { "name" => name }, "data" => dry_run_secret)
      when "networkpolicy", "networkpolicies"
        json_response(
          "items" => [
            { "metadata" => { "name" => "browser-runtime-default-deny" } },
            { "metadata" => { "name" => "browser-runtime-private-access" } },
          ],
        )
      else
        json_response({})
      end
    end

    def dry_run_deployments
      Config::DEFAULT_DEPLOYMENTS.map do |deployment|
        {
          "metadata" => { "name" => deployment },
          "spec" => {
            "template" => {
              "spec" => {
                "containers" => [
                  {
                    "name" => deployment,
                    "image" => "ghcr.io/kurodakayn/mpp-#{deployment}:sha-dryrun",
                  },
                ],
              },
            },
          },
        }
      end
    end

    def dry_run_pods(selector)
      if selector.to_s.include?("app.kubernetes.io/component=browser-runtime")
        [
          {
            "metadata" => {
              "name" => "mpp-browser-session-dry-run",
              "labels" => {
                "mpp.kurodakayn.dev/runtime-driver" => "kubernetes",
                "mpp.kurodakayn.dev/session-id" => "dry-run-session",
                "mpp.kurodakayn.dev/session-owner-hash" => "dry-run-owner",
              },
              "annotations" => {
                "mpp.kurodakayn.dev/expires-at" => "2099-01-01T00:00:00Z",
              },
            },
            "status" => { "phase" => "Running" },
          },
        ]
      else
        [
          {
            "metadata" => { "name" => "mpp-app-dry-run" },
            "status" => {
              "phase" => "Running",
              "conditions" => [{ "type" => "Ready", "status" => "True" }],
            },
          },
        ]
      end
    end

    def dry_run_config_map
      {
        "BACKEND_API_BASE_URL" => "http://backend:8080",
        "BROWSER_WORKER_URL" => "http://browser-worker:8081",
        "AI_SERVICE_URL" => "http://ai-service:8000",
        "CONTENT_PIPELINE_HOST" => "content-pipeline-service",
        "CONTENT_PIPELINE_PORT" => "50051",
        "COLLAB_INTERNAL_URL" => "http://collab-service:8090",
        "COLLAB_WEBSOCKET_URL_BASE" => "wss://mpp.example.com",
        "DB_HOST" => "postgres.example.com",
        "DB_SSLMODE" => "verify-full",
        "REDIS_ADDR" => "redis.example.com:6379",
        "REDIS_TLS" => "true",
      }
    end

    def dry_run_secret
      Config::REQUIRED_SECRET_KEYS.to_h { |key| [key, "encoded-value"] }
    end

    def option_value(args, option)
      index = args.index(option)
      return nil unless index

      args[index + 1]
    end

    def json_response(value)
      "#{JSON.generate(value)}\n"
    end
  end
end
