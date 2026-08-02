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
	events := []pendingCLogCreepEvent{
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
	h.pendingHeroKills = []pendingCLogCreepEvent{
		{creepName: testMeleeName, gameTime: 246.0},
	}
	if !h.hasPendingLasthit(testMeleeName, 246.1) {
		t.Fatal("expected self kill in window")
	}
	if h.hasPendingLasthit(testMeleeName, 250.0) {
		t.Fatal("self kill outside window should not match")
	}
	if h.hasPendingLasthit(testRangedName, 246.1) {
		t.Fatal("wrong creep name should not match")
	}
}

func TestHeroDamageCorrelates(t *testing.T) {
	pd := pendingCLogCreepEvent{health: 137, damage: 59}

	tests := []struct {
		name           string
		prev, health   int32
		healthReduced  bool
		wantCorrelates bool
	}{
		{"exact delta", 137 + 59, 137, true, true},
		{"wrong prev", 200, 137, true, false},
		{"same post health no drop", 137, 137, false, false},
		{"collision creep already at 137", 137, 137, false, false},
		{"wrong post health", 137 + 59, 136, true, false},
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
	const idx int32 = 42
	h.pendingHeroDamage = []pendingCLogCreepEvent{
		{id: 1, creepName: testMeleeName, gameTime: 100, health: 50, damage: 30},
	}
	track := &creepTrack{prevHealth: 80, hasHealth: true}
	h.creepTracks[idx] = track

	h.correlateHeroDamage(idx, track, 50, true)
	h.closePendingHeroDamageBefore(100.1)

	if track.creepName != testMeleeName {
		t.Fatalf("creepName = %q, want %q", track.creepName, testMeleeName)
	}
	if track.heroDamagedAt != 100 {
		t.Fatalf("heroDamagedAt = %v, want 100", track.heroDamagedAt)
	}
	if track.conflictGroupID != 0 {
		t.Fatalf("conflictGroupID = %d, want 0 for unique match", track.conflictGroupID)
	}
	if !h.pendingHeroDamage[0].consumed {
		t.Fatal("pending hero damage should be consumed")
	}
}

func TestMissedLastHit_EnemyStealsAfterHeroDamage(t *testing.T) {
	h := testHandler()
	const idx int32 = 42

	h.pendingHeroDamage = []pendingCLogCreepEvent{
		{creepName: testMeleeName, gameTime: 100, health: 50, damage: 30},
	}
	seedCreep(h, idx, 80, 99.9)
	h.onCreepHealthUpdate(idx, 50, manta.EntityOpUpdated, 100.0)

	h.pendingOtherDeath = []pendingCLogCreepEvent{
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

func TestLastHit_LasthitClearsMiss(t *testing.T) {
	h := testHandler()
	const idx int32 = 42

	h.pendingHeroDamage = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: 246, health: 120, damage: 40},
	}
	seedCreep(h, idx, 160, 245.9)
	h.onCreepHealthUpdate(idx, 120, manta.EntityOpUpdated, 246.0)

	h.pendingHeroKills = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: 246.1},
	}
	h.onCreepHealthUpdate(idx, 0, manta.EntityOpUpdated, 246.1)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents len = %d, want 0 after lasthit", len(h.missedEvents))
	}
}

// Regression for Warlock ~4:06 LH + Ember ~4:07 kill on another ranged creep.
// Hero damage must stay bound to the entity idx that actually received the hit.
// Entity death can be processed before the combat-log hero-kill line arrives.
func TestTwoRangedCreepsSamePostHealth_NoFalseMissWhenHeroGetsLH(t *testing.T) {
	h := testHandler()
	const (
		creepA int32 = 100
		creepB int32 = 101
	)

	h.pendingHeroDamage = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: 245.8, health: 120, damage: 40},
	}

	// Both creeps share post-damage HP; B updates first and steals correlation.
	seedCreep(h, creepA, 160, 245.8)
	seedCreep(h, creepB, 160, 245.7)
	h.onCreepHealthUpdate(creepB, 120, manta.EntityOpUpdated, 245.8)
	h.onCreepHealthUpdate(creepA, 120, manta.EntityOpUpdated, 245.9)

	// Warlock last-hits creep A (entity); combat-log kill confirms hero LH in group.
	h.pendingHeroKills = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: 246.0},
	}
	h.onCreepHealthUpdate(creepA, 0, manta.EntityOpUpdated, 246.0)

	// Ember kills creep B — must not count as Warlock miss.
	h.pendingOtherDeath = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: 247.0},
	}
	h.onCreepHealthUpdate(creepB, 0, manta.EntityOpUpdated, 247.0)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents = %+v, want none (health collision false positive)", h.missedEvents)
	}
}

