# frozen_string_literal: true

class CreateCachedMatchDetails < ActiveRecord::Migration[8.1]
  def change
    create_table :cached_match_details do |t|
      t.string :match_id, null: false
      t.json :payload, null: false

      t.timestamps
    end

    add_index :cached_match_details, :match_id, unique: true
  end
end
