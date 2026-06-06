# frozen_string_literal: true

module KubernetesValidation
  module Images
    module_function

    def validate(context)
      local_images = rendered_image_issues(context) { |image| image.start_with?("mpp-") }
      unless local_images.empty?
        context.add_error("rendered manifests contain local app images: #{local_images.join('; ')}")
      end

      latest_images = rendered_image_issues(context) { |image| image.end_with?(":latest") }
      unless latest_images.empty?
        context.add_error("rendered manifests contain latest image tags: #{latest_images.join('; ')}")
      end

      if context.deployable_package?
        placeholder_images = rendered_image_issues(context) { |image| placeholder_image?(image) }
        unless placeholder_images.empty?
          context.add_error(
            "rendered manifests contain placeholder app images: #{placeholder_images.join('; ')}",
          )
        end
      end

      runtime_image_issues(context, reject_placeholders: context.deployable_package?).each do |issue|
        context.add_error(issue)
      end
    end

    def rendered_image_issues(context)
      image_lines(context)
        .select { |line| yield(line[:value]) }
        .map { |line| "#{line[:line_number]}:#{line[:line]}" }
    end

    def image_lines(context)
      context.rendered.lines.each_with_index.map do |line, index|
        match = line.match(/^\s*image:\s*([^#\s]+)\s*(?:#.*)?$/)
        next unless match

        {
          line_number: index + 1,
          line: line.chomp,
          value: Context.unquote_scalar(match[1]),
        }
      end.compact
    end

    def runtime_image_issues(context, reject_placeholders:)
      issues = []
      lines = context.rendered.lines.map(&:chomp)

      lines.each_with_index do |line, index|
        next unless line.match?(/^\s*-\s+name:\s+BROWSER_RUNTIME_IMAGE\s*$/)

        lines[(index + 1)..-1].to_a.each_with_index do |candidate, offset|
          break if offset.positive? && candidate.match?(/^\s*-\s+name:/)

          match = candidate.match(/^\s*value:\s*([^#\s]+)\s*(?:#.*)?$/)
          next unless match

          value = Context.unquote_scalar(match[1])
          if value.start_with?("mpp-") || value.end_with?(":latest")
            issues << "BROWSER_RUNTIME_IMAGE is unresolved at line #{index + offset + 2}: #{candidate}"
          end
          if reject_placeholders && placeholder_image?(value)
            issues << "BROWSER_RUNTIME_IMAGE has a placeholder value at line #{index + offset + 2}: #{candidate}"
          end
          break
        end
      end

      issues
    end

    def placeholder_image?(image)
      image.include?("replace-me") || image.start_with?("registry.example.invalid/")
    end
  end
end
