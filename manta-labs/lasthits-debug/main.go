// Command lasthits-debug traces combat-log and entity events to debug missed
// last-hit / deny correlation in parser/internal/lasthits.
//
// See README.md for usage and examples.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

const (
	modeTrace           = "trace"
	modeWarlock         = "warlock-badguys"
	modeEntityNames     = "entity-names"
	modeHealthMatch     = "health-match"
	modeDumpFields         = "dump-fields"
	modeBuildPathcornerMap      = "build-pathcorner-map"
	modeBuildPathcornerLaneSpawn = "build-pathcorner-lane-spawn"
)

func main() {
	log.SetOutput(os.Stderr)

	replay := flag.String("replay", "", "path to .dem replay")
	replays := flag.String("replays", "", "comma-separated .dem paths (merges spawns for lane table)")
	mode := flag.String("mode", modeTrace, "trace | warlock-badguys | entity-names | health-match | dump-fields | build-pathcorner-map | build-pathcorner-lane-spawn")
	format := flag.String("format", "text", "for build-pathcorner-map: text|json; for build-pathcorner-lane-spawn: text|table|tsv|markdown|json")
	mapVotes := flag.String("map-votes", "unique", "for build-pathcorner-map: unique | spawn")
	from := flag.Float64("from", 160, "window start (game seconds, creep spawn = 0)")
	to := flag.Float64("to", 170, "window end (game seconds)")
	health := flag.Int("health", 0, "for entity-names / health-match: filter entity updates to this m_iHealth")
	heroFilter := flag.String("hero", "", "optional combat-log attacker substring filter (e.g. warlock)")
	targetFilter := flag.String("target", "", "optional combat-log target substring filter (e.g. badguys)")
	outPath := flag.String("out", "", "write trace to file instead of stdout")
	flag.Parse()

	replayPaths := parseReplayPaths(*replay, *replays)
	if len(replayPaths) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	out := io.Writer(os.Stdout)
	if *outPath != "" {
		file, err := os.Create(*outPath)
		if err != nil {
			log.Fatalf("create output: %v", err)
		}
		defer file.Close()
		out = file
	}

	if *mode == modeBuildPathcornerLaneSpawn {
		if err := runPathcornerLaneSpawn(replayPaths, out, *format); err != nil {
			log.Fatalf("pathcorner lane spawn: %v", err)
		}
		return
	}

	f, err := os.Open(replayPaths[0])
	if err != nil {
		log.Fatalf("open replay: %v", err)
	}
	defer f.Close()

	p, err := manta.NewStreamParser(f)
	if err != nil {
		log.Fatalf("parser: %v", err)
	}

	tp := newGameClock()
	tp.register(p)

	windowStart := float32(*from)
	windowEnd := float32(*to)

	switch *mode {
	case modeTrace:
		registerTraceCallbacks(p, tp, out, windowStart, windowEnd, *heroFilter, *targetFilter)
	case modeWarlock:
		registerWarlockBadguysCallbacks(p, tp, out, windowStart, windowEnd)
	case modeEntityNames:
		registerEntityNamesCallbacks(p, tp, out, windowStart, windowEnd, int32(*health))
	case modeHealthMatch:
		registerHealthMatchCallbacks(p, tp, out, windowStart, windowEnd, int32(*health))
	case modeDumpFields:
		registerDumpFieldsCallbacks(p, tp, out, windowStart, windowEnd, int32(*health))
	case modeBuildPathcornerMap:
		registerBuildPathcornerMapCallbacks(p, tp, *mapVotes)
	default:
		log.Fatalf("unknown mode %q (use trace, warlock-badguys, entity-names, health-match, dump-fields, build-pathcorner-map, build-pathcorner-lane-spawn)", *mode)
	}

	if err := p.Start(); err != nil && err != io.EOF {
		log.Fatalf("parse: %v", err)
	}

	if printPathcornerMapSummary != nil {
		if *format == "json" {
			writePathcornerMapJSON(out, pathcornerMapBuiltState, replayPaths[0])
		} else {
			printPathcornerMapSummary(out, replayPaths[0])
		}
	}
}

