# frozen_string_literal: true

require "yaml"

module KubernetesOverlayImages
  DEFAULT_REGISTRY = "ghcr.io/kurodakayn"
  APP_IMAGE_REPOSITORIES = {
    "registry.example.invalid/kurodakayn/mpp-frontend" => "mpp-frontend",
    "registry.example.invalid/kurodakayn/mpp-backend" => "mpp-backend",
    "registry.example.invalid/kurodakayn/mpp-browser-worker" => "mpp-browser-worker",
    "registry.example.invalid/kurodakayn/mpp-ai-service" => "mpp-ai-service",
    "registry.example.invalid/kurodakayn/mpp-content-pipeline-service" => "mpp-content-pipeline-service",
    "registry.example.invalid/kurodakayn/mpp-collab-service" => "mpp-collab-service",
  }.freeze
  BROWSER_RUNTIME_REPOSITORY = "mpp-browser-runtime"

  class PinResult
    attr_reader :updated_files, :errors

    def initialize(updated_files:, errors:)
      @updated_files = updated_files
      @errors = errors
    end

    def valid?
      errors.empty?
    end
  end

  class Pinner
    def initialize(overlay:, git_sha:, registry: DEFAULT_REGISTRY)
      @overlay = overlay.to_s
      @git_sha = git_sha.to_s
      @registry = registry.to_s.sub(%r{/+\z}, "")
      @errors = []
    end

    def pin
      validate_inputs
      return result([]) unless errors.empty?

      kustomization_path = File.join(overlay, "kustomization.yaml")
      runtime_patch_path = File.join(overlay, "runtime-image-patch.yaml")
      kustomization = load_yaml(kustomization_path)
      runtime_patch = load_yaml(runtime_patch_path)

      validate_kustomization(kustomization)
      validate_runtime_patch(runtime_patch)
      return result([]) unless errors.empty?

      pin_app_images(kustomization)
      pin_runtime_image(runtime_patch)
      write_yaml(kustomization_path, kustomization)
      write_yaml(runtime_patch_path, runtime_patch)
      result([kustomization_path, runtime_patch_path])
    end

    private

    attr_reader :overlay, :git_sha, :registry, :errors

    def result(updated_files)
      PinResult.new(updated_files: updated_files, errors: errors)
    end

    def validate_inputs
      add_error("overlay must be set") if overlay.strip.empty?
      add_error("overlay directory does not exist: #{overlay}") unless File.directory?(overlay)
      add_error("registry must be set") if registry.strip.empty?
      unless git_sha.match?(/\A[0-9a-f]{40}\z/)
        add_error("git SHA must be 40 lowercase hexadecimal characters")
      end
    end

    def validate_kustomization(document)
      unless document.is_a?(Hash)
        add_error("#{kustomization_path_label} must be a YAML mapping")
        return
      end

      images = document["images"]
      unless images.is_a?(Array)
        add_error("#{kustomization_path_label} must define images")
        return
      end

      missing_images = APP_IMAGE_REPOSITORIES.keys - images.map { |image| image["name"] }
      unless missing_images.empty?
        add_error("#{kustomization_path_label} is missing image entries: #{missing_images.join(', ')}")
      end
    end

    def validate_runtime_patch(document)
      unless document.is_a?(Hash)
        add_error("#{runtime_patch_path_label} must be a YAML mapping")
        return
      end

      unless runtime_image_env(document)
        add_error("#{runtime_patch_path_label} must set BROWSER_RUNTIME_IMAGE")
      end
    end

    def pin_app_images(document)
      images = document["images"]
      APP_IMAGE_REPOSITORIES.each do |source_name, repository|
        image = images.find { |entry| entry["name"] == source_name }
        image["newName"] = "#{registry}/#{repository}"
        image["newTag"] = image_tag
      end
    end

    def pin_runtime_image(document)
      runtime_image_env(document)["value"] = "#{registry}/#{BROWSER_RUNTIME_REPOSITORY}:#{image_tag}"
    end

    def runtime_image_env(document)
      containers = document.dig("spec", "template", "spec", "containers")
      return nil unless containers.is_a?(Array)

      browser_worker = containers.find { |container| container["name"] == "browser-worker" }
      return nil unless browser_worker

      env = browser_worker["env"]
      return nil unless env.is_a?(Array)

      env.find { |entry| entry["name"] == "BROWSER_RUNTIME_IMAGE" }
    end

    def image_tag
      "sha-#{git_sha}"
    end

    def load_yaml(path)
      parsed = YAML.safe_load(
        File.read(path),
        permitted_classes: [],
        permitted_symbols: [],
        aliases: true,
      )
      parsed || {}
    rescue Errno::ENOENT
      add_error("file does not exist: #{path}")
      {}
    rescue Psych::Exception => error
      add_error("#{path} failed to parse: #{error.message}")
      {}
    end

    def write_yaml(path, document)
      File.write(path, YAML.dump(document))
    end

    def kustomization_path_label
      File.join(overlay, "kustomization.yaml")
    end

    def runtime_patch_path_label
      File.join(overlay, "runtime-image-patch.yaml")
    end

    def add_error(message)
      errors << message
    end
  end
end
