package matchsummary

import (
	"reflect"
	"testing"
)

func TestHeroDamageByHeroTableSort(t *testing.T) {
	h := &Handler{
		heroDamageByHero: map[string]uint32{
			"npc_dota_hero_lich":               500,
			"npc_dota_hero_dark_seer":          1000,
			"npc_dota_hero_ancient_apparition": 1000,
		},
	}
	got := h.heroDamageByHeroTable()
	want := []HeroDamageRow{
		{Hero: "npc_dota_hero_ancient_apparition", Damage: 1000},
		{Hero: "npc_dota_hero_dark_seer", Damage: 1000},
		{Hero: "npc_dota_hero_lich", Damage: 500},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("heroDamageByHeroTable() = %+v, want %+v", got, want)
	}
}

func TestHeroDamageByHeroTableEmpty(t *testing.T) {
	h := &Handler{}
	if got := h.heroDamageByHeroTable(); got != nil {
		t.Fatalf("heroDamageByHeroTable() = %+v, want nil", got)
	}
}