func parseReplayPaths(replay, replays string) []string {
	if replays != "" {
		var paths []string
		for _, p := range strings.Split(replays, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				paths = append(paths, p)
			}
		}
		return paths
	}
	if replay != "" {
		return []string{replay}
	}
	return nil
}

func inWindow(gt, start, end float32) bool {
	return gt >= start && gt <= end
}

func isLaneOrNeutralCreep(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(name, "npc_dota_creep_") || strings.HasPrefix(name, "npc_dota_neutral_")
}

func isCreepEntityClass(className string) bool {
	return strings.HasPrefix(className, "CDOTA_BaseNPC_Creep")
}

// trace: interleaved COMBAT + ENTITY lines for creeps in the time window.
func registerTraceCallbacks(p *manta.Parser, tp *gameClock, out io.Writer, start, end float32, heroFilter, targetFilter string) {
	type creepState struct {
		prevHealth int32
		hasHealth  bool
	}
	tracks := make(map[int32]*creepState)

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		gt := tp.currentGameTime()
		if !inWindow(gt, start, end) {
			return nil
		}
		attacker, _ := p.LookupStringByIndex("CombatLogNames", int32(m.GetAttackerName()))
		target, _ := p.LookupStringByIndex("CombatLogNames", int32(m.GetTargetName()))
		if !isLaneOrNeutralCreep(target) && !isLaneOrNeutralCreep(attacker) {
			return nil
		}
		if heroFilter != "" && !strings.Contains(strings.ToLower(attacker), strings.ToLower(heroFilter)) &&
			!strings.Contains(strings.ToLower(target), strings.ToLower(heroFilter)) {
			return nil
		}
		if targetFilter != "" && !strings.Contains(strings.ToLower(target), strings.ToLower(targetFilter)) &&
			!strings.Contains(strings.ToLower(attacker), strings.ToLower(targetFilter)) {
			return nil
		}
		if m.GetType() != dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE &&
			m.GetType() != dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DEATH {
			return nil
		}
		fmt.Fprintf(out, "COMBAT gt=%.3f tick=%d type=%d attacker=%s target=%s health=%d value=%d\n",
			gt, p.Tick, m.GetType(), attacker, target, m.GetHealth(), m.GetValue())
		return nil
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		gt := tp.currentGameTime()
		if !inWindow(gt, start, end) || !isCreepEntityClass(e.GetClassName()) {
			return nil
		}
		health, ok := e.GetInt32("m_iHealth")
		if !ok {
			return nil
		}
		idx := e.GetIndex()
		st := tracks[idx]
		if st == nil {
			st = &creepState{}
			tracks[idx] = st
		}
		reduced := st.hasHealth && health < st.prevHealth
		died := (st.hasHealth && health <= 0 && st.prevHealth > 0) || op.Flag(manta.EntityOpDeleted) || op.Flag(manta.EntityOpDeletedLeft)
		if reduced || died || !st.hasHealth {
			nameIdx, _ := e.GetInt32("m_iUnitNameIndex")
			name, _ := p.LookupStringByIndex("EntityNames", nameIdx)
			fmt.Fprintf(out, "ENTITY gt=%.3f tick=%d idx=%d op=%s class=%s unitNameIndex=%q health=%d prev=%d reduced=%v died=%v\n",
				gt, p.Tick, idx, op.String(), e.GetClassName(), name, health, st.prevHealth, reduced, died)
		}
		st.prevHealth = health
		st.hasHealth = true
		return nil
	})
}

// warlock-badguys: compact DAMAGE/DEATH lines for Warlock or badguys lane creeps.
func registerWarlockBadguysCallbacks(p *manta.Parser, tp *gameClock, out io.Writer, start, end float32) {
	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		gt := tp.currentGameTime()
		if !inWindow(gt, start, end) {
			return nil
		}
		attacker, _ := p.LookupStringByIndex("CombatLogNames", int32(m.GetAttackerName()))
		target, _ := p.LookupStringByIndex("CombatLogNames", int32(m.GetTargetName()))
		if !strings.Contains(attacker, "warlock") && !strings.Contains(target, "badguys") {
			return nil
		}
		typeName := map[dota.DOTA_COMBATLOG_TYPES]string{
			dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE: "DAMAGE",
			dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DEATH:  "DEATH",
		}[m.GetType()]
		if typeName == "" {
			return nil
		}
		fmt.Fprintf(out, "gt=%.3f %s attacker=%s target=%s health=%d value=%d tick=%d\n",
			gt, typeName, attacker, target, m.GetHealth(), m.GetValue(), p.Tick)
		return nil
	})
}

