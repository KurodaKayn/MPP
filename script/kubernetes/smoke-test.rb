#!/usr/bin/env ruby
# frozen_string_literal: true

require_relative "smoke/checks"
require_relative "smoke/config"
require_relative "smoke/http_client"
require_relative "smoke/kubectl"
require_relative "smoke/reporter"

config = KubernetesSmoke::Config.parse(ARGV, ENV)
reporter = KubernetesSmoke::Reporter.new($stdout, verbose: config.verbose)
kubectl = KubernetesSmoke::Kubectl.new(reporter: reporter, dry_run: config.dry_run)
http = KubernetesSmoke::HttpClient.new(timeout: config.request_timeout)

suite = KubernetesSmoke::Checks::Suite.new(
  config: config,
  kubectl: kubectl,
  reporter: reporter,
  http: http,
)

suite.run
exit(reporter.exit_code)
