# frozen_string_literal: true

require "set"

module EnvFile
  ParseResult = Struct.new(:env, :parse_errors, :duplicate_keys, keyword_init: true)

  module_function

  def parse_string(text)
    parse_lines(text.lines)
  end

  def parse_lines(lines)
    env = {}
    parse_errors = []
    seen = Set.new
    duplicate_keys = Set.new

    lines.each_with_index do |line, index|
      raw = line.chomp
      stripped = raw.strip
      next if stripped.empty? || stripped.start_with?("#")

      stripped = stripped.sub(/\Aexport\s+/, "")
      unless stripped.include?("=")
        parse_errors << "line #{index + 1} is not KEY=VALUE"
        next
      end

      key, value = stripped.split("=", 2)
      key = key.strip
      unless key.match?(/\A[A-Za-z_][A-Za-z0-9_]*\z/)
        parse_errors << "line #{index + 1} has invalid key #{key.inspect}"
        next
      end

      duplicate_keys << key if seen.include?(key)
      seen << key
      env[key] = unquote_env_value(value.strip)
    end

    ParseResult.new(env: env, parse_errors: parse_errors, duplicate_keys: duplicate_keys.to_a.sort)
  end

  def unquote_env_value(value)
    return value[1...-1] if value.length >= 2 && value.start_with?('"') && value.end_with?('"')
    return value[1...-1] if value.length >= 2 && value.start_with?("'") && value.end_with?("'")

    value
  end
end
