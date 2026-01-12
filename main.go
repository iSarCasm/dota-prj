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

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		ctype := m.GetType()
		attackerName := m.GetAttackerName()
		targetName := m.GetTargetName()
		damageSourceName := m.GetDamageSourceName()
		inflictorName := m.GetInflictorName()
		isAttackerIllusion := m.GetIsAttackerIllusion()
		isAttackerHero := m.GetIsAttackerHero()
		isTargetIllusion := m.GetIsTargetIllusion()
		isTargetHero := m.GetIsTargetHero()
		isVisibleRadiant := m.GetIsVisibleRadiant()
		isVisibleDire := m.GetIsVisibleDire()
		value := m.GetValue()
		// Print everything first so you can see what keys/values look like in YOUR version
		log.Printf("CombatLogEntry type=%d attackerName=%d targetName=%d damageSourceName=%d inflictorName=%d isAttackerIllusion=%d isAttackerHero=%d isTargetIllusion=%d isTargetHero=%d isVisibleRadiant=%d isVisibleDire=%d value=%d", ctype, attackerName, targetName, damageSourceName, inflictorName, isAttackerIllusion, isAttackerHero, isTargetIllusion, isTargetHero, isVisibleRadiant, isVisibleDire, value)

		return nil
	})

	// IMPORTANT: actually check Start() error
	if err := p.Start(); err != nil && err != io.EOF {
		log.Fatalf("parse error: %v", err)
	}

	log.Printf("Parse Complete!")
}
