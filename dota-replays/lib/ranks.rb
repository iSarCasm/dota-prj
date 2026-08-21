# frozen_string_literal: true

module Ranks
  # OpenDota rank_tier: tens = medal, ones = stars (1–5). Immortal is 80+.
  MEDALS = {
    "herald" => 10..15,
    "guardian" => 20..25,
    "crusader" => 30..35,
    "archon" => 40..45,
    "legend" => 50..55,
    "ancient" => 60..65,
    "divine" => 70..75,
    "immortal" => 80..85
  }.freeze

  module_function

  def names
    MEDALS.keys
  end

  def range_for(name)
    key = name.to_s.strip.downcase
    range = MEDALS[key]
    raise ArgumentError, "unknown rank #{name.inspect} (#{names.join(', ')})" unless range

    range
  end

  def name_for(rank_tier)
    return "unknown" if rank_tier.nil?

    tier = rank_tier.to_i
    MEDALS.each do |name, range|
      return name if range.cover?(tier) || (name == "immortal" && tier >= 80)
    end

    "unknown"
  end

  def min_max_for(name)
    range = range_for(name)
    [range.begin, range.end]
  end
end
