# frozen_string_literal: true

class CreateWaitlistSignups < ActiveRecord::Migration[8.1]
  def change
    create_table :waitlist_signups do |t|
      t.string :email, null: false

      t.timestamps
    end

    add_index :waitlist_signups, :email, unique: true
  end
end
