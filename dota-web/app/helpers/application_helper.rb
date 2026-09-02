module ApplicationHelper
  include DotaImagesHelper

  def hero_display_name_for_npc(npc_name)
    return npc_name if npc_name.blank?

    heroes = Rails.configuration.x.constants.heroes
    hero = heroes.values.find { |h| h["name"] == npc_name.to_s }
    hero&.dig("localized_name") || npc_name.to_s.delete_prefix("npc_dota_hero_").tr("_", " ").split.map(&:capitalize).join(" ")
  end
end
