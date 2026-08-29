package lasthits

import (
	"testing"

	"github.com/dotabuff/manta"

	"dota2/internal/creeps"
	"dota2/internal/timeandpauses"
)

const (
	testCreepClass = "CDOTA_BaseNPC_Creep_Lane"
	testRangedName = "npc_dota_creep_badguys_ranged"
	testMeleeName  = "npc_dota_creep_badguys_melee"

	tickDamage1 uint32 = 100
	tickDamage2 uint32 = 150
	tickDamage3 uint32 = 200
	tickHeroLH  uint32 = 300
)

func testHandler() *Handler {
	h := NewHandler(timeandpauses.NewHandler())
	h.heroClass = "CDOTA_Unit_Hero_Warlock"
	return h
}

// flushTick runs the production end-of-combat-tick correlation (bind damages, resolve deaths).
func flushTick(h *Handler, tick uint32) {
	h.correlateLastTickDamages(tick, 0)
}

func seedCreep(h *Handler, idx int32, health int32, tick uint32, kind, side string) {
	h.onCreepHealthUpdate(idx, health, true, 0, 0, manta.EntityOpCreatedEntered, tick, 0, "", testCreepClass, kind)
	track := h.creepTracks[idx]
	track.prevHealth = health
	track.hasPrevHealth = true
	track.creepKind = kind
	track.side = side
}

func seedRanged(h *Handler, idx int32, health int32, tick uint32) {
	seedCreep(h, idx, health, tick, creeps.KindRanged, "bad")
}

func seedMelee(h *Handler, idx int32, health int32, tick uint32) {
	seedCreep(h, idx, health, tick, creeps.KindMelee, "bad")
}

func updateCreepHealth(h *Handler, idx int32, health int32, op manta.EntityOp, tick uint32) {
	track := h.creepTracks[idx]
	kind, side := "", ""
	if track != nil {
		kind, side = track.creepKind, track.side
	}
	h.onCreepHealthUpdate(idx, health, false, 0, 0, op, tick, 0, "", testCreepClass, kind)
	if track != nil {
		track.side = side
		track.creepKind = kind
	}
}

func heroDamageCreepCombatLog(h *Handler, creepName string, tick uint32, health, damage int32) {
	h.pendingHeroDamageLogs = append(h.pendingHeroDamageLogs, pendingCLogCreepEvent{
		id: h.GetNextUniqueId(), creepName: creepName, tick: tick, health: health, damage: damage,
	})
}

func heroKillCreepCombatLog(h *Handler, creepName string, tick uint32) {
	h.pendingHeroKillLogs = append(h.pendingHeroKillLogs, pendingCLogCreepEvent{
		creepName: creepName, tick: tick,
	})
}

func enemyKillCreepCombatLog(h *Handler, creepName string, tick uint32) {
	h.pendingOtherKillLogs = append(h.pendingOtherKillLogs, pendingCLogCreepEvent{
		creepName: creepName, tick: tick,
	})
}

func creepDiedEntityUpdate(h *Handler, idx int32, tick uint32) {
	updateCreepHealth(h, idx, 0, manta.EntityOpUpdated, tick)
}

func TestHasPendingHeroKill_SameTickOnly(t *testing.T) {
	h := testHandler()
	const killTick uint32 = 7380
	h.pendingHeroKillLogs = []pendingCLogCreepEvent{
		{creepName: testMeleeName, tick: killTick},
	}
	if !h.hasPendingHeroKill(testMeleeName, killTick) {
		t.Fatal("expected hero kill on same tick")
	}
	if h.hasPendingHeroKill(testMeleeName, killTick+3) {
		t.Fatal("hero kill on a different tick should not match")
	}
	if h.hasPendingHeroKill(testRangedName, killTick) {
		t.Fatal("wrong creep name should not match")
	}
}

func TestHeroDamageCorrelates(t *testing.T) {
	tests := []struct {
		name           string
		prev, health   int32
		pdTick         uint32
		entityTick     uint32
		wantCorrelates bool
	}{
		{"exact delta", 137 + 59, 137, 100, 100, true},
		{"wrong prev", 200, 137, 100, 100, false},
		{"same post health no drop", 137, 137, 100, 100, false},
		{"collision creep already at 137", 137, 137, 100, 100, false},
		{"wrong post health", 137 + 59, 136, 100, 100, false},
		{"same-tick death skip", 24 + 67, 0, 100, 100, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := pendingCLogCreepEvent{tick: tt.pdTick, health: 137, damage: 59}
			if tt.name == "same-tick death skip" {
				pd.health = 24
				pd.damage = 67
			}
			got := heroDamageCorrelates(pd, tt.prev, tt.health, tt.entityTick)
			if got != tt.wantCorrelates {
				t.Fatalf("heroDamageCorrelates() = %v, want %v", got, tt.wantCorrelates)
			}
		})
	}
}

