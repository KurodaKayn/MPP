# frozen_string_literal: true

require "yaml"

module KubernetesValidation
  class Document
    attr_reader :raw, :object

    def initialize(raw, object)
      @raw = raw
      @object = object || {}
    end

    def kind
      object["kind"].to_s
    end

    def metadata
      hash(object["metadata"])
    end

    def name
      metadata["name"].to_s
    end

    def namespace
      metadata["namespace"].to_s
    end

    def labels
      hash(metadata["labels"])
    end

    def spec
      hash(object["spec"])
    end

    def data
      hash(object["data"])
    end

    def pod_spec
      template = hash(spec["template"])
      template_spec = hash(template["spec"])
      return template_spec unless template_spec.empty?

      spec
    end

    def pod_labels
      template = hash(spec["template"])
      metadata = hash(template["metadata"])
      hash(metadata["labels"])
    end

    def containers
      array(pod_spec["containers"])
    end

    def container(name = nil)
      return containers.first if name.nil?

      containers.find { |container| container["name"] == name }
    end

    def [](key)
      object[key]
    end

    private

    def hash(value)
      value.is_a?(Hash) ? value : {}
    end

    def array(value)
      value.is_a?(Array) ? value : []
    end
  end

  class Context
    attr_reader :package_dir, :rendered_path, :rendered, :documents, :errors

    def self.from_file(package_dir, rendered_path)
      new(package_dir, rendered_path, File.read(rendered_path))
    end

    def initialize(package_dir, rendered_path, rendered)
      @package_dir = package_dir.tr("\\", "/")
      @rendered_path = rendered_path
      @rendered = rendered
      @errors = []
      @documents = parse_documents
    end

    def valid?
      errors.empty?
    end

    def add_error(message)
      errors << message
    end

    def path_suffix?(suffix)
      package_dir == suffix || package_dir.end_with?("/#{suffix}")
    end

    def deployable_package?
      !path_suffix?("deploy/kubernetes/app-baseline") &&
        !path_suffix?("deploy/kubernetes/browser-runtime-control") &&
        !package_dir.start_with?("validation/") &&
        !package_dir.include?("/validation/")
    end

    def find_lines(pattern)
      rendered.lines.each_with_index.map do |line, index|
        next unless line.match?(pattern)

        "#{index + 1}:#{line.chomp}"
      end.compact
    end

    def document(kind, name, namespace = nil)
      documents.find do |document|
        document.kind == kind &&
          document.name == name &&
          (namespace.nil? || document.namespace == namespace)
      end
    end

    def require_document(kind, name, namespace = nil)
      found = document(kind, name, namespace)
      return found if found

      add_error(
        if namespace.nil?
          "rendered manifests are missing #{kind}/#{name}"
        else
          "rendered manifests are missing #{kind}/#{namespace}/#{name}"
        end,
      )
      nil
    end

    def require_rendered(pattern, message)
      add_error(message) unless rendered.match?(pattern)
    end

    def require_document_pattern(document, pattern, message)
      add_error(message) unless document.raw.match?(pattern)
    end

    def self.unquote_scalar(value)
      text = value.to_s.strip
      first = text[0]
      last = text[-1]
      if (first == '"' && last == '"') || (first == "'" && last == "'")
        text[1...-1]
      else
        text
      end
    end

    private

    def parse_documents
      split_documents.each_with_index.map do |document, index|
        object = YAML.safe_load(document, [], [], true)
        next if object.nil?

        Document.new(document, object)
      rescue Psych::Exception => error
        add_error("rendered YAML document #{index + 1} failed to parse: #{error.message}")
        nil
      end.compact
    end

    def split_documents
      rendered
        .split(/^---\s*$/)
        .map(&:strip)
        .reject(&:empty?)
    end
  end
end
