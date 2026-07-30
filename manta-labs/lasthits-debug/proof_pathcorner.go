package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

type proofDamageHit struct {
	gameTime     float32
	combatTarget string
	postHealth   int32
	consumed     bool
}

type combatEntityMismatch struct {
	gameTime     float32
	entityIdx    int32
	combatTarget string
	postHealth   int32
	pathcorner   string
}

type spawnSnapshot struct {
	gameTime  float32
	entityIdx int32
	maxHealth int32
}

type proofPathcornerState struct {
	entityNPCNameCount int
	pathcornerSamples  []string
	entitySpawnSeen    map[int32]bool // first full-health observation per entity (spawn proxy)
	spawnByPathcorner  map[string][]spawnSnapshot
	pendingDamage      []proofDamageHit
	mismatches         []combatEntityMismatch
}

var printProofPathcornerSummary func(out io.Writer)

func registerProofPathcornerCallbacks(p *manta.Parser, tp *gameClock, out io.Writer, start, end float32) {
	st := &proofPathcornerState{
		entitySpawnSeen:   make(map[int32]bool),
		spawnByPathcorner: make(map[string][]spawnSnapshot),
	}

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		gt := tp.currentGameTime()
		if !inWindow(gt, start, end) {
			return nil
		}
		if m.GetType() != dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE {
			return nil
		}
		target, ok := p.LookupStringByIndex("CombatLogNames", int32(m.GetTargetName()))
		if !ok || !isLaneOrNeutralCreep(target) {
			return nil
		}
		st.pendingDamage = append(st.pendingDamage, proofDamageHit{
			gameTime:     gt,
			combatTarget: target,
			postHealth:   m.GetHealth(),
		})
		return nil
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		gt := tp.currentGameTime()
		if e.GetClassName() != "CDOTA_BaseNPC_Creep_Lane" {
			return nil
		}
		health, ok := e.GetInt32("m_iHealth")
		if !ok {
			return nil
		}
		idx := e.GetIndex()
		maxHealth := health
		if mh, ok := e.GetInt32("m_iMaxHealth"); ok && mh > 0 {
			maxHealth = mh
		}

		nameIdx, _ := e.GetInt32("m_iUnitNameIndex")
		pathcorner, okName := p.LookupStringByIndex("EntityNames", nameIdx)

		if okName && strings.Contains(pathcorner, "pathcorner") {
			if len(st.pathcornerSamples) < 5 && !containsString(st.pathcornerSamples, pathcorner) {
				st.pathcornerSamples = append(st.pathcornerSamples, pathcorner)
			}
		}

		if !st.entitySpawnSeen[idx] {
			st.entitySpawnSeen[idx] = true
			if okName && strings.Contains(pathcorner, "pathcorner") && health > 0 && health == maxHealth {
				st.spawnByPathcorner[pathcorner] = append(st.spawnByPathcorner[pathcorner], spawnSnapshot{
					gameTime: gt, entityIdx: idx, maxHealth: maxHealth,
				})
			}
		}

		if !inWindow(gt, start, end) {
			return nil
		}

		if okName && strings.HasPrefix(pathcorner, "npc_dota_creep_") {
			st.entityNPCNameCount++
		}

		for i := range st.pendingDamage {
			pd := &st.pendingDamage[i]
			if pd.consumed || pd.postHealth != health {
				continue
			}
			if okName && pathcorner != "" {
				st.mismatches = append(st.mismatches, combatEntityMismatch{
					gameTime:     gt,
					entityIdx:    idx,
					combatTarget: pd.combatTarget,
					postHealth:   health,
					pathcorner:   pathcorner,
				})
			}
			pd.consumed = true
			break
		}
		return nil
	})

	printProofPathcornerSummary = func(w io.Writer) {
		writeProofPathcornerSummary(w, st)
	}
}

