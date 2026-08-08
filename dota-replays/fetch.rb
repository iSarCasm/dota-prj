#!/usr/bin/env ruby
# frozen_string_literal: true

# Download and decompress Dota 2 replays from OpenDota.
# Same flow as dota-web/app/jobs/match_analysis_job.rb + lib/open_dota_api.rb
#
# With no arguments, fetches every match ID listed in REPLAYS.md.

require "json"
require "net/http"
require "uri"
require "fileutils"
require "open3"

BASE_URL = "https://api.opendota.com/api"
ROOT = ENV.fetch("DOTA_REPLAYS_DIR", File.expand_path(__dir__))
CATALOG = File.join(__dir__, "REPLAYS.md")
PBDEMS2_MAGIC = "PBDEMS2\x00".b
DECOMPRESSOR = File.expand_path("../parser/cmd/replay-decompress", __dir__)

def usage
  warn <<~MSG
    Usage: #{$PROGRAM_NAME} [MATCH_ID ...]

    With no arguments, fetches all match IDs from #{CATALOG}.
  MSG
  exit 1
end

def catalog_match_ids
  return [] unless File.exist?(CATALOG)

  File.readlines(CATALOG).filter_map do |line|
    next unless line.start_with?("|")
    next if line.include?("Match ID") || line.include?("---")

    match = line.match(/\|\s*(\d{8,})\s*\|/)
    match&.[](1)
  end.uniq
end

def request_parse(match_id)
  uri = URI("#{BASE_URL}/request/#{match_id}")
  response = Net::HTTP.post(uri, "")
  JSON.parse(response.body)
rescue StandardError => e
  warn "[fetch] request_parse #{match_id}: #{e.message}"
  nil
end

def get_match_details(match_id)
  uri = URI("#{BASE_URL}/matches/#{match_id}")
  response = Net::HTTP.get_response(uri)
  raise "HTTP #{response.code} for match #{match_id}" unless response.is_a?(Net::HTTPSuccess)

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

def valid_dem?(dem_path)
  return false unless File.exist?(dem_path)

  File.binread(dem_path, PBDEMS2_MAGIC.bytesize) == PBDEMS2_MAGIC
end

def decompress_replay(compressed_path, dem_path)
  parser_dir = File.expand_path("../parser", __dir__)
  stdout, stderr, status = Open3.capture3(
    "go", "run", DECOMPRESSOR,
    chdir: parser_dir,
    binmode: true,
    stdin_data: File.binread(compressed_path)
  )
  raise "decompress failed: #{stderr}" unless status.success?

  unless stdout.start_with?(PBDEMS2_MAGIC)
    raise "decompressed replay missing PBDEMS2 header"
  end

  File.binwrite(dem_path, stdout)
end

def fetch_match(match_id)
  dem_path = File.join(ROOT, "#{match_id}.dem")
  bz2_path = "#{dem_path}.bz2"

  if File.exist?(dem_path) && valid_dem?(dem_path)
    puts "skip #{match_id}: #{dem_path} exists"
    return
  end

  if File.exist?(dem_path)
    warn "[fetch] #{match_id}: replacing invalid #{dem_path}"
    FileUtils.rm_f(dem_path)
  end

  puts "[fetch] #{match_id}: requesting parse..."
  puts "[fetch] request_parse response: #{request_parse(match_id).inspect}"
  sleep 2

  puts "[fetch] #{match_id}: fetching match details..."
  details = get_match_details(match_id)
  replay_url = details["replay_url"]
  if replay_url.nil? || replay_url.empty?
    warn "[fetch] #{match_id}: replay_url is blank"
    return
  end

  puts "[fetch] #{match_id}: downloading -> #{bz2_path}"
  download_replay(replay_url, bz2_path)

  puts "[fetch] #{match_id}: decompressing -> #{dem_path}"
  decompress_replay(bz2_path, dem_path)
  FileUtils.rm_f(bz2_path)

  puts "[fetch] #{match_id}: done"
end

if ARGV.include?("-h") || ARGV.include?("--help")
  usage
end

match_ids = if ARGV.empty?
              catalog_match_ids
            else
              ARGV.map(&:strip)
            end

if match_ids.empty?
  warn "[fetch] no match IDs (add rows to #{CATALOG} or pass IDs on the command line)"
  exit 1
end

FileUtils.mkdir_p(ROOT)
puts "[fetch] #{match_ids.length} replay(s): #{match_ids.join(', ')}"

match_ids.each { |match_id| fetch_match(match_id) }