func TestHeroDamageCorrelatesExactly(t *testing.T) {
	pd := pendingCLogCreepEvent{health: 50, damage: 30}
	if !heroDamageCorrelatesExactly(pd, 80, 50) {
		t.Fatal("exact match should succeed")
	}
	if heroDamageCorrelatesExactly(pd, 80, 0) {
		t.Fatal("death coalesce should not exact-match")
	}
}

func TestCorrelateLastTick_BindsExactHealthDrop(t *testing.T) {
	h := testHandler()
	const idx int32 = 42
	const tick uint32 = 100

	seedMelee(h, idx, 80, tick-1)
	heroDamageCreepCombatLog(h, testMeleeName, tick, 50, 30)
	updateCreepHealth(h, idx, 50, manta.EntityOpUpdated, tick)
	flushTick(h, tick)

	track := h.creepTracks[idx]
	if track.combatLogCreepname != testMeleeName {
		t.Fatalf("creepName = %q, want %q", track.combatLogCreepname, testMeleeName)
	}
	if track.heroDamagedTick != tick {
		t.Fatalf("heroDamagedTick = %v, want %v", track.heroDamagedTick, tick)
	}
	if track.conflictGroupID != 0 {
		t.Fatalf("conflictGroupID = %d, want 0 for unique match", track.conflictGroupID)
	}
}

func TestMissedLastHit_EnemyStealsAfterHeroDamage(t *testing.T) {
	h := testHandler()
	const idx int32 = 42
	const tick uint32 = 100

	seedMelee(h, idx, 80, tick-1)
	heroDamageCreepCombatLog(h, testMeleeName, tick, 50, 30)
	updateCreepHealth(h, idx, 50, manta.EntityOpUpdated, tick)
	flushTick(h, tick)

	enemyKillCreepCombatLog(h, testMeleeName, tick+15)
	creepDiedEntityUpdate(h, idx, tick+15)
	flushTick(h, tick+15)

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
	const tick uint32 = 246

	seedRanged(h, idx, 160, tick-1)
	heroDamageCreepCombatLog(h, testRangedName, tick, 120, 40)
	updateCreepHealth(h, idx, 120, manta.EntityOpUpdated, tick)
	flushTick(h, tick)

	heroKillCreepCombatLog(h, testRangedName, tick+3)
	creepDiedEntityUpdate(h, idx, tick+3)
	flushTick(h, tick+3)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents len = %d, want 0 after lasthit", len(h.missedEvents))
	}
}

func TestTwoRangedCreepsSamePostHealth_NoFalseMissWhenHeroGetsLH(t *testing.T) {
	h := testHandler()
	const (
		creepA     int32 = 100
		creepB     int32 = 101
		damageTick       = uint32(500)
	)

	seedRanged(h, creepA, 160, damageTick-1)
	seedRanged(h, creepB, 160, damageTick-1)
	heroDamageCreepCombatLog(h, testRangedName, damageTick, 120, 40)
	updateCreepHealth(h, creepB, 120, manta.EntityOpUpdated, damageTick)
	updateCreepHealth(h, creepA, 120, manta.EntityOpUpdated, damageTick)
	flushTick(h, damageTick)

	heroKillCreepCombatLog(h, testRangedName, damageTick+6)
	creepDiedEntityUpdate(h, creepA, damageTick+6)
	flushTick(h, damageTick+6)

	enemyKillCreepCombatLog(h, testRangedName, damageTick+36)
	creepDiedEntityUpdate(h, creepB, damageTick+36)
	flushTick(h, damageTick+36)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents = %+v, want none (health collision false positive)", h.missedEvents)
	}
}

