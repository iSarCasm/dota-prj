#!/usr/bin/env ruby
# frozen_string_literal: true

# Download and decompress Dota 2 replays from OpenDota.
# Same flow as dota-web/app/jobs/match_analysis_job.rb + lib/open_dota_api.rb
#
# With no arguments, fetches every match ID listed in REPLAYS.md.

require "fileutils"
require_relative "lib/opendota"
require_relative "lib/replay"

ROOT = ENV.fetch("DOTA_REPLAYS_DIR", File.expand_path(__dir__))
CATALOG = File.join(__dir__, "REPLAYS.md")

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

def fetch_match(match_id)
  dem_path = File.join(ROOT, "#{match_id}.dem")
  bz2_path = "#{dem_path}.bz2"
  Replay.fetch_into(match_id, dem_path: dem_path, bz2_path: bz2_path, keep_bz2: false, log_prefix: "[fetch]")
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
