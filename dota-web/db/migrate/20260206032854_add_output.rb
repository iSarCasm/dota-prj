class AddOutput < ActiveRecord::Migration[8.1]
  def change
    add_column :dota_matches, :output, :json, default: {}
  end
end
