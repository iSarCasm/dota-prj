// Command lasthits prints a heuristic quality report for missed last-hit detection.
// Not part of the build gate — edit cases.go freely.
//
// Usage (from parser/):
//
//	go run ./cmd/quality/lasthits/
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/dotabuff/manta"

	"dota2/internal/common"
	"dota2/internal/lasthits"
	"dota2/internal/timeandpauses"
)

func main() {
	log.SetOutput(os.Stderr)

	results, err := runQualityCases(qualityCases)
	if err != nil {
		log.Fatal(err)
	}
	writeReport(os.Stdout, results)
}

func runQualityCases(cases []qualityCase) ([]caseResult, error) {
	cache := make(map[groupKey][]lasthits.Event)

	results := make([]caseResult, len(cases))
	for i, c := range cases {
		key := groupKey{Replay: c.Replay, Hero: c.Hero}
		misses, ok := cache[key]
		if !ok {
			replayPath, err := resolveReplayPath(c.Replay)
			if err != nil {
				return nil, fmt.Errorf("case %q: %w", c.Label, err)
			}
			misses, err = parseMissedEvents(c.Hero, replayPath)
			if err != nil {
				return nil, fmt.Errorf("case %q: %w", c.Label, err)
			}
			cache[key] = misses
		}
		results[i] = evaluateCase(c, misses)
	}
	return results, nil
}

func resolveReplayPath(matchID string) (string, error) {
	if dir := os.Getenv("DOTA_REPLAYS_DIR"); dir != "" {
		path := filepath.Join(dir, matchID+".dem")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	name := matchID + ".dem"
	for _, base := range []string{
		filepath.Join("..", "dota-replays", name),                     // cwd: parser/
		filepath.Join("..", "..", "..", "..", "dota-replays", name), // cwd: parser/cmd/quality/lasthits/
	} {
		if _, err := os.Stat(base); err == nil {
			return base, nil
		}
	}
	return "", fmt.Errorf("replay %s not found (fetch: ruby dota-replays/fetch.rb)", name)
}

func parseMissedEvents(heroName, replayPath string) ([]lasthits.Event, error) {
	f, err := os.Open(replayPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	p, err := manta.NewStreamParser(f)
	if err != nil {
		return nil, err
	}

	ctx := &common.ParseContext{HeroName: heroName, TickInterval: 0.033333335}
	tp := timeandpauses.NewHandler()
	if err := tp.Init(ctx); err != nil {
		return nil, err
	}
	h := lasthits.NewHandler(tp)
	if err := h.Init(ctx); err != nil {
		return nil, err
	}

	tp.RegisterCallbacks(p, ctx)
	h.RegisterCallbacks(p, ctx)

	oldLog := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(oldLog)

	if err := p.Start(); err != nil && err.Error() != "EOF" {
		return nil, err
	}

	return h.MissedEvents(), nil
}
