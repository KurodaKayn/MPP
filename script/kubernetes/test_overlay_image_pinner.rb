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
        assert_equal [
          File.join(overlay, "kustomization.yaml"),
          File.join(overlay, "runtime-image-patch.yaml"),
        ], result.updated_files

        images = load_yaml(File.join(overlay, "kustomization.yaml")).fetch("images")
        APP_IMAGE_REPOSITORIES.values.each do |repository|
          image = images.find { |entry| entry["newName"] == "#{DEFAULT_REGISTRY}/#{repository}" }
          assert image, "missing pinned image for #{repository}"
          assert_equal TAG, image["newTag"]
        end

        runtime_image = runtime_image_value(File.join(overlay, "runtime-image-patch.yaml"))
        assert_equal "#{DEFAULT_REGISTRY}/#{BROWSER_RUNTIME_REPOSITORY}:#{TAG}", runtime_image
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

    def test_rejects_empty_runtime_patch
      with_overlay_copy do |overlay|
        File.write(File.join(overlay, "runtime-image-patch.yaml"), "")

        result = Pinner.new(overlay: overlay, git_sha: SHA).pin

        refute result.valid?
        assert_includes result.errors.join("\n"), "runtime-image-patch.yaml must set BROWSER_RUNTIME_IMAGE"
      end
    end

    def test_rolls_back_when_second_file_replace_fails
      with_overlay_copy do |overlay|
        kustomization_path = File.join(overlay, "kustomization.yaml")
        runtime_patch_path = File.join(overlay, "runtime-image-patch.yaml")
        original_kustomization = File.read(kustomization_path)
        original_runtime_patch = File.read(runtime_patch_path)

        result = Pinner.new(
          overlay: overlay,
          git_sha: SHA,
          writer: FailingSecondMoveWriter.new,
        ).pin

        refute result.valid?
        assert_includes result.errors.join("\n"), "failed to write pinned overlay images"
        assert_equal original_kustomization, File.read(kustomization_path)
        assert_equal original_runtime_patch, File.read(runtime_patch_path)
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
                     runtime_image_value(File.join(overlay, "runtime-image-patch.yaml"))
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

    class FailingSecondMoveWriter < AtomicYamlWriter
      private

      def move_file(source, target)
        @move_count = @move_count.to_i + 1
        raise Errno::EACCES, target if @move_count == 2

        super
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
      document = load_yaml(path)
      container = document.dig("spec", "template", "spec", "containers").find do |entry|
        entry["name"] == "browser-worker"
      end
      container.fetch("env").find { |entry| entry["name"] == "BROWSER_RUNTIME_IMAGE" }.fetch("value")
    end
  end
end
