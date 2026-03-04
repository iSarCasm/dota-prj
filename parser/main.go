package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/abilities"
	"dota2/internal/common"
	"dota2/internal/mana"
	"dota2/internal/pt"
	"dota2/internal/strategic_states"
	"dota2/internal/timeandpauses"
)

func main() {
	if len(os.Args) != 5 {
		log.Fatalf("usage: %s <match_id> <hero_name> <replay_path> <output_dir>", os.Args[0])
	}
	matchID := os.Args[1]
	heroName := os.Args[2]
	replayPath := os.Args[3]
	outputDir := os.Args[4]

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("mkdir output dir: %v", err)
	}

	f, err := os.Open(replayPath)
	if err != nil {
		log.Fatalf("open replay: %v", err)
	}
	defer f.Close()

	p, err := manta.NewStreamParser(f)
	if err != nil {
		log.Fatalf("NewStreamParser: %v", err)
	}

	ctx := &common.ParseContext{
		MatchID:        matchID,
		OutputDir:      outputDir,
		HeroName:       heroName,
		ReplayPath:     replayPath,
		PlayerIDToHero: make(map[uint32]common.HeroRef, 16),
	}

	// Common callbacks (must run first)
	tickInterval := float32(0.033333335)
	p.Callbacks.OnCSVCMsg_ServerInfo(func(m *dota.CSVCMsg_ServerInfo) error {
		if ti := m.GetTickInterval(); ti > 0 {
			tickInterval = ti
			ctx.TickInterval = ti
			log.Printf("Tick interval: %f", tickInterval)
		}
		return nil
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil {
			return nil
		}
		cn := e.GetClassName()

		if strings.HasPrefix(cn, "CDOTA_Unit_Hero_") {
			if pidAny, ok := e.Map()["m_iPlayerID"]; ok {
				if pid, ok := pidAny.(uint32); ok {
					ctx.PlayerIDToHero[pid] = common.HeroRef{
						ClassName: cn,
						EntityIdx: uint32(e.GetIndex()),
					}
				}
			}
		}
		return nil
	})

	// Handlers (mana and abilities before PT so PT can take references)
	timeAndPausesHandler := timeandpauses.NewHandler()
	manaHandler := mana.NewHandler(0, 15, timeAndPausesHandler)
	abilitiesHandler := abilities.NewHandler(timeAndPausesHandler)
	strategicStatesHandler := strategic_states.NewHandler(timeAndPausesHandler)
	// PT needs to know about abilities and mana to be able to make insights
	ptHandler := pt.NewHandler(abilitiesHandler, manaHandler)

	replayHandlers := []common.ReplayHandler{timeAndPausesHandler, manaHandler, abilitiesHandler, strategicStatesHandler, ptHandler}

	for _, h := range replayHandlers {
		if err := h.Init(ctx); err != nil {
			log.Fatalf("handler init: %v", err)
		}
		h.RegisterCallbacks(p, ctx)
	}

	// Populate ctx.TickInterval for handlers that run before ServerInfo (fallback)
	if ctx.TickInterval == 0 {
		ctx.TickInterval = tickInterval
	}

	if err := p.Start(); err != nil && err != io.EOF {
		log.Fatalf("parse error: %v", err)
	}

	out := map[string]interface{}{
		"heroName": ctx.HeroName,
	}
	var insights []common.Insight
	for _, h := range replayHandlers {
		for k, v := range h.Output(ctx) {
			if k == "insights" {
				if arr, ok := v.([]common.Insight); ok {
					insights = append(insights, arr...)
				}
			} else {
				out[k] = v
			}
		}
	}
	out["insights"] = insights

	if jsonPath, err := saveJsonOutput(out, matchID, outputDir); err == nil {
		log.Printf("Parse complete: match_id=%s hero=%s -> %s", matchID, heroName, jsonPath)
	} else {
		log.Fatalf("save json output: %v", err)
	}
}

func saveJsonOutput(out map[string]interface{}, matchID string, outputDir string) (string, error) {
	jsonPath := filepath.Join(outputDir, matchID+"_output.json")
	var outFile *os.File
	outFile, err := os.Create(jsonPath)
	if err != nil {
		return "", fmt.Errorf("create output file: %v", err)
	}
	defer outFile.Close()
	enc := json.NewEncoder(outFile)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return "", fmt.Errorf("encode output: %v", err)
	}
	return jsonPath, nil
}
