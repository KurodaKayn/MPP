#!/usr/bin/env ruby
# frozen_string_literal: true

require "optparse"

require_relative "app_secret_materializer"

options = {
  env_file: nil,
  schema: KubernetesAppSecret::DEFAULT_SCHEMA_PATH,
  name: KubernetesAppSecret::DEFAULT_SECRET_NAME,
  namespace: KubernetesAppSecret::DEFAULT_NAMESPACE,
  require_redis_password: false,
  allow_placeholders: false,
}

parser = OptionParser.new do |opts|
  opts.banner = "Usage: script/kubernetes/render-app-secret.rb --env-file PATH [options]"

  opts.on("--env-file PATH", "Read KEY=VALUE secrets from PATH. Defaults to stdin.") do |value|
    options[:env_file] = value
  end
  opts.on("--schema PATH", "Env schema path. Defaults to #{options[:schema]}.") do |value|
    options[:schema] = value
  end
  opts.on("--name NAME", "Secret name. Defaults to #{options[:name]}.") do |value|
    options[:name] = value
  end
  opts.on("--namespace NAME", "Secret namespace. Defaults to #{options[:namespace]}.") do |value|
    options[:namespace] = value
  end
  opts.on("--require-redis-password", "Require REDIS_PASSWORD in addition to app secret keys.") do
    options[:require_redis_password] = true
  end
  opts.on("--allow-placeholders", "Allow placeholder-looking values for example-only rendering.") do
    options[:allow_placeholders] = true
  end
  opts.on("-h", "--help", "Show this help.") do
    puts opts
    exit
  end
end

parser.parse!

source = options[:env_file] || "stdin"
input = begin
  if options[:env_file]
    File.read(options[:env_file])
  else
    $stdin.read
  end
rescue Errno::ENOENT => error
  warn "render-app-secret: #{error.message}"
  exit 1
end

parse_result = KubernetesAppSecret.parse_env(input)
parse_errors = parse_result.parse_errors.map { |message| "#{source}: #{message}" }
duplicate_warnings = parse_result.duplicate_keys.map { |key| "#{source}: #{key} is defined more than once" }

materializer = KubernetesAppSecret::Materializer.new(
  env: parse_result.env,
  source: source,
  schema_path: options[:schema],
  name: options[:name],
  namespace: options[:namespace],
  require_redis_password: options[:require_redis_password],
  allow_placeholders: options[:allow_placeholders],
)

rendered = materializer.render
warnings = duplicate_warnings + materializer.warnings
warnings.each { |message| warn "render-app-secret: warning: #{message}" }

errors = parse_errors + materializer.errors
unless errors.empty?
  warn "render-app-secret: #{errors.length} error(s)"
  errors.each { |message| warn "  - #{message}" }
  exit 1
end

puts rendered
