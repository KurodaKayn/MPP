#!/usr/bin/env ruby
# frozen_string_literal: true

require "optparse"

require_relative "overlay_image_pinner"

options = {
  overlay: nil,
  git_sha: ENV.fetch("GITHUB_SHA", ""),
  image_namespace: ENV.fetch("MPP_IMAGE_NAMESPACE", KubernetesOverlayImages::DEFAULT_IMAGE_NAMESPACE),
}

parser = OptionParser.new do |opts|
  opts.banner = "Usage: script/kubernetes/pin-overlay-images.rb --overlay PATH --git-sha SHA [options]"

  opts.on("--overlay PATH", "Kubernetes overlay directory to update.") do |value|
    options[:overlay] = value
  end
  opts.on("--git-sha SHA", "Full 40-character Git SHA. Defaults to GITHUB_SHA.") do |value|
    options[:git_sha] = value
  end
  opts.on("--image-namespace VALUE", "Target image namespace. Defaults to MPP_IMAGE_NAMESPACE or ghcr.io/kurodakayn.") do |value|
    options[:image_namespace] = value
  end
  opts.on("-h", "--help", "Show this help.") do
    puts opts
    exit
  end
end

begin
  parser.parse!
rescue OptionParser::ParseError => error
  warn "pin-overlay-images: #{error.message}"
  exit 1
end

pinner = KubernetesOverlayImages::Pinner.new(
  overlay: options[:overlay],
  git_sha: options[:git_sha],
  image_namespace: options[:image_namespace],
)
result = pinner.pin

unless result.valid?
  warn "pin-overlay-images: #{result.errors.length} error(s)"
  result.errors.each { |message| warn "  - #{message}" }
  exit 1
end

result.updated_files.each { |path| puts "pinned #{path}" }