// entity-names: show why EntityNames lookup fails for creep NPC names (pathcorner vs generic lane).
func registerEntityNamesCallbacks(p *manta.Parser, tp *gameClock, out io.Writer, start, end float32, healthFilter int32) {
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		gt := tp.currentGameTime()
		if !inWindow(gt, start, end) || e.GetClassName() != "CDOTA_BaseNPC_Creep_Lane" {
			return nil
		}
		health, ok := e.GetInt32("m_iHealth")
		if !ok {
			return nil
		}
		if healthFilter != 0 && health != healthFilter {
			return nil
		}
		nameIdx, _ := e.GetInt32("m_iUnitNameIndex")
		tableIdx, _ := e.GetInt32("m_pEntity.m_nameStringTableIndex")
		name1, ok1 := p.LookupStringByIndex("EntityNames", nameIdx)
		name2, ok2 := p.LookupStringByIndex("EntityNames", tableIdx)
		fmt.Fprintf(out, "gt=%.3f idx=%d health=%d m_iUnitNameIndex=%d->%q(ok=%v) m_nameStringTableIndex=%d->%q(ok=%v)\n",
			gt, e.GetIndex(), health, nameIdx, name1, ok1, tableIdx, name2, ok2)
		return nil
	})
}

// health-match: follow one post-damage health value from combat log to entity idx (correlation proof).
func registerHealthMatchCallbacks(p *manta.Parser, tp *gameClock, out io.Writer, start, end float32, matchHealth int32) {
	if matchHealth == 0 {
		matchHealth = 137 // default: Warlock flagbearer hit in replay 8915936762 @ ~164s
	}

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		gt := tp.currentGameTime()
		if !inWindow(gt, start, end) {
			return nil
		}
		if m.GetType() != dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE {
			return nil
		}
		target, _ := p.LookupStringByIndex("CombatLogNames", int32(m.GetTargetName()))
		if m.GetHealth() != matchHealth {
			return nil
		}
		fmt.Fprintf(out, "COMBAT gt=%.3f target=%s postDamageHealth=%d tick=%d\n",
			gt, target, m.GetHealth(), p.Tick)
		return nil
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		gt := tp.currentGameTime()
		if !inWindow(gt, start, end) || !isCreepEntityClass(e.GetClassName()) {
			return nil
		}
		health, ok := e.GetInt32("m_iHealth")
		if !ok || health != matchHealth {
			return nil
		}
		fmt.Fprintf(out, "ENTITY gt=%.3f idx=%d health=%d op=%s tick=%d\n",
			gt, e.GetIndex(), health, op.String(), p.Tick)
		return nil
	})
}

// dump-fields: spew identity-related entity fields (find where creep type lives vs pathcorner).
func registerDumpFieldsCallbacks(p *manta.Parser, tp *gameClock, out io.Writer, start, end float32, healthFilter int32) {
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		gt := tp.currentGameTime()
		if !inWindow(gt, start, end) || e.GetClassName() != "CDOTA_BaseNPC_Creep_Lane" {
			return nil
		}
		health, ok := e.GetInt32("m_iHealth")
		if !ok || (healthFilter != 0 && health != healthFilter) {
			return nil
		}
		fmt.Fprintf(out, "=== idx=%d gt=%.3f health=%d op=%s ===\n", e.GetIndex(), gt, health, op.String())
		for _, k := range sortedKeys(e.Map()) {
			kl := strings.ToLower(k)
			if strings.Contains(kl, "name") || strings.Contains(kl, "unit") ||
				strings.Contains(kl, "creep") || strings.Contains(kl, "model") ||
				strings.Contains(kl, "type") || strings.Contains(kl, "label") ||
				strings.Contains(kl, "team") {
				fmt.Fprintf(out, "  %s = %#v\n", k, e.Map()[k])
			}
		}
		return nil
	})
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
