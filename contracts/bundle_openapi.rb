#!/usr/bin/env ruby
# frozen_string_literal: true

require "pathname"
require "yaml"

def load_yaml(path)
  YAML.safe_load(path.read, aliases: true)
end

def deep_copy(value)
  Marshal.load(Marshal.dump(value))
end

def resolve_pointer(document, pointer)
  return document if pointer.nil? || pointer.empty? || pointer == "/"

  pointer
    .sub(%r{\A/}, "")
    .split("/")
    .map { |segment| segment.gsub("~1", "/").gsub("~0", "~") }
    .reduce(document) do |current, segment|
      current.fetch(segment)
    end
end

def bundle_node(node, base_path, cache)
  case node
  when Array
    node.map { |item| bundle_node(item, base_path, cache) }
  when Hash
    ref = node["$ref"]
    if node.keys == ["$ref"] && ref.is_a?(String) && !ref.start_with?("#")
      relative_path, pointer = ref.split("#", 2)
      target_path = base_path.dirname.join(relative_path).cleanpath
      target_doc = cache[target_path.to_s] ||= load_yaml(target_path)
      target_node = deep_copy(resolve_pointer(target_doc, pointer))
      return bundle_node(target_node, target_path, cache)
    end

    node.transform_values { |value| bundle_node(value, base_path, cache) }
  else
    node
  end
end

input_path = Pathname.new(ARGV.fetch(0)).expand_path
document = load_yaml(input_path)
bundled = bundle_node(document, input_path, { input_path.to_s => document })

$stdout.write(YAML.dump(bundled))
