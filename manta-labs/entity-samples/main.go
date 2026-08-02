// Command replay-samples extracts example combat-log and entity lines from replays.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dotabuff/manta"
)

func processReplay(path string, combat *combatLogCollector, entities *entityCollector) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	p, err := manta.NewStreamParser(f)
	if err != nil {
		return err
	}

	replayName := filepath.Base(path)
	combat.register(p, replayName)
	entities.register(p, replayName)

	return p.Start()
}

func main() {
	log.SetOutput(os.Stderr)

	out := flag.String("out", "output", "output directory")
	maxPerType := flag.Int("max-per-type", 30, "max examples per combat log type (all replays)")
	maxPerReplayType := flag.Int("max-per-replay-type", 10, "max combat log examples per type per replay")
	maxPerClass := flag.Int("max-per-class", 5, "max entity examples per class (all replays, unique idx)")
	maxPerReplayClass := flag.Int("max-per-replay-class", 3, "max entity examples per class per replay")
	var replays replayList
	flag.Var(&replays, "replay", "replay .dem path (repeatable)")
	flag.Parse()

	replayPaths := collectReplayPaths(replays, flag.Args())
	if len(replayPaths) == 0 {
		fmt.Fprintf(os.Stderr, "usage: %s [-out dir] [-replay path.dem ...] [path.dem ...]\n", os.Args[0])
		os.Exit(2)
	}

	combatDir := filepath.Join(*out, "combat_logs")
	entityDir := filepath.Join(*out, "entities")

	combat := newCombatLogCollector(*maxPerType, *maxPerReplayType)
	entities := newEntityCollector(*maxPerClass, *maxPerReplayClass)

	for _, replay := range replayPaths {
		log.Printf("parsing %s", replay)
		if err := processReplay(replay, combat, entities); err != nil {
			log.Fatalf("parse %s: %v", replay, err)
		}
	}

	combatWritten, err := combat.write(combatDir)
	if err != nil {
		log.Fatalf("write combat logs: %v", err)
	}
	entityWritten, err := entities.write(entityDir)
	if err != nil {
		log.Fatalf("write entities: %v", err)
	}

	combatSummaryPath := filepath.Join(*out, "combat_logs_summary.txt")
	cf, err := os.Create(combatSummaryPath)
	if err != nil {
		log.Fatalf("combat summary: %v", err)
	}
	writeCombatLogSummary(cf, combat.total)
	cf.Close()

	entitySummaryPath := filepath.Join(*out, "entities_summary.txt")
	ef, err := os.Create(entitySummaryPath)
	if err != nil {
		log.Fatalf("entity summary: %v", err)
	}
	writeEntitySummary(ef, entities.total)
	ef.Close()

	log.Printf("wrote %d combat log files -> %s", combatWritten, combatDir)
	log.Printf("wrote %d entity class files -> %s", entityWritten, entityDir)
	log.Printf("summaries: %s, %s", combatSummaryPath, entitySummaryPath)
}
