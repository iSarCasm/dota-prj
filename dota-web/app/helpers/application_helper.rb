module ApplicationHelper
  include DotaImagesHelper

  def hero_display_name_for_npc(npc_name)
    return npc_name if npc_name.blank?

    heroes = Rails.configuration.x.constants.heroes
    hero = heroes.values.find { |h| h["name"] == npc_name.to_s }
    hero&.dig("localized_name") || npc_name.to_s.delete_prefix("npc_dota_hero_").tr("_", " ").split.map(&:capitalize).join(" ")
  end

  def inflictor_display_name(inflictor)
    return "Auto Attack" if inflictor.to_s == "auto_attack"

    name = inflictor.to_s
    abilities = Rails.configuration.x.constants.abilities
    ability = abilities[name]
    return ability["dname"] if ability.is_a?(Hash) && ability["dname"].present?

    if name.start_with?("item_")
      item_key = name.delete_prefix("item_")
      item = Rails.configuration.x.constants.items[item_key]
      return item["dname"] if item.is_a?(Hash) && item["dname"].present?
    end

    name.tr("_", " ").split.map(&:capitalize).join(" ")
  end
end
