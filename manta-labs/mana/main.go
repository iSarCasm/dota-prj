package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

type HeroRef struct {
	ClassName string
	EntityIdx uint32
}

type ManaSnapshot struct {
	Tick        uint32
	Time        float32
	Mana        float32
	MaxMana     float32
	ManaPercent float32
}

func guessHeroClassFromNPC(npc string) string {
	const prefix = "npc_dota_hero_"
	if !strings.HasPrefix(npc, prefix) {
		return ""
	}
	raw := strings.TrimPrefix(npc, prefix) // e.g. "zuus" or "keeper_of_the_light"
	parts := strings.Split(raw, "_")
	var b strings.Builder
	b.WriteString("CDOTA_Unit_Hero_")
	for _, p2 := range parts {
		if p2 == "" {
			continue
		}
		r := []rune(p2)
		r[0] = unicode.ToUpper(r[0])
		b.WriteString(string(r))
	}
	return b.String()
}

// heroNameToClass maps a hero name (e.g. "Puck", "keeper_of_the_light") to CDOTA_Unit_Hero_* class name.
func heroNameToClass(heroName string) string {
	heroName = strings.TrimSpace(strings.ReplaceAll(heroName, " ", "_"))
	if heroName == "" {
		return ""
	}
	parts := strings.Split(heroName, "_")
	titled := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(strings.ToLower(p))
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
			titled = append(titled, string(r))
		}
	}
	return "CDOTA_Unit_Hero_" + strings.Join(titled, "_")
}

func main() {
	if len(os.Args) != 5 {
		log.Fatalf("usage: %s <match_id> <hero_name> <replay_path> <output_dir>", os.Args[0])
	}
	matchID := os.Args[1]
	heroName := os.Args[2]
	replayPath := os.Args[3]
	outputDir := os.Args[4]

	manaTickInterval := 30 // record mana every 30 ticks

	heroClass := heroNameToClass(heroName)
	if heroClass == "" {
		log.Fatalf("invalid hero name: %q", heroName)
	}

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

	// Used to convert parser ticks -> seconds for entity events.
	// Will be populated from CSVCMsg_ServerInfo when available.
	tickInterval := float32(0.033333335) // fallback (30 ticks/sec)
	p.Callbacks.OnCSVCMsg_ServerInfo(func(m *dota.CSVCMsg_ServerInfo) error {
		if ti := m.GetTickInterval(); ti > 0 {
			tickInterval = ti
			log.Printf("Tick interval: %f", tickInterval)
		}
		return nil
	})

	playerIDToHero := make(map[uint32]HeroRef, 16)

	allManaSnapshots := make([]*ManaSnapshot, 0, 1024)

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		entityTick := p.Tick
		entityTime := float32(entityTick) * tickInterval

		if e == nil {
			return nil
		}
		cn := e.GetClassName()

		// Build playerID -> hero mapping from hero entities.
		// This is what you want for translating item.m_iPlayerOwnerID -> hero.
		if strings.HasPrefix(cn, "CDOTA_Unit_Hero_") {
			if pidAny, ok := e.Map()["m_iPlayerID"]; ok {
				if pid, ok := pidAny.(uint32); ok {
					playerIDToHero[pid] = HeroRef{
						ClassName: cn,
						EntityIdx: uint32(e.GetIndex()),
					}
				}
			}
		}

		if cn == heroClass {
			maxMana, ok := e.GetFloat32("m_flMaxMana")
			if !ok {
				log.Printf("%s mana: missing m_flMaxMana", heroClass)
				return nil
			}
			if entityTick%uint32(manaTickInterval) != 0 {
				return nil
			}

			mana, ok := e.GetFloat32("m_flMana")
			if !ok {
				log.Printf("%s mana: missing m_flMana", heroClass)
				return nil
			}
			s := &ManaSnapshot{Tick: entityTick, Time: entityTime, Mana: mana, MaxMana: maxMana, ManaPercent: mana / maxMana * 100}
			allManaSnapshots = append(allManaSnapshots, s)
			// log.Printf("Puck mana: %f / %f (%.2f%%)", mana, maxMana, mana/maxMana*100)
			return nil
		}

		return nil
	})

	// IMPORTANT: actually check Start() error
	if err := p.Start(); err != nil && err != io.EOF {
		log.Fatalf("parse error: %v", err)
	}

	// Build mana as array of arrays: [ [tick, time, mana, max_mana, mana_percent], ... ]
	manaRows := make([][]interface{}, 0, len(allManaSnapshots))
	for _, s := range allManaSnapshots {
		manaRows = append(manaRows, []interface{}{s.Tick, s.Time, s.Mana, s.MaxMana, s.ManaPercent})
	}
	out := map[string]interface{}{"mana": manaRows}

	jsonPath := filepath.Join(outputDir, matchID+"_output.json")
	fJSON, err := os.Create(jsonPath)
	if err != nil {
		log.Fatalf("create %s: %v", jsonPath, err)
	}
	defer fJSON.Close()

	enc := json.NewEncoder(fJSON)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		log.Fatalf("encode JSON: %v", err)
	}

	log.Printf("Parse complete: match_id=%s hero=%s -> %s (%d rows)", matchID, heroName, jsonPath, len(allManaSnapshots))
}
