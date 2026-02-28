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
      mark_error!(dota_match, "No account_id in players array")
      return
    end

    api = OpenDotaApi.new
    dota_match.update(status: "requesting_parse", analysis_error_message: nil, analysis_error_details: nil)
    response = api.request_parse(match_id: dota_match.match_id)
    Rails.logger.info "[MatchAnalysisJob] request_parse response: #{response}"
    sleep 2
    dota_match.update(status: "fetching_match_data")
    match_details = api.get_match_details(match_id: dota_match.match_id)
    replay_url = match_details["replay_url"]

    hero_name = hero_name_for_account(match_details, account_id)
    hero_name = hero_name.delete(" ")
    if hero_name.blank?
      mark_error!(dota_match, "Could not resolve hero for account_id #{account_id}")
      return
    end

    if replay_url.blank?
      mark_error!(dota_match, "Replay URL is blank for match #{dota_match.match_id}")
      return
    end

    replay_path = Rails.root.join("storage", "replays", "#{dota_match.match_id}.dem")
    bz2_path = "#{replay_path}.bz2"
    FileUtils.mkdir_p(replay_path.parent)

    dota_match.update(status: "downloading_replay")
    Rails.logger.info "[MatchAnalysisJob] downloading replay (bz2) to #{bz2_path}"
    api.download_replay(replay_url: replay_url, file_name: bz2_path)

    unless decompress_bz2(bz2_path, replay_path.to_s)
      mark_error!(dota_match, "Failed to decompress replay")
      return
    end
    FileUtils.rm_f(bz2_path)

    dota_match.update(replay_file: replay_path.to_s, status: "downloaded")

    run_parser(replay_path.to_s, dota_match, hero_name)
  rescue StandardError => e
    if dota_match.present?
      mark_error!(dota_match, "Unexpected error: #{e.class}: #{e.message}", details: e.backtrace&.first(20)&.join("\n"))
    else
      Rails.logger.error "[MatchAnalysisJob] unexpected error before dota_match load: #{e.class}: #{e.message}"
      Rails.logger.error e.backtrace.first(20).join("\n") if e.backtrace.present?
    end
  end

  private

  def hero_name_for_account(match_details, account_id)
    players = Array(match_details["players"])
    player = players.find { |p| p["account_id"].to_i == account_id.to_i }
    # return nil if player.blank?
    raise "Player not found for account_id #{account_id}" if player.blank?

    hero_id = player["hero_id"]&.to_s
    raise "Hero ID is blank for player #{player}" if hero_id.blank?

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
      mark_error!(dota_match, "Parser binary not found or not executable: #{bin}")
      return
    end

    output_dir = Rails.root.join("storage", "replays", dota_match.match_id).to_s
    FileUtils.mkdir_p(output_dir)

    dota_match.update(status: "parsing")
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
      mark_error!(dota_match, "Parser failed", details: stderr.to_s.strip.presence || stdout.to_s.strip)
      return
    end

    # Save output to dota_match.output
    output_path = Rails.root.join("storage", "replays", dota_match.match_id, "#{dota_match.match_id}_output.json")
    output = JSON.parse(File.read(output_path))
    dota_match.update(output: output, status: "parsed", analysis_error_message: nil, analysis_error_details: nil)
    Rails.logger.info "[MatchAnalysisJob] parse complete and saved output for match #{dota_match.match_id}"
  end

  def mark_error!(dota_match, message, details: nil)
    details_text = details.to_s.strip.presence

    dota_match.update(
      status: "error",
      analysis_error_message: message,
      analysis_error_details: details_text
    )

    Rails.logger.error("[MatchAnalysisJob] #{message}")
    Rails.logger.error(details_text) if details_text.present?
  end
end
