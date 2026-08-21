# frozen_string_literal: true

require "fileutils"
require "open3"
require_relative "opendota"

module Replay
  PBDEMS2_MAGIC = "PBDEMS2\x00".b
  DECOMPRESSOR = File.expand_path("../../parser/cmd/replay-decompress", __dir__)
  PARSER_DIR = File.expand_path("../../parser", __dir__)

  module_function

  def valid_dem?(dem_path)
    return false unless File.exist?(dem_path)

    File.binread(dem_path, PBDEMS2_MAGIC.bytesize) == PBDEMS2_MAGIC
  end

  def decompress(compressed_path, dem_path)
    stdout, stderr, status = Open3.capture3(
      "go", "run", DECOMPRESSOR,
      chdir: PARSER_DIR,
      binmode: true,
      stdin_data: File.binread(compressed_path)
    )
    raise "decompress failed: #{stderr}" unless status.success?

    unless stdout.start_with?(PBDEMS2_MAGIC)
      raise "decompressed replay missing PBDEMS2 header"
    end

    File.binwrite(dem_path, stdout)
  end

  # Request parse, resolve replay_url, download .dem.bz2, decompress to .dem.
  # Returns match details hash, :skipped, or nil if replay_url unavailable.
  def fetch_into(match_id, dem_path:, bz2_path:, keep_bz2: false, log_prefix: "[replay]")
    if File.exist?(dem_path) && valid_dem?(dem_path)
      puts "#{log_prefix} skip #{match_id}: #{dem_path} exists"
      return :skipped
    end

    if File.exist?(dem_path)
      warn "#{log_prefix} #{match_id}: replacing invalid #{dem_path}"
      FileUtils.rm_f(dem_path)
    end

    FileUtils.mkdir_p(File.dirname(dem_path))

    puts "#{log_prefix} #{match_id}: requesting parse..."
    puts "#{log_prefix} request_parse: #{OpenDota.request_parse(match_id).inspect}"
    sleep 2

    puts "#{log_prefix} #{match_id}: fetching match details..."
    details = OpenDota.get_match_details(match_id)
    replay_url = details["replay_url"]
    if replay_url.nil? || replay_url.empty?
      warn "#{log_prefix} #{match_id}: replay_url is blank"
      return nil
    end

    puts "#{log_prefix} #{match_id}: downloading -> #{bz2_path}"
    OpenDota.download_replay(replay_url, bz2_path)

    puts "#{log_prefix} #{match_id}: decompressing -> #{dem_path}"
    decompress(bz2_path, dem_path)
    FileUtils.rm_f(bz2_path) unless keep_bz2

    puts "#{log_prefix} #{match_id}: done"
    details
  end
end
