package main

// Proves manta callback tick ordering on a real replay:
// 1) Outer ticks are non-decreasing across combat + entity callbacks.
// 2) After the first combat-log callback at tick T, no entity update with tick < T
//    appears later (so all entity updates for tick T-1 finished before tick-T combat).
// 3) Within the same tick, at least one combat callback is observed before the first
//    entity callback for that tick (when both exist).

import (
	"fmt"
	"os"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

func main() {
	replay := "../../dota-replays/8915936762.dem"
	if v := os.Getenv("REPLAY"); v != "" {
		replay = v
	}
	if len(os.Args) > 1 {
		replay = os.Args[1]
	}

	f, err := os.Open(replay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	p, err := manta.NewStreamParser(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parser: %v\n", err)
		os.Exit(1)
	}

	var (
		prevTick           uint32
		havePrev           bool
		violationsDecr     int
		violationsLateEnt  int // entity tick < some already-seen combat tick
		maxCombatTickSeen  uint32
		haveCombat         bool
		sameTickCombatFirst = 0
		sameTickEntityFirst = 0
		ticksWithBoth       = 0

		// per tick: did we see combat before first entity?
		tickSawCombat  = map[uint32]bool{}
		tickSawEntity  = map[uint32]bool{}
		tickCombatFirst = map[uint32]bool{} // combat seen before any entity on this tick

		combatCallbacks int
		entityCallbacks int
	)

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		t := p.Tick
		combatCallbacks++
		if havePrev && t < prevTick {
			violationsDecr++
			fmt.Printf("FAIL non-decreasing: combat tick %d after tick %d\n", t, prevTick)
		}
		prevTick = t
		havePrev = true

		if !tickSawCombat[t] && !tickSawEntity[t] {
			tickCombatFirst[t] = true
		}
		tickSawCombat[t] = true

		if !haveCombat || t > maxCombatTickSeen {
			maxCombatTickSeen = t
		}
		haveCombat = true
		return nil
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		t := p.Tick
		entityCallbacks++
		if havePrev && t < prevTick {
			violationsDecr++
			fmt.Printf("FAIL non-decreasing: entity tick %d after tick %d\n", t, prevTick)
		}
		prevTick = t
		havePrev = true

		if haveCombat && t < maxCombatTickSeen {
			violationsLateEnt++
			if violationsLateEnt <= 5 {
				fmt.Printf("FAIL late entity: entity tick %d after combat already seen at tick %d\n", t, maxCombatTickSeen)
			}
		}

		if !tickSawEntity[t] {
			if tickSawCombat[t] {
				tickCombatFirst[t] = true
			}
		}
		tickSawEntity[t] = true
		return nil
	})

	if err := p.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start: %v\n", err)
		os.Exit(1)
	}

	for t, sawC := range tickSawCombat {
		if !sawC || !tickSawEntity[t] {
			continue
		}
		ticksWithBoth++
		if tickCombatFirst[t] {
			sameTickCombatFirst++
		} else {
			sameTickEntityFirst++
		}
	}

	fmt.Printf("# tick ordering proof\n")
	fmt.Printf("# replay: %s\n", replay)
	fmt.Printf("combat_callbacks\t%d\n", combatCallbacks)
	fmt.Printf("entity_callbacks\t%d\n", entityCallbacks)
	fmt.Printf("ticks_with_both_combat_and_entity\t%d\n", ticksWithBoth)
	fmt.Printf("same_tick_combat_before_first_entity\t%d\n", sameTickCombatFirst)
	fmt.Printf("same_tick_entity_before_first_combat\t%d\n", sameTickEntityFirst)
	fmt.Printf("violations_tick_decreased\t%d\n", violationsDecr)
	fmt.Printf("violations_entity_after_later_combat\t%d\n", violationsLateEnt)

	ok := violationsDecr == 0 && violationsLateEnt == 0 && sameTickEntityFirst == 0
	if ok {
		fmt.Printf("RESULT\tPASS\n")
		fmt.Printf("# implication: all entity updates for tick N finish before first combat log for tick N+1\n")
	} else {
		fmt.Printf("RESULT\tFAIL\n")
		os.Exit(1)
	}
}
