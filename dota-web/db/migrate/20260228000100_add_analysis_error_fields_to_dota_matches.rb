# frozen_string_literal: true

class AddAnalysisErrorFieldsToDotaMatches < ActiveRecord::Migration[8.1]
  def change
    add_column :dota_matches, :analysis_error_message, :text
    add_column :dota_matches, :analysis_error_details, :text
  end
end
