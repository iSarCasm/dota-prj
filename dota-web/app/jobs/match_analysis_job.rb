# frozen_string_literal: true

require "open3"

class MatchAnalysisJob < ApplicationJob
  queue_as :default

  def perform(dota_match_id)
    dota_match = DotaMatch.find_by(id: dota_match_id)
    if dota_match.blank?
      Rails.logger.error "[MatchAnalysisJob] dota_match not found for id #{dota_match_id}"
      return
    end

    account_id = dota_match.players&.first
    if account_id.blank?
      dota_match.update(status: "error")
      Rails.logger.error "[MatchAnalysisJob] no account_id in players array"
      return
    end

    api = OpenDotaApi.new
    response = api.request_parse(match_id: dota_match.match_id)
    Rails.logger.info "[MatchAnalysisJob] request_parse response: #{response}"
    sleep 2
    match_details = api.get_match_details(match_id: dota_match.match_id)
    replay_url = match_details["replay_url"]

    hero_name = hero_name_for_account(match_details, account_id)
    if hero_name.blank?
      dota_match.update(status: "error")
      Rails.logger.error "[MatchAnalysisJob] could not resolve hero for account_id #{account_id}"
      return
    end

    if replay_url.blank?
      dota_match.update(status: "error")
      Rails.logger.warn "[MatchAnalysisJob] replay url blank for match #{dota_match.match_id}"
      return
    end

    replay_path = Rails.root.join("storage", "replays", "#{dota_match.match_id}.dem")
    bz2_path = "#{replay_path}.bz2"
    FileUtils.mkdir_p(replay_path.parent)

    Rails.logger.info "[MatchAnalysisJob] downloading replay (bz2) to #{bz2_path}"
    api.download_replay(replay_url: replay_url, file_name: bz2_path)

    unless decompress_bz2(bz2_path, replay_path.to_s)
      dota_match.update(status: "error")
      Rails.logger.error "[MatchAnalysisJob] failed to decompress replay"
      return
    end
    FileUtils.rm_f(bz2_path)

    dota_match.update(replay_file: replay_path.to_s, status: "downloaded")

    run_parser(replay_path.to_s, dota_match, hero_name)
  end

  private

  def hero_name_for_account(match_details, account_id)
    players = Array(match_details["players"])
    player = players.find { |p| p["account_id"].to_i == account_id.to_i }
    return nil if player.blank?

    hero_id = player["hero_id"]&.to_s
    return nil if hero_id.blank?

    Rails.configuration.x.constants.heroes.dig(hero_id, "localized_name")
  end

  def decompress_bz2(bz2_path, dem_path)
    stdout, stderr, status = Open3.capture3("bunzip2", "-f", "-c", bz2_path)
    if status.success?
      File.binwrite(dem_path, stdout)
      true
    else
      Rails.logger.error "[MatchAnalysisJob] bunzip2 failed: #{stderr}"
      false
    end
  end

  def run_parser(replay_path, dota_match, hero_name)
    bin = ENV.fetch("REPLAY_PARSER_BIN")
    unless File.executable?(bin)
      dota_match.update(status: "error")
      Rails.logger.error "[MatchAnalysisJob] parser binary not found or not executable: #{bin}"
      return
    end

    output_dir = Rails.root.join("storage", "replays", dota_match.match_id).to_s
    FileUtils.mkdir_p(output_dir)

    Rails.logger.info "[MatchAnalysisJob] running parser: #{bin} #{dota_match.match_id} #{hero_name} #{replay_path} #{output_dir}"
    stdout, stderr, status = Open3.capture3(
      bin,
      dota_match.match_id.to_s,
      hero_name,
      replay_path,
      output_dir,
      chdir: File.dirname(bin)
    )

    unless status.success?
      dota_match.update(status: "error")
      Rails.logger.error "[MatchAnalysisJob] parser failed: #{stderr}"
      return
    end

    dota_match.update(status: "parsed")
    Rails.logger.info "[MatchAnalysisJob] parse complete for match #{dota_match.match_id}"

    # Save output to dota_match.output
    output_path = Rails.root.join("storage", "replays", dota_match.match_id, "#{dota_match.match_id}_output.json")
    output = JSON.parse(File.read(output_path))
    dota_match.update(output: output)
    Rails.logger.info "[MatchAnalysisJob] saved output to dota_match.output"
  end
end
