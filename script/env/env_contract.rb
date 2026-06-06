# frozen_string_literal: true

require "yaml"

module EnvContract
  module_function

  def load_schema(path)
    YAML.safe_load(
      File.read(path),
      permitted_classes: [],
      permitted_symbols: [],
      aliases: false,
    )
  rescue Errno::ENOENT
    abort "schema not found: #{path}"
  rescue Psych::Exception => error
    abort "schema failed to parse: #{error.message}"
  end

  def profile(schema, profile_name)
    profiles = schema.fetch("profiles")
    profile = profiles[profile_name]
    return profile if profile

    abort "unknown profile #{profile_name.inspect}; expected one of: #{profiles.keys.sort.join(', ')}"
  end

  def example_path(schema, profile_name)
    profile(schema, profile_name).fetch("example_file")
  end

  def example_env(schema, profile_name)
    env = {}
    example_items(schema, profile_name).each do |item|
      next unless item["name"]

      env[item.fetch("name")] = item.fetch("value", "").to_s
    end
    env
  end

  def render_example(schema, profile_name)
    lines = [
      "# Generated from contracts/env.schema.yaml. Do not edit by hand.",
      "# Run script/env/generate_examples.rb to refresh this file.",
      "",
    ]

    example_sections(schema, profile_name).each_with_index do |section, index|
      lines << "" if index.positive?
      lines << "# #{section.fetch('title')}"
      Array(section["items"]).each do |item|
        if item.key?("comment")
          Array(item["comment"]).each { |comment| lines << "# #{comment}" }
        else
          lines << "#{item.fetch('name')}=#{item.fetch('value', '')}"
        end
      end
    end

    lines.join("\n") + "\n"
  end

  def validate_example!(schema, profile_name)
    variables = schema.fetch("variables")
    example_items(schema, profile_name).each do |item|
      next unless item["name"]

      name = item.fetch("name")
      abort "#{profile_name} example references unknown env key #{name}" unless variables.key?(name)
    end
  end

  def example_sections(schema, profile_name)
    Array(profile(schema, profile_name)["example_sections"])
  end

  def example_items(schema, profile_name)
    example_sections(schema, profile_name).flat_map { |section| Array(section["items"]) }
  end
end
