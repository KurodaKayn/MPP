# frozen_string_literal: true

module KubernetesOverlayClassification
  PRODUCTION_OVERLAY_NAME_PATTERN = /\A(?:[a-z0-9]+-)*production(?:-[a-z0-9]+)*\z/

  module_function

  def production_overlay_name?(name)
    name.to_s.match?(PRODUCTION_OVERLAY_NAME_PATTERN)
  end

  def production_overlay_path?(path)
    production_overlay_name?(File.basename(path.to_s.tr("\\", "/")))
  end

  def production_overlay_package?(package_dir)
    normalized = package_dir.to_s.tr("\\", "/")
    match = normalized.match(%r{(?:\A|/)deploy/kubernetes/overlays/([^/]+)\z})
    match && production_overlay_name?(match[1])
  end
end
