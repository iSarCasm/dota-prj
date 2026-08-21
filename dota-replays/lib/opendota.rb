# frozen_string_literal: true

require "json"
require "net/http"
require "uri"

module OpenDota
  BASE_URL = "https://api.opendota.com/api"

  module_function

  def request_parse(match_id)
    uri = URI("#{BASE_URL}/request/#{match_id}")
    response = Net::HTTP.post(uri, "")
    JSON.parse(response.body)
  rescue StandardError => e
    warn "[opendota] request_parse #{match_id}: #{e.message}"
    nil
  end

  def get_match_details(match_id)
    uri = URI("#{BASE_URL}/matches/#{match_id}")
    response = Net::HTTP.get_response(uri)
    raise "HTTP #{response.code} for match #{match_id}" unless response.is_a?(Net::HTTPSuccess)

    JSON.parse(response.body)
  end

  def get_public_matches(min_rank: nil, max_rank: nil, less_than_match_id: nil)
    params = {}
    params[:min_rank] = min_rank if min_rank
    params[:max_rank] = max_rank if max_rank
    params[:less_than_match_id] = less_than_match_id if less_than_match_id

    uri = URI("#{BASE_URL}/publicMatches")
    uri.query = URI.encode_www_form(params) unless params.empty?

    response = Net::HTTP.get_response(uri)
    raise "HTTP #{response.code} for publicMatches" unless response.is_a?(Net::HTTPSuccess)

    JSON.parse(response.body)
  end

  def get_heroes
    uri = URI("#{BASE_URL}/heroes")
    response = Net::HTTP.get_response(uri)
    raise "HTTP #{response.code} for heroes" unless response.is_a?(Net::HTTPSuccess)

    JSON.parse(response.body)
  end

  def download_replay(replay_url, file_name)
    uri = URI(replay_url)
    Net::HTTP.start(uri.host, uri.port, use_ssl: uri.scheme == "https") do |http|
      request = Net::HTTP::Get.new(uri)
      http.request(request) do |response|
        raise "download HTTP #{response.code}" unless response.is_a?(Net::HTTPSuccess)

        File.open(file_name, "wb") do |file|
          response.read_body { |chunk| file.write(chunk) }
        end
      end
    end
  end
end
