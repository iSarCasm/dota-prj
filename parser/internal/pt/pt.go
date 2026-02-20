package pt

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/davecgh/go-spew/spew"
	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/common"
)

// PTUsage represents a Power Treads switch event from the combat log.
type PTUsage struct {
	Timestamp float32
	Hero      string
	Attacker  uint32
	Inflictor uint32
}

// PTSwitchEvent is a single PT switch for JSON output (timestamp, hero class, attribute).
type PTSwitchEvent struct {
	Timestamp     float32 `json:"timestamp"`
	Attribute     string  `json:"attribute"`
	PrevHealth    int32   `json:"prev_health"`
	PrevMaxHealth int32   `json:"prev_max_health"`
	PrevMana      float32 `json:"prev_mana"`
	PrevMaxMana   float32 `json:"prev_max_mana"`
	Health        int32   `json:"health"`
	MaxHealth     int32   `json:"max_health"`
	Mana          float32 `json:"mana"`
	MaxMana       float32 `json:"max_mana"`
}

// HeroSnapshot captures hero state at a tick (for PT correlation with max health/mana changes).
type HeroSnapshot struct {
	Tick      uint32
	Time      float32
	Health    int32
	MaxHealth int32
	Mana      float32
	MaxMana   float32
	HasHealth bool
	HasMana   bool
}

var statStrings = []string{"str", "int", "agi"}

// Handler implements common.ReplayHandler for Power Treads switch detection.
type Handler struct {
	heroClass  string
	usages     []PTUsage
	switches   []PTSwitchEvent
	emitted    map[string]bool // dedupe: key = "timestamp_hero"
	heroPrev   *HeroSnapshot
	heroCur    *HeroSnapshot
	dumpOutput io.Writer // optional: combat/entity dumps
}

// NewHandler creates a PT handler.
func NewHandler() *Handler {
	return &Handler{
		usages:   make([]PTUsage, 0, 256),
		switches: make([]PTSwitchEvent, 0, 256),
		emitted:  make(map[string]bool),
	}
}

// Init sets up the handler. If outputDir is non-empty, creates dump files there.
func (h *Handler) Init(ctx *common.ParseContext) error {
	h.heroClass = common.HeroNameToClass(ctx.HeroName)
	if ctx.OutputDir != "" {
		if err := os.MkdirAll(ctx.OutputDir, 0755); err != nil {
			return err
		}
		f, err := os.Create(filepath.Join(ctx.OutputDir, "combined_dump.log"))
		if err != nil {
			return err
		}
		h.dumpOutput = f
	}
	return nil
}

