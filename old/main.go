package main

import (
	"flag"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

func main() {
	replayPath := flag.String("replay", "my_replay3.dem", "path to .dem file")
	maxPrint := flag.Int("max", 0, "max ability/item events to print (0 = unlimited)")
	printChat := flag.Bool("print_chat", false, "print chat usermessages (SayText2 + DOTA ChatMessage)")
	printCombatLog := flag.Bool("print_combatlog", true, "print ability/item usage from CMsgDOTACombatLogEntry")
	printLegacyCombatLog := flag.Bool("print_legacy_combatlog", false, "print ability/item usage from legacy combatlog game event (noisier; mostly for debugging)")
	printStats := flag.Bool("print_stats", true, "when printing ability/item usage, also try to print attacker/target hp+mana from entity state (best-effort)")
	flag.Parse()

	f, err := os.Open(*replayPath)
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

	// Keep your previous code (chat callbacks) as-is, just optionally gated behind a flag
	// to avoid excessive output.
	userMsgs := 0
	p.Callbacks.OnCUserMessageSayText2(func(m *dota.CUserMessageSayText2) error {
		userMsgs++
		if *printChat {
			log.Printf("[SayText2] %+v", m)
		}
		return nil
	})

	chatMsg := 0
	p.Callbacks.OnCDOTAUserMsg_ChatMessage(func(m *dota.CDOTAUserMsg_ChatMessage) error {
		chatMsg++
		if *printChat {
			log.Printf("[ChatMessage] %+v", m)
		}
		return nil
	})

	// Ability + Item usage is typically emitted via the "combatlog" legacy game event.
	// The event name varies across builds, so we discover it from the event list at runtime.
	printed := 0
	registered := false

	// Best-effort mapping from combatlog unit names (e.g. "npc_dota_hero_zuus")
	// to the current hero Entity, so we can query netprops like m_iHealth/m_flMana.
	heroByNPCName := map[string]*manta.Entity{}
	heroByClassName := map[string]*manta.Entity{}

	guessHeroClassFromNPC := func(npc string) string {
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

	getEntityHPMana := func(e *manta.Entity) (hp, maxHP int32, mana, maxMana float32, ok bool) {
		if e == nil {
			return 0, 0, 0, 0, false
		}
		hpV, okHP := e.GetInt32("m_iHealth")
		maxHPV, okMaxHP := e.GetInt32("m_iMaxHealth")
		manaV, okMana := e.GetFloat32("m_flMana")
		maxManaV, okMaxMana := e.GetFloat32("m_flMaxMana")
		if !(okHP || okMana || okMaxHP || okMaxMana) {
			return 0, 0, 0, 0, false
		}
		return hpV, maxHPV, manaV, maxManaV, true
	}

	resolveHeroEntity := func(npc string) *manta.Entity {
		if npc == "" {
			return nil
		}
		if e := heroByNPCName[npc]; e != nil {
			return e
		}
		if cn := guessHeroClassFromNPC(npc); cn != "" {
			if e := heroByClassName[cn]; e != nil {
				return e
			}
		}
		return nil
	}

	// Maintain our best-effort hero lookup index as entity updates stream in.
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil {
			return nil
		}
		cn := e.GetClassName()
		// log.Printf("Entity: %s", cn)
		// e.Dump()
		if !strings.HasPrefix(cn, "CDOTA_Unit_Hero_") {
			return nil
		}

		// If the hero leaves / is deleted, drop it from our indexes.
		if op.Flag(manta.EntityOpLeft) || op.Flag(manta.EntityOpDeleted) {
			for k, v := range heroByNPCName {
				if v == e {
					delete(heroByNPCName, k)
				}
			}
			if heroByClassName[cn] == e {
				delete(heroByClassName, cn)
			}
			return nil
		}

		// Track last-seen hero entity for this class name (helps for most heroes).
		heroByClassName[cn] = e

		// If the replay exposes the unit name netprop, prefer exact mapping.
		// (Key names can vary across builds; we try a small set.)
		for _, k := range []string{"m_iszUnitName", "m_iszUnitNameString", "m_szUnitName"} {
			if unitName, ok := e.GetString(k); ok && strings.HasPrefix(unitName, "npc_dota_") {
				log.Printf("Unit Name: %s", unitName)
				log.Printf("Entity: %s", e.String())
				heroByNPCName[unitName] = e
				break
			}
		}
		return nil
	})

	safeGetString := func(e *manta.GameEvent, names ...string) string {
		for _, n := range names {
			if v, err := e.GetString(n); err == nil {
				return v
			}
		}
		return ""
	}

	handleCombatLog := func(e *manta.GameEvent) error {
		if !*printLegacyCombatLog {
			return nil
		}

		switch e.Type() {
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ABILITY,
			dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ITEM,
			dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ABILITY_TRIGGER:
			// keep going
		default:
			return nil
		}

		inflictor := safeGetString(e, "inflictorname", "inflictor_name")
		attacker := safeGetString(e, "attackername", "attacker_name", "sourcename", "source_name")
		target := safeGetString(e, "targetname", "target_name")

		// One-line summary (when fields exist) + full dump (always).
		if inflictor != "" || attacker != "" || target != "" {
			log.Printf("[tick=%d] %s attacker=%q target=%q inflictor=%q", p.Tick, e.TypeName(), attacker, target, inflictor)
		}
		log.Print(e.String())

		printed++
		if *maxPrint > 0 && printed >= *maxPrint {
			p.Stop()
		}
		return nil
	}

	// Primary, stable combatlog path: CMsgDOTACombatLogEntry usermessage.
	// This is usually the easiest way to get "ability/item used" across replays.
	lookupCombatLogName := func(idx uint32) string {
		if idx == 0 {
			return ""
		}
		if s, ok := p.LookupStringByIndex("CombatLogNames", int32(idx)); ok {
			return s
		}
		return ""
	}

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		if !*printCombatLog {
			return nil
		}

		switch m.GetType() {
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ABILITY,
			dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ITEM,
			dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ABILITY_TRIGGER:
			// keep going
		default:
			return nil
		}

		attacker := lookupCombatLogName(m.GetAttackerName())
		target := lookupCombatLogName(m.GetTargetName())
		inflictor := lookupCombatLogName(m.GetInflictorName())

		line := strings.Builder{}
		line.WriteString("[tick=")
		line.WriteString((func() string { return strconv.FormatUint(uint64(p.Tick), 10) })())
		line.WriteString(" time=")
		line.WriteString((func() string { return strconv.FormatFloat(float64(m.GetTimestamp()), 'f', 3, 32) })())
		line.WriteString("] ")
		line.WriteString(m.GetType().String())
		line.WriteString(" attacker=")
		line.WriteString(strconv.Quote(attacker))
		line.WriteString(" target=")
		line.WriteString(strconv.Quote(target))
		line.WriteString(" inflictor=")
		line.WriteString(strconv.Quote(inflictor))
		line.WriteString(" value=")
		line.WriteString(strconv.FormatUint(uint64(m.GetValue()), 10))

		if *printStats {
			if aEnt := resolveHeroEntity(attacker); aEnt != nil {
				hp, maxHP, mana, maxMana, ok := getEntityHPMana(aEnt)
				if ok {
					line.WriteString(" attacker_hp=")
					line.WriteString(strconv.FormatInt(int64(hp), 10))
					if maxHP > 0 {
						line.WriteString("/")
						line.WriteString(strconv.FormatInt(int64(maxHP), 10))
					}
					line.WriteString(" attacker_mana=")
					line.WriteString(strconv.FormatFloat(float64(mana), 'f', 1, 32))
					if maxMana > 0 {
						line.WriteString("/")
						line.WriteString(strconv.FormatFloat(float64(maxMana), 'f', 1, 32))
					}
				}
			}
			if tEnt := resolveHeroEntity(target); tEnt != nil {
				hp, maxHP, mana, maxMana, ok := getEntityHPMana(tEnt)
				if ok {
					line.WriteString(" target_hp=")
					line.WriteString(strconv.FormatInt(int64(hp), 10))
					if maxHP > 0 {
						line.WriteString("/")
						line.WriteString(strconv.FormatInt(int64(maxHP), 10))
					}
					line.WriteString(" target_mana=")
					line.WriteString(strconv.FormatFloat(float64(mana), 'f', 1, 32))
					if maxMana > 0 {
						line.WriteString("/")
						line.WriteString(strconv.FormatFloat(float64(maxMana), 'f', 1, 32))
					}
				}
			}
		}

		log.Print(line.String())

		printed++
		if *maxPrint > 0 && printed >= *maxPrint {
			p.Stop()
		}
		return nil
	})

	p.Callbacks.OnCMsgSource1LegacyGameEventList(func(m *dota.CMsgSource1LegacyGameEventList) error {
		// Avoid duplicate registrations if the event list shows up more than once.
		if registered {
			return nil
		}
		registered = true

		var registeredNames []string
		for _, d := range m.GetDescriptors() {
			name := d.GetName()
			if strings.Contains(strings.ToLower(name), "combatlog") {
				registeredNames = append(registeredNames, name)
				p.OnGameEvent(name, handleCombatLog)
			}
		}

		if len(registeredNames) == 0 {
			log.Printf("No combatlog game event found in event list; ability/item usage may not be emitted in this replay/build.")
		} else {
			log.Printf("Registered combatlog handlers for: %v", registeredNames)
		}
		return nil
	})

	// IMPORTANT: actually check Start() error
	if err := p.Start(); err != nil && err != io.EOF {
		log.Fatalf("parse error: %v", err)
	}

	log.Printf("Parse Complete! ticks=%d printed_ability_item_events=%d", p.Tick, printed)
	log.Printf("Chat counters: saytext2=%d chat_message=%d", userMsgs, chatMsg)
}
