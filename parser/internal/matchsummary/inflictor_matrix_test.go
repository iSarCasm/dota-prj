package matchsummary

import (
	"reflect"
	"testing"
)

func TestBuildInflictorMatrixSort(t *testing.T) {
	acc := inflictorHeroAccumulator{
		"auto_attack": {
			"npc_dota_hero_lich": 500,
		},
		"phantom_assassin_stifling_dagger": {
			"npc_dota_hero_dark_seer": 1000,
			"npc_dota_hero_lich":      200,
		},
	}
	got := buildInflictorMatrix(acc)
	want := &InflictorMatrix{
		Heroes: []string{"npc_dota_hero_dark_seer", "npc_dota_hero_lich"},
		Rows: []InflictorMatrixRow{
			{Inflictor: "phantom_assassin_stifling_dagger", Values: []uint32{1000, 200}},
			{Inflictor: "auto_attack", Values: []uint32{0, 500}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildInflictorMatrix() = %+v, want %+v", got, want)
	}
}

func TestBuildInflictorMatrixEmpty(t *testing.T) {
	if got := buildInflictorMatrix(nil); got != nil {
		t.Fatalf("buildInflictorMatrix(nil) = %+v, want nil", got)
	}
}
