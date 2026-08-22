package lasthits

import "testing"

func TestLoadLasthitInflictors_firstFieldOnly(t *testing.T) {
	raw := "# comment\n\nslark_dark_pact\t5040\t5\n"
	got := loadLasthitInflictors(raw)
	if len(got) != 1 || got[0] != "slark_dark_pact" {
		t.Fatalf("loadLasthitInflictors() = %v, want [slark_dark_pact]", got)
	}
}

func TestIsLasthitInflictorDamage(t *testing.T) {
	list := []string{"slark_dark_pact", "zuus_arc_lightning"}
	lookup := func(idx uint32) (string, bool) {
		switch idx {
		case 1:
			return "dota_unknown", true
		case 2:
			return "slark_dark_pact", true
		case 3:
			return "nevermore_shadowraze1", true
		default:
			return "", false
		}
	}
	if !isLasthitInflictorDamage(0, list, lookup) {
		t.Fatal("zero inflictor should count")
	}
	if !isLasthitInflictorDamage(1, list, lookup) {
		t.Fatal("dota_unknown should count")
	}
	if !isLasthitInflictorDamage(2, list, lookup) {
		t.Fatal("listed spell should count")
	}
	if isLasthitInflictorDamage(3, list, lookup) {
		t.Fatal("unlisted spell should not count")
	}
}