func TestLasthits(t *testing.T) {
	t.Run("when 1 damage combat log", func(t *testing.T) {
		t.Run("with 3 matching creeps", func(t *testing.T) {
			t.Run("binds them into conflict group", func(t *testing.T) {
				h, creepA, creepB, creepC := setupThreeMatchingCreeps(t)
				groupID := correlateThreeMatchingCreeps(h, creepA, creepB, creepC)
				assertThreeCreepsInConflictGroup(t, h, creepA, creepB, creepC, groupID, 1)
			})

			t.Run("counts as a miss if none were killed by player", func(t *testing.T) {
				h, creepA, creepB, creepC := setupThreeMatchingCreeps(t)
				correlateThreeMatchingCreeps(h, creepA, creepB, creepC)

				enemyKillCreepCombatLog(h, testRangedName, tickDamage1+15)
				creepDiedEntityUpdate(h, creepA, tickDamage1+15)
				flushTick(h, tickDamage1+15)

				enemyKillCreepCombatLog(h, testRangedName, tickDamage1+18)
				creepDiedEntityUpdate(h, creepB, tickDamage1+18)
				flushTick(h, tickDamage1+18)

				enemyKillCreepCombatLog(h, testRangedName, tickDamage1+21)
				creepDiedEntityUpdate(h, creepC, tickDamage1+21)
				flushTick(h, tickDamage1+21)

				if len(h.missedEvents) != 1 {
					t.Fatalf("missedEvents = %+v, want 1", h.missedEvents)
				}
			})

			t.Run("does not count as a miss when hero gets one last hit and enemies take the rest on same tick", func(t *testing.T) {
				h, creepA, creepB, creepC := setupThreeMatchingCreeps(t)
				correlateThreeMatchingCreeps(h, creepA, creepB, creepC)

				heroKillCreepCombatLog(h, testRangedName, tickDamage1)
				enemyKillCreepCombatLog(h, testRangedName, tickDamage1)
				enemyKillCreepCombatLog(h, testRangedName, tickDamage1)

				creepDiedEntityUpdate(h, creepA, tickDamage1)
				creepDiedEntityUpdate(h, creepB, tickDamage1)
				creepDiedEntityUpdate(h, creepC, tickDamage1)
				flushTick(h, tickDamage1)

				if len(h.missedEvents) != 0 {
					t.Fatalf("missedEvents = %+v, want none (hero kill fulfills the damage slot)", h.missedEvents)
				}
			})
		})
	})

	t.Run("when 3 damage combat logs on same tick", func(t *testing.T) {
		t.Run("with 4 matching creeps", func(t *testing.T) {
			t.Run("counts as 1 miss when 2 hero kills and 2 enemy kills", func(t *testing.T) {
				h, creepsIdx := setupFourMatchingCreepsThreeDamageLogs(t)
				creepA, creepB, creepC, creepD := creepsIdx[0], creepsIdx[1], creepsIdx[2], creepsIdx[3]
				groupID := correlateFourMatchingCreeps(h, creepsIdx)
				if h.conflictGroups[groupID].remainingCombatLogsCount != 3 {
					t.Fatalf("remainingCombatLogsCount = %d, want 3", h.conflictGroups[groupID].remainingCombatLogsCount)
				}

				heroKillCreepCombatLog(h, testRangedName, tickDamage3)
				heroKillCreepCombatLog(h, testRangedName, tickDamage3)
				enemyKillCreepCombatLog(h, testRangedName, tickDamage3)
				enemyKillCreepCombatLog(h, testRangedName, tickDamage3)

				creepDiedEntityUpdate(h, creepA, tickDamage3)
				creepDiedEntityUpdate(h, creepB, tickDamage3)
				creepDiedEntityUpdate(h, creepC, tickDamage3)
				creepDiedEntityUpdate(h, creepD, tickDamage3)
				flushTick(h, tickDamage3)

				if len(h.missedEvents) != 2 {
					t.Fatalf("missedEvents = %+v, want 2 (one miss per enemy death while slots remain)", h.missedEvents)
				}
			})

			t.Run("does not count as miss when 3 hero kills and 1 enemy kill", func(t *testing.T) {
				h, creepsIdx := setupFourMatchingCreepsThreeDamageLogs(t)
				creepA, creepB, creepC, creepD := creepsIdx[0], creepsIdx[1], creepsIdx[2], creepsIdx[3]
				correlateFourMatchingCreeps(h, creepsIdx)

				heroKillCreepCombatLog(h, testRangedName, tickDamage3)
				heroKillCreepCombatLog(h, testRangedName, tickDamage3)
				heroKillCreepCombatLog(h, testRangedName, tickDamage3)
				enemyKillCreepCombatLog(h, testRangedName, tickDamage3)

				creepDiedEntityUpdate(h, creepA, tickDamage3)
				creepDiedEntityUpdate(h, creepB, tickDamage3)
				creepDiedEntityUpdate(h, creepC, tickDamage3)
				creepDiedEntityUpdate(h, creepD, tickDamage3)
				flushTick(h, tickDamage3)

				if len(h.missedEvents) != 0 {
					t.Fatalf("missedEvents = %+v, want none (all combat-log slots fulfilled)", h.missedEvents)
				}
			})
		})
	})

	t.Run("when hero last-hits and enemy kills another creep on same tick", func(t *testing.T) {
		const (
			creep1    int32 = 30
			creep2    int32 = 31
			deathTick       = tickHeroLH + 6
		)

		run := func(t *testing.T, creep1DiesFirst bool) {
			t.Helper()
			h := testHandler()

			seedRanged(h, creep1, 160, tickHeroLH-1)
			seedRanged(h, creep2, 160, tickHeroLH-1)
			heroDamageCreepCombatLog(h, testRangedName, tickHeroLH, 120, 40)
			updateCreepHealth(h, creep1, 120, manta.EntityOpUpdated, tickHeroLH)
			flushTick(h, tickHeroLH)
			if h.creepTracks[creep1].heroDamagedTick != tickHeroLH {
				t.Fatalf("creep1 heroDamagedTick = %v, want %v", h.creepTracks[creep1].heroDamagedTick, tickHeroLH)
			}

			heroKillCreepCombatLog(h, testRangedName, deathTick)
			enemyKillCreepCombatLog(h, testRangedName, deathTick)

			if creep1DiesFirst {
				creepDiedEntityUpdate(h, creep1, deathTick)
				creepDiedEntityUpdate(h, creep2, deathTick)
			} else {
				creepDiedEntityUpdate(h, creep2, deathTick)
				creepDiedEntityUpdate(h, creep1, deathTick)
			}
			flushTick(h, deathTick)

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

				const deathTick = tickDamage2 + 15

				heroKillCreepCombatLog(h, testRangedName, deathTick)
				heroKillCreepCombatLog(h, testRangedName, deathTick)
				heroKillCreepCombatLog(h, testRangedName, deathTick)
				enemyKillCreepCombatLog(h, testRangedName, deathTick)

				creepDiedEntityUpdate(h, creepA, deathTick)
				creepDiedEntityUpdate(h, creepB, deathTick)
				creepDiedEntityUpdate(h, creepC, deathTick)
				creepDiedEntityUpdate(h, creepD, deathTick)
				flushTick(h, deathTick)

				if len(h.missedEvents) != 0 {
					t.Fatalf("missedEvents = %+v, want none (both damage slots fulfilled by hero kills)", h.missedEvents)
				}
			})

			t.Run("does not count as a miss when 2 hero kills and 2 enemy kills on same tick", func(t *testing.T) {
				h, creepA, creepB, creepC, creepD := setupTwoDamageThreeMatchingCreepsAndD(t)
				correlateTwoDamageThreeMatchingCreeps(h, creepA, creepB, creepC)

				const deathTick = tickDamage2 + 15

				heroKillCreepCombatLog(h, testRangedName, deathTick)
				heroKillCreepCombatLog(h, testRangedName, deathTick)
				enemyKillCreepCombatLog(h, testRangedName, deathTick)
				enemyKillCreepCombatLog(h, testRangedName, deathTick)

				creepDiedEntityUpdate(h, creepA, deathTick)
				creepDiedEntityUpdate(h, creepD, deathTick)
				creepDiedEntityUpdate(h, creepB, deathTick)
				creepDiedEntityUpdate(h, creepC, deathTick)
				flushTick(h, deathTick)

				if len(h.missedEvents) != 0 {
					t.Fatalf("missedEvents = %+v, want none (both damage slots fulfilled by hero kills)", h.missedEvents)
				}
			})
		})
	})
}

