require 'httparty'
require 'json'

class OpenDotaApi
  def initialize
    @base_url = "https://api.opendota.com/api"
  end


  # GET /publicMatches
  def get_public_matches(min_rank:, max_rank:, less_than_match_id: nil)
    response = HTTParty.get("#{@base_url}/publicMatches?min_rank=#{min_rank}&max_rank=#{max_rank}#{less_than_match_id ? "&less_than_match_id=#{less_than_match_id}" : ''}")
    JSON.parse(response.body)
  end

  def get_match_details(match_id:)
    response = HTTParty.get("#{@base_url}/matches/#{match_id}")
    JSON.parse(response.body)
  end

  def download_replay(replay_url:)
    response = HTTParty.get(replay_url)
    File.write("replay.dem", response.body)
  end
end


results = OpenDotaApi.new.get_public_matches(min_rank: 80, max_rank: 80, less_than_match_id: 8646005862)
results.each do |v|
  v['start_time'] = Time.at(v['start_time'])
end
puts "results: #{results}"

binding.irb


match = results.last
match_details = OpenDotaApi.new.get_match_details(match_id: match['match_id'])
puts "match_details: #{match_details}"

binding.irb


# replay_url = match_details['replay_url']
# OpenDotaApi.new.download_replay(replay_url: replay_url)

binding.irb