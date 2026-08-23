// Command lasthits prints a heuristic quality report for missed last-hit detection.
// Not part of the build gate — edit cases.go freely.
//
// Usage (from parser/):
//
//	go run ./cmd/quality/lasthits/
//	go run ./cmd/quality/lasthits/ -replay 8943058440 -hero Zuus -from 2:15 -to 2:17
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dotabuff/manta"

	"dota2/internal/common"
	"dota2/internal/lasthits"
	"dota2/internal/timeandpauses"
)

func main() {
	log.SetOutput(os.Stderr)

	replay := flag.String("replay", "", "filter: match ID")
	hero := flag.String("hero", "", "filter: hero name (substring, case-insensitive)")
	from := flag.String("from", "", "filter: window start (M:SS or seconds); case must overlap")
	to := flag.String("to", "", "filter: window end (M:SS or seconds); case must overlap")
	tag := flag.String("tag", "", "filter: require this case tag")
	label := flag.String("label", "", "filter: label substring (case-insensitive)")
	flag.Parse()

	cases, err := filterQualityCases(qualityCases, caseFilter{
		Replay: *replay,
		Hero:   *hero,
		From:   *from,
		To:     *to,
		Tag:    *tag,
		Label:  *label,
	})
	if err != nil {
		log.Fatal(err)
	}
	if len(cases) == 0 {
		log.Fatal("no quality cases matched filters")
	}

	start := time.Now()
	results, err := runQualityCases(cases)
	if err != nil {
		log.Fatal(err)
	}
	writeReport(os.Stdout, results, time.Since(start))
}

type caseFilter struct {
	Replay string
	Hero   string
	From   string
	To     string
	Tag    string
	Label  string
}

func filterQualityCases(cases []qualityCase, f caseFilter) ([]qualityCase, error) {
	var fromSec, toSec float32
	var err error
	if f.From != "" {
		fromSec, err = parseFilterClock(f.From)
		if err != nil {
			return nil, fmt.Errorf("-from: %w", err)
		}
	}
	if f.To != "" {
		toSec, err = parseFilterClock(f.To)
		if err != nil {
			return nil, fmt.Errorf("-to: %w", err)
		}
	}
	if f.From != "" && f.To != "" && fromSec > toSec {
		return nil, fmt.Errorf("-from (%s) is after -to (%s)", f.From, f.To)
	}

	heroNeedle := strings.ToLower(strings.TrimSpace(f.Hero))
	labelNeedle := strings.ToLower(strings.TrimSpace(f.Label))
	tagNeedle := CaseTag(strings.TrimSpace(f.Tag))

	out := make([]qualityCase, 0, len(cases))
	for _, c := range cases {
		if f.Replay != "" && c.ReplayHero.Replay.ID != f.Replay {
			continue
		}
		if heroNeedle != "" && !strings.Contains(strings.ToLower(c.ReplayHero.Hero), heroNeedle) {
			continue
		}
		if labelNeedle != "" && !strings.Contains(strings.ToLower(c.Label), labelNeedle) {
			continue
		}
		if tagNeedle != "" && !hasAllTags(c.Tags, tagNeedle) {
			continue
		}
		if f.From != "" || f.To != "" {
			if f.From != "" && f.To != "" {
				// both bounds: case window must lie inside [from, to]
				if c.From < fromSec || c.To > toSec {
					continue
				}
			} else if f.From != "" {
				if c.To < fromSec {
					continue
				}
			} else if c.From > toSec {
				continue
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// parseFilterClock accepts "M:SS", "M:SS.ss", or plain seconds.
func parseFilterClock(s string) (float32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty time")
	}
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		if len(parts) != 2 {
			return 0, fmt.Errorf("want M:SS, got %q", s)
		}
		min, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("minutes: %w", err)
		}
		sec, err := strconv.ParseFloat(parts[1], 32)
		if err != nil {
			return 0, fmt.Errorf("seconds: %w", err)
		}
		return float32(min)*60 + float32(sec), nil
	}
	sec, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0, err
	}
	return float32(sec), nil
}

func runQualityCases(cases []qualityCase) ([]caseResult, error) {
	untilByHero := lastCaseTimeByHero(cases)
	cache := make(map[*ReplayHero][]lasthits.Event)

	results := make([]caseResult, len(cases))
	for i, c := range cases {
		rh := c.ReplayHero
		misses, ok := cache[rh]
		if !ok {
			replayPath, err := resolveReplayPath(rh.Replay.ID)
			if err != nil {
				return nil, fmt.Errorf("case %q: %w", c.Label, err)
			}
			misses, err = parseMissedEvents(rh.Hero, replayPath, untilByHero[rh])
			if err != nil {
				return nil, fmt.Errorf("case %q: %w", c.Label, err)
			}
			cache[rh] = misses
		}
		results[i] = evaluateCase(c, misses)
	}
	return results, nil
}

func lastCaseTimeByHero(cases []qualityCase) map[*ReplayHero]float32 {
	until := make(map[*ReplayHero]float32)
	for _, c := range cases {
		if c.To > until[c.ReplayHero] {
			until[c.ReplayHero] = c.To
		}
	}
	return until
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
		filepath.Join("..", "dota-replays", name),                   // cwd: parser/
		filepath.Join("..", "..", "..", "..", "dota-replays", name), // cwd: parser/cmd/quality/lasthits/
	} {
		if _, err := os.Stat(base); err == nil {
			return base, nil
		}
	}
	return "", fmt.Errorf("replay %s not found (fetch: ruby dota-replays/fetch.rb)", name)
}

func parseMissedEvents(heroName, replayPath string, until float32) ([]lasthits.Event, error) {
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
	tp := timeandpauses.NewHandlerWithStopTime(until)
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
