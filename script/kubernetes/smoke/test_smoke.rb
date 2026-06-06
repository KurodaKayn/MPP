# frozen_string_literal: true

require "minitest/autorun"
require "open3"
require "rbconfig"
require "stringio"

require_relative "checks"

module KubernetesSmoke
  module Checks
    class SmokeSuiteTest < Minitest::Test
      def test_secret_check_fails_when_required_secret_keys_are_empty
        reporter = Reporter.new(StringIO.new)
        suite = suite_with(
          reporter: reporter,
          kubectl: FakeKubectl.new(secret_data: {}),
        )

        suite.send(:configuration)

        assert_includes reporter.failures.map(&:first), "mpp-app-secrets keys"
        assert_match(/JWT_SECRET/, reporter.failures.assoc("mpp-app-secrets keys").last)
      end

      def test_secret_check_passes_when_required_secret_keys_are_populated
        reporter = Reporter.new(StringIO.new)
        suite = suite_with(
          reporter: reporter,
          kubectl: FakeKubectl.new(
            secret_data: Config::REQUIRED_SECRET_KEYS.to_h { |key| [key, "encoded-value"] },
          ),
        )

        suite.send(:configuration)

        refute_includes reporter.failures.map(&:first), "mpp-app-secrets keys"
      end

      def test_browser_session_probe_failure_is_required
        reporter = Reporter.new(StringIO.new)
        suite = suite_with(
          reporter: reporter,
          http: FakeHttp.new(
            post_response: HttpClient::Response.new(500, "failed to start", {}),
          ),
        )

        suite.send(:browser_session_probe)

        assert_includes reporter.failures.map(&:first), "remote browser session lifecycle"
        assert_empty reporter.warnings
      end

      def test_dry_run_exits_successfully_and_prints_kubectl_intent
        stdout, stderr, status = Open3.capture3(
          RbConfig.ruby,
          "script/kubernetes/smoke-test.rb",
          "--dry-run",
          "--skip-public",
        )

        assert status.success?, stdout + stderr
        assert_includes stdout, "DRY-RUN kubectl config current-context"
      end

      private

      def suite_with(reporter:, kubectl: FakeKubectl.new, http: FakeHttp.new)
        config = Config.parse(
          [
            "--public-url",
            "https://mpp.example.com",
            "--auth-token",
            "smoke-token",
          ],
          {},
        )
        Suite.new(config: config, kubectl: kubectl, reporter: reporter, http: http)
      end

      class FakeKubectl
        def initialize(secret_data: Config::REQUIRED_SECRET_KEYS.to_h { |key| [key, "encoded-value"] })
          @secret_data = secret_data
        end

        def resource(kind, name, namespace:)
          case [kind, name, namespace]
          when ["configmap", "mpp-app-config", "mpp-system"]
            {
              "data" => Config::REQUIRED_CONFIG_KEYS.to_h { |key| [key, "configured-value"] },
            }
          when ["secret", "mpp-app-secrets", "mpp-system"]
            { "data" => @secret_data }
          else
            {}
          end
        end
      end

      class FakeHttp
        def initialize(post_response: HttpClient::Response.new(201, '{"session_id":"session-1"}', {}))
          @post_response = post_response
        end

        def post(_url, headers: {}, json: nil)
          @post_response
        end

        def get(_url, headers: {})
          HttpClient::Response.new(200, '{"status":"ready"}', {})
        end

        def delete(_url, headers: {})
          HttpClient::Response.new(200, '{"status":"expired"}', {})
        end
      end
    end
  end
end
