# frozen_string_literal: true

require "fileutils"
require "minitest/autorun"
require "open3"
require "rbconfig"
require "tmpdir"
require "yaml"

require_relative "overlay_image_pinner"

module KubernetesOverlayImages
  class PinnerTest < Minitest::Test
    SHA = "1234567890abcdef1234567890abcdef12345678"
    TAG = "sha-#{SHA}"

    def test_pins_app_and_runtime_images_to_git_sha
      with_overlay_copy do |overlay|
        result = Pinner.new(overlay: overlay, git_sha: SHA).pin

        assert result.valid?, result.errors.join("\n")
        assert_equal [File.join(overlay, "kustomization.yaml")], result.updated_files

        images = load_yaml(File.join(overlay, "kustomization.yaml")).fetch("images")
        APP_IMAGE_REPOSITORIES.values.each do |repository|
          image = images.find { |entry| entry["newName"] == "#{DEFAULT_REGISTRY}/#{repository}" }
          assert image, "missing pinned image for #{repository}"
          assert_equal TAG, image["newTag"]
        end

        runtime_image = runtime_image_value(File.join(overlay, "kustomization.yaml"))
        assert_equal "#{DEFAULT_REGISTRY}/#{BROWSER_RUNTIME_REPOSITORY}:#{TAG}", runtime_image
      end
    end

    def test_pins_images_to_custom_namespace
      image_namespace = "123456789012.dkr.ecr.us-east-1.amazonaws.com/mpp"

      with_overlay_copy do |overlay|
        result = Pinner.new(overlay: overlay, git_sha: SHA, image_namespace: image_namespace).pin

        assert result.valid?, result.errors.join("\n")

        images = load_yaml(File.join(overlay, "kustomization.yaml")).fetch("images")
        APP_IMAGE_REPOSITORIES.values.each do |repository|
          image = images.find { |entry| entry["newName"] == "#{image_namespace}/#{repository}" }
          assert image, "missing pinned image for #{repository}"
          assert_equal TAG, image["newTag"]
        end

        assert_equal "#{image_namespace}/#{BROWSER_RUNTIME_REPOSITORY}:#{TAG}",
                     runtime_image_value(File.join(overlay, "kustomization.yaml"))
      end
    end

    def test_pins_path_based_runtime_image_patch_for_provider_overlay
      image_namespace = "us-docker.pkg.dev/example-production/mpp"

      Dir.mktmpdir("mpp-overlay-image-pinner") do |dir|
        overlay = File.join(dir, "gcp-production")
        FileUtils.cp_r("deploy/kubernetes/overlays/staging-managed", overlay)

        result = Pinner.new(overlay: overlay, git_sha: SHA, image_namespace: image_namespace).pin

        assert result.valid?, result.errors.join("\n")
        assert_equal [
          File.join(overlay, "kustomization.yaml"),
          File.join(overlay, "runtime-image-patch.yaml"),
        ], result.updated_files
        assert_equal "#{image_namespace}/#{BROWSER_RUNTIME_REPOSITORY}:#{TAG}",
                     runtime_image_patch_value(File.join(overlay, "runtime-image-patch.yaml"))
      end
    end

    def test_rejects_invalid_git_sha_without_writing_files
      with_overlay_copy do |overlay|
        kustomization_path = File.join(overlay, "kustomization.yaml")
        before = File.read(kustomization_path)

        result = Pinner.new(overlay: overlay, git_sha: "abc").pin

        refute result.valid?
        assert_includes result.errors.join("\n"), "git SHA must be 40 lowercase hexadecimal characters"
        assert_equal before, File.read(kustomization_path)
      end
    end

    def test_rejects_non_production_managed_overlay
      Dir.mktmpdir("mpp-overlay-image-pinner") do |dir|
        overlay = File.join(dir, "staging-managed")
        FileUtils.cp_r("deploy/kubernetes/overlays/staging-managed", overlay)

        result = Pinner.new(overlay: overlay, git_sha: SHA).pin

        refute result.valid?
        assert_includes result.errors.join("\n"), "supports only production overlays"
      end
    end

    def test_rejects_invalid_image_namespace_without_writing_files
      with_overlay_copy do |overlay|
        kustomization_path = File.join(overlay, "kustomization.yaml")
        before = File.read(kustomization_path)

        result = Pinner.new(overlay: overlay, git_sha: SHA, image_namespace: "https://registry.example/mpp").pin

        refute result.valid?
        assert_includes result.errors.join("\n"), "image namespace must not include a URL scheme"
        assert_equal before, File.read(kustomization_path)
      end
    end

    def test_rejects_runtime_patch_paths_outside_overlay
      Dir.mktmpdir("mpp-overlay-image-pinner") do |dir|
        overlay = File.join(dir, "gcp-production")
        FileUtils.cp_r("deploy/kubernetes/overlays/staging-managed", overlay)
        kustomization_path = File.join(overlay, "kustomization.yaml")
        kustomization = load_yaml(kustomization_path)
        kustomization.fetch("patches").find { |entry| entry["path"] == "runtime-image-patch.yaml" }["path"] =
          "../runtime-image-patch.yaml"
        File.write(kustomization_path, YAML.dump(kustomization))

        result = Pinner.new(overlay: overlay, git_sha: SHA).pin

        refute result.valid?
        assert_includes result.errors.join("\n"), "runtime image patch path must stay inside the overlay directory"
      end
    end

    def test_rejects_missing_image_entry
      with_overlay_copy do |overlay|
        kustomization_path = File.join(overlay, "kustomization.yaml")
        kustomization = load_yaml(kustomization_path)
        kustomization["images"].reject! do |entry|
          entry["name"] == "registry.example.invalid/kurodakayn/mpp-frontend"
        end
        File.write(kustomization_path, YAML.dump(kustomization))

        result = Pinner.new(overlay: overlay, git_sha: SHA).pin

        refute result.valid?
        assert_includes result.errors.join("\n"), "mpp-frontend"
      end
    end

    def test_rejects_missing_runtime_image_patch
      with_overlay_copy do |overlay|
        kustomization_path = File.join(overlay, "kustomization.yaml")
        kustomization = load_yaml(kustomization_path)
        kustomization["patches"].reject! { |entry| entry["patch"].to_s.include?("BROWSER_RUNTIME_IMAGE") }
        File.write(kustomization_path, YAML.dump(kustomization))

        result = Pinner.new(overlay: overlay, git_sha: SHA).pin

        refute result.valid?
        assert_includes result.errors.join("\n"), "kustomization.yaml must set BROWSER_RUNTIME_IMAGE"
      end
    end

    def test_preserves_kustomization_file_mode
      with_overlay_copy do |overlay|
        kustomization_path = File.join(overlay, "kustomization.yaml")
        File.chmod(0o644, kustomization_path)

        result = Pinner.new(overlay: overlay, git_sha: SHA).pin

        assert result.valid?, result.errors.join("\n")
        assert_equal 0o644, File.stat(kustomization_path).mode & 0o777
      end
    end

    def test_does_not_write_when_writer_reports_failure
      with_overlay_copy do |overlay|
        kustomization_path = File.join(overlay, "kustomization.yaml")
        original_kustomization = File.read(kustomization_path)

        result = Pinner.new(overlay: overlay, git_sha: SHA, writer: FailingWriter.new).pin

        refute result.valid?
        assert_includes result.errors.join("\n"), "failed to write pinned overlay images"
        assert_equal original_kustomization, File.read(kustomization_path)
      end
    end

    def test_cli_pins_from_github_sha
      with_overlay_copy do |overlay|
        stdout, stderr, status = Open3.capture3(
          { "GITHUB_SHA" => SHA },
          RbConfig.ruby,
          "script/kubernetes/pin-overlay-images.rb",
          "--overlay",
          overlay,
        )

        assert status.success?, stderr
        assert_includes stdout, "pinned #{File.join(overlay, 'kustomization.yaml')}"
        assert_empty stderr
        assert_equal "#{DEFAULT_REGISTRY}/#{BROWSER_RUNTIME_REPOSITORY}:#{TAG}",
                     runtime_image_value(File.join(overlay, "kustomization.yaml"))
      end
    end

    def test_cli_pins_to_image_namespace
      with_overlay_copy do |overlay|
        stdout, stderr, status = Open3.capture3(
          RbConfig.ruby,
          "script/kubernetes/pin-overlay-images.rb",
          "--overlay",
          overlay,
          "--git-sha",
          SHA,
          "--image-namespace",
          "registry.internal.example/mpp",
        )

        assert status.success?, stderr
        assert_includes stdout, "pinned #{File.join(overlay, 'kustomization.yaml')}"
        assert_empty stderr
        assert_equal "registry.internal.example/mpp/#{BROWSER_RUNTIME_REPOSITORY}:#{TAG}",
                     runtime_image_value(File.join(overlay, "kustomization.yaml"))
      end
    end

    def test_cli_rejects_unknown_options_without_stack_trace
      stdout, stderr, status = Open3.capture3(
        RbConfig.ruby,
        "script/kubernetes/pin-overlay-images.rb",
        "--registry",
        "registry.internal.example/mpp",
      )

      refute status.success?
      assert_empty stdout
      assert_includes stderr, "pin-overlay-images: invalid option: --registry"
      refute_includes stderr, "OptionParser::InvalidOption"
    end

    class FailingWriter
      def write(_path, _document)
        ["failed to write pinned overlay images: permission denied"]
      end
    end

    private

    def with_overlay_copy
      Dir.mktmpdir("mpp-overlay-image-pinner") do |dir|
        overlay = File.join(dir, "production-managed")
        FileUtils.cp_r("deploy/kubernetes/overlays/production-managed", overlay)
        yield overlay
      end
    end

    def load_yaml(path)
      YAML.safe_load(
        File.read(path),
        permitted_classes: [],
        permitted_symbols: [],
        aliases: true,
      )
    end

    def runtime_image_value(path)
      kustomization = load_yaml(path)
      patch = kustomization.fetch("patches").find { |entry| entry["patch"].to_s.include?("BROWSER_RUNTIME_IMAGE") }
      document = YAML.safe_load(
        patch.fetch("patch"),
        permitted_classes: [],
        permitted_symbols: [],
        aliases: true,
      )
      container = document.dig("spec", "template", "spec", "containers").find do |entry|
        entry["name"] == "browser-worker"
      end
      container.fetch("env").find { |entry| entry["name"] == "BROWSER_RUNTIME_IMAGE" }.fetch("value")
    end

    def runtime_image_patch_value(path)
      document = load_yaml(path)
      container = document.dig("spec", "template", "spec", "containers").find do |entry|
        entry["name"] == "browser-worker"
      end
      container.fetch("env").find { |entry| entry["name"] == "BROWSER_RUNTIME_IMAGE" }.fetch("value")
    end
  end
end
