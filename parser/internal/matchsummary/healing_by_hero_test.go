package matchsummary

import (
	"reflect"
	"testing"
)

func TestHealingByHeroTableSort(t *testing.T) {
	h := &Handler{
		healingByHero: map[string]uint32{
			"npc_dota_hero_lich":               500,
			"npc_dota_hero_dark_seer":          1000,
			"npc_dota_hero_ancient_apparition": 1000,
		},
	}
	got := h.healingByHeroTable()
	want := []HeroHealingRow{
		{Hero: "npc_dota_hero_ancient_apparition", Healing: 1000},
		{Hero: "npc_dota_hero_dark_seer", Healing: 1000},
		{Hero: "npc_dota_hero_lich", Healing: 500},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("healingByHeroTable() = %+v, want %+v", got, want)
	}
}

func TestHealingByHeroTableEmpty(t *testing.T) {
	h := &Handler{}
	if got := h.healingByHeroTable(); got != nil {
		t.Fatalf("healingByHeroTable() = %+v, want nil", got)
	}
}
