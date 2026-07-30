package lasthits

import (
	"testing"

	"github.com/dotabuff/manta"

	"dota2/internal/timeandpauses"
)

const (
	testCreepClass = "CDOTA_BaseNPC_Creep_Lane"
	testRangedName = "npc_dota_creep_badguys_ranged"
	testMeleeName  = "npc_dota_creep_badguys_melee"
)

func testHandler() *Handler {
	h := NewHandler(timeandpauses.NewHandler())
	h.heroClass = "CDOTA_Unit_Hero_Warlock"
	return h
}

// seedCreep establishes baseline health before a correlated damage drop.
func seedCreep(h *Handler, idx int32, health int32, gameTime float32) {
	h.onCreepHealthUpdate(idx, health, manta.EntityOpCreatedEntered, gameTime)
}

func TestCreepTypeFromTargetName(t *testing.T) {
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
			if got := creepTypeFromTargetName(tt.target); got != tt.want {
				t.Fatalf("creepTypeFromTargetName(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestIsCreepEntityClass(t *testing.T) {
	if !isCreepEntityClass("CDOTA_BaseNPC_Creep_Lane") {
		t.Fatal("expected lane creep class")
	}
	if isCreepEntityClass("CDOTA_Unit_Hero_Warlock") {
		t.Fatal("hero should not be creep class")
	}
}

func TestPrunePendingByTime(t *testing.T) {
	events := []pendingCreepEvent{
		{gameTime: 10, creepName: "a"},
		{gameTime: 20, creepName: "b"},
		{gameTime: 30, creepName: "c", consumed: true},
	}
	got := prunePendingByTime(events, 15)
	if len(got) != 1 || got[0].creepName != "b" {
		t.Fatalf("prunePendingByTime() = %+v, want only b", got)
	}
}

func TestHasPendingSelfKill(t *testing.T) {
	h := testHandler()
	h.pendingSelfKill = []pendingCreepEvent{
		{creepName: testMeleeName, gameTime: 246.0},
	}
	if !h.hasPendingSelfKill(testMeleeName, 246.1) {
		t.Fatal("expected self kill in window")
	}
	if h.hasPendingSelfKill(testMeleeName, 250.0) {
		t.Fatal("self kill outside window should not match")
	}
	if h.hasPendingSelfKill(testRangedName, 246.1) {
		t.Fatal("wrong creep name should not match")
	}
}

func TestHeroDamageCorrelates(t *testing.T) {
	pd := pendingCreepEvent{health: 137, damage: 59}

	tests := []struct {
		name           string
		prev, health   int32
		healthReduced  bool
		wantCorrelates bool
	}{
		{"exact delta", 196, 137, true, true},
		{"wrong prev", 200, 137, true, false},
		{"same post health no drop", 137, 137, false, false},
		{"collision creep already at 137", 137, 137, false, false},
		{"wrong post health", 196, 136, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := heroDamageCorrelates(pd, tt.prev, tt.health, tt.healthReduced)
			if got != tt.wantCorrelates {
				t.Fatalf("heroDamageCorrelates() = %v, want %v", got, tt.wantCorrelates)
			}
		})
	}
}

func TestCorrelateHeroDamage_BindsFirstMatchingPending(t *testing.T) {
	h := testHandler()
	h.pendingHeroDamage = []pendingCreepEvent{
		{creepName: testMeleeName, gameTime: 100, health: 50, damage: 30},
	}
	track := &creepTrack{prevHealth: 80, hasHealth: true}

	h.correlateHeroDamage(track, 50, true)

	if track.creepName != testMeleeName {
		t.Fatalf("creepName = %q, want %q", track.creepName, testMeleeName)
	}
	if track.heroDamagedAt != 100 {
		t.Fatalf("heroDamagedAt = %v, want 100", track.heroDamagedAt)
	}
	if !h.pendingHeroDamage[0].consumed {
		t.Fatal("pending hero damage should be consumed")
	}
}

func TestMissedLastHit_EnemyStealsAfterHeroDamage(t *testing.T) {
	h := testHandler()
	const idx int32 = 42

	h.pendingHeroDamage = []pendingCreepEvent{
		{creepName: testMeleeName, gameTime: 100, health: 50, damage: 30},
	}
	seedCreep(h, idx, 80, 99.9)
	h.onCreepHealthUpdate(idx, 50, manta.EntityOpUpdated, 100.0)

	h.pendingOtherDeath = []pendingCreepEvent{
		{creepName: testMeleeName, gameTime: 100.5},
	}
	h.onCreepHealthUpdate(idx, 0, manta.EntityOpUpdated, 100.5)

	if len(h.missedEvents) != 1 {
		t.Fatalf("missedEvents len = %d, want 1", len(h.missedEvents))
	}
	if h.missedEvents[0].Type != "missed_last_hit" {
		t.Fatalf("event type = %q, want missed_last_hit", h.missedEvents[0].Type)
	}
}

func TestLastHit_SelfKillClearsMiss(t *testing.T) {
	h := testHandler()
	const idx int32 = 42

	h.pendingHeroDamage = []pendingCreepEvent{
		{creepName: testRangedName, gameTime: 246, health: 120, damage: 40},
	}
	seedCreep(h, idx, 160, 245.9)
	h.onCreepHealthUpdate(idx, 120, manta.EntityOpUpdated, 246.0)

	h.pendingSelfKill = []pendingCreepEvent{
		{creepName: testRangedName, gameTime: 246.1},
	}
	h.onCreepHealthUpdate(idx, 0, manta.EntityOpUpdated, 246.1)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents len = %d, want 0 after self kill", len(h.missedEvents))
	}
}

