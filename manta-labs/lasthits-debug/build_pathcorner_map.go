package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

type pendingCombatHit struct {
	gameTime     float32
	combatTarget string
	postHealth   int32
	consumed     bool
}

type pathcornerMapEntry struct {
	Pathcorner     string            `json:"pathcorner"`
	Presumed       string            `json:"presumed"`
	Votes          map[string]int    `json:"votes"`
	Total          int               `json:"total"`
	Conflict       bool              `json:"conflict"`
	SpawnMaxHealth map[int32]int     `json:"spawn_max_health,omitempty"`
}

type pathcornerEntitySnap struct {
	idx        int32
	pathcorner string
	health     int32
	maxHealth  int32
	gameTime   float32
}

type pathcornerMapState struct {
	pending     []pendingCombatHit
	votes       map[string]map[string]int
	spawnMaxHP  map[string]map[int32]int
	entitySpawn map[int32]bool
	tickSnaps   []pathcornerEntitySnap
	lastTick    uint32
	voteMode    string // unique (default) | spawn
}

var (
	printPathcornerMapSummary func(out io.Writer, replayPath string)
	pathcornerMapBuiltState   *pathcornerMapState
)

func registerBuildPathcornerMapCallbacks(p *manta.Parser, tp *gameClock, voteMode string) {
	if voteMode == "" {
		voteMode = "unique"
	}
	st := &pathcornerMapState{
		votes:       make(map[string]map[string]int),
		spawnMaxHP:  make(map[string]map[int32]int),
		entitySpawn: make(map[int32]bool),
		voteMode:    voteMode,
	}

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		gt := tp.currentGameTime()
		switch m.GetType() {
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE:
			target, ok := p.LookupStringByIndex("CombatLogNames", int32(m.GetTargetName()))
			if !ok || !isLaneCreepCombatName(target) {
				return nil
			}
			st.pending = append(st.pending, pendingCombatHit{
				gameTime:     gt,
				combatTarget: target,
				postHealth:   m.GetHealth(),
			})
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DEATH:
			target, ok := p.LookupStringByIndex("CombatLogNames", int32(m.GetTargetName()))
			if !ok || !isLaneCreepCombatName(target) {
				return nil
			}
			st.pending = append(st.pending, pendingCombatHit{
				gameTime:     gt,
				combatTarget: target,
				postHealth:   0,
			})
		}
		return nil
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		gt := tp.currentGameTime()
		if e.GetClassName() != "CDOTA_BaseNPC_Creep_Lane" {
			return tryConsumePendingPathcornerVotes(p, st, nil)
		}
		health, ok := e.GetInt32("m_iHealth")
		if !ok {
			return tryConsumePendingPathcornerVotes(p, st, nil)
		}
		idx := e.GetIndex()
		maxHealth := health
		if mh, ok := e.GetInt32("m_iMaxHealth"); ok && mh > 0 {
			maxHealth = mh
		}

		nameIdx, _ := e.GetInt32("m_iUnitNameIndex")
		pathcorner, okName := p.LookupStringByIndex("EntityNames", nameIdx)

		var snap *pathcornerEntitySnap
		if okName && strings.Contains(pathcorner, "pathcorner") {
			snap = &pathcornerEntitySnap{
				idx:        idx,
				pathcorner: pathcorner,
				health:     health,
				maxHealth:  maxHealth,
				gameTime:   gt,
			}
			if !st.entitySpawn[idx] {
				st.entitySpawn[idx] = true
				if health > 0 && health == maxHealth {
					if st.spawnMaxHP[pathcorner] == nil {
						st.spawnMaxHP[pathcorner] = make(map[int32]int)
					}
					st.spawnMaxHP[pathcorner][maxHealth]++
				}
			}
		}
		return tryConsumePendingPathcornerVotes(p, st, snap)
	})

	printPathcornerMapSummary = func(w io.Writer, rp string) {
		finalizePathcornerMap(st)
		writePathcornerMapSummary(w, st, rp)
	}
	pathcornerMapBuiltState = st
}

func isLaneCreepCombatName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(name, "npc_dota_creep_") && !strings.Contains(name, "neutral")
}

func pathcornerTeam(pathcorner string) string {
	pathcorner = strings.ToLower(pathcorner)
	switch {
	case strings.Contains(pathcorner, "badguys"):
		return "badguys"
	case strings.Contains(pathcorner, "goodguys"):
		return "goodguys"
	default:
		return ""
	}
}

func combatNameMatchesPathcorner(combatTarget, pathcorner string) bool {
	team := pathcornerTeam(pathcorner)
	if team == "" {
		return true
	}
	return strings.Contains(strings.ToLower(combatTarget), team)
}

func recordPathcornerVote(st *pathcornerMapState, pathcorner, combatTarget string) {
	if st.votes[pathcorner] == nil {
		st.votes[pathcorner] = make(map[string]int)
	}
	st.votes[pathcorner][combatTarget]++
}

