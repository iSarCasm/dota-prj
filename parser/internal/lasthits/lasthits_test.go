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

func TestPrunePendingByTime(t *testing.T) {
	events := []pendingCLogCreepEvent{
		{gameTime: 10, creepName: "a"},
		{gameTime: 20, creepName: "b"},
		{gameTime: 30, creepName: "c", entityMatched: true},
	}
	got := prunePendingByTime(events, 15)
	if len(got) != 1 || got[0].creepName != "b" {
		t.Fatalf("prunePendingByTime() = %+v, want only b", got)
	}
}

func TestHasPendingSelfKill(t *testing.T) {
	h := testHandler()
	h.pendingHeroKillLogs = []pendingCLogCreepEvent{
		{creepName: testMeleeName, gameTime: 246.0},
	}
	if !h.hasPendingHeroKill(testMeleeName, 246.1) {
		t.Fatal("expected self kill in window")
	}
	if h.hasPendingHeroKill(testMeleeName, 250.0) {
		t.Fatal("self kill outside window should not match")
	}
	if h.hasPendingHeroKill(testRangedName, 246.1) {
		t.Fatal("wrong creep name should not match")
	}
}

func TestHeroDamageCorrelates(t *testing.T) {
	pd := pendingCLogCreepEvent{gameTime: 100, health: 137, damage: 59}

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
			got := heroDamageCorrelates(pd, tt.prev, tt.health, 100.0)
			if got != tt.wantCorrelates {
				t.Fatalf("heroDamageCorrelates() = %v, want %v", got, tt.wantCorrelates)
			}
		})
	}
}

func TestCorrelateHeroDamage_BindsFirstMatchingPending(t *testing.T) {
	h := testHandler()
	const idx int32 = 42
	heroDamageCreepCombatLog(h, testMeleeName, 100, 50, 30)
	track := &creepTrack{prevHealth: 80, hasPrevHealth: true}
	h.creepTracks[idx] = track

	h.correlateHeroDamage(idx, track, 50, 100.0)
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
	if !h.pendingHeroDamageLogs[0].entityMatched {
		t.Fatal("pending hero damage should be consumed")
	}
}

func TestMissedLastHit_EnemyStealsAfterHeroDamage(t *testing.T) {
	h := testHandler()
	const idx int32 = 42

	heroDamageCreepCombatLog(h, testMeleeName, 100, 50, 30)
	seedCreep(h, idx, 80, 99.9)
	h.onCreepHealthUpdate(idx, 50, manta.EntityOpUpdated, 100.0)

	enemyKillCreepCombatLog(h, testMeleeName, 100.5)
	creepDiedEntityUpdate(h, idx, 100.5)

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

	heroDamageCreepCombatLog(h, testRangedName, 246, 120, 40)
	seedCreep(h, idx, 160, 245.9)
	h.onCreepHealthUpdate(idx, 120, manta.EntityOpUpdated, 246.0)

	heroKillCreepCombatLog(h, testRangedName, 246.1)
	creepDiedEntityUpdate(h, idx, 246.1)

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

	heroDamageCreepCombatLog(h, testRangedName, 245.8, 120, 40)

	// Both creeps share post-damage HP; B updates first and steals correlation.
	seedCreep(h, creepA, 160, 245.8)
	seedCreep(h, creepB, 160, 245.7)
	h.onCreepHealthUpdate(creepB, 120, manta.EntityOpUpdated, 245.8)
	h.onCreepHealthUpdate(creepA, 120, manta.EntityOpUpdated, 245.9)

	// Warlock last-hits creep A (entity); combat-log kill confirms hero LH in group.
	heroKillCreepCombatLog(h, testRangedName, 246.0)
	creepDiedEntityUpdate(h, creepA, 246.0)

	// Ember kills creep B — must not count as Warlock miss.
	enemyKillCreepCombatLog(h, testRangedName, 247.0)
	creepDiedEntityUpdate(h, creepB, 247.0)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents = %+v, want none (health collision false positive)", h.missedEvents)
	}
}

