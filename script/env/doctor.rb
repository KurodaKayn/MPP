#!/usr/bin/env ruby
# frozen_string_literal: true

require "optparse"
require "uri"

require_relative "env_contract"
require_relative "env_file"

BOOLEAN_VALUES = %w[1 0 true false yes no y n on off].freeze
DURATION_UNITS = {
  "ns" => 1.0e-9,
  "us" => 1.0e-6,
  "ms" => 1.0e-3,
  "s" => 1.0,
  "m" => 60.0,
  "h" => 3600.0,
}.freeze
PLACEHOLDER_PATTERNS = [
  /replace-with/i,
  /change-me/i,
  /your[-_]/i,
  /example\.invalid/i,
  /your-domain\.example/i,
].freeze

class EnvDoctor
  def initialize(options)
    @schema_path = options.fetch(:schema)
    @profile = options.fetch(:profile)
    @env_files = options.fetch(:files)
    @services = options.fetch(:services)
    @allow_placeholders = options.fetch(:allow_placeholders)
    @strict_unknown = options.fetch(:strict_unknown)
    @errors = []
    @warnings = []
  end

  def run
    schema = load_schema
    validate_profile!(schema)
    @examples = profile_examples(schema)
    all_variables = schema.fetch("variables")
    @all_variables = all_variables
    variables = selected_variables(all_variables)

    @env_files.each do |path|
      validate_env_file(path, variables, all_variables)
    end

    print_report
    @errors.empty? ? 0 : 1
  end

  private

  def load_schema
    EnvContract.load_schema(@schema_path)
  end

  def validate_profile!(schema)
    profiles = schema.fetch("profiles")
    return if profiles.key?(@profile)

    abort "unknown profile #{@profile.inspect}; expected one of: #{profiles.keys.sort.join(', ')}"
  end

  def profile_examples(schema)
    EnvContract.example_env(schema, @profile)
  end

  def selected_variables(variables)
    return variables if @services.empty?

    variables.select do |_name, spec|
      service_names = Array(spec["services"])
      (service_names & @services).any?
    end
  end

  def validate_env_file(path, variables, all_variables)
    unless File.file?(path)
      @errors << "#{path}: env file does not exist"
      return
    end

    env, parse_errors, duplicate_keys = parse_env_file(path)
    parse_errors.each { |message| @errors << "#{path}: #{message}" }
    duplicate_keys.each { |key| @warnings << "#{path}: #{key} is defined more than once" }

    validate_required(path, env, variables)
    validate_known_values(path, env, variables)
    validate_unknown_keys(path, env, all_variables)
  end

  def parse_env_file(path)
    result = EnvFile.parse_lines(File.readlines(path))
    [result.env, result.parse_errors, result.duplicate_keys]
  end

  def validate_required(path, env, variables)
    variables.each do |name, spec|
      next unless required_for_profile?(spec) || required_by_condition?(env, spec)
      next if present?(env[name])

      @errors << with_hint("#{path}: #{name} is required for #{@profile}", name, spec)
    end
  end

  def required_for_profile?(spec)
    required_in = Array(spec["required_in"])
    spec["required"] == true || required_in.include?(@profile)
  end

  def required_by_condition?(env, spec)
    Array(spec["required_when"]).any? do |condition|
      value = env[condition.fetch("name")]
      if condition.key?("equals")
        value.to_s.strip.downcase == condition.fetch("equals").to_s.strip.downcase
      elsif condition.key?("present")
        condition.fetch("present") ? present?(value) : !present?(value)
      else
        false
      end
    end
  end

  def validate_known_values(path, env, variables)
    env.each do |name, value|
      spec = variables[name]
      next unless spec
      next unless present?(value)

      value_is_placeholder = placeholder?(value)
      if !@allow_placeholders && value_is_placeholder
        @errors << with_hint("#{path}: #{name} still looks like a placeholder", name, spec)
      end
      next if @allow_placeholders && value_is_placeholder

      validate_type(path, name, value, spec)
      validate_length(path, name, value, spec)
    end
  end

  def validate_unknown_keys(path, env, variables)
    unknown = env.keys.reject { |key| variables.key?(key) }.sort
    unknown.each do |key|
      message = "#{path}: #{key} is not declared in #{@schema_path}"
      @strict_unknown ? @errors << message : @warnings << message
    end
  end

  def validate_type(path, name, value, spec)
    type = spec.fetch("type", "string")
    case type
    when "string", "secret", "path", "kubernetes_quantity"
      nil
    when "boolean"
      add_error(path, name, "must be a boolean") unless BOOLEAN_VALUES.include?(value.downcase)
    when "integer"
      validate_number(path, name, value, spec, integer: true)
    when "float"
      validate_number(path, name, value, spec, integer: false)
    when "port"
      validate_port(path, name, value)
    when "duration"
      validate_duration(path, name, value, spec)
    when "enum"
      add_error(path, name, "must be one of: #{Array(spec['values']).join(', ')}") unless Array(spec["values"]).map(&:to_s).include?(value)
    when "url"
      validate_url(path, name, value)
    when "websocket_url"
      validate_url(path, name, value, schemes: %w[ws wss http https])
    when "email"
      validate_email(path, name, value)
    when "currency"
      add_error(path, name, "must be a three-letter currency code") unless value.match?(/\A[A-Za-z]{3}\z/)
    when "hostport_or_url"
      validate_hostport_or_url(path, name, value)
    when "address"
      validate_hostport(path, name, value)
    when "csv"
      add_error(path, name, "must contain at least one comma-separated value") if csv_values(value).empty?
    when "csv_origin"
      validate_csv_origin(path, name, value)
    when "csv_hostport"
      csv_values(value).each { |entry| validate_hostport(path, name, entry) }
    else
      add_error(path, name, "has unknown schema type #{type.inspect}")
    end
  end

  def validate_number(path, name, value, spec, integer:)
    parsed = integer ? Integer(value, 10) : Float(value)
    min = spec["min"]
    max = spec["max"]
    add_error(path, name, "must be >= #{min}") if min && parsed < min
    add_error(path, name, "must be <= #{max}") if max && parsed > max
  rescue ArgumentError
    add_error(path, name, integer ? "must be an integer" : "must be a number")
  end

  def validate_port(path, name, value)
    port = Integer(value, 10)
    add_error(path, name, "must be between 1 and 65535") unless port.between?(1, 65_535)
  rescue ArgumentError
    add_error(path, name, "must be a port number")
  end

  def validate_duration(path, name, value, spec)
    parsed = parse_duration_seconds(value)
    unless parsed
      add_error(path, name, "must be a duration such as 30s, 5m, or 1h")
      return
    end

    min_duration = spec["min_duration"]
    min_parsed = parse_duration_seconds(min_duration.to_s) if min_duration
    add_error(path, name, "must be >= #{min_duration}") if min_parsed && parsed < min_parsed
  end

  def parse_duration_seconds(value)
    raw = value.to_s.strip
    if (match = raw.match(/\A(\d+(?:\.\d+)?)(ns|us|ms|s|m|h)\z/))
      return match[1].to_f * DURATION_UNITS.fetch(match[2])
    end
    return raw.to_i if raw.match?(/\A\d+\z/)

    nil
  end

  def validate_url(path, name, value, schemes: nil)
    parsed = URI.parse(value)
    if parsed.scheme.to_s.empty?
      add_error(path, name, "must include a URL scheme")
      return
    end
    if schemes && !schemes.include?(parsed.scheme)
      add_error(path, name, "must use one of these schemes: #{schemes.join(', ')}")
    end
    add_error(path, name, "must include a URL host") if parsed.host.to_s.empty? && parsed.scheme != "chrome-extension"
  rescue URI::InvalidURIError
    add_error(path, name, "must be a valid URL")
  end

  def validate_email(path, name, value)
    add_error(path, name, "must be an email address") unless value.match?(/\A[^@\s]+@[^@\s]+\.[^@\s]+\z/)
  end

  def validate_hostport_or_url(path, name, value)
    unless value.include?("://")
      validate_hostport(path, name, value)
      return
    end

    parsed = URI.parse(value)
    if parsed.scheme
      validate_url(path, name, value, schemes: %w[redis rediss http https])
    else
      validate_hostport(path, name, value)
    end
  rescue URI::InvalidURIError
    validate_hostport(path, name, value)
  end

  def validate_hostport(path, name, value)
    host, raw_port = value.rpartition(":").values_at(0, 2)
    if host.empty? || raw_port.empty?
      add_error(path, name, "must be host:port")
      return
    end
    validate_port(path, name, raw_port)
  end

  def validate_csv_origin(path, name, value)
    values = csv_values(value)
    if values.empty?
      add_error(path, name, "must contain at least one origin")
      return
    end
    values.each { |origin| validate_url(path, name, origin, schemes: %w[http https chrome-extension]) }
  end

  def validate_length(path, name, value, spec)
    exact_bytes = spec["exact_bytes"]
    min_length = spec["min_length"]
    add_error(path, name, "must be exactly #{exact_bytes} bytes") if exact_bytes && value.bytesize != exact_bytes
    add_error(path, name, "must be at least #{min_length} characters") if min_length && value.length < min_length
  end

  def csv_values(value)
    value.split(",").map(&:strip).reject(&:empty?)
  end

  def placeholder?(value)
    PLACEHOLDER_PATTERNS.any? { |pattern| value.match?(pattern) }
  end

  def present?(value)
    !value.nil? && !value.to_s.strip.empty?
  end

  def add_error(path, name, message)
    @errors << with_hint("#{path}: #{name} #{message}", name, @all_variables[name])
  end

  def with_hint(message, name, spec)
    hints = []
    example = example_value(name, spec)
    hints << "example: #{name}=#{example}" if present?(example)
    generator = spec && spec["generator"]
    hints << "generate: script/secret/gen_app_secrets.py #{generator}" if present?(generator)
    return message if hints.empty?

    "#{message} (#{hints.join('; ')})"
  end

  def example_value(name, spec)
    return @examples[name] if @examples && present?(@examples[name])
    return nil unless spec

    case spec.fetch("type", "string")
    when "boolean"
      "true"
    when "integer"
      spec.key?("min") ? [spec["min"].to_i, 1].max.to_s : "1"
    when "float"
      spec.key?("min") ? [spec["min"].to_f, 1.0].max.to_s : "1.0"
    when "port"
      "8080"
    when "duration"
      "30s"
    when "enum"
      Array(spec["values"]).first.to_s
    when "url"
      "http://localhost:8080"
    when "websocket_url"
      "ws://localhost:8090"
    when "email"
      "admin@example.com"
    when "currency"
      "USD"
    when "hostport_or_url", "csv_hostport"
      "redis:6379"
    when "address"
      "0.0.0.0:8080"
    when "csv"
      "value-a,value-b"
    when "csv_origin"
      "http://localhost:3000"
    when "secret"
      "generated-secret"
    else
      "value"
    end
  end

  def print_report
    if @errors.empty? && @warnings.empty?
      puts "env doctor: ok"
      return
    end

    unless @errors.empty?
      warn "env doctor: #{@errors.length} error(s)"
      @errors.each { |message| warn "  - #{message}" }
    end
    unless @warnings.empty?
      warn "env doctor: #{@warnings.length} warning(s)"
      @warnings.each { |message| warn "  - #{message}" }
    end
  end
end

options = {
  schema: "contracts/env.schema.yaml",
  profile: "dev",
  files: [],
  services: [],
  allow_placeholders: false,
  strict_unknown: false,
}

parser = OptionParser.new do |opts|
  opts.banner = "Usage: script/env/doctor.rb --profile dev --file deploy/docker/.env"

  opts.on("--schema PATH", "Schema path. Defaults to contracts/env.schema.yaml.") do |value|
    options[:schema] = value
  end
  opts.on("--profile NAME", "Environment profile to validate, for example dev or deploy.") do |value|
    options[:profile] = value
  end
  opts.on("--file PATH", "Env file to validate. Can be passed more than once.") do |value|
    options[:files] << value
  end
  opts.on("--service NAME", "Limit required/value checks to one service. Can be passed more than once.") do |value|
    options[:services] << value
  end
  opts.on("--allow-placeholders", "Allow placeholder-looking values in example files.") do
    options[:allow_placeholders] = true
  end
  opts.on("--strict-unknown", "Treat undeclared env keys as errors instead of warnings.") do
    options[:strict_unknown] = true
  end
end

parser.parse!

if options[:files].empty?
  options[:files] << "deploy/docker/.env"
end

exit EnvDoctor.new(options).run
