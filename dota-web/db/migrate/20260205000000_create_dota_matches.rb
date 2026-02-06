# frozen_string_literal: true

class CreateDotaMatches < ActiveRecord::Migration[8.1]
  def change
    create_table :dota_matches do |t|
      t.string :match_id, null: false
      t.string :players, array: true, default: []
      t.string :replay_file
      t.string :status, null: false
      t.jsonb :output, default: {}

      t.timestamps
    end

    add_index :dota_matches, :match_id
    add_index :dota_matches, :status
  end
end