func writeProofPathcornerSummary(out io.Writer, st *proofPathcornerState) {
	fmt.Fprintln(out, "=== PROOF: pathcorner names ≠ combat-log creep NPC names ===")
	fmt.Fprintln(out)

	fmt.Fprintln(out, "CHECK 1: m_iUnitNameIndex never resolves to npc_dota_creep_* on lane creeps")
	fmt.Fprintf(out, "  entity updates with npc_dota_creep_* name: %d (expected 0)\n", st.entityNPCNameCount)
	check1 := st.entityNPCNameCount == 0
	fmt.Fprintf(out, "  result: %s\n\n", passFail(check1))

	fmt.Fprintln(out, "CHECK 2: combat-log creep type ≠ entity pathcorner on health-correlated entity")
	check2Count := 0
	for _, m := range st.mismatches {
		if !strings.HasPrefix(m.combatTarget, "npc_dota_creep_") || !strings.Contains(m.pathcorner, "pathcorner") {
			continue
		}
		check2Count++
		if check2Count <= 3 {
			fmt.Fprintf(out, "  gt=%.3f idx=%d combat=%q entity_pathcorner=%q health=%d\n",
				m.gameTime, m.entityIdx, m.combatTarget, m.pathcorner, m.postHealth)
		}
	}
	check2 := check2Count > 0
	if check2Count > 3 {
		fmt.Fprintf(out, "  ... and %d more mismatches\n", check2Count-3)
	}
	fmt.Fprintf(out, "  correlated mismatches: %d (expected >0)\n", check2Count)
	fmt.Fprintf(out, "  result: %s\n\n", passFail(check2))

	fmt.Fprintln(out, "CHECK 3: same pathcorner at creep spawn (full health) with different max health (entire replay)")
	proofs := pathcornerDistinctMaxHealthProofs(st.spawnByPathcorner)
	check3 := len(proofs) > 0
	for i, p := range proofs {
		if i >= 2 {
			fmt.Fprintf(out, "  ... and %d more pathcorners\n", len(proofs)-2)
			break
		}
		fmt.Fprintf(out, "  pathcorner=%q distinct_max_health=%v\n", p.pathcorner, p.maxHealths)
		for _, s := range p.examples {
			fmt.Fprintf(out, "    spawn gt=%.3f idx=%d max_health=%d (health==max at first sight)\n", s.gameTime, s.entityIdx, s.maxHealth)
		}
	}
	if !check3 {
		fmt.Fprintln(out, "  (no pathcorner with 2+ distinct max health at creep spawn)")
	}
	fmt.Fprintf(out, "  pathcorner proofs: %d (expected >0)\n", len(proofs))
	fmt.Fprintf(out, "  result: %s\n\n", passFail(check3))

	fmt.Fprintln(out, "Sample pathcorner strings seen on entities:")
	for _, s := range st.pathcornerSamples {
		fmt.Fprintf(out, "  - %q\n", s)
	}
	fmt.Fprintln(out)

	if check1 && check2 && check3 {
		fmt.Fprintln(out, "CONCLUSION: PASS — pathcorner names cannot map entity → combat-log creep NPC name.")
	} else {
		fmt.Fprintln(out, "CONCLUSION: FAIL — expected all checks to pass; review output above.")
	}
}

type pathcornerMaxHealthProof struct {
	pathcorner string
	maxHealths []int32
	examples   []spawnSnapshot
}

// pathcornerDistinctMaxHealthProofs finds pathcorners where different creeps at spawn
// (first full-health tick) carried that pathcorner with different m_iMaxHealth.
func pathcornerDistinctMaxHealthProofs(spawnByPathcorner map[string][]spawnSnapshot) []pathcornerMaxHealthProof {
	var out []pathcornerMaxHealthProof
	for pathcorner, spawns := range spawnByPathcorner {
		maxHealths := distinctMaxHealths(spawns)
		if len(maxHealths) < 2 {
			continue
		}
		examples := make([]spawnSnapshot, 0, len(maxHealths))
		for _, mh := range maxHealths {
			for _, s := range spawns {
				if s.maxHealth == mh {
					examples = append(examples, s)
					break
				}
			}
		}
		out = append(out, pathcornerMaxHealthProof{
			pathcorner: pathcorner,
			maxHealths: maxHealths,
			examples:   examples,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		spreadI := out[i].maxHealths[len(out[i].maxHealths)-1] - out[i].maxHealths[0]
		spreadJ := out[j].maxHealths[len(out[j].maxHealths)-1] - out[j].maxHealths[0]
		if spreadI != spreadJ {
			return spreadI > spreadJ
		}
		return out[i].pathcorner < out[j].pathcorner
	})
	return out
}

func distinctMaxHealths(spawns []spawnSnapshot) []int32 {
	seen := make(map[int32]struct{})
	out := make([]int32, 0, 4)
	for _, s := range spawns {
		if _, ok := seen[s.maxHealth]; ok {
			continue
		}
		seen[s.maxHealth] = struct{}{}
		out = append(out, s.maxHealth)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func containsString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
