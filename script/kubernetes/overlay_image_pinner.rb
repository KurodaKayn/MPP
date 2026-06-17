# frozen_string_literal: true

require "fileutils"
require "tempfile"
require "yaml"

module KubernetesOverlayImages
  DEFAULT_IMAGE_NAMESPACE = "ghcr.io/kurodakayn"
  DEFAULT_REGISTRY = DEFAULT_IMAGE_NAMESPACE
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
    def initialize(overlay:, git_sha:, image_namespace: DEFAULT_IMAGE_NAMESPACE, writer: AtomicYamlWriter.new)
      @overlay = overlay.to_s
      @git_sha = git_sha.to_s
      @image_namespace = normalize_image_namespace(image_namespace)
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
      runtime_patch = pin_runtime_image(kustomization)
      write_document(kustomization_path, kustomization)
      write_document(runtime_patch.path, runtime_patch.document) if runtime_patch.path
      return result([]) unless errors.empty?

      updated_files = [kustomization_path]
      updated_files << runtime_patch.path if runtime_patch.path
      result(updated_files)
    end

    private

    RuntimePatch = Struct.new(:entry, :document, :path, keyword_init: true)

    attr_reader :overlay, :git_sha, :image_namespace, :writer, :errors

    def result(updated_files)
      PinResult.new(updated_files: updated_files, errors: errors)
    end

    def validate_inputs
      add_error("overlay must be set") if overlay.strip.empty?
      add_error("overlay directory does not exist: #{overlay}") unless File.directory?(overlay)
      unless production_overlay?
        add_error("image pinning supports only production overlays")
      end
      unless git_sha.match?(/\A[0-9a-f]{40}\z/)
        add_error("git SHA must be 40 lowercase hexadecimal characters")
      end
      if image_namespace.empty?
        add_error("image namespace must be set")
      elsif image_namespace.match?(/\s/) || image_namespace.include?("@")
        add_error("image namespace must not contain whitespace or digests")
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
        image["newName"] = image_repository(repository)
        image["newTag"] = image_tag
      end
    end

    def pin_runtime_image(kustomization)
      patch = runtime_patch(kustomization)
      runtime_image_env(patch.document)["value"] = "#{image_repository(BROWSER_RUNTIME_REPOSITORY)}:#{image_tag}"
      patch.entry["patch"] = YAML.dump(patch.document).sub(/\A---\s*\n/, "") unless patch.path
      patch
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

      patches.map { |entry| parse_runtime_patch(entry) }.compact.first
    end

    def parse_runtime_patch(entry)
      inline_patch = entry["patch"]
      if inline_patch.is_a?(String) && inline_patch.include?("BROWSER_RUNTIME_IMAGE")
        document = parse_patch_document(inline_patch)
        return RuntimePatch.new(entry:, document:) if document.is_a?(Hash) && runtime_image_env(document)
      end

      path = entry["path"]
      return nil unless path.is_a?(String)

      patch_path = File.expand_path(path, overlay)
      return nil unless File.file?(patch_path)

      document = load_patch_document(patch_path)
      RuntimePatch.new(entry:, document:, path: patch_path) if document.is_a?(Hash) && runtime_image_env(document)
    rescue Psych::Exception
      nil
    end

    def parse_patch_document(raw)
      YAML.safe_load(
        raw,
        permitted_classes: [],
        permitted_symbols: [],
        aliases: true,
      )
    end

    def load_patch_document(path)
      parse_patch_document(File.read(path))
    rescue Errno::ENOENT
      nil
    end

    def write_document(path, document)
      writer.write(path, document).each { |message| add_error(message) }
    end

    def image_repository(repository)
      "#{image_namespace}/#{repository}"
    end

    def image_tag
      "sha-#{git_sha}"
    end

    def normalize_image_namespace(value)
      value.to_s.strip.sub(%r{/+\z}, "")
    end

    def production_overlay?
      File.basename(overlay).include?("production")
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
