# frozen_string_literal: true

require "fileutils"
require "tempfile"
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

  class AtomicYamlWriter
    def write(path, document)
      temp_path = write_temp_file(path, document)
      move_file(temp_path, path)
      []
    rescue SystemCallError, IOError => error
      ["failed to write pinned overlay images: #{error.message}"]
    ensure
      cleanup_temp_file(temp_path)
    end

    private

    def write_temp_file(path, document)
      stat = File.stat(path)
      temp = Tempfile.create(["#{File.basename(path)}.", ".tmp"], File.dirname(path))
      temp.write(YAML.dump(document))
      temp.close
      File.chmod(stat.mode & 0o7777, temp.path)
      temp.path
    rescue SystemCallError, IOError
      temp&.close
      FileUtils.rm_f(temp&.path)
      raise
    end

    def move_file(source, target)
      FileUtils.mv(source, target)
    end

    def cleanup_temp_file(path)
      FileUtils.rm_f(path) if path
    end
  end

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
    def initialize(overlay:, git_sha:, writer: AtomicYamlWriter.new)
      @overlay = overlay.to_s
      @git_sha = git_sha.to_s
      @writer = writer
      @errors = []
    end

    def pin
      validate_inputs
      return result([]) unless errors.empty?

      kustomization_path = File.join(overlay, "kustomization.yaml")
      kustomization = load_yaml(kustomization_path)

      validate_kustomization(kustomization)
      return result([]) unless errors.empty?

      pin_app_images(kustomization)
      pin_runtime_image(kustomization)
      writer.write(kustomization_path, kustomization).each { |message| add_error(message) }
      return result([]) unless errors.empty?

      result([kustomization_path])
    end

    private

    attr_reader :overlay, :git_sha, :writer, :errors

    def result(updated_files)
      PinResult.new(updated_files: updated_files, errors: errors)
    end

    def validate_inputs
      add_error("overlay must be set") if overlay.strip.empty?
      add_error("overlay directory does not exist: #{overlay}") unless File.directory?(overlay)
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

      unless runtime_patch(document)
        add_error("#{kustomization_path_label} must set BROWSER_RUNTIME_IMAGE")
      end
    end

    def pin_app_images(document)
      images = document["images"]
      APP_IMAGE_REPOSITORIES.each do |source_name, repository|
        image = images.find { |entry| entry["name"] == source_name }
        image["newName"] = "#{DEFAULT_REGISTRY}/#{repository}"
        image["newTag"] = image_tag
      end
    end

    def pin_runtime_image(kustomization)
      patch_entry, patch_document = runtime_patch(kustomization)
      runtime_image_env(patch_document)["value"] = "#{DEFAULT_REGISTRY}/#{BROWSER_RUNTIME_REPOSITORY}:#{image_tag}"
      patch_entry["patch"] = YAML.dump(patch_document).sub(/\A---\s*\n/, "")
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

    def runtime_patch(kustomization)
      patches = kustomization["patches"]
      return nil unless patches.is_a?(Array)

      patches.filter_map { |entry| parse_runtime_patch(entry) }.first
    end

    def parse_runtime_patch(entry)
      patch = entry["patch"]
      return nil unless patch.is_a?(String) && patch.include?("BROWSER_RUNTIME_IMAGE")

      document = YAML.safe_load(
        patch,
        permitted_classes: [],
        permitted_symbols: [],
        aliases: true,
      )
      [entry, document] if document.is_a?(Hash) && runtime_image_env(document)
    rescue Psych::Exception
      nil
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

    def kustomization_path_label
      File.join(overlay, "kustomization.yaml")
    end

    def add_error(message)
      errors << message
    end
  end
end