func TestLasthits(t *testing.T) {
	t.Run("when 1 damage combat log", func(t *testing.T) {
		t.Run("with 3 matching creeps", func(t *testing.T) {
			t.Run("binds them into conflict group", func(t *testing.T) {
				h, creepA, creepB, creepC := setupThreeMatchingCreeps(t)

				h.onCreepHealthUpdate(creepB, 120, manta.EntityOpUpdated, oneDamageTick+0.03)
				h.onCreepHealthUpdate(creepA, 120, manta.EntityOpUpdated, oneDamageTick+0.03)

				if h.creepTracks[creepA].conflictGroupID != 0 || h.creepTracks[creepB].conflictGroupID != 0 {
					t.Fatal("expected pending to stay open collecting candidates before tick window closes")
				}
				if len(h.pendingHeroDamageLogs) != 1 || h.pendingHeroDamageLogs[0].entityMatched {
					t.Fatal("pending hero damage should stay open after 2 matching drops")
				}
				if len(h.pendingHeroDamageLogs[0].candidates) != 2 {
					t.Fatalf("candidates = %v, want 2 before 3rd drop", h.pendingHeroDamageLogs[0].candidates)
				}

				groupID := correlateThirdMatchingCreep(h, creepA, creepB, creepC)
				assertThreeCreepsInConflictGroup(t, h, creepA, creepB, creepC, groupID, 1)
			})

			t.Run("counts as a miss if none were killed by player", func(t *testing.T) {
				h, creepA, creepB, creepC := setupThreeMatchingCreeps(t)
				correlateThreeMatchingCreeps(h, creepA, creepB, creepC)

				enemyKillCreepCombatLog(h, testRangedName, oneDamageTick+0.5)
				enemyKillCreepCombatLog(h, testRangedName, oneDamageTick+0.6)
				enemyKillCreepCombatLog(h, testRangedName, oneDamageTick+0.7)

				creepDiedEntityUpdate(h, creepA, oneDamageTick+0.5)
				creepDiedEntityUpdate(h, creepB, oneDamageTick+0.6)
				creepDiedEntityUpdate(h, creepC, oneDamageTick+0.7)

				if len(h.missedEvents) != 1 {
					t.Fatalf("missedEvents = %+v, want 1", h.missedEvents)
				}
			})

			t.Run("does not count as a miss when hero gets one last hit and enemies take the rest on same tick", func(t *testing.T) {
				h, creepA, creepB, creepC := setupThreeMatchingCreeps(t)
				correlateThreeMatchingCreeps(h, creepA, creepB, creepC)

				heroKillCreepCombatLog(h, testRangedName, oneDamageTick)
				enemyKillCreepCombatLog(h, testRangedName, oneDamageTick)
				enemyKillCreepCombatLog(h, testRangedName, oneDamageTick)

				creepDiedEntityUpdate(h, creepA, oneDamageTick)
				creepDiedEntityUpdate(h, creepB, oneDamageTick)
				creepDiedEntityUpdate(h, creepC, oneDamageTick)

				if len(h.missedEvents) != 0 {
					t.Fatalf("missedEvents = %+v, want none (both damage slots fulfilled by hero kill)", h.missedEvents)
				}
			})
		})
	})

	t.Run("when 3 damage combat logs on same tick", func(t *testing.T) {
		t.Run("with 4 matching creeps", func(t *testing.T) {
			t.Run("counts as 1 miss when 2 hero kills and 2 enemy kills", func(t *testing.T) {
				h, creeps := setupFourMatchingCreepsThreeDamageLogs(t)
				creepA, creepB, creepC, creepD := creeps[0], creeps[1], creeps[2], creeps[3]
				groupID := correlateFourMatchingCreeps(h, creeps)
				if h.conflictGroups[groupID].remainingCombatLogsCount != 3 {
					t.Fatalf("remainingCombatLogsCount = %d, want 3", h.conflictGroups[groupID].remainingCombatLogsCount)
				}

				heroKillCreepCombatLog(h, testRangedName, threeDamageTick)
				heroKillCreepCombatLog(h, testRangedName, threeDamageTick)
				enemyKillCreepCombatLog(h, testRangedName, threeDamageTick)
				enemyKillCreepCombatLog(h, testRangedName, threeDamageTick)

				creepDiedEntityUpdate(h, creepA, threeDamageTick)
				creepDiedEntityUpdate(h, creepB, threeDamageTick)
				creepDiedEntityUpdate(h, creepC, threeDamageTick)
				creepDiedEntityUpdate(h, creepD, threeDamageTick)

				if len(h.missedEvents) != 1 {
					t.Fatalf("missedEvents = %+v, want 1 (one unfulfilled combat-log slot)", h.missedEvents)
				}
			})

			t.Run("does not count as miss when 3 hero kills and 1 enemy kill", func(t *testing.T) {
				h, creeps := setupFourMatchingCreepsThreeDamageLogs(t)
				creepA, creepB, creepC, creepD := creeps[0], creeps[1], creeps[2], creeps[3]
				correlateFourMatchingCreeps(h, creeps)

				heroKillCreepCombatLog(h, testRangedName, threeDamageTick)
				heroKillCreepCombatLog(h, testRangedName, threeDamageTick)
				heroKillCreepCombatLog(h, testRangedName, threeDamageTick)
				enemyKillCreepCombatLog(h, testRangedName, threeDamageTick)

				creepDiedEntityUpdate(h, creepA, threeDamageTick)
				creepDiedEntityUpdate(h, creepB, threeDamageTick)
				creepDiedEntityUpdate(h, creepC, threeDamageTick)
				creepDiedEntityUpdate(h, creepD, threeDamageTick)

				if len(h.missedEvents) != 0 {
					t.Fatalf("missedEvents = %+v, want none (all combat-log slots fulfilled)", h.missedEvents)
				}
			})
		})
	})

	t.Run("when hero last-hits and enemy kills another creep on same tick", func(t *testing.T) {
		const (
			creep1    int32 = 30 // hero damaged and last-hit
			creep2    int32 = 31 // enemy last-hit, same creep type
			tick            = float32(300.0)
			deathTick       = tick + 0.2
		)

		run := func(t *testing.T, creep1DiesFirst bool) {
			t.Helper()
			h := testHandler()

			// 1. combat log hero damage
			heroDamageCreepCombatLog(h, testRangedName, tick, 120, 40)
			seedCreep(h, creep1, 160, tick-0.1)
			seedCreep(h, creep2, 160, tick-0.1)

			// 2. creep 1 matched combat log
			h.onCreepHealthUpdate(creep1, 120, manta.EntityOpUpdated, tick+timeandpauses.TickDuration+0.001)
			if h.creepTracks[creep1].heroDamagedAt != tick {
				t.Fatalf("creep1 heroDamagedAt = %v, want %v", h.creepTracks[creep1].heroDamagedAt, tick)
			}

			// 3. same tick combat logs: hero kills creep 1, other hero kills creep 2
			heroKillCreepCombatLog(h, testRangedName, deathTick)
			enemyKillCreepCombatLog(h, testRangedName, deathTick)

			// 4–5. entity deaths (order varies in replay)
			if creep1DiesFirst {
				creepDiedEntityUpdate(h, creep1, deathTick)
				creepDiedEntityUpdate(h, creep2, deathTick)
			} else {
				creepDiedEntityUpdate(h, creep2, deathTick)
				creepDiedEntityUpdate(h, creep1, deathTick)
			}

			if len(h.missedEvents) != 0 {
				t.Fatalf("missedEvents = %+v, want none (hero got LH; enemy kill was on other creep)", h.missedEvents)
			}
		}

		t.Run("creep1 dies first", func(t *testing.T) { run(t, true) })
		t.Run("creep2 dies first", func(t *testing.T) { run(t, false) })
	})

	t.Run("when 2 damage combat logs on same tick", func(t *testing.T) {
		t.Run("with 3 matching creeps and a fourth unrelated creep", func(t *testing.T) {
			t.Run("does not count as miss when 3 hero kills and 1 enemy kill on same tick", func(t *testing.T) {
				h, creepA, creepB, creepC, creepD := setupTwoDamageThreeMatchingCreepsAndD(t)
				groupID := correlateTwoDamageThreeMatchingCreeps(h, creepA, creepB, creepC)
				if h.conflictGroups[groupID].remainingCombatLogsCount != 2 {
					t.Fatalf("remainingCombatLogsCount = %d, want 2", h.conflictGroups[groupID].remainingCombatLogsCount)
				}
				if h.creepTracks[creepD].conflictGroupID != 0 {
					t.Fatal("creep d must not join the conflict group")
				}

				const deathTick = twoDamageTick + 0.5

				// Combat logs first (manta: CL before entity updates on same tick).
				heroKillCreepCombatLog(h, testRangedName, deathTick)
				heroKillCreepCombatLog(h, testRangedName, deathTick)
				heroKillCreepCombatLog(h, testRangedName, deathTick)
				enemyKillCreepCombatLog(h, testRangedName, deathTick)

				creepDiedEntityUpdate(h, creepA, deathTick)
				creepDiedEntityUpdate(h, creepB, deathTick)
				creepDiedEntityUpdate(h, creepC, deathTick)
				creepDiedEntityUpdate(h, creepD, deathTick)

				if len(h.missedEvents) != 0 {
					t.Fatalf("missedEvents = %+v, want none (both damage slots fulfilled by hero kills)", h.missedEvents)
				}
			})

			t.Run("does not count as a miss when 2 hero kills and 2 enemy kills on same tick", func(t *testing.T) {
				h, creepA, creepB, creepC, creepD := setupTwoDamageThreeMatchingCreepsAndD(t)
				correlateTwoDamageThreeMatchingCreeps(h, creepA, creepB, creepC)

				const deathTick = twoDamageTick + 0.5

				heroKillCreepCombatLog(h, testRangedName, deathTick)
				heroKillCreepCombatLog(h, testRangedName, deathTick)
				enemyKillCreepCombatLog(h, testRangedName, deathTick)
				enemyKillCreepCombatLog(h, testRangedName, deathTick)

				creepDiedEntityUpdate(h, creepA, deathTick)
				creepDiedEntityUpdate(h, creepD, deathTick)
				creepDiedEntityUpdate(h, creepB, deathTick)
				creepDiedEntityUpdate(h, creepC, deathTick)

				if len(h.missedEvents) != 0 {
					t.Fatalf("missedEvents = %+v, want none (both damage slots fulfilled by hero kills)", h.missedEvents)
				}
			})
		})
	})
}