func setupTwoDamageThreeMatchingCreepsAndD(t *testing.T) (*Handler, int32, int32, int32, int32) {
	t.Helper()
	const (
		creepA int32 = 40
		creepB int32 = 41
		creepC int32 = 42
		creepD int32 = 43
	)
	h := testHandler()
	heroDamageCreepCombatLog(h, testRangedName, tickDamage2, 120, 40)
	heroDamageCreepCombatLog(h, testRangedName, tickDamage2, 120, 40)
	seedRanged(h, creepA, 160, tickDamage2-1)
	seedRanged(h, creepB, 160, tickDamage2-1)
	seedRanged(h, creepC, 160, tickDamage2-1)
	seedRanged(h, creepD, 500, tickDamage2)
	return h, creepA, creepB, creepC, creepD
}

func correlateTwoDamageThreeMatchingCreeps(h *Handler, creepA, creepB, creepC int32) uint64 {
	updateCreepHealth(h, creepB, 120, manta.EntityOpUpdated, tickDamage2)
	updateCreepHealth(h, creepA, 120, manta.EntityOpUpdated, tickDamage2)
	updateCreepHealth(h, creepC, 120, manta.EntityOpUpdated, tickDamage2)
	flushTick(h, tickDamage2)
	return h.creepTracks[creepA].conflictGroupID
}