// RegisterCallbacks registers PT-specific callbacks.
func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		if m.GetType() != dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ITEM {
			return nil
		}

		inflictorName := m.GetInflictorName()
		inflictorAbilityName, inflictorOk := p.LookupStringByIndex("CombatLogNames", int32(inflictorName))
		if !inflictorOk || inflictorAbilityName != "item_power_treads" {
			return nil
		}

		timestamp := m.GetTimestamp()
		attackerName := m.GetAttackerName()
		realAttackerName, _ := p.LookupStringByIndex("CombatLogNames", int32(attackerName))

		h.usages = append(h.usages, PTUsage{
			Timestamp: timestamp - 10, // ~10 sec start screen offset
			Hero:      realAttackerName,
			Attacker:  attackerName,
			Inflictor: inflictorName,
		})

		// if h.dumpOutput != nil {
		// 	damageSourceName := m.GetDamageSourceName()
		// 	targetName := m.GetTargetName()
		// 	damageSourceAbilityName, _ := p.LookupStringByIndex("CombatLogNames", int32(damageSourceName))
		// 	targetAbilityName, _ := p.LookupStringByIndex("CombatLogNames", int32(targetName))
		// 	fmt.Fprintf(h.dumpOutput,
		// 		"\n=== CMsgDOTACombatLogEntry ===\nType=%v Timestamp=%.4f Attacker=%d Inflictor=%d (%s) DamageSource=%d (%s) Target=%d (%s)\n",
		// 		m.GetType(), timestamp, attackerName, inflictorName, inflictorAbilityName,
		// 		damageSourceName, damageSourceAbilityName, targetName, targetAbilityName,
		// 	)
		// 	spew.Fdump(h.dumpOutput, m)
		// }
		return nil
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil {
			return nil
		}
		entityTick := p.Tick
		entityTime := ctx.TickInterval * float32(entityTick)
		cn := e.GetClassName()

		if cn == "CDOTA_Item_PowerTreads" {
			ownerID := uint32(0)
			if v, ok := e.GetUint32("m_iPlayerOwnerID"); ok {
				ownerID = v
			} else {
				log.Printf("PT OnEntity tick=%d: missing m_iPlayerOwnerID on %s", entityTick, e.String())
				return nil
			}

			ownerHero := ""
			if hr, ok := ctx.PlayerIDToHero[ownerID]; ok {
				ownerHero = hr.ClassName
			}

			if h.dumpOutput != nil {
				spew.Fdump(h.dumpOutput, e.Map())
			}

			for _, ptUsage := range h.usages {
				heroClassName := common.GuessHeroClassFromNPC(ptUsage.Hero)
				toAttr, okAttr := e.GetInt32("m_iStat")
				assembledAt, okAssembledAt := e.GetFloat32("m_flAssembledTime")

				attrString := "unknown"
				if okAttr && toAttr >= 0 && int(toAttr) < len(statStrings) {
					attrString = statStrings[int(toAttr)]
				}

				if heroClassName == ownerHero && ownerHero == h.heroClass && okAssembledAt && ptUsage.Timestamp > assembledAt-0.01 {
					emitKey := fmt.Sprintf("%.3f_%s", ptUsage.Timestamp, ownerHero)
					if !h.emitted[emitKey] {
						h.emitted[emitKey] = true
						h.switches = append(h.switches, PTSwitchEvent{
							Timestamp:     ptUsage.Timestamp,
							Attribute:     attrString,
							PrevHealth:    h.heroPrev.Health,
							PrevMaxHealth: h.heroPrev.MaxHealth,
							PrevMana:      h.heroPrev.Mana,
							PrevMaxMana:   h.heroPrev.MaxMana,
							Health:        h.heroCur.Health,
							MaxHealth:     h.heroCur.MaxHealth,
							Mana:          h.heroCur.Mana,
							MaxMana:       h.heroCur.MaxMana,
						})
					}
					// if okAttr {
					// 	log.Printf("%s: uses Power Treads at %.3f changing attribute to %s (%d) (ticktime %.3f)", ownerHero, ptUsage.Timestamp, attrString, toAttr, entityTime)
					// } else {
					// 	log.Printf("%s: uses Power Treads at %.3f (ticktime %.3f) [m_iStat missing]", ownerHero, ptUsage.Timestamp, entityTime)
					// }

					// if h.heroPrev != nil && h.heroCur != nil &&
					// 	h.heroPrev.HasHealth && h.heroCur.HasHealth && h.heroPrev.HasMana && h.heroCur.HasMana {
					// 	log.Printf("%s snapshot delta: maxHealth %d -> %d, maxMana %.3f -> %.3f (prevT=%.3fs curT=%.3fs)",
					// 		h.heroClass, h.heroPrev.MaxHealth, h.heroCur.MaxHealth,
					// 		h.heroPrev.MaxMana, h.heroCur.MaxMana,
					// 		h.heroPrev.Time, h.heroCur.Time,
					// 	)
					// }
				}
			}
			return nil
		}

		if cn == h.heroClass {
			h.heroPrev = h.heroCur
			s := &HeroSnapshot{Tick: entityTick, Time: entityTime}
			if v, ok := e.GetInt32("m_iMaxHealth"); ok {
				s.MaxHealth = v
				s.Health, _ = e.GetInt32("m_iHealth")
				s.HasHealth = true
			}
			if v, ok := e.GetFloat32("m_flMaxMana"); ok {
				s.MaxMana = v
				s.Mana, _ = e.GetFloat32("m_flMana")
				s.HasMana = true
			}
			h.heroCur = s
			return nil
		}

		return nil
	})
}

// Output returns the handler's contribution to the final JSON (key "power_treads").
func (h *Handler) Output(ctx *common.ParseContext) map[string]interface{} {
	return map[string]interface{}{
		"pt_switches": h.switches,
	}
}
