# frozen_string_literal: true

require "json"
require "open3"
require "securerandom"

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
      @reporter.command(command)
      return "" if @dry_run

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
  end
end