// Regression for Warlock ~4:06 LH + Ember ~4:07 kill on another ranged creep.
// Hero damage must stay bound to the entity idx that actually received the hit.
// Entity death can be processed before the combat-log self-kill line arrives.
func TestTwoRangedCreepsSamePostHealth_NoFalseMissWhenHeroGetsLH(t *testing.T) {
	h := testHandler()
	const (
		creepA int32 = 100
		creepB int32 = 101
	)

	h.pendingHeroDamage = []pendingCreepEvent{
		{creepName: testRangedName, gameTime: 245.8, health: 120, damage: 40},
	}

	// Both creeps share post-damage HP; B updates first and steals correlation.
	seedCreep(h, creepB, 160, 245.7)
	h.onCreepHealthUpdate(creepB, 120, manta.EntityOpUpdated, 245.8)

	seedCreep(h, creepA, 160, 245.8)
	h.onCreepHealthUpdate(creepA, 120, manta.EntityOpUpdated, 245.9)

	// Warlock last-hits creep A (entity); combat-log self-kill does not clear B's track.
	h.onCreepHealthUpdate(creepA, 0, manta.EntityOpUpdated, 246.0)

	// Ember kills creep B — must not count as Warlock miss.
	h.pendingOtherDeath = []pendingCreepEvent{
		{creepName: testRangedName, gameTime: 247.0},
	}
	h.onCreepHealthUpdate(creepB, 0, manta.EntityOpUpdated, 247.0)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents = %+v, want none (health collision false positive)", h.missedEvents)
	}
}

func TestMissedLastHit_OutsideWindowNotCounted(t *testing.T) {
	h := testHandler()
	const idx int32 = 42

	h.pendingHeroDamage = []pendingCreepEvent{
		{creepName: testMeleeName, gameTime: 100, health: 50, damage: 30},
	}
	seedCreep(h, idx, 80, 99.9)
	h.onCreepHealthUpdate(idx, 50, manta.EntityOpUpdated, 100.0)

	h.pendingOtherDeath = []pendingCreepEvent{
		{creepName: testMeleeName, gameTime: 103.0}, // >2s after damage
	}
	h.onCreepHealthUpdate(idx, 0, manta.EntityOpUpdated, 103.0)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents len = %d, want 0 outside 2s window", len(h.missedEvents))
	}
}
