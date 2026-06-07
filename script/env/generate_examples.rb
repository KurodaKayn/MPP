#!/usr/bin/env ruby
# frozen_string_literal: true

require "fileutils"
require "optparse"

require_relative "env_contract"

options = {
  schema: "contracts/env.schema.yaml",
  profiles: [],
  check: false,
}

parser = OptionParser.new do |opts|
  opts.banner = "Usage: script/env/generate_examples.rb [--check] [--profile dev]"

  opts.on("--schema PATH", "Schema path. Defaults to contracts/env.schema.yaml.") do |value|
    options[:schema] = value
  end
  opts.on("--profile NAME", "Profile to render. Can be passed more than once.") do |value|
    options[:profiles] << value
  end
  opts.on("--check", "Fail if generated example files are not up to date.") do
    options[:check] = true
  end
end

parser.parse!

schema = EnvContract.load_schema(options[:schema])
profiles = options[:profiles]
profiles = schema.fetch("profiles").keys if profiles.empty?
stale = []

profiles.each do |profile|
  EnvContract.validate_example!(schema, profile)
  path = EnvContract.example_path(schema, profile)
  rendered = EnvContract.render_example(schema, profile)

  if options[:check]
    current = File.file?(path) ? File.read(path) : nil
    stale << path unless current == rendered
    next
  end

  FileUtils.mkdir_p(File.dirname(path))
  File.write(path, rendered)
  puts "wrote #{path}"
end

if options[:check] && stale.any?
  warn "env examples are out of date:"
  stale.each { |path| warn "  - #{path}" }
  warn "run script/env/generate_examples.rb"
  exit 1
end

puts "env examples: ok" if options[:check]
