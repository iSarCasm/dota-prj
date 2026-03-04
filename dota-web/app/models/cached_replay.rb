# frozen_string_literal: true

class CachedReplay < ApplicationRecord
  validates :match_id, presence: true, uniqueness: true
end
