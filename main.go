package main

import (
	"io"
	"log"
	"os"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

func main() {
	f, err := os.Open("replay1.dem")
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

	// Map to store entity index -> ability name
	// Note: This is a simplified approach. In practice, extracting ability names
	// from PacketEntities can be complex and may require additional parsing.
	// entityToAbilityName := make(map[uint32]string)
	// var entityMapMutex sync.RWMutex

	// Build entity index to ability name mapping
	// Note: Extracting ability names from PacketEntities in manta v1.4.7 is complex
	// because PacketEntities contains encoded entity deltas. A practical workaround
	// is to use entity indices directly from combat log entries.

	// Discovery mode: Set to true to log all ability casts for debugging
	discoveryMode := false
	if discoveryMode {
		log.Printf("Discovery mode: Logging all ability casts")
	}

	// 1) Heartbeat: ticks
	ticks := 0
	p.Callbacks.OnCNETMsg_Tick(func(m *dota.CNETMsg_Tick) error {
		ticks++
		if ticks == 1 || ticks%3000 == 0 { // every ~3000 ticks just to prove progress
			log.Printf("tick=%d", m.GetTick())
		}
		return nil
	})

	// 2) Heartbeat: count *any* usermessages
	userMsgs := 0
	p.Callbacks.OnCUserMessageSayText2(func(m *dota.CUserMessageSayText2) error {
		userMsgs++
		log.Printf("[SayText2] %+v", m)
		return nil
	})

	// 3) Dota chat paths (often used instead of SayText2)
	chatMsg := 0
	p.Callbacks.OnCDOTAUserMsg_ChatMessage(func(m *dota.CDOTAUserMsg_ChatMessage) error {
		chatMsg++
		log.Printf("[ChatMessage] %+v", m)
		return nil
	})

	// chatEvent := 0
	// p.Callbacks.OnCDOTAUserMsg_ChatEvent(func(m *dota.CDOTAUserMsg_ChatEvent) error {
	// 	chatEvent++
	// 	log.Printf("[ChatEvent] %+v", m)
	// 	return nil
	// })

	// Counter for filtered abilities
	qopScreamCount := 0

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		ctype := m.GetType()
		inflictorName := m.GetInflictorName() // Entity index of the ability/item
		damageSourceName := m.GetDamageSourceName()
		targetName := m.GetTargetName()

		// Filter for ability casts (QoP Scream is typically DOTA_COMBATLOG_ABILITY or DOTA_COMBATLOG_DAMAGE)
		if ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ABILITY ||
			ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE ||
			ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ITEM {

			// Look up ability names using stringtables
			inflictorAbilityName, inflictorOk := p.LookupStringByIndex("CombatLogNames", int32(inflictorName))
			damageSourceAbilityName, _ := p.LookupStringByIndex("CombatLogNames", int32(damageSourceName))
			targetAbilityName, _ := p.LookupStringByIndex("CombatLogNames", int32(targetName))

			// Check if either inflictor or damage source is QoP Scream
			isFiletered := false
			// QoP Scream
			// qopScreamAbilityName := "queenofpain_scream_of_pain"
			// if inflictorOk && inflictorAbilityName == qopScreamAbilityName {
			// 	isFiletered = true
			// }
			// if damageSourceOk && damageSourceAbilityName == qopScreamAbilityName {
			// 	isFiletered = true
			// }
			// PT switch
			if inflictorOk && inflictorAbilityName == "item_power_treads" {
				isFiletered = true
			}
			// attackerName := m.GetAttackerName()
			// attackerStringName, attackerOk := p.LookupStringByIndex("CombatLogNames", int32(attackerName))
			// if attackerOk && attackerStringName == "npc_dota_hero_puck" && inflictorOk && inflictorAbilityName != "dota_unknown" {
			// 	isFiletered = true
			// }

			if isFiletered {
				qopScreamCount++
				timestamp := m.GetTimestamp()
				attackerName := m.GetAttackerName()

				log.Printf("[Filtered Ability #%d] Type=%d, Timestamp=%.2f, Attacker=%d, Inflictor=%d (%s), DamageSource=%d (%s), Target=%d (%s)",
					qopScreamCount, ctype, timestamp, attackerName, inflictorName, inflictorAbilityName, damageSourceName, damageSourceAbilityName, targetName, targetAbilityName)
			}
		}

		return nil
	})

	// IMPORTANT: actually check Start() error
	if err := p.Start(); err != nil && err != io.EOF {
		log.Fatalf("parse error: %v", err)
	}

	log.Printf("Parse Complete!")
	log.Printf("Total QoP Scream ability instances found: %d", qopScreamCount)
}