// Two hero DAMAGE combat-log lines on the same tick, but three creeps could match
// the shared post-health signature. Only two should bind (one per damage line);
// the third must stay uncorrelated so an enemy kill there is not a false miss.
func TestTwoSameTickHeroDamage_ThreeMatchingCreeps_NoFalseMiss(t *testing.T) {
	h := testHandler()
	const (
		creepA int32 = 10 // hero actually damaged
		creepB int32 = 11 // not damaged — must not steal a pending line
		creepC int32 = 12 // hero actually damaged
	)
	const tick = float32(100.0)

	h.pendingHeroDamage = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: tick, health: 120, damage: 40},
		{creepName: testRangedName, gameTime: tick, health: 120, damage: 40},
	}

	// All three creeps share 160→120; update order B, A, C.
	seedCreep(h, creepA, 160, tick-0.1)
	seedCreep(h, creepB, 160, tick-0.1)
	seedCreep(h, creepC, 160, tick-0.1)
	h.onCreepHealthUpdate(creepB, 120, manta.EntityOpUpdated, tick+0.03)
	h.onCreepHealthUpdate(creepA, 120, manta.EntityOpUpdated, tick+0.03)
	h.onCreepHealthUpdate(creepC, 120, manta.EntityOpUpdated, tick+0.1)

	groupID := h.creepTracks[creepA].conflictGroupID
	if groupID == 0 {
		t.Fatal("expected ambiguous match to create a conflict group")
	}
	if h.creepTracks[creepB].conflictGroupID != groupID || h.creepTracks[creepC].conflictGroupID != groupID {
		t.Fatal("all matching creeps should share the same conflict group")
	}
	if h.conflictGroups[groupID].remainingCombatLogsCount != 2 {
		t.Fatalf("remainingCombatLogsCount = %d, want 2", h.conflictGroups[groupID].remainingCombatLogsCount)
	}

	// Hero last-hits A and C.
	h.pendingHeroKills = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: tick + 0.5},
	}
	h.onCreepHealthUpdate(creepA, 0, manta.EntityOpUpdated, tick+0.5)
	h.pendingHeroKills = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: tick + 0.6},
	}
	h.onCreepHealthUpdate(creepC, 0, manta.EntityOpUpdated, tick+0.6)

	// Enemy kills B — must not count as hero miss.
	h.pendingOtherDeath = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: tick + 1.0},
	}
	h.onCreepHealthUpdate(creepB, 0, manta.EntityOpUpdated, tick+1.0)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents = %+v, want none (third creep stole pending or wrong idx bound)", h.missedEvents)
	}
}

// Wrong creep steals health correlation, dies first to an enemy, then hero last-hits
// the creep they actually damaged. Enemy kill must not emit a false missed CS.
func TestFalseCorrelatedCreepDiesFirst_ThenHeroLastHitsTrueCreep_NoFalseMiss(t *testing.T) {
	h := testHandler()
	const (
		creepA int32 = 100 // hero actually damaged
		creepB int32 = 101 // falsely correlated
	)

	h.pendingHeroDamage = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: 245.8, health: 120, damage: 40},
	}

	seedCreep(h, creepA, 160, 245.7)
	seedCreep(h, creepB, 160, 245.7)
	h.onCreepHealthUpdate(creepB, 120, manta.EntityOpUpdated, 245.8)
	h.onCreepHealthUpdate(creepA, 120, manta.EntityOpUpdated, 245.9)

	// Enemy kills falsely correlated creep B first.
	h.pendingOtherDeath = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: 246.5},
	}
	h.onCreepHealthUpdate(creepB, 0, manta.EntityOpUpdated, 246.5)

	// Hero then last-hits the creep they actually damaged.
	h.pendingHeroKills = []pendingCLogCreepEvent{
		{creepName: testRangedName, gameTime: 247.0},
	}
	h.onCreepHealthUpdate(creepA, 0, manta.EntityOpUpdated, 247.0)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents = %+v, want none (false miss on B before hero LH on A)", h.missedEvents)
	}
}

func TestFlagbearerMiss_EntityBeforeCombatLogOnSameTick(t *testing.T) {
	// Replay 8915936762 ~2:45: manta processes entity updates before combat log per tick.
	const idx int32 = 2382
	const flagbearer = "npc_dota_creep_badguys_flagbearer"
	h := testHandler()

	// Tick 11315: entity drop first, then combat damage.
	seedCreep(h, idx, 196, 164.2)
	h.onCreepHealthUpdate(idx, 137, manta.EntityOpUpdated, 164.267)

	h.nextPendingDamageID++
	h.pendingHeroDamage = append(h.pendingHeroDamage, pendingCLogCreepEvent{
		id: 1, creepName: flagbearer, gameTime: 164.2, health: 137, damage: 59,
	})
	h.retroactiveCorrelateOpenPending()
	h.closePendingHeroDamageBefore(164.267)

	if h.creepTracks[idx].heroDamagedAt == 0 {
		t.Fatalf("track not bound after retroactive correlate: %+v", h.creepTracks[idx])
	}

	// Later damage updates (huskar).
	h.onCreepHealthUpdate(idx, 118, manta.EntityOpUpdated, 164.4)
	h.onCreepHealthUpdate(idx, 20, manta.EntityOpUpdated, 165.4)

	// Tick 11359: entity death first, then combat death.
	h.onCreepHealthUpdate(idx, 0, manta.EntityOpUpdated, 165.733)
	if !h.creepTracks[idx].awaitingDeathCombatLog {
		t.Fatal("expected awaitingDeathCombatLog before combat-log DEATH")
	}

	h.pendingOtherDeath = append(h.pendingOtherDeath, pendingCLogCreepEvent{
		creepName: flagbearer, gameTime: 165.667,
	})
	h.resolveAwaitingDeathCombatLog(flagbearer, 165.667)

	if len(h.missedEvents) != 1 {
		t.Fatalf("missedEvents = %+v, want 1 flagbearer miss", h.missedEvents)
	}
}

func TestMissedLastHit_OutsideWindowNotCounted(t *testing.T) {
	h := testHandler()
	const idx int32 = 42

	h.pendingHeroDamage = []pendingCLogCreepEvent{
		{creepName: testMeleeName, gameTime: 100, health: 50, damage: 30},
	}
	seedCreep(h, idx, 80, 99.9)
	h.onCreepHealthUpdate(idx, 50, manta.EntityOpUpdated, 100.0)

	h.pendingOtherDeath = []pendingCLogCreepEvent{
		{creepName: testMeleeName, gameTime: 103.0}, // >2s after damage
	}
	h.onCreepHealthUpdate(idx, 0, manta.EntityOpUpdated, 103.0)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents len = %d, want 0 outside 2s window", len(h.missedEvents))
	}
}