func setupFourMatchingCreepsThreeDamageLogs(t *testing.T) (*Handler, [4]int32) {
	t.Helper()
	creepsIdx := [4]int32{20, 21, 22, 23}
	h := testHandler()
	heroDamageCreepCombatLog(h, testRangedName, tickDamage3, 120, 40)
	heroDamageCreepCombatLog(h, testRangedName, tickDamage3, 120, 40)
	heroDamageCreepCombatLog(h, testRangedName, tickDamage3, 120, 40)
	for _, idx := range creepsIdx {
		seedRanged(h, idx, 160, tickDamage3-1)
	}
	return h, creepsIdx
}

func correlateFourMatchingCreeps(h *Handler, creepsIdx [4]int32) uint64 {
	for _, idx := range creepsIdx {
		updateCreepHealth(h, idx, 120, manta.EntityOpUpdated, tickDamage3)
	}
	flushTick(h, tickDamage3)
	return h.creepTracks[creepsIdx[0]].conflictGroupID
}

func setupThreeMatchingCreeps(t *testing.T) (*Handler, int32, int32, int32) {
	t.Helper()
	const (
		creepA int32 = 10
		creepB int32 = 11
		creepC int32 = 12
	)
	h := testHandler()
	heroDamageCreepCombatLog(h, testRangedName, tickDamage1, 120, 40)
	seedRanged(h, creepA, 160, tickDamage1-1)
	seedRanged(h, creepB, 160, tickDamage1-1)
	seedRanged(h, creepC, 160, tickDamage1-1)
	return h, creepA, creepB, creepC
}

func correlateThreeMatchingCreeps(h *Handler, creepA, creepB, creepC int32) uint64 {
	updateCreepHealth(h, creepB, 120, manta.EntityOpUpdated, tickDamage1)
	updateCreepHealth(h, creepA, 120, manta.EntityOpUpdated, tickDamage1)
	updateCreepHealth(h, creepC, 120, manta.EntityOpUpdated, tickDamage1)
	flushTick(h, tickDamage1)
	return h.creepTracks[creepA].conflictGroupID
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

func TestTwoSameTickHeroDamage_ThreeMatchingCreeps_NoFalseMiss(t *testing.T) {
	h := testHandler()
	const (
		creepA int32 = 10
		creepB int32 = 11
		creepC int32 = 12
		tick         = tickDamage1
	)

	heroDamageCreepCombatLog(h, testRangedName, tick, 120, 40)
	heroDamageCreepCombatLog(h, testRangedName, tick, 120, 40)

	seedRanged(h, creepA, 160, tick-1)
	seedRanged(h, creepB, 160, tick-1)
	seedRanged(h, creepC, 160, tick-1)
	updateCreepHealth(h, creepB, 120, manta.EntityOpUpdated, tick)
	updateCreepHealth(h, creepA, 120, manta.EntityOpUpdated, tick)
	updateCreepHealth(h, creepC, 120, manta.EntityOpUpdated, tick)
	flushTick(h, tick)

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

	heroKillCreepCombatLog(h, testRangedName, tick+15)
	creepDiedEntityUpdate(h, creepA, tick+15)
	flushTick(h, tick+15)

	heroKillCreepCombatLog(h, testRangedName, tick+18)
	creepDiedEntityUpdate(h, creepC, tick+18)
	flushTick(h, tick+18)

	enemyKillCreepCombatLog(h, testRangedName, tick+30)
	creepDiedEntityUpdate(h, creepB, tick+30)
	flushTick(h, tick+30)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents = %+v, want none (third creep stole pending or wrong idx bound)", h.missedEvents)
	}
}

