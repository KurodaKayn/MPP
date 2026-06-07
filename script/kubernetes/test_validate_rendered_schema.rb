# frozen_string_literal: true

require "minitest/autorun"
require "open3"
require "rbconfig"
require "tempfile"

class ValidateRenderedSchemaTest < Minitest::Test
  def test_invokes_kubeconform_with_strict_kubernetes_schema_flags
    with_tempfile("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: test\n") do |rendered|
      with_fake_kubeconform(<<~RUBY) do |kubeconform, log|
        puts "schema ok"
        exit 0
      RUBY
        stdout, stderr, status = run_validator(rendered.path, kubeconform)

        assert status.success?, stderr
        assert_includes stdout, "schema ok"
        assert_empty stderr

        args = File.read(log).split("\n")
        assert_includes args, "-strict"
        assert_includes args, "-summary"
        assert_includes args, "-ignore-missing-schemas"
        assert_includes args, "-kubernetes-version"
        assert_includes args, "1.33.0"
        assert_includes args, rendered.path
      end
    end
  end

  def test_allows_kubernetes_version_override
    with_tempfile("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: test\n") do |rendered|
      with_fake_kubeconform("exit 0") do |kubeconform, log|
        _stdout, stderr, status = run_validator(
          rendered.path,
          kubeconform,
          "KUBECONFORM_KUBERNETES_VERSION" => "1.32.0",
        )

        assert status.success?, stderr
        assert_includes File.read(log).split("\n"), "1.32.0"
      end
    end
  end

  def test_fails_when_rendered_file_is_missing
    stdout, stderr, status = Open3.capture3(
      RbConfig.ruby,
      "script/kubernetes/validate-rendered-schema.rb",
      "deploy/kubernetes/app-baseline",
      "/tmp/mpp-missing-rendered-schema.yaml",
    )

    refute status.success?
    assert_empty stdout
    assert_includes stderr, "Rendered manifest file does not exist"
  end

  def test_reports_missing_kubeconform_binary
    with_tempfile("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: test\n") do |rendered|
      stdout, stderr, status = run_validator(rendered.path, "/tmp/mpp-missing-kubeconform")

      refute status.success?
      assert_empty stdout
      assert_includes stderr, "kubeconform binary not found"
    end
  end

  private

  def run_validator(rendered_path, kubeconform, env = {})
    Open3.capture3(
      { "KUBECONFORM_BIN" => kubeconform }.merge(env),
      RbConfig.ruby,
      "script/kubernetes/validate-rendered-schema.rb",
      "deploy/kubernetes/app-baseline",
      rendered_path,
    )
  end

  def with_tempfile(content)
    file = Tempfile.new(["mpp-rendered-schema", ".yaml"])
    file.write(content)
    file.close
    yield file
  ensure
    file&.unlink
  end

  def with_fake_kubeconform(body)
    log = Tempfile.new("mpp-kubeconform-args")
    log.close

    executable = Tempfile.new("mpp-kubeconform")
    executable.write(<<~RUBY)
      #!#{RbConfig.ruby}
      File.write(#{log.path.inspect}, ARGV.join("\\n"))
      #{body}
    RUBY
    executable.close
    File.chmod(0o755, executable.path)

    yield executable.path, log.path
  ensure
    executable&.unlink
    log&.unlink
  end
end
