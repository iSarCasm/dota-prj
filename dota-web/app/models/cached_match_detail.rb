# frozen_string_literal: true

class CachedMatchDetail < ApplicationRecord
  validates :match_id, presence: true, uniqueness: true
  validates :payload, presence: true
end