func tryConsumePendingPathcornerVotes(p *manta.Parser, st *pathcornerMapState, snap *pathcornerEntitySnap) error {
	if snap != nil {
		if p.Tick != st.lastTick {
			consumeTickPathcornerVotes(st)
			st.tickSnaps = st.tickSnaps[:0]
			st.lastTick = p.Tick
		}
		st.tickSnaps = append(st.tickSnaps, *snap)
	}
	return nil
}

func consumeTickPathcornerVotes(st *pathcornerMapState) {
	if len(st.tickSnaps) == 0 || len(st.pending) == 0 {
		return
	}
	for i := range st.pending {
		pd := &st.pending[i]
		if pd.consumed {
			continue
		}
		var matches []pathcornerEntitySnap
		for _, s := range st.tickSnaps {
			if s.health != pd.postHealth {
				continue
			}
			if st.voteMode == "spawn" && s.health != s.maxHealth {
				continue
			}
			if !combatNameMatchesPathcorner(pd.combatTarget, s.pathcorner) {
				continue
			}
			matches = append(matches, s)
		}
		if len(matches) != 1 {
			continue
		}
		recordPathcornerVote(st, matches[0].pathcorner, pd.combatTarget)
		pd.consumed = true
	}
}

func finalizePathcornerMap(st *pathcornerMapState) {
	consumeTickPathcornerVotes(st)
}

func buildPathcornerMapEntries(st *pathcornerMapState) []pathcornerMapEntry {
	pathcorners := make([]string, 0, len(st.votes))
	for pc := range st.votes {
		pathcorners = append(pathcorners, pc)
	}
	sort.Strings(pathcorners)

	out := make([]pathcornerMapEntry, 0, len(pathcorners))
	for _, pc := range pathcorners {
		votes := st.votes[pc]
		total := 0
		for _, n := range votes {
			total += n
		}
		presumed, conflict := presumedCombatName(votes)
		entry := pathcornerMapEntry{
			Pathcorner: pc,
			Presumed:   presumed,
			Votes:      votes,
			Total:      total,
			Conflict:   conflict,
		}
		if mh := st.spawnMaxHP[pc]; len(mh) > 0 {
			entry.SpawnMaxHealth = mh
		}
		out = append(out, entry)
	}
	return out
}

func presumedCombatName(votes map[string]int) (string, bool) {
	if len(votes) == 0 {
		return "", false
	}
	type pair struct {
		name  string
		count int
	}
	pairs := make([]pair, 0, len(votes))
	for name, count := range votes {
		pairs = append(pairs, pair{name, count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].name < pairs[j].name
	})
	conflict := len(pairs) > 1 && pairs[1].count > 0 &&
		float64(pairs[1].count) >= 0.25*float64(pairs[0].count)
	return pairs[0].name, conflict
}

func writePathcornerMapSummary(out io.Writer, st *pathcornerMapState, replayPath string) {
	entries := buildPathcornerMapEntries(st)

	fmt.Fprintf(out, "# pathcorner → combat-log mapping\n")
	fmt.Fprintf(out, "# replay: %s\n", replayPath)
	fmt.Fprintf(out, "# mode: build-pathcorner-map (vote_mode=%s, team-filtered)\n", st.voteMode)
	fmt.Fprintf(out, "# pathcorner | presumed | votes | total | conflict\n")
	for _, e := range entries {
		fmt.Fprintf(out, "%s | %s | %s | %d | %v\n",
			e.Pathcorner, e.Presumed, formatVoteCounts(e.Votes), e.Total, e.Conflict)
		if len(e.SpawnMaxHealth) > 0 {
			fmt.Fprintf(out, "  spawn_max_health: %s\n", formatMaxHealthCounts(e.SpawnMaxHealth))
		}
	}
	fmt.Fprintf(out, "# entries: %d\n", len(entries))
}

func writePathcornerMapJSON(out io.Writer, st *pathcornerMapState, replayPath string) {
	finalizePathcornerMap(st)
	payload := struct {
		Replay   string               `json:"replay"`
		VoteMode string               `json:"vote_mode"`
		Entries  []pathcornerMapEntry `json:"entries"`
		Lookup   map[string]string    `json:"lookup"`
	}{
		Replay:   replayPath,
		VoteMode: st.voteMode,
		Entries:  buildPathcornerMapEntries(st),
		Lookup:   make(map[string]string),
	}
	for _, e := range payload.Entries {
		if e.Presumed != "" {
			payload.Lookup[e.Pathcorner] = e.Presumed
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func formatVoteCounts(votes map[string]int) string {
	names := make([]string, 0, len(votes))
	for name := range votes {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		short := strings.TrimPrefix(name, "npc_dota_creep_")
		parts = append(parts, fmt.Sprintf("%s:%d", short, votes[name]))
	}
	return strings.Join(parts, " ")
}

func formatMaxHealthCounts(counts map[int32]int) string {
	vals := make([]int32, 0, len(counts))
	for mh := range counts {
		vals = append(vals, mh)
	}
	sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })
	parts := make([]string, 0, len(vals))
	for _, mh := range vals {
		parts = append(parts, fmt.Sprintf("%d:%d", mh, counts[mh]))
	}
	return strings.Join(parts, " ")
}
