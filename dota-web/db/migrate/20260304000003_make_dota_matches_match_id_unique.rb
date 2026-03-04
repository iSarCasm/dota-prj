# frozen_string_literal: true

class MakeDotaMatchesMatchIdUnique < ActiveRecord::Migration[8.1]
  def up
    dupes = select_values(<<~SQL)
      SELECT match_id
      FROM dota_matches
      GROUP BY match_id
      HAVING COUNT(*) > 1
      ORDER BY match_id
    SQL

    if dupes.any?
      raise ActiveRecord::IrreversibleMigration,
            "Cannot add unique index: duplicate dota_matches.match_id values exist: #{dupes.first(20).join(', ')}" \
            "#{' ...' if dupes.size > 20}"
    end

    remove_index :dota_matches, :match_id if index_exists?(:dota_matches, :match_id, name: "index_dota_matches_on_match_id")
    add_index :dota_matches, :match_id, unique: true
  end

  def down
    remove_index :dota_matches, :match_id if index_exists?(:dota_matches, :match_id)
    add_index :dota_matches, :match_id
  end
end