const oneDamageTick = float32(100.0)
const twoDamageTick = float32(150.0)
const threeDamageTick = float32(200.0)

func setupTwoDamageThreeMatchingCreepsAndD(t *testing.T) (*Handler, int32, int32, int32, int32) {
	t.Helper()
	const (
		creepA int32 = 40
		creepB int32 = 41
		creepC int32 = 42
		creepD int32 = 43
	)
	h := testHandler()
	heroDamageCreepCombatLog(h, testRangedName, twoDamageTick, 120, 40)
	heroDamageCreepCombatLog(h, testRangedName, twoDamageTick, 120, 40)
	seedCreep(h, creepA, 160, twoDamageTick-0.1)
	seedCreep(h, creepB, 160, twoDamageTick-0.1)
	seedCreep(h, creepC, 160, twoDamageTick-0.1)
	seedCreep(h, creepD, 500, twoDamageTick) // unrelated; never matches hero damage
	return h, creepA, creepB, creepC, creepD
}

func correlateTwoDamageThreeMatchingCreeps(h *Handler, creepA, creepB, creepC int32) uint64 {
	h.onCreepHealthUpdate(creepB, 120, manta.EntityOpUpdated, twoDamageTick+0.03)
	h.onCreepHealthUpdate(creepA, 120, manta.EntityOpUpdated, twoDamageTick+0.03)
	h.onCreepHealthUpdate(creepC, 120, manta.EntityOpUpdated, twoDamageTick+0.1)
	return h.creepTracks[creepA].conflictGroupID
}

