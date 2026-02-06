# frozen_string_literal: true

class DotaMatch < ApplicationRecord
  validates :match_id, presence: true
  validates :status, presence: true
end
