# frozen_string_literal: true

require "json"
require "net/http"
require "uri"

module KubernetesSmoke
  class HttpClient
    Response = Struct.new(:status, :body, :headers)

    def initialize(timeout:)
      @timeout = timeout
    end

    def get(url, headers: {})
      request("GET", url, headers: headers)
    end

    def post(url, headers: {}, json: nil)
      request("POST", url, headers: headers, json: json)
    end

    def delete(url, headers: {})
      request("DELETE", url, headers: headers)
    end

    def request(method, url, headers: {}, json: nil)
      uri = URI.parse(url)
      http = Net::HTTP.new(uri.host, uri.port)
      http.use_ssl = uri.scheme == "https"
      http.open_timeout = @timeout
      http.read_timeout = @timeout

      request = request_class(method).new(uri)
      headers.each { |key, value| request[key] = value }
      if json
        request["Content-Type"] = "application/json"
        request.body = JSON.generate(json)
      end

      response = http.request(request)
      Response.new(response.code.to_i, response.body.to_s, response.to_hash)
    rescue URI::InvalidURIError => error
      raise CheckFailure, "invalid URL #{url.inspect}: #{error.message}"
    rescue IOError, SystemCallError, Timeout::Error, SocketError => error
      raise CheckFailure, "HTTP request to #{url} failed: #{error.class}: #{error.message}"
    end

    private

    def request_class(method)
      case method
      when "GET"
        Net::HTTP::Get
      when "POST"
        Net::HTTP::Post
      when "DELETE"
        Net::HTTP::Delete
      else
        raise ArgumentError, "unsupported HTTP method #{method}"
      end
    end
  end
end
