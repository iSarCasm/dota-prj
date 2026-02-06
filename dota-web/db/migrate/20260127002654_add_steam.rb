class AddSteam < ActiveRecord::Migration[8.1]
  def change
    add_column :users, :steam_id, :string
    add_column :users, :steam_name, :string
    add_column :users, :steam_avatar_url, :string

    add_index :users, :steam_id, unique: true
  end
end
