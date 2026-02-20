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

// HeroSnapshot captures hero state at a tick (for PT correlation with max health/mana changes).
type HeroSnapshot struct {
	Tick      uint32
	Time      float32
	MaxHealth int32
	MaxMana   float32
	HasHealth bool
	HasMana   bool
}

var statStrings = []string{"str", "int", "agi"}

// Handler implements common.ReplayHandler for Power Treads switch detection.
type Handler struct {
	usages     []PTUsage
	puckPrev   *HeroSnapshot
	puckCur    *HeroSnapshot
	dumpOutput io.Writer // optional: combat/entity dumps
}

// NewHandler creates a PT handler.
func NewHandler() *Handler {
	return &Handler{
		usages: make([]PTUsage, 0, 256),
	}
}

// Init sets up the handler. If outputDir is non-empty, creates dump files there.
func (h *Handler) Init(ctx *common.ParseContext) error {
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

		if h.dumpOutput != nil {
			damageSourceName := m.GetDamageSourceName()
			targetName := m.GetTargetName()
			damageSourceAbilityName, _ := p.LookupStringByIndex("CombatLogNames", int32(damageSourceName))
			targetAbilityName, _ := p.LookupStringByIndex("CombatLogNames", int32(targetName))
			fmt.Fprintf(h.dumpOutput,
				"\n=== CMsgDOTACombatLogEntry ===\nType=%v Timestamp=%.4f Attacker=%d Inflictor=%d (%s) DamageSource=%d (%s) Target=%d (%s)\n",
				m.GetType(), timestamp, attackerName, inflictorName, inflictorAbilityName,
				damageSourceName, damageSourceAbilityName, targetName, targetAbilityName,
			)
			spew.Fdump(h.dumpOutput, m)
		}
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

				if heroClassName == ownerHero && okAssembledAt && ptUsage.Timestamp > assembledAt-0.01 {
					if okAttr {
						log.Printf("%s: uses Power Treads at %.3f changing attribute to %s (%d) (ticktime %.3f)", ownerHero, ptUsage.Timestamp, attrString, toAttr, entityTime)
					} else {
						log.Printf("%s: uses Power Treads at %.3f (ticktime %.3f) [m_iStat missing]", ownerHero, ptUsage.Timestamp, entityTime)
					}

					if ownerHero == "CDOTA_Unit_Hero_Puck" && h.puckPrev != nil && h.puckCur != nil &&
						h.puckPrev.HasHealth && h.puckCur.HasHealth && h.puckPrev.HasMana && h.puckCur.HasMana {
						log.Printf("Puck snapshot delta: maxHealth %d -> %d, maxMana %.3f -> %.3f (prevT=%.3fs curT=%.3fs)",
							h.puckPrev.MaxHealth, h.puckCur.MaxHealth,
							h.puckPrev.MaxMana, h.puckCur.MaxMana,
							h.puckPrev.Time, h.puckCur.Time,
						)
					}
				}
			}
		}

		if cn == "CDOTA_Unit_Hero_Puck" {
			h.puckPrev = h.puckCur
			s := &HeroSnapshot{Tick: entityTick, Time: entityTime}
			if v, ok := e.GetInt32("m_iMaxHealth"); ok {
				s.MaxHealth = v
				s.HasHealth = true
			}
			if v, ok := e.GetFloat32("m_flMaxMana"); ok {
				s.MaxMana = v
				s.HasMana = true
			}
			h.puckCur = s
		}

		return nil
	})
}
