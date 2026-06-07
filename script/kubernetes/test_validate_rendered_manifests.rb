# frozen_string_literal: true

require "minitest/autorun"
require "open3"
require "rbconfig"
require "tempfile"

class ValidateRenderedManifestsTest < Minitest::Test
  STAGING_OVERLAYS = [
    "deploy/kubernetes/overlays/staging-managed",
    "deploy/kubernetes/overlays/staging-self-hosted",
  ].freeze

  def test_deployable_validation_rejects_checked_in_staging_examples
    STAGING_OVERLAYS.each do |overlay|
      rendered = render_overlay(overlay)

      _stdout, stderr, status = Open3.capture3(
        { "MPP_KUBERNETES_VALIDATE_DEPLOYABLE" => "1" },
        RbConfig.ruby,
        "script/kubernetes/validate-rendered-manifests.rb",
        overlay,
        rendered.path,
      )

      refute status.success?, "#{overlay} unexpectedly passed deployable validation"
      assert_includes stderr, "must not use example.invalid"
      assert_includes stderr, "must not use the all-zero example sha tag"
      assert_includes stderr, "must not use an example value"
    ensure
      rendered&.unlink
    end
  end

  private

  def render_overlay(overlay)
    rendered = Tempfile.new(["mpp-rendered-manifests", ".yaml"])
    stdout, stderr, status = Open3.capture3("kubectl", "kustomize", overlay)
    raise "#{overlay} failed to render: #{stderr}" unless status.success?

    rendered.write(stdout)
    rendered.close
    rendered
  end
end
