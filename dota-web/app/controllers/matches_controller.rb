class MatchesController < ApplicationController
  before_action :authenticate_user!

  def index
    api = OpenDotaApi.new

    steam_id = steam64id_for(params[:playerid]) || current_user.steam_id

    matches =
      begin
        api.get_player_matches(account_id: steam_id)
      rescue StandardError => e
        Rails.logger.error("[MatchesController#index] OpenDotaApi error: #{e.class}: #{e.message}")
        flash.now[:alert] = "Could not load matches from OpenDota."
        []
      end

    @matches = matches.map do |m|
      hero = hero_for_id(m["hero_id"])
      {
        id: m["match_id"],
        hero_name: hero&.dig("localized_name") || "Unknown",
        hero_img: hero&.dig("img"),
        start_time: m["start_time"],
        duration_seconds: m["duration"],
        duration: format_duration(m["duration"]),
        result: match_result_label(radiant_win: m["radiant_win"], player_slot: m["player_slot"]),
        kda: format_kda(kills: m["kills"], deaths: m["deaths"], assists: m["assists"])
      }
    end
  end

  def show
    api = OpenDotaApi.new

    steam_id = steam64id_for(params[:playerid]) || current_user.steam_id

    @match_details =
      begin
        api.get_match_details(match_id: params[:id])
      rescue StandardError => e
        Rails.logger.error("[MatchesController#show] OpenDotaApi error: #{e.class}: #{e.message}")
        flash.now[:alert] = "Could not load match details from OpenDota."
        nil
      end

    return if @match_details.blank?

    my_account_id32 = api.steam64id_to_32id(steam64id: steam_id)
    players = Array(@match_details["players"])
    @me = players.find { |p| p["account_id"].to_i == my_account_id32.to_i }

    @radiant_players = players.select { |p| player_radiant?(p) }
    @dire_players = players.reject { |p| player_radiant?(p) }

    hero = hero_for_id(@me&.dig("hero_id"))
    @match_summary = {
      id: @match_details["match_id"] || params[:id].to_i,
      duration_seconds: @match_details["duration"],
      duration: format_duration(@match_details["duration"]),
      start_time: @match_details["start_time"],
      radiant_win: @match_details["radiant_win"],
      radiant_score: @match_details["radiant_score"],
      dire_score: @match_details["dire_score"],
      game_mode: mode_for_id(@match_details["game_mode"])&.dig("name"),
      lobby_type: lobby_for_id(@match_details["lobby_type"])&.dig("name"),
      region: region_for_id(@match_details["region"]),
      patch: patch_for_id(@match_details["patch"])&.dig("name"),
      hero_name: hero&.dig("localized_name"),
      hero_img: hero&.dig("img")
    }

    @me_summary =
      if @me.present?
        {
          kda: format_kda(kills: @me["kills"], deaths: @me["deaths"], assists: @me["assists"]),
          gpm: @me["gold_per_min"],
          xpm: @me["xp_per_min"],
          hero_damage: @me["hero_damage"],
          tower_damage: @me["tower_damage"],
          last_hits: @me["last_hits"],
          denies: @me["denies"],
          items: player_items(@me)
        }
      else
        {}
      end
  end

  private

  def constants
    Rails.configuration.x.constants
  end

  def hero_for_id(hero_id)
    return nil if hero_id.blank?
    constants.heroes[hero_id.to_s]
  end

  def mode_for_id(mode_id)
    return nil if mode_id.blank?
    constants.game_mode[mode_id.to_s]
  end

  def lobby_for_id(lobby_id)
    return nil if lobby_id.blank?
    constants.lobby_type[lobby_id.to_s]
  end

  def region_for_id(region_id)
    return nil if region_id.blank?
    constants.region[region_id.to_s]
  end

  def patch_for_id(patch_id)
    return nil if patch_id.blank?
    Array(constants.patch).find { |p| p["id"].to_i == patch_id.to_i }
  end

  def player_radiant?(player)
    return player["isRadiant"] if player.key?("isRadiant")
    player["player_slot"].to_i < 128
  end

  def match_result_label(radiant_win:, player_slot:)
    return "Unknown" if radiant_win.nil? || player_slot.nil?
    is_radiant = player_slot.to_i < 128
    won = (radiant_win == true && is_radiant) || (radiant_win == false && !is_radiant)
    won ? "Victory" : "Defeat"
  end

  def format_kda(kills:, deaths:, assists:)
    return nil if kills.nil? || deaths.nil? || assists.nil?
    "#{kills}/#{deaths}/#{assists}"
  end

  def format_duration(seconds)
    return nil if seconds.blank?
    secs = seconds.to_i
    minutes = secs / 60
    remaining = secs % 60
    format("%d:%02d", minutes, remaining)
  end

  def item_for_id(item_id)
    return nil if item_id.blank?

    key = constants.item_ids[item_id.to_s]
    return nil if key.blank?

    constants.items[key]
  end

  # Accepts either SteamID64 or Steam "account id" (32-bit).
  # Returns SteamID64 as an Integer, or nil if blank/invalid.
  def steam64id_for(value)
    return nil if value.blank?

    id = value.to_s.strip.to_i
    return nil if id <= 0

    steam64_base = 76_561_197_960_265_728
    id < steam64_base ? (steam64_base + id) : id
  end

  def player_items(player)
    item_ids = [
      player["item_0"],
      player["item_1"],
      player["item_2"],
      player["item_3"],
      player["item_4"],
      player["item_5"],
      player["backpack_0"],
      player["backpack_1"],
      player["backpack_2"],
      player["item_neutral"],
      player["item_neutral2"]
    ].compact

    item_ids.map do |id|
      item = item_for_id(id)
      next if item.blank?

      { id: id, name: item["dname"] || item["name"], img: item["img"] }
    end.compact
  end
end
