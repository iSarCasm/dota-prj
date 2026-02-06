class MatchesController < ApplicationController
  before_action :authenticate_user!

  def index
    # Dummy data for matches
    @matches = [
      {
        id: 1234567890,
        hero: "Pudge",
        hero_matchup: "vs Anti-Mage",
        duration: "42:15",
        result: "Victory"
      },
      {
        id: 1234567891,
        hero: "Invoker",
        hero_matchup: "vs Shadow Fiend",
        duration: "38:22",
        result: "Defeat"
      },
      {
        id: 1234567892,
        hero: "Crystal Maiden",
        hero_matchup: "vs Pudge",
        duration: "35:10",
        result: "Victory"
      },
      {
        id: 1234567893,
        hero: "Juggernaut",
        hero_matchup: "vs Phantom Assassin",
        duration: "45:33",
        result: "Victory"
      },
      {
        id: 1234567894,
        hero: "Windranger",
        hero_matchup: "vs Tinker",
        duration: "32:08",
        result: "Defeat"
      }
    ]
  end

  def show
    # Dummy data for a single match
    @match = {
      id: params[:id].to_i,
      hero: "Pudge",
      hero_matchup: "vs Anti-Mage",
      duration: "42:15",
      result: "Victory",
      kda: "12/5/18",
      gpm: 485,
      xpm: 512,
      hero_damage: 18500,
      tower_damage: 3200,
      last_hits: 142
    }
  end
end
