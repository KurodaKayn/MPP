# frozen_string_literal: true

module KubernetesImageNamespace
  REGISTRY_PATTERN = /\A[a-z0-9]+(?:[.-][a-z0-9]+)*(?::[0-9]+)?\z/
  REPOSITORY_COMPONENT_PATTERN = /\A[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*\z/

  module_function

  def normalize(value)
    value.to_s.strip.sub(%r{/+\z}, "")
  end

  def validation_errors(value)
    namespace = normalize(value)
    errors = []

    if namespace.empty?
      errors << "image namespace must be set"
      return errors
    end

    errors << "image namespace must not contain whitespace" if namespace.match?(/\s/)
    errors << "image namespace must not include digests" if namespace.include?("@")
    errors << "image namespace must not include a URL scheme" if namespace.include?("://")
    errors << "image namespace must not contain empty path components" if namespace.split("/").any?(&:empty?)

    components = namespace.split("/")
    registry = components.first.to_s
    unless registry.match?(REGISTRY_PATTERN) || registry == "localhost"
      errors << "image namespace registry must be lowercase and registry-qualified"
    end

    components.drop(1).each do |component|
      if component.include?(":")
        errors << "image namespace must not include tags"
        next
      end
      unless component.match?(REPOSITORY_COMPONENT_PATTERN)
        errors << "image namespace path components must use lowercase OCI repository names"
      end
    end

    errors.uniq
  end
end