func TestFalseCorrelatedCreepDiesFirst_ThenHeroLastHitsTrueCreep_NoFalseMiss(t *testing.T) {
	h := testHandler()
	const (
		creepA     int32 = 100
		creepB     int32 = 101
		damageTick       = uint32(500)
	)

	seedRanged(h, creepA, 160, damageTick-1)
	seedRanged(h, creepB, 160, damageTick-1)
	heroDamageCreepCombatLog(h, testRangedName, damageTick, 120, 40)
	updateCreepHealth(h, creepB, 120, manta.EntityOpUpdated, damageTick)
	updateCreepHealth(h, creepA, 120, manta.EntityOpUpdated, damageTick)
	flushTick(h, damageTick)

	enemyKillCreepCombatLog(h, testRangedName, damageTick+21)
	creepDiedEntityUpdate(h, creepB, damageTick+21)
	flushTick(h, damageTick+21)

	heroKillCreepCombatLog(h, testRangedName, damageTick+36)
	creepDiedEntityUpdate(h, creepA, damageTick+36)
	flushTick(h, damageTick+36)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents = %+v, want none (false miss on B before hero LH on A)", h.missedEvents)
	}
}

func TestFlagbearerMiss_SameTickCombatLogThenEntity(t *testing.T) {
	const idx int32 = 2382
	const flagbearer = "npc_dota_creep_badguys_flagbearer"
	const tick uint32 = 5000
	h := testHandler()

	seedCreep(h, idx, 196, tick, creeps.KindFlagbearer, "bad")
	heroDamageCreepCombatLog(h, flagbearer, tick, 137, 59)
	updateCreepHealth(h, idx, 137, manta.EntityOpUpdated, tick)
	flushTick(h, tick)

	if h.creepTracks[idx].heroDamagedTick == 0 {
		t.Fatalf("track not bound after flush: %+v", h.creepTracks[idx])
	}

	updateCreepHealth(h, idx, 118, manta.EntityOpUpdated, tick+6)
	flushTick(h, tick+6)
	updateCreepHealth(h, idx, 20, manta.EntityOpUpdated, tick+36)
	flushTick(h, tick+36)

	enemyKillCreepCombatLog(h, flagbearer, tick+44)
	creepDiedEntityUpdate(h, idx, tick+44)
	flushTick(h, tick+44)

	if len(h.missedEvents) != 1 {
		t.Fatalf("missedEvents = %+v, want 1 flagbearer miss", h.missedEvents)
	}
}

func TestMissedLastHit_SameTickDeathCoalesce_KindFallback(t *testing.T) {
	const idx int32 = 42
	const creep = testMeleeName
	const damageTick uint32 = 11061
	h := testHandler()

	startHealth := int32(100)
	damage := int32(70)
	seedMelee(h, idx, startHealth, damageTick-1)
	heroDamageCreepCombatLog(h, creep, damageTick, startHealth-damage, damage)
	enemyKillCreepCombatLog(h, creep, damageTick)
	creepDiedEntityUpdate(h, idx, damageTick)
	flushTick(h, damageTick)

	if len(h.missedEvents) != 1 {
		t.Fatalf("missedEvents = %+v, want 1 same-tick death coalesce miss", h.missedEvents)
	}
}

func TestMissedLastHit_OutsideWindowNotCounted(t *testing.T) {
	h := testHandler()
	const idx int32 = 42
	const tick uint32 = 100

	seedMelee(h, idx, 80, tick-1)
	heroDamageCreepCombatLog(h, testMeleeName, tick, 50, 30)
	updateCreepHealth(h, idx, 50, manta.EntityOpUpdated, tick)
	flushTick(h, tick)

	outTick := tick + missedLastHitWindowTicks + 1
	enemyKillCreepCombatLog(h, testMeleeName, outTick)
	creepDiedEntityUpdate(h, idx, outTick)
	flushTick(h, outTick)

	if len(h.missedEvents) != 0 {
		t.Fatalf("missedEvents len = %d, want 0 outside window", len(h.missedEvents))
	}
}
