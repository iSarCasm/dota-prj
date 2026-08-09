package creeps

import "testing"

func TestTypeFromTargetName(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{"lane badguys melee", "npc_dota_creep_badguys_melee", "lane"},
		{"lane goodguys ranged", "npc_dota_creep_goodguys_ranged", "lane"},
		{"lane siege", "npc_dota_creep_siege", "lane"},
		{"jungle neutral", "npc_dota_neutral_kobold", "jungle"},
		{"hero not creep", "npc_dota_hero_warlock", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TypeFromTargetName(tt.target); got != tt.want {
				t.Fatalf("TypeFromTargetName(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestIsEntityClass(t *testing.T) {
	if !IsLaneCreep("CDOTA_BaseNPC_Creep_Lane") {
		t.Fatal("expected lane creep class")
	}
	if IsLaneCreep("CDOTA_Unit_Hero_Warlock") {
		t.Fatal("hero should not be creep class")
	}
}
