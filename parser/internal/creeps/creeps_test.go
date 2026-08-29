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
		{"lane goodguys siege", "npc_dota_goodguys_siege", "lane"},
		{"lane badguys siege", "npc_dota_badguys_siege", "lane"},
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

func TestKindFromAttackRange(t *testing.T) {
	tests := []struct {
		ar   int32
		want string
	}{
		{AttackRangeMelee, KindMelee},
		{AttackRangeRanged, KindRanged},
		{AttackRangeSiege, KindSiege},
		{0, ""},
		{250, ""},
	}
	for _, tt := range tests {
		if got := KindFromAttackRange(tt.ar); got != tt.want {
			t.Fatalf("KindFromAttackRange(%d) = %q, want %q", tt.ar, got, tt.want)
		}
	}
}

func TestKindFromTargetName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"npc_dota_creep_goodguys_melee", KindMelee},
		{"npc_dota_creep_badguys_flagbearer", KindFlagbearer},
		{"npc_dota_creep_badguys_ranged", KindRanged},
		{"npc_dota_creep_goodguys_siege", KindSiege},
		{"npc_dota_goodguys_siege", KindSiege},
		{"npc_dota_badguys_siege", KindSiege},
		{"npc_dota_hero_phantom_assassin", ""},
	}
	for _, tt := range tests {
		if got := KindFromTargetName(tt.name); got != tt.want {
			t.Fatalf("KindFromTargetName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestGetCreepSide_PathcornerMapCorner(t *testing.T) {
	// Spawn geography (not name suffix): SW/bottom-left → good, NE/top-right → bad.
	tests := []struct {
		name string
		want string
	}{
		{"lane_mid_pathcorner_badguys_7", "good"}, // SW; suffix says badguys
		{"lane_mid_pathcorner_goodguys_1", "bad"}, // NE; suffix says goodguys
		{"lane_mid_pathcorner_goodguys_3", "bad"},
		{"lane_bot_pathcorner_goodguys_2", "good"},
		{"lane_bot_pathcorner_badguys_3", "bad"}, // NE outlier
		{"lane_top_pathcorner_badguys_4", "good"},
		{"npc_dota_creep_badguys_ranged", "bad"}, // combat-log fallback
		{"npc_dota_creep_goodguys_melee", "good"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetCreepSide(tt.name); got != tt.want {
				t.Fatalf("GetCreepSide(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestGetCreepLaneFromSpawnLocation(t *testing.T) {
	// Centroids / nearby unique spawn slots from spawn-lane-clusters proof.
	tests := []struct {
		name string
		x, y float32
		want string
		side string
	}{
		{"good top slot", -6656, -4096, "top", "good"},
		{"good mid slot", -5120, -4608, "mid", "good"},
		{"good bot slot", -3840, -6144, "bot", "good"},
		{"bad top slot", 3072, 5632, "top", "bad"},
		{"bad mid slot", 4096, 3584, "mid", "bad"},
		{"bad bot slot", 6144, 3584, "bot", "bad"},
		{"good top centroid", -6720.7, -4100.7, "top", "good"},
		{"bad mid centroid", 4001.4, 3495.1, "mid", "bad"},
		// Mid-path / default cells — not spawn clusters (see lasthits ERROR logs).
		{"default cells", -768, -768, "", ""},
		{"nw mid-path", -6400, 3840, "", ""},
		{"se mid-path", 5632, -5376, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetCreepSideFromSpawnLocation(tt.x, tt.y); got != tt.side {
				t.Fatalf("side(%v,%v) = %q, want %q", tt.x, tt.y, got, tt.side)
			}
			if got := GetCreepLaneFromSpawnLocation(tt.x, tt.y); got != tt.want {
				t.Fatalf("lane(%v,%v) = %q, want %q", tt.x, tt.y, got, tt.want)
			}
		})
	}
}
