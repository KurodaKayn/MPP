#!/usr/bin/env ruby
# frozen_string_literal: true

require "open3"

package_dir, rendered_path = ARGV

if package_dir.nil? || rendered_path.nil?
  warn "Usage: validate-rendered-schema.rb <package-dir> <rendered-yaml>"
  exit 2
end

unless File.file?(rendered_path)
  warn "Rendered manifest file does not exist: #{rendered_path}"
  exit 2
end

kubeconform = ENV.fetch("KUBECONFORM_BIN", "kubeconform")
kubernetes_version = ENV.fetch("KUBECONFORM_KUBERNETES_VERSION", "1.33.0")

command = [
  kubeconform,
  "-strict",
  "-summary",
  "-ignore-missing-schemas",
  "-kubernetes-version",
  kubernetes_version,
  "-",
]

begin
  stdout, stderr, status = Open3.capture3(*command, stdin_data: File.read(rendered_path))
rescue Errno::ENOENT
  warn "kubeconform binary not found. Install kubeconform or set KUBECONFORM_BIN."
  exit 127
end

print stdout unless stdout.empty?
warn stderr unless stderr.empty?

exit 0 if status.success?

warn "Kubernetes schema validation failed for #{package_dir}"
exit status.exitstatus || 1
