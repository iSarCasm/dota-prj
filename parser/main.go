package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/common"
	"dota2/internal/mana"
	"dota2/internal/pt"
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

		if ctx.GameStartTime == 0 && cn == "CDOTAGamerulesProxy" {
			ctx.GameStartTime, _ = e.GetFloat32("m_pGameRules.m_flGameStartTime")
		}

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

	// Handlers
	manaHandler := mana.NewHandler(30)
	ptHandler := pt.NewHandler()

	for _, h := range []common.ReplayHandler{manaHandler, ptHandler} {
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
		"gameStartTime": ctx.GameStartTime,
		"heroName":      ctx.HeroName,
	}
	for _, h := range []common.ReplayHandler{manaHandler, ptHandler} {
		for k, v := range h.Output(ctx) {
			out[k] = v
		}
	}

	jsonPath := filepath.Join(outputDir, matchID+"_output.json")
	var outFile *os.File
	outFile, err = os.Create(jsonPath)
	if err != nil {
		log.Fatalf("create output file: %v", err)
	}
	defer outFile.Close()
	enc := json.NewEncoder(outFile)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		log.Fatalf("encode output: %v", err)
	}
	log.Printf("Parse complete: match_id=%s hero=%s -> %s", matchID, heroName, jsonPath)
}