func setupFourMatchingCreepsThreeDamageLogs(t *testing.T) (*Handler, [4]int32) {
	t.Helper()
	creeps := [4]int32{20, 21, 22, 23}
	h := testHandler()
	heroDamageCreepCombatLog(h, testRangedName, threeDamageTick, 120, 40)
	heroDamageCreepCombatLog(h, testRangedName, threeDamageTick, 120, 40)
	heroDamageCreepCombatLog(h, testRangedName, threeDamageTick, 120, 40)
	for _, idx := range creeps {
		seedCreep(h, idx, 160, threeDamageTick-0.1)
	}
	return h, creeps
}

func correlateFourMatchingCreeps(h *Handler, creeps [4]int32) uint64 {
	for i, idx := range creeps {
		h.onCreepHealthUpdate(idx, 120, manta.EntityOpUpdated, threeDamageTick+0.02+float32(i)*0.01)
	}
	return h.creepTracks[creeps[0]].conflictGroupID
}

func setupThreeMatchingCreeps(t *testing.T) (*Handler, int32, int32, int32) {
	t.Helper()
	const (
		creepA int32 = 10
		creepB int32 = 11
		creepC int32 = 12
	)
	h := testHandler()
	heroDamageCreepCombatLog(h, testRangedName, oneDamageTick, 120, 40)
	seedCreep(h, creepA, 160, oneDamageTick-0.1)
	seedCreep(h, creepB, 160, oneDamageTick-0.1)
	seedCreep(h, creepC, 160, oneDamageTick-0.1)
	return h, creepA, creepB, creepC
}

