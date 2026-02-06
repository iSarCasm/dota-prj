# frozen_string_literal: true

# Ruby 4.0 removed CGI.parse. ruby-openid (used by omniauth-steam) calls it.
# Define it when missing so Steam OAuth works.
if defined?(CGI) && !CGI.respond_to?(:parse)
  require "uri"

  CGI.define_singleton_method(:parse) do |query_string|
    return {} if query_string.nil? || query_string.to_s.empty?

    h = Hash.new { |hash, key| hash[key] = [] }
    URI.decode_www_form(query_string.to_s).each { |k, v| h[k] << v }
    h
  end
end
