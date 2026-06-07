# frozen_string_literal: true

require "json"
require "time"

require_relative "config"
require_relative "http_client"
require_relative "kubectl"
require_relative "reporter"

module KubernetesSmoke
  module Checks
    class Suite
      RUNTIME_POD_SELECTOR = "app.kubernetes.io/name=mpp,app.kubernetes.io/component=browser-runtime,mpp.kurodakayn.dev/runtime-driver=kubernetes"

      def initialize(config:, kubectl:, reporter:, http:)
        @config = config
        @kubectl = kubectl
        @reporter = reporter
        @http = http
      end

      def run
        preflight
        cluster_shape
        workload_rollouts
        service_endpoints
        configuration
        internal_readiness unless @config.skip_internal_http
        runtime_controls unless @config.skip_runtime_rbac
        runtime_cleanup_state unless @config.skip_runtime_cleanup
        public_gateway unless @config.skip_public
        authenticated_user_flows
      end

      private

      def preflight
        @reporter.section("Preflight")
        check("kubectl context") do
          context = @kubectl.current_context
          assert_present(context, "kubectl current context is empty")
          "context=#{context}"
        end
        check("kubectl client version") do
          version = @kubectl.client_version
          describe_client_version(version)
        end
      end

      def cluster_shape
        @reporter.section("Cluster Shape")
        [@config.app_namespace, @config.runtime_namespace].each do |namespace|
          check("namespace #{namespace}") do
            resource = @kubectl.namespace(namespace)
            assert_equal(namespace, dig(resource, "metadata", "name"), "namespace name mismatch")
            labels = dig(resource, "metadata", "labels") || {}
            "labels=#{labels.keys.sort.join(',')}"
          end
        end

        check("runtime manager ServiceAccount") do
          account = @kubectl.resource("serviceaccount", "browser-worker-runtime-manager", namespace: @config.app_namespace)
          assert_equal("browser-worker-runtime-manager", dig(account, "metadata", "name"), "missing browser-worker runtime ServiceAccount")
          "namespace=#{@config.app_namespace}"
        end
      end

      def workload_rollouts
        @reporter.section("Workloads")
        Config::DEFAULT_DEPLOYMENTS.each do |deployment|
          check("rollout deployment/#{deployment}") do
            @kubectl.rollout_status("deployment/#{deployment}", namespace: @config.app_namespace, timeout: @config.rollout_timeout)
          end
        end

        check("app Pod readiness") do
          pods = @kubectl.resource_list(
            "pods",
            namespace: @config.app_namespace,
            selector: "app.kubernetes.io/name=mpp",
          )
          assert(!pods.empty?, "no app Pods found")
          not_ready = pods.reject { |pod| pod_ready?(pod) }
          assert(
            not_ready.empty?,
            "not-ready Pods: #{not_ready.map { |pod| dig(pod, 'metadata', 'name') }.join(', ')}",
          )
          "#{pods.length} Pods ready"
        end

        check("immutable app images") do
          deployments = @kubectl.resource_list("deployments", namespace: @config.app_namespace)
          images = deployments.flat_map { |deployment| deployment_images(deployment) }
          assert(!images.empty?, "no container images found")
          unresolved = images.select { |image| unresolved_image?(image) }
          assert(unresolved.empty?, "unresolved images: #{unresolved.join(', ')}")
          "#{images.length} images checked"
        end
      end

      def service_endpoints
        @reporter.section("Service Discovery")
        Config::DEFAULT_SERVICES.each do |service|
          check("Service #{service} endpoints") do
            endpoint = @kubectl.resource("endpoints", service, namespace: @config.app_namespace)
            addresses = endpoint_addresses(endpoint)
            assert(!addresses.empty?, "Service #{service} has no ready endpoint addresses")
            "#{addresses.length} endpoint addresses"
          end
        end
      end

      def configuration
        @reporter.section("Configuration")
        check("mpp-app-config keys") do
          config_map = @kubectl.resource("configmap", "mpp-app-config", namespace: @config.app_namespace)
          data = dig(config_map, "data") || {}
          missing = Config::REQUIRED_CONFIG_KEYS.reject { |key| data.key?(key) }
          assert(missing.empty?, "missing keys: #{missing.join(', ')}")
          unresolved = data.select { |_key, value| placeholder_value?(value) }
          assert(unresolved.empty?, "unresolved config values: #{unresolved.keys.join(', ')}")
          "#{Config::REQUIRED_CONFIG_KEYS.length} keys present"
        end

        check("mpp-app-secrets keys") do
          secret = @kubectl.resource("secret", "mpp-app-secrets", namespace: @config.app_namespace)
          data = dig(secret, "data") || {}
          missing = Config::REQUIRED_SECRET_KEYS.select { |key| data[key].to_s.empty? }
          assert(missing.empty?, "missing or empty keys: #{missing.join(', ')}")
          "#{Config::REQUIRED_SECRET_KEYS.length} required keys present"
        end

        check("publish-worker dependency config") do
          config_map = @kubectl.resource("configmap", "mpp-app-config", namespace: @config.app_namespace)
          data = dig(config_map, "data") || {}
          required = {
            "DB_HOST" => data["DB_HOST"],
            "REDIS_ADDR" => data["REDIS_ADDR"],
            "CONTENT_PIPELINE_HOST" => data["CONTENT_PIPELINE_HOST"],
            "CONTENT_PIPELINE_PORT" => data["CONTENT_PIPELINE_PORT"],
          }
          empty = required.select { |_key, value| value.to_s.strip.empty? }
          assert(empty.empty?, "publish dependencies are empty: #{empty.keys.join(', ')}")
          required.map { |key, value| "#{key}=#{value}" }.join(", ")
        end
      end

      def internal_readiness
        @reporter.section("Internal Readiness")
        in_cluster_http("frontend readiness", "http://frontend:3000/api/ready")
        in_cluster_http("backend readiness", "http://backend:8080/ready")
        in_cluster_http("ai-service readiness", "http://ai-service:8000/ready")
        in_cluster_http("collab-service readiness", "http://collab-service:8090/ready")
        in_cluster_http("content-pipeline metrics", "http://content-pipeline-service:9090/metrics")

        check("browser-worker readiness from backend") do
          body = @kubectl.exec(
            "deployment/backend",
            ["wget", "-qO-", "--timeout=#{@config.request_timeout}", "http://browser-worker:8081/ready"],
            namespace: @config.app_namespace,
            container: "backend",
          )
          assert(body.include?("ready"), "browser-worker readiness response did not contain ready")
          "backend can reach browser-worker"
        end

        check("publish-worker readiness in its Pod") do
          body = @kubectl.exec(
            "deployment/publish-worker",
            ["wget", "-qO-", "--timeout=#{@config.request_timeout}", "http://127.0.0.1:8080/ready"],
            namespace: @config.app_namespace,
            container: "publish-worker",
          )
          assert(body.include?("ready"), "publish-worker readiness response did not contain ready")
          "publish-worker dependencies are ready"
        end
      end

      def runtime_controls
        @reporter.section("Browser Runtime Control")
        check("runtime namespace NetworkPolicies") do
          policies = @kubectl.resource_list("networkpolicy", namespace: @config.runtime_namespace)
          names = policies.map { |policy| dig(policy, "metadata", "name") }
          required = ["browser-runtime-default-deny", "browser-runtime-private-access"]
          missing = required.reject { |name| names.include?(name) }
          assert(missing.empty?, "missing NetworkPolicies: #{missing.join(', ')}")
          "policies=#{required.join(',')}"
        end

        ["create", "get", "list", "watch", "delete"].each do |verb|
          check("runtime manager can #{verb} Pods") do
            answer = @kubectl.auth_can_i(
              verb,
              "pods",
              as: runtime_manager_service_account,
              namespace: @config.runtime_namespace,
            )
            assert_equal("yes", answer, "expected kubectl auth can-i to return yes")
            "allowed"
          end
        end
      end

      def runtime_cleanup_state
        @reporter.section("Browser Runtime Cleanup")
        check("runtime Pod cleanup state") do
          pods = @kubectl.resource_list(
            "pods",
            namespace: @config.runtime_namespace,
            selector: RUNTIME_POD_SELECTOR,
          )
          if pods.empty?
            next "no active runtime Pods"
          end

          stale = pods.select { |pod| stale_runtime_pod?(pod) }
          assert(
            stale.empty?,
            "stale runtime Pods: #{stale.map { |pod| dig(pod, 'metadata', 'name') }.join(', ')}",
          )
          missing_metadata = pods.select { |pod| runtime_metadata_missing?(pod) }
          assert(
            missing_metadata.empty?,
            "runtime Pods missing session metadata: #{missing_metadata.map { |pod| dig(pod, 'metadata', 'name') }.join(', ')}",
          )
          "#{pods.length} active runtime Pods have cleanup metadata"
        end
      end

      def public_gateway
        @reporter.section("Public Gateway")
        unless @config.public_url_configured?
          message = "set --public-url or MPP_PUBLIC_URL to probe the public frontend"
          return missing_optional_input("public frontend", message)
        end

        check("public frontend root") do
          response = @http.get("#{@config.public_url}/")
          assert_status(response, [200, 301, 302, 307, 308], "public root")
          "status=#{response.status}"
        end

        check("public frontend readiness") do
          response = @http.get("#{@config.public_url}/api/ready")
          assert_status(response, [200], "public frontend readiness")
          assert(response.body.include?("ready") || response.body.include?("healthy"), "unexpected readiness body")
          "status=#{response.status}"
        end
      end

      def authenticated_user_flows
        @reporter.section("Authenticated User Flows")
        unless @config.run_user_flow_probes
          return @reporter.skip("authenticated flow probes", "pass --run-user-flow-probes to enable")
        end

        unless @config.user_flow_inputs_configured?
          message = "requires --api-base-url or --public-url plus --auth-token"
          return missing_optional_input("authenticated flow probes", message)
        end

        check("authenticated dashboard session") do
          response = api_get("/api/user/dashboard/stats")
          assert_status(response, [200], "dashboard stats")
          "status=#{response.status}"
        end

        check("project list query") do
          response = api_get("/api/user/dashboard/projects")
          assert_status(response, [200], "project list")
          "status=#{response.status}"
        end

        project_scoped_user_flows
        browser_session_probe if @config.run_browser_session_probe
      end

      def project_scoped_user_flows
        unless @config.project_configured?
          message = "set --project-id or MPP_SMOKE_PROJECT_ID to enable"
          return missing_optional_input(
            "project-scoped collaboration and publishing probes",
            message,
          )
        end

        check("collaboration session creation") do
          response = api_post("/api/user/dashboard/projects/#{@config.project_id}/collab/session")
          assert_status(response, [200, 201], "collaboration session")
          body = parse_json(response.body)
          assert_present(body["token"], "collaboration session response is missing token")
          assert_present(body["document_id"], "collaboration session response is missing document_id")
          "document_id=#{body['document_id']}"
        end

        check("publishing dependency read path") do
          response = api_get("/api/user/dashboard/projects/#{@config.project_id}/publications")
          assert_status(response, [200], "project publications")
          "status=#{response.status}"
        end
      end

      def browser_session_probe
        check("remote browser session lifecycle") do
          session_id = nil
          start = api_post("/api/user/dashboard/settings/platforms/#{@config.browser_platform}/browser-session")
          assert_status(start, [200, 201], "browser session start")
          body = parse_json(start.body)
          session_id = body["session_id"] || body["sessionId"] || body["id"]
          assert_present(session_id, "browser session response is missing session_id")

          status = api_get("/api/user/dashboard/browser-sessions/#{session_id}")
          assert_status(status, [200], "browser session status")

          cancelled = api_delete("/api/user/dashboard/browser-sessions/#{session_id}")
          assert_status(cancelled, [200, 202, 204], "browser session cancel")
          "session_id=#{session_id} cancelled"
        ensure
          if session_id
            begin
              api_delete("/api/user/dashboard/browser-sessions/#{session_id}")
            rescue StandardError
              nil
            end
          end
        end
      end

      def in_cluster_http(name, url)
        check(name) do
          body = @kubectl.curl_from_ephemeral_pod(
            namespace: @config.app_namespace,
            image: @config.curl_image,
            url: url,
            timeout: @config.request_timeout,
          )
          assert(!body.strip.empty?, "#{url} returned an empty body")
          "#{url} responded"
        end
      end

      def missing_optional_input(name, message)
        if @config.require_user_flows
          check(name) { raise CheckFailure, message }
        else
          @reporter.skip(name, message)
        end
      end

      def api_get(path)
        @http.get(api_url(path), headers: auth_headers)
      end

      def api_post(path, json: nil)
        @http.post(api_url(path), headers: auth_headers, json: json)
      end

      def api_delete(path)
        @http.delete(api_url(path), headers: auth_headers)
      end

      def api_url(path)
        "#{@config.api_base_url}#{path}"
      end

      def auth_headers
        { "Authorization" => "Bearer #{@config.auth_token}" }
      end

      def runtime_manager_service_account
        "system:serviceaccount:#{@config.app_namespace}:browser-worker-runtime-manager"
      end

      def check(name, required: true, &block)
        @reporter.check(name, required: required, &block)
      end

      def assert(condition, message)
        raise CheckFailure, message unless condition
      end

      def assert_equal(expected, actual, message)
        raise CheckFailure, "#{message}: expected #{expected.inspect}, got #{actual.inspect}" unless expected == actual
      end

      def assert_present(value, message)
        text = value.to_s.strip
        raise CheckFailure, message if text.empty?
      end

      def assert_status(response, allowed, label)
        return if allowed.include?(response.status)

        body = response.body.to_s[0, 300]
        raise CheckFailure, "#{label} returned HTTP #{response.status}: #{body}"
      end

      def parse_json(body)
        JSON.parse(body.to_s)
      rescue JSON::ParserError => error
        raise CheckFailure, "response body is not JSON: #{error.message}"
      end

      def dig(object, *path)
        path.reduce(object) do |current, key|
          current.is_a?(Hash) ? current[key] : nil
        end
      end

      def describe_client_version(version)
        return version if version.is_a?(String)

        git_version = dig(version, "clientVersion", "gitVersion") ||
                      dig(version, "clientVersion", "git_version")
        git_version ? "client=#{git_version}" : "client version detected"
      end

      def pod_ready?(pod)
        return false if dig(pod, "metadata", "deletionTimestamp")

        conditions = dig(pod, "status", "conditions") || []
        ready = conditions.find { |condition| condition["type"] == "Ready" }
        ready && ready["status"] == "True"
      end

      def deployment_images(deployment)
        containers = dig(deployment, "spec", "template", "spec", "containers") || []
        containers.map { |container| container["image"].to_s }
      end

      def unresolved_image?(image)
        image.empty? ||
          image.start_with?("mpp-") ||
          image.end_with?(":latest") ||
          image.include?("replace-me") ||
          image.start_with?("registry.example.invalid/")
      end

      def endpoint_addresses(endpoint)
        subsets = endpoint["subsets"] || []
        subsets.flat_map { |subset| subset["addresses"] || [] }.map { |address| address["ip"] }.compact
      end

      def placeholder_value?(value)
        text = value.to_s
        text.include?("replace-with-") ||
          text.include?("replace-me") ||
          text.start_with?("registry.example.invalid/")
      end

      def stale_runtime_pod?(pod)
        phase = dig(pod, "status", "phase")
        return true if ["Succeeded", "Failed"].include?(phase)
        return true if dig(pod, "metadata", "deletionTimestamp")

        false
      end

      def runtime_metadata_missing?(pod)
        labels = dig(pod, "metadata", "labels") || {}
        annotations = dig(pod, "metadata", "annotations") || {}
        labels["mpp.kurodakayn.dev/runtime-driver"] != "kubernetes" ||
          labels["mpp.kurodakayn.dev/session-id"].to_s.empty? ||
          labels["mpp.kurodakayn.dev/owner-hash"].to_s.empty? ||
          annotations["mpp.kurodakayn.dev/expires-at"].to_s.empty?
      end
    end
  end
end
