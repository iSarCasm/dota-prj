package matchsummary

import (
	"reflect"
	"testing"
)

func TestHeroesKilledTableSort(t *testing.T) {
	h := &Handler{
		heroesKilled: map[string]int{
			"npc_dota_hero_lich":               2,
			"npc_dota_hero_dark_seer":          1,
			"npc_dota_hero_ancient_apparition": 1,
		},
	}
	got := h.heroesKilledTable()
	want := []HeroKillRow{
		{Hero: "npc_dota_hero_lich", Kills: 2},
		{Hero: "npc_dota_hero_ancient_apparition", Kills: 1},
		{Hero: "npc_dota_hero_dark_seer", Kills: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("heroesKilledTable() = %+v, want %+v", got, want)
	}
}

func TestHeroesKilledTableEmpty(t *testing.T) {
	h := &Handler{}
	if got := h.heroesKilledTable(); got != nil {
		t.Fatalf("heroesKilledTable() = %+v, want nil", got)
	}
}
