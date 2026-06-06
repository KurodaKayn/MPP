# frozen_string_literal: true

module KubernetesSmoke
  class CheckFailure < StandardError; end

  class CheckSkip < StandardError; end

  class Reporter
    attr_reader :failures, :passes, :skips, :warnings

    def initialize(io, verbose: false)
      @io = io
      @verbose = verbose
      @failures = []
      @passes = []
      @skips = []
      @warnings = []
      @section = nil
    end

    def section(title)
      return if @section == title

      @section = title
      @io.puts
      @io.puts "#{title}:"
    end

    def check(name, required: true)
      detail = yield
      pass(name, detail)
    rescue CheckSkip => error
      skip(name, error.message)
    rescue CheckFailure => error
      if required
        fail(name, error.message)
      else
        warn(name, error.message)
      end
    rescue Kubectl::CommandError => error
      message = command_error_message(error)
      if required
        fail(name, message)
      else
        warn(name, message)
      end
    rescue StandardError => error
      message = "#{error.class}: #{error.message}"
      if required
        fail(name, message)
      else
        warn(name, message)
      end
    end

    def pass(name, detail = nil)
      @passes << [name, detail]
      line("PASS", name, detail)
    end

    def fail(name, detail)
      @failures << [name, detail]
      line("FAIL", name, detail)
    end

    def warn(name, detail)
      @warnings << [name, detail]
      line("WARN", name, detail)
    end

    def skip(name, detail)
      @skips << [name, detail]
      line("SKIP", name, detail)
    end

    def info(message)
      @io.puts "  INFO #{message}" if @verbose
    end

    def command(command)
      info("$ #{command.join(' ')}")
    end

    def summary
      @io.puts
      @io.puts "Summary:"
      @io.puts "  passes: #{passes.length}"
      @io.puts "  warnings: #{warnings.length}"
      @io.puts "  skips: #{skips.length}"
      @io.puts "  failures: #{failures.length}"
      return if failures.empty?

      @io.puts
      @io.puts "Failures:"
      failures.each do |name, detail|
        @io.puts "  - #{name}: #{detail}"
      end
    end

    def exit_code
      summary
      failures.empty? ? 0 : 1
    end

    private

    def line(status, name, detail)
      if detail.nil? || detail.to_s.empty?
        @io.puts "  #{status} #{name}"
      else
        @io.puts "  #{status} #{name} - #{detail}"
      end
    end

    def command_error_message(error)
      parts = ["#{error.command.join(' ')} exited #{error.status.exitstatus}"]
      stderr = error.stderr.to_s.strip
      stdout = error.stdout.to_s.strip
      parts << "stderr: #{stderr}" unless stderr.empty?
      parts << "stdout: #{stdout}" unless stdout.empty?
      parts.join("; ")
    end
  end
end
