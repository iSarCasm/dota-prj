package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"unicode"

	"github.com/davecgh/go-spew/spew"
	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

type PTUsage struct {
	Timestamp float32
	Hero      string
	Attacker  uint32
	Inflictor uint32
}

type HeroRef struct {
	ClassName string
	EntityIdx uint32
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

// Returns the name of the calling function
func _caller(n int) string {
	if pc, _, _, ok := runtime.Caller(n); ok {
		fns := strings.Split(runtime.FuncForPC(pc).Name(), "/")
		return fns[len(fns)-1]
	}

	return "unknown"
}

// dump named object
func _dump(w io.Writer, label string, args ...interface{}) {
	fmt.Fprintf(w, "%s: %s", _caller(2), label)
	// spew.Dump(args...)
	spew.Fdump(w, args...)
}

// Dump prints the current entity state to standard output
func fDump(w io.Writer, e *manta.Entity) {
	_dump(w, e.String(), e.Map())
}

var PT_I_STAT_STRING = []string{"str", "int", "agi"}

type PuckSnapshot struct {
	Tick      uint32
	Time      float32
	MaxHealth int32
	MaxMana   float32
	HasHealth bool
	HasMana   bool
}

func main() {

	f, err := os.Open("../replay1.dem")
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer f.Close()

	// Dump filtered combat log entries (full protobuf contents)
	fCombatLogDump, err := os.Create("combatlog_dump.log")
	if err != nil {
		log.Fatalf("create combatlog_dump.log: %v", err)
	}
	defer fCombatLogDump.Close()

	// Combine Event Dump with combat dump
	fCombinedDump, err := os.Create("combined_dump.log")
	if err != nil {
		log.Fatalf("create combined_dump.log: %v", err)
	}
	defer fCombinedDump.Close()

	// Quick sanity: file size
	if st, err := f.Stat(); err == nil {
		log.Printf("Replay size: %d bytes", st.Size())
	}

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

	pt_usages := make([]PTUsage, 0, 256)
	playerIDToHero := make(map[uint32]HeroRef, 16)

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		ctype := m.GetType()
		inflictorName := m.GetInflictorName() // Entity index of the ability/item
		damageSourceName := m.GetDamageSourceName()
		targetName := m.GetTargetName()

		// Filter for ability casts (QoP Scream is typically DOTA_COMBATLOG_ABILITY or DOTA_COMBATLOG_DAMAGE)
		if ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ITEM {
			// Look up ability names using stringtables
			inflictorAbilityName, inflictorOk := p.LookupStringByIndex("CombatLogNames", int32(inflictorName))
			damageSourceAbilityName, _ := p.LookupStringByIndex("CombatLogNames", int32(damageSourceName))
			targetAbilityName, _ := p.LookupStringByIndex("CombatLogNames", int32(targetName))

			isFiletered := false
			// PT switch
			if inflictorOk && inflictorAbilityName == "item_power_treads" {
				isFiletered = true
			}

			if isFiletered {
				timestamp := m.GetTimestamp()
				attackerName := m.GetAttackerName()
				realAttackerName, _ := p.LookupStringByIndex("CombatLogNames", int32(attackerName))

				pt_usage := PTUsage{
					Timestamp: timestamp - 10, // 10 secs is prob the start game screen
					Hero:      realAttackerName,
					Attacker:  attackerName,
					Inflictor: inflictorName,
				}
				pt_usages = append(pt_usages, pt_usage)
				// log.Printf("PT Usage: %+v", pt_usage)

				// log.Printf("[Filtered Ability] Type=%d, Timestamp=%.2f, Attacker=%d, Inflictor=%d (%s), DamageSource=%d (%s), Target=%d (%s)",
				// 	ctype, timestamp, attackerName, inflictorName, inflictorAbilityName, damageSourceName, damageSourceAbilityName, targetName,
				// 	targetAbilityName)

				// Full dump (like entity dumps): header + entire CMsgDOTACombatLogEntry
				fmt.Fprintf(
					fCombatLogDump,
					"\n=== CMsgDOTACombatLogEntry ===\nType=%v Timestamp=%.4f Attacker=%d Inflictor=%d (%s) DamageSource=%d (%s) Target=%d (%s)\n",
					ctype,
					timestamp,
					attackerName,
					inflictorName, inflictorAbilityName,
					damageSourceName, damageSourceAbilityName,
					targetName, targetAbilityName,
				)
				spew.Fdump(fCombatLogDump, m)
				spew.Fdump(fCombinedDump, m)
			}
		}

		return nil
	})

	// (string) (len=13) "m_hItems.0000": (uint32) 9340920,
	// (string) (len=13) "m_hItems.0001": (uint32) 3574043,
	// (string) (len=13) "m_hItems.0002": (uint32) 4246443,
	// (string) (len=13) "m_hItems.0003": (uint32) 7308954,
	// (string) (len=13) "m_hItems.0004": (uint32) 6671105,
	// (string) (len=13) "m_hItems.0005": (uint32) 15354432,
	// (string) (len=13) "m_hItems.0006": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0007": (uint32) 13339623,
	// (string) (len=13) "m_hItems.0008": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0009": (uint32) 5491033,
	// (string) (len=13) "m_hItems.0010": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0011": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0012": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0013": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0014": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0015": (uint32) 5130626,
	// (string) (len=13) "m_hItems.0016": (uint32) 838124,
	// (string) (len=13) "m_hItems.0017": (uint32) 4295331,
	// (string) (len=13) "m_hItems.0018": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0019": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0020": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0021": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0022": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0023": (uint32) 16777215,
	// (string) (len=13) "m_hItems.0024": (uint32) 16777215,
	listToFind := []uint32{9340920, 3574043, 4246443, 7308954, 6671105, 15354432, 16777215, 13339623, 16777215, 5491033, 16777215, 16777215, 16777215, 16777215, 16777215, 5130626, 838124, 4295331, 16777215, 16777215, 16777215, 16777215, 16777215, 16777215, 4571519}
	foundHandleMap := make(map[uint32]string)

	var puckPrev *PuckSnapshot
	var puckCur *PuckSnapshot

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

		if cn == "CDOTA_Item_PowerTreads" {
			ownerID := uint32(0)
			if v, ok := e.GetUint32("m_iPlayerOwnerID"); ok {
				ownerID = v
			} else {
				log.Printf("PT OnEntity tick=%d: missing m_iPlayerOwnerID on %s", entityTick, e.String())
			}

			ownerHero := ""
			if hr, ok := playerIDToHero[ownerID]; ok {
				ownerHero = hr.ClassName
			}

			// log.Printf("Found Power Treads entity %d: %s (m_iPlayerOwnerID=%d hero=%s)", e.GetIndex(), e.String(), ownerID, ownerHero)
			spew.Fdump(fCombinedDump, e.Map())

			// lets check for any PT usages in the last 0.1 second for the same hero
			for _, pt_usage := range pt_usages {
				heroClassName := guessHeroClassFromNPC(pt_usage.Hero)

				toAttr, okAttr := e.GetInt32("m_iStat")
				assembledAt, okAssembledAt := e.GetFloat32("m_flAssembledTime")

				// NOTE: don't index PT_I_STAT_STRING unless we validated bounds.
				attrString := "unknown"
				if okAttr && toAttr >= 0 && int(toAttr) < len(PT_I_STAT_STRING) {
					attrString = PT_I_STAT_STRING[int(toAttr)]
				}

				// Only attempt correlation if we have assembled time.
				if heroClassName == ownerHero && okAssembledAt && pt_usage.Timestamp > assembledAt-0.01 {
					if okAttr {
						log.Printf("%s: uses Power Treads at %.3f changing attribute to %s (ticktime %.3f)", ownerHero, pt_usage.Timestamp, attrString, entityTime)
					} else {
						log.Printf("%s: uses Power Treads at %.3f (ticktime %.3f) [m_iStat missing]", ownerHero, pt_usage.Timestamp, entityTime)
					}

					// IMPORTANT: max health/mana are hero fields, not item fields.
					if ownerHero == "CDOTA_Unit_Hero_Puck" && puckPrev != nil && puckCur != nil && puckPrev.HasHealth && puckCur.HasHealth && puckPrev.HasMana && puckCur.HasMana {
						log.Printf("Puck snapshot delta: maxHealth %d -> %d, maxMana %.3f -> %.3f (prevT=%.3fs curT=%.3fs)",
							puckPrev.MaxHealth, puckCur.MaxHealth,
							puckPrev.MaxMana, puckCur.MaxMana,
							puckPrev.Time, puckCur.Time,
						)
					}
				}
			}
		}

		if cn == "CDOTA_Unit_Hero_Puck" {
			puckPrev = puckCur

			s := &PuckSnapshot{Tick: entityTick, Time: entityTime}
			if v, ok := e.GetInt32("m_iMaxHealth"); ok {
				s.MaxHealth = v
				s.HasHealth = true
			}
			if v, ok := e.GetFloat32("m_flMaxMana"); ok {
				s.MaxMana = v
				s.HasMana = true
			}
			puckCur = s

			// if s.HasHealth && s.HasMana {
			// 	log.Printf("Puck updated (ticktime %.3f): maxHealth=%d maxMana=%.3f", entityTime, s.MaxHealth, s.MaxMana)
			// } else {
			// 	log.Printf("Puck updated (ticktime %.3f): hasMaxHealth=%v hasMaxMana=%v", entityTime, s.HasHealth, s.HasMana)
			// }
		}

		for _, handle := range listToFind {
			fe := p.FindEntityByHandle(uint64(handle))
			if fe != nil {
				foundHandleMap[handle] = fe.GetClassName()
			}
		}

		return nil
	})

	// IMPORTANT: actually check Start() error
	if err := p.Start(); err != nil && err != io.EOF {
		log.Fatalf("parse error: %v", err)
	}

	// Print Found Handle Map
	// for handle, className := range foundHandleMap {
	// 	log.Printf("Found handle: %d, class: %s", handle, className)
	// }

	// Print PT Usages
	// for _, pt_usage := range pt_usages {
	// 	log.Printf("PT Usage: %+v", pt_usage)
	// }

	log.Printf("Parse Complete!")
}
