package matchsummary

import "testing"

func TestIsTowerDamageTarget(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"npc_dota_goodguys_tower1_mid", true},
		{"npc_dota_badguys_melee_rax_top", true},
		{"npc_dota_badguys_fort", true},
		{"npc_dota_goodguys_fillers", false},
		{"npc_dota_goodguys_healers", false},
	}
	for _, tc := range tests {
		if got := isTowerDamageTarget(tc.name); got != tc.want {
			t.Fatalf("isTowerDamageTarget(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
