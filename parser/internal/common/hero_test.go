package common

import "testing"

func TestGetHeroClassName(t *testing.T) {
	tests := []struct {
		npc  string
		want string
	}{
		{"npc_dota_hero_warlock", "CDOTA_Unit_Hero_Warlock"},
		{"npc_dota_hero_phantom_assassin", "CDOTA_Unit_Hero_PhantomAssassin"},
		{"npc_dota_hero_keeper_of_the_light", "CDOTA_Unit_Hero_KeeperOfTheLight"},
		{"not_a_hero", ""},
	}
	for _, tt := range tests {
		t.Run(tt.npc, func(t *testing.T) {
			if got := GetHeroClassName(tt.npc); got != tt.want {
				t.Fatalf("GetHeroClassName(%q) = %q, want %q", tt.npc, got, tt.want)
			}
		})
	}
}

func TestHeroNameToClass_matchesGetHeroClassName(t *testing.T) {
	tests := []struct {
		heroName string
		npc      string
	}{
		{"Warlock", "npc_dota_hero_warlock"},
		{"Phantom Assassin", "npc_dota_hero_phantom_assassin"},
		{"Keeper of the Light", "npc_dota_hero_keeper_of_the_light"},
		{"Shadow Fiend", "npc_dota_hero_nevermore"},
		{"Zeus", "npc_dota_hero_zuus"},
		{"Zuus", "npc_dota_hero_zuus"},
		{"Obsidian Destroyer", "npc_dota_hero_obsidian_destroyer"},
		{"Outworld Destroyer", "npc_dota_hero_obsidian_destroyer"},
		{"Anti-Mage", "npc_dota_hero_antimage"},
		{"Antimage", "npc_dota_hero_antimage"},
	}
	for _, tt := range tests {
		t.Run(tt.heroName, func(t *testing.T) {
			fromName := HeroNameToClass(tt.heroName)
			fromNPC := GetHeroClassName(tt.npc)
			if fromName == "" || fromName != fromNPC {
				t.Fatalf("HeroNameToClass(%q) = %q, GetHeroClassName(%q) = %q",
					tt.heroName, fromName, tt.npc, fromNPC)
			}
		})
	}
}
