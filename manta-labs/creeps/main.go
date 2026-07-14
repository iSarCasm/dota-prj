package main

import (
	"io"
	"log"
	"math/rand"
	"os"
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

// GemDota implementation of getLocation
func getLocation(e *manta.Entity) (float32, float32) {
	x, _ := e.GetUint64("CBodyComponent.m_cellX")
	y, _ := e.GetUint64("CBodyComponent.m_cellY")
	vx, _ := e.GetFloat32("CBodyComponent.m_vecVelocity.x")
	vy, _ := e.GetFloat32("CBodyComponent.m_vecVelocity.y")
	locX := float32(x)*128 + vx - 16384
	locY := float32(y)*128 + vy - 16384
	return locX, locY
}

func getLocation2(e *manta.Entity) (float32, float32) {
	x, _ := e.GetUint64("CBodyComponent.m_cellX")
	y, _ := e.GetUint64("CBodyComponent.m_cellY")
	vx, _ := e.GetFloat32("CBodyComponent.m_vecVelocity.x")
	vy, _ := e.GetFloat32("CBodyComponent.m_vecVelocity.y")
	locX := (float32(x)*128 + vx) / 128
	locY := (float32(y)*128 + vy) / 128
	return locX, locY
}

func creepTypeFromTargetName(targetName string) string {
	targetName = strings.ToLower(strings.TrimSpace(targetName))
	if targetName == "" {
		return ""
	}
	if strings.HasPrefix(targetName, "npc_dota_neutral_") {
		return "jungle"
	}
	if strings.HasPrefix(targetName, "npc_dota_creep_goodguys_") || strings.HasPrefix(targetName, "npc_dota_creep_badguys_") {
		return "lane"
	}
	if strings.HasPrefix(targetName, "npc_dota_creep_siege") {
		return "lane"
	}
	return ""
}

func main() {
	// f, err := os.Open("../replay1.dem")
	f, err := os.Open("/Users/igortsykalo/workspace/dota2/dota-web/storage/replays/8676648471.dem")
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer f.Close()

	// Quick sanity: file size
	if st, err := f.Stat(); err == nil {
		log.Printf("Replay size: %d bytes", st.Size())
	}

	p, err := manta.NewStreamParser(f)
	if err != nil {
		log.Fatalf("NewStreamParser: %v", err)
	}

	// map entityId -> [health, name]
	entityIdToHealthName := make(map[uint64]map[string]interface{}, 0)
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil {
			return nil
		}

		className := e.GetClassName()

		if className != "CDOTA_BaseNPC_Creep_Lane" {
			return nil
		}

		// entityId := e.GetIndex()
		entityId2, _ := e.GetUint64("m_nEntityId")
		// log.Printf("Entity: %s, EntityId: %d, EntityId2: %d", className, entityId, entityId2)
		health, _ := e.GetInt32("m_iHealth")
		nameIndex, _ := e.GetInt32("m_pEntity.m_nameStringableIndex")
		nameIndex2, _ := e.GetInt32("m_iUnitNameIndex")
		name, ok := p.LookupStringByIndex("EntityNames", nameIndex)
		name2, ok2 := p.LookupStringByIndex("EntityNames", nameIndex2)

		// all string tables
		// for k, v := range p.GetStringTables() {
		// 	log.Printf("String Table: %s, Index: %d", k, v.GetIndex())
		// }

		if !ok && !ok2 {
			// log.Printf("Failed to lookup name for entityId: %d", entityId2)
			return nil
		}

		x, _ := e.GetUint64("CBodyComponent.m_cellX")
		y, _ := e.GetUint64("CBodyComponent.m_cellY")
		eId := e.GetIndex()
		log.Printf("Name: %s, Name2: %s, className: %s, index1: %d, index2: %d (X: %d, Y: %d), eId: %d", name, name2, className, nameIndex, nameIndex2, x, y, eId)

		if !strings.HasPrefix(name, "npc_dota_creep_") {
			return nil
		}

		// Early exit to avoid parsing the entire replay
		if rand.Intn(200000) == 0 {
			os.Exit(1)
		}

		entityIdToHealthName[entityId2] = map[string]interface{}{
			"health": health,
			"name":   name,
		}
		return nil
	})

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		return nil
		attackerNameIdx := m.GetAttackerName()
		targetNameIdx := m.GetTargetName()
		realAttackerName, okA := p.LookupStringByIndex("CombatLogNames", int32(attackerNameIdx))
		if !okA {
			return nil
		}
		realTargetName, okT := p.LookupStringByIndex("CombatLogNames", int32(targetNameIdx))
		if !okT {
			return nil
		}

		ctype := m.GetType()

		if ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE {
			if creepTypeFromTargetName(realTargetName) == "" {
				return nil
			}

			entityId := uint64(0)
			for k, v := range entityIdToHealthName {
				vHealth, ok := v["health"].(int32)
				if !ok {
					log.Printf("Failed to get health for entityId: %d", k)
					continue
				}
				if vHealth == m.GetHealth() {
					entityId = k
					break
				}
			}

			if entityId == 0 {
				log.Printf("Failed to find entityId for damage: %s -> %s (timestamp: %f, health: %d, value: %d)", realAttackerName, realTargetName, m.GetTimestamp(), m.GetHealth(), m.GetValue())
				for k, v := range entityIdToHealthName {
					log.Printf("EntityId: %d, Health: %d, Name: %s", k, v["health"], v["name"])
				}
				return nil
			}

			log.Printf("Damage: %s -> %s (timestamp: %f, health: %d, value: %d) = %d", realAttackerName, realTargetName, m.GetTimestamp(), m.GetHealth(), m.GetValue(), entityId)
		}

		if ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DEATH {
			if creepTypeFromTargetName(realTargetName) == "" {
				return nil
			}

			log.Printf("Death: %s -> %s (timestamp: %f, health: %d, value: %d)", realAttackerName, realTargetName, m.GetTimestamp(), m.GetHealth(), m.GetValue())
		}

		return nil
	})

	// IMPORTANT: actually check Start() error
	if err := p.Start(); err != nil && err != io.EOF {
		log.Fatalf("parse error: %v", err)
	}
	log.Printf("Parse Complete!")
}
