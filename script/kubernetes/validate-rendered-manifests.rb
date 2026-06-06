#!/usr/bin/env ruby
# frozen_string_literal: true

require_relative "validation/context"
require_relative "validation/images"
require_relative "validation/app_baseline"
require_relative "validation/browser_runtime_control"
require_relative "validation/observability"
require_relative "validation/data_services"

package_dir, rendered_path = ARGV

if package_dir.nil? || rendered_path.nil?
  warn "Usage: validate-rendered-manifests.rb <package-dir> <rendered-yaml>"
  exit 2
end

context = KubernetesValidation::Context.from_file(package_dir, rendered_path)

KubernetesValidation::Images.validate(context)

if context.path_suffix?("validation/app-baseline")
  KubernetesValidation::AppBaseline.validate_overlay(context)
end

if context.path_suffix?("deploy/kubernetes/app-baseline") ||
   context.path_suffix?("validation/app-baseline")
  KubernetesValidation::AppBaseline.validate_workloads(context)
end

if context.path_suffix?("deploy/kubernetes/browser-runtime-control") ||
   context.path_suffix?("validation/app-baseline")
  KubernetesValidation::BrowserRuntimeControl.validate(context)
end

if context.path_suffix?("deploy/kubernetes/observability")
  KubernetesValidation::Observability.validate(context)
end

if context.path_suffix?("deploy/kubernetes/data-services/managed")
  KubernetesValidation::DataServices.validate_managed(context)
end

if context.path_suffix?("deploy/kubernetes/data-services/self-hosted")
  KubernetesValidation::DataServices.validate_self_hosted(context)
end

unless context.valid?
  warn context.errors.join("\n")
  exit 1
end
