# frozen_string_literal: true

class CreateCachedReplays < ActiveRecord::Migration[8.1]
  def change
    create_table :cached_replays do |t|
      t.string :match_id, null: false
      t.string :zip_path
      t.string :demo_path

      t.timestamps
    end

    add_index :cached_replays, :match_id, unique: true
  end
end