func correlateThirdMatchingCreep(h *Handler, creepA, creepB, creepC int32) uint64 {
	h.onCreepHealthUpdate(creepC, 120, manta.EntityOpUpdated, oneDamageTick+0.1)
	return h.creepTracks[creepA].conflictGroupID
}

func correlateThreeMatchingCreeps(h *Handler, creepA, creepB, creepC int32) uint64 {
	h.onCreepHealthUpdate(creepB, 120, manta.EntityOpUpdated, oneDamageTick+0.03)
	h.onCreepHealthUpdate(creepA, 120, manta.EntityOpUpdated, oneDamageTick+0.03)
	return correlateThirdMatchingCreep(h, creepA, creepB, creepC)
}

func assertThreeCreepsInConflictGroup(t *testing.T, h *Handler, creepA, creepB, creepC int32, groupID uint64, wantRemaining int) {
	t.Helper()
	if groupID == 0 {
		t.Fatal("expected conflict group")
	}
	if h.creepTracks[creepB].conflictGroupID != groupID || h.creepTracks[creepC].conflictGroupID != groupID {
		t.Fatal("all three creeps should share the same conflict group")
	}
	if h.conflictGroups[groupID].remainingCombatLogsCount != wantRemaining {
		t.Fatalf("remainingCombatLogsCount = %d, want %d", h.conflictGroups[groupID].remainingCombatLogsCount, wantRemaining)
	}
}

func heroDamageCreepCombatLog(h *Handler, creepName string, gameTime float32, health, damage int32) {
	h.pendingHeroDamageLogs = append(h.pendingHeroDamageLogs, pendingCLogCreepEvent{
		id:        h.GetNextUniqueId(),
		creepName: creepName,
		gameTime:  gameTime,
		health:    health,
		damage:    damage,
	})
}

func heroKillCreepCombatLog(h *Handler, creepName string, gameTime float32) {
	h.pendingHeroKillLogs = append(h.pendingHeroKillLogs, pendingCLogCreepEvent{
		creepName: creepName, gameTime: gameTime,
	})
}

func enemyKillCreepCombatLog(h *Handler, creepName string, gameTime float32) {
	h.pendingOtherKillLogs = append(h.pendingOtherKillLogs, pendingCLogCreepEvent{
		creepName: creepName, gameTime: gameTime,
	})
}

func creepDiedEntityUpdate(h *Handler, idx int32, gameTime float32) {
	h.onCreepHealthUpdate(idx, 0, manta.EntityOpUpdated, gameTime)
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

	heroDamageCreepCombatLog(h, testRangedName, tick, 120, 40)
	heroDamageCreepCombatLog(h, testRangedName, tick, 120, 40)

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
	heroKillCreepCombatLog(h, testRangedName, tick+0.5)
	creepDiedEntityUpdate(h, creepA, tick+0.5)
	heroKillCreepCombatLog(h, testRangedName, tick+0.6)
	creepDiedEntityUpdate(h, creepC, tick+0.6)

	// Enemy kills B — must not count as hero miss.
	enemyKillCreepCombatLog(h, testRangedName, tick+1.0)
	creepDiedEntityUpdate(h, creepB, tick+1.0)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents = %+v, want none (third creep stole pending or wrong idx bound)", h.missedEvents)
	}
}

