# frozen_string_literal: true

require "json"
require_relative "opendota"
require_relative "replay"
require_relative "ranks"

module Heroes
  module_function

  def catalog
    OpenDota.get_heroes
  end

  def all_ids
    catalog.map { |h| h["id"].to_i }.sort
  end

  def id_to_name
    catalog.to_h { |h| [h["id"].to_i, h["localized_name"]] }
  end

  def resolve(query)
    return query.to_i if query.to_s.match?(/\A\d+\z/)

    needle = normalize(query)
    heroes = catalog
    match = heroes.find { |h| normalize(h["localized_name"]) == needle } ||
            heroes.find { |h| normalize(h["localized_name"]).include?(needle) } ||
            heroes.find { |h| normalize(h["name"].sub(/\Anpc_dota_hero_/, "")) == needle }

    raise ArgumentError, "unknown hero #{query.inspect}" unless match

    match["id"].to_i
  end

  def name_for(id)
    id_to_name[id.to_i] || id.to_s
  end

  # Heroes present in explored/**/*.json (and optionally requiring a valid .dem).
  def covered_in(explored_dir, require_dem: true)
    covered = {}
    Dir.glob(File.join(explored_dir, "*", "*", "*.json")).each do |json_path|
      dem_path = json_path.sub(/\.json\z/, ".dem")
      next if require_dem && !Replay.valid_dem?(dem_path)

      data = JSON.parse(File.read(json_path))
      mid = data.dig("public_match", "match_id") || data.dig("match", "match_id")
      heroes_in_match(data).each do |hid|
        covered[hid] ||= mid
      end
    end
    covered
  end

  def heroes_in_match(data)
    pm = data["public_match"] || {}
    heroes = Array(pm["radiant_team"]) + Array(pm["dire_team"])
    if heroes.empty? || heroes.any? { |h| h.to_i <= 0 }
      players = data.dig("match", "players") || []
      heroes = players.filter_map { |pl| pl["hero_id"] }
    end
    heroes.map(&:to_i).reject(&:zero?).uniq
  end

  def normalize(name)
    name.to_s.downcase.gsub(/[^a-z0-9]+/, "")
  end
  private_class_method :normalize
end
