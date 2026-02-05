package main

import (
	"fmt"
	"io"
	"log"
	"os"
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

func main() {

	f, err := os.Open("../replay1.dem")
	if err != nil {
		log.Fatalf("open: %v", err)
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

		if cn == "CDOTA_Unit_Hero_Puck" {
			maxMana, ok := e.GetFloat32("m_flMaxMana")
			if !ok {
				log.Printf("Puck mana: missing m_flMaxMana")
				return nil
			}

			mana, ok := e.GetFloat32("m_flMana")
			if !ok {
				log.Printf("Puck mana: missing m_flMana")
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

	// Dump all mana snapshots into a CSV file
	fManaCSV, err := os.Create("mana.csv")
	if err != nil {
		log.Fatalf("create mana.csv: %v", err)
	}
	defer fManaCSV.Close()

	for _, s := range allManaSnapshots {
		fManaCSV.WriteString(fmt.Sprintf("%d,%f,%f,%f,%f\n", s.Tick, s.Time, s.Mana, s.MaxMana, s.ManaPercent))
	}

	log.Printf("Parse Complete!")
}