// Regression: any creep death used to close ALL pending hero-damage candidate search
// (closeAllPendingHeroDamage), finalizing every line with zero candidates before entity
// health drops arrived.
//
// Scenario:
//  1. Combat-log damage 1
//  2. Combat-log damage 2  (same signature, same tick)
//  3. Unrelated creep dies   (must not close CL1/CL2 while they still have no candidates)
//  4. Creep health drop 1
//  5. Creep health drop 2
//
// Want: both combat-log lines bound into one conflict group with both creeps.
// Bad:  both lines finalized with no candidates.
func TestUnrelatedCreepDeath_DoesNotPrematurelyFinalizeOtherPending(t *testing.T) {
	h := testHandler()
	const (
		creep1    int32 = 10
		creep2    int32 = 11
		unrelated int32 = 99
	)
	const tick = float32(100.0)

	heroDamageCreepCombatLog(h, testRangedName, tick, 120, 40)
	heroDamageCreepCombatLog(h, testRangedName, tick, 120, 40)

	seedCreep(h, creep1, 160, tick-0.1)
	seedCreep(h, creep2, 160, tick-0.1)
	seedCreep(h, unrelated, 500, tick)

	// Step 3: unrelated creep dies before any hero-damage health drops.
	creepDiedEntityUpdate(h, unrelated, tick+0.01)
	for _, pd := range h.pendingHeroDamageLogs {
		if pd.entityMatched {
			t.Fatal("unrelated creep death must not consume open hero-damage pending lines")
		}
	}

	// Steps 4–5: entity health drops for the two hero-damaged creeps.
	h.onCreepHealthUpdate(creep1, 120, manta.EntityOpUpdated, tick+0.02)
	h.onCreepHealthUpdate(creep2, 120, manta.EntityOpUpdated, tick+timeandpauses.TickDuration+0.001)

	groupID := h.creepTracks[creep1].conflictGroupID
	if groupID == 0 {
		t.Fatal("expected conflict group after both health drops correlated")
	}
	if h.creepTracks[creep2].conflictGroupID != groupID {
		t.Fatalf("creep2 conflictGroupID = %d, want %d", h.creepTracks[creep2].conflictGroupID, groupID)
	}
	if h.conflictGroups[groupID].remainingCombatLogsCount != 2 {
		t.Fatalf("remainingCombatLogsCount = %d, want 2 (one slot per combat-log line)", h.conflictGroups[groupID].remainingCombatLogsCount)
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

	heroDamageCreepCombatLog(h, testRangedName, 245.8, 120, 40)

	seedCreep(h, creepA, 160, 245.7)
	seedCreep(h, creepB, 160, 245.7)
	h.onCreepHealthUpdate(creepB, 120, manta.EntityOpUpdated, 245.8)
	h.onCreepHealthUpdate(creepA, 120, manta.EntityOpUpdated, 245.9)

	// Enemy kills falsely correlated creep B first.
	enemyKillCreepCombatLog(h, testRangedName, 246.5)
	creepDiedEntityUpdate(h, creepB, 246.5)

	// Hero then last-hits the creep they actually damaged.
	heroKillCreepCombatLog(h, testRangedName, 247.0)
	creepDiedEntityUpdate(h, creepA, 247.0)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents = %+v, want none (false miss on B before hero LH on A)", h.missedEvents)
	}
}

func TestFlagbearerMiss_SameTickCombatLogThenEntity(t *testing.T) {
	// Replay 8915936762 ~2:45 — manta runs combat log before entity updates on the same tick.
	const idx int32 = 2382
	const flagbearer = "npc_dota_creep_badguys_flagbearer"
	h := testHandler()

	// Tick 11315: combat damage queued, then entity drop (forward correlation).
	seedCreep(h, idx, 196, 164.2)
	heroDamageCreepCombatLog(h, flagbearer, 164.2, 137, 59)
	h.onCreepHealthUpdate(idx, 137, manta.EntityOpUpdated, 164.267)

	if h.creepTracks[idx].heroDamagedAt == 0 {
		t.Fatalf("track not bound after forward correlate: %+v", h.creepTracks[idx])
	}

	// Later damage updates (huskar).
	h.onCreepHealthUpdate(idx, 118, manta.EntityOpUpdated, 164.4)
	h.onCreepHealthUpdate(idx, 20, manta.EntityOpUpdated, 165.4)

	// Tick 11359: combat death queued, then entity death (manta order).
	enemyKillCreepCombatLog(h, flagbearer, 165.667)
	creepDiedEntityUpdate(h, idx, 165.733)

	if len(h.missedEvents) != 1 {
		t.Fatalf("missedEvents = %+v, want 1 flagbearer miss", h.missedEvents)
	}
}

func TestMissedLastHit_OutsideWindowNotCounted(t *testing.T) {
	h := testHandler()
	const idx int32 = 42

	heroDamageCreepCombatLog(h, testMeleeName, 100, 50, 30)
	seedCreep(h, idx, 80, 99.9)
	h.onCreepHealthUpdate(idx, 50, manta.EntityOpUpdated, 100.0)

	enemyKillCreepCombatLog(h, testMeleeName, 103.0) // >2s after damage
	creepDiedEntityUpdate(h, idx, 103.0)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents len = %d, want 0 outside 2s window", len(h.missedEvents))
	}
}
