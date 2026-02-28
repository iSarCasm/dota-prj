# frozen_string_literal: true

class DotaMatch < ApplicationRecord
  STATUS_PROGRESS = {
    "init" => 5,
    "requesting_parse" => 20,
    "fetching_match_data" => 35,
    "downloading_replay" => 60,
    "downloaded" => 75,
    "parsing" => 90,
    "parsed" => 100,
    "error" => 100
  }.freeze

  STATUS_LABELS = {
    "init" => "Queued",
    "requesting_parse" => "Requesting parse",
    "fetching_match_data" => "Fetching match data",
    "downloading_replay" => "Downloading replay",
    "downloaded" => "Replay ready",
    "parsing" => "Running parser",
    "parsed" => "Completed",
    "error" => "Failed"
  }.freeze

  validates :match_id, presence: true
  validates :status, presence: true

  after_create_commit :broadcast_analysis_output
  after_update_commit :broadcast_analysis_output, if: :saved_change_to_status_or_output?

  def analysis_stream_name
    "match_analysis_#{id}"
  end

  def analysis_output_dom_id
    "analysis_output_#{id}"
  end

  def analysis_progress_percent
    STATUS_PROGRESS.fetch(status.to_s, 0)
  end

  def analysis_status_label
    STATUS_LABELS.fetch(status.to_s, status.to_s.humanize)
  end

  def analysis_error?
    status.to_s == "error"
  end

  def analysis_completed?
    status.to_s == "parsed"
  end

  def analysis_in_progress?
    analysis_progress_percent.positive? && analysis_progress_percent < 100
  end

  def analysis_status_message
    return "Analysis failed. Check Sidekiq logs and retry." if analysis_error?
    return "Analysis output is ready below." if analysis_completed?

    "Processing match #{match_id}..."
  end

  private

  def saved_change_to_status_or_output?
    saved_change_to_status? || saved_change_to_output?
  end

  def broadcast_analysis_output
    broadcast_update_to(
      analysis_stream_name,
      target: analysis_output_dom_id,
      partial: "matches/analysis_output",
      locals: { dota_match: self }
    )
  end
end
