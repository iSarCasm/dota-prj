package pt

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/davecgh/go-spew/spew"
	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/abilities"
	"dota2/internal/common"
	"dota2/internal/mana"
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
	heroClass        string
	abilitiesHandler *abilities.Handler
	manaHandler      *mana.Handler
	usages           []PTUsage
	switches         []PTSwitchEvent
	emitted          map[string]bool // dedupe: key = "timestamp_hero"
	heroPrev         *HeroSnapshot
	heroCur          *HeroSnapshot
	dumpOutput       io.Writer // optional: combat/entity dumps
}

// NewHandler creates a PT handler. abilitiesHandler and manaHandler may be nil; if set, PT can read ability usages and mana-at-time for insights.
func NewHandler(abilitiesHandler *abilities.Handler, manaHandler *mana.Handler) *Handler {
	return &Handler{
		abilitiesHandler: abilitiesHandler,
		manaHandler:      manaHandler,
		usages:           make([]PTUsage, 0, 256),
		switches:         make([]PTSwitchEvent, 0, 256),
		emitted:          make(map[string]bool),
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
				heroClassName := common.GetHeroClassName(ptUsage.Hero)
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

func missedManaSave(currentMana float32, currentMaxMana float32, manacost int32) int32 {
	wouldBeMaxMana := currentMaxMana + 120 // TODO: this is some sort of patch constant
	wouldBeMana := currentMana / currentMaxMana * wouldBeMaxMana
	return int32((wouldBeMana-float32(manacost))/wouldBeMaxMana*currentMaxMana) - (int32(currentMana) - manacost)
}

// attributeAtTime returns the PT attribute at time t (from last switch with timestamp <= t). Empty if unknown.
func (h *Handler) attributeAtTime(t float32) string {
	var last string
	for _, s := range h.switches {
		if s.Timestamp <= t {
			last = s.Attribute
		} else {
			break
		}
	}
	return last
}

// buildInsights returns PT-related insights: (1) mana ability used without PT on int => bad minor, (2) int -> mana ability -> non-int within 10s => good.
func (h *Handler) buildInsights() []common.Insight {
	var out []common.Insight
	const goodWindowSec = 10.0 // TODO: make this a constant

	if h.abilitiesHandler == nil {
		return out
	}
	abilityUsages := h.abilitiesHandler.Usages()

	// 1. Mana ability used without PT on int => mistake (minor). Show how much mana could be saved (using mana handler for accurate mana at use time).
	for _, u := range abilityUsages {
		if u.Type != abilities.UsageTypeAbility || u.ManaCost <= 0 {
			continue
		}
		attr := h.attributeAtTime(u.Timestamp)
		if attr == "" || attr == "int" {
			continue
		}
		details := map[string]interface{}{
			"ability_name": u.AbilityName,
			"mana_cost":    u.ManaCost,
			"pt_was_on":    attr,
		}
		if h.manaHandler != nil {
			if manaVal, maxManaVal, ok := h.manaHandler.ManaAtTime(u.Timestamp); ok && maxManaVal > 0 {
				details["mana_at_use"] = manaVal
				details["max_mana_at_use"] = maxManaVal
				details["could_save"] = missedManaSave(manaVal, maxManaVal, u.ManaCost)
			}
		}
		out = append(out, common.Insight{
			Type:       "pt_mana_ability_not_on_int",
			Timestamps: []float32{u.Timestamp},
			Verdict:    "bad",
			Level:      "minor",
			Details:    details,
		})
	}

	// 2. Switch to INT -> use mana ability/abilities -> switch to non-INT within 10s => good. Capture all abilities in the window.
	for i, s := range h.switches {
		if s.Attribute != "int" {
			continue
		}
		t1 := s.Timestamp
		// Find next switch to non-int within (t1, t1+10]
		var t3 float32
		for j := i + 1; j < len(h.switches); j++ {
			next := h.switches[j]
			if next.Timestamp > t1+goodWindowSec {
				break
			}
			if next.Attribute != "int" {
				t3 = next.Timestamp
				break
			}
		}
		if t3 == 0 {
			continue
		}
		// Collect all mana abilities used in (t1, t3]
		type abAt struct {
			t    float32
			name string
			cost int32
		}
		var used []abAt
		for _, ab := range abilityUsages {
			if ab.Type != abilities.UsageTypeAbility || ab.ManaCost <= 0 {
				continue
			}
			if ab.Timestamp > t1 && ab.Timestamp <= t3 {
				used = append(used, abAt{ab.Timestamp, ab.AbilityName, ab.ManaCost})
			}
		}
		if len(used) == 0 {
			continue
		}
		// Sort by timestamp (usages are usually ordered but ensure)
		sort.Slice(used, func(a, b int) bool { return used[a].t < used[b].t })
		abilitiesDetail := make([]map[string]interface{}, 0, len(used))
		abilityTimestamps := make([]float32, 0, len(used))
		for _, u := range used {
			abilityTimestamps = append(abilityTimestamps, u.t)
			abilitiesDetail = append(abilitiesDetail, map[string]interface{}{
				"ability_name": u.name,
				"timestamp":    u.t,
				"mana_cost":    u.cost,
			})
		}
		// timestamps: t1, then all ability timestamps, then t3
		ts := make([]float32, 0, 2+len(abilityTimestamps))
		ts = append(ts, t1)
		ts = append(ts, abilityTimestamps...)
		ts = append(ts, t3)
		details := map[string]interface{}{
			"abilities":    abilitiesDetail,
			"duration_sec": t3 - t1,
		}
		if h.manaHandler != nil {
			if manaVal, maxManaVal, ok := h.manaHandler.ManaAtTime(t1); ok {
				details["mana_before_int_switch"] = manaVal
				details["max_mana_before_int_switch"] = maxManaVal
			}
			if len(used) > 0 {
				if manaVal, maxManaVal, ok := h.manaHandler.ManaAtTime(used[len(used)-1].t); ok && maxManaVal > 0 {
					details["total_mana_saved"] = missedManaSave(manaVal, maxManaVal, used[len(used)-1].cost)
				}
			}
		}
		out = append(out, common.Insight{
			Type:       "pt_efficient_switch",
			Timestamps: ts,
			Verdict:    "good",
			Level:      "minor",
			Details:    details,
		})
	}

	// Sort by first timestamp so good and bad insights are interleaved chronologically.
	sort.Slice(out, func(a, b int) bool {
		ta := float32(0)
		if len(out[a].Timestamps) > 0 {
			ta = out[a].Timestamps[0]
		}
		tb := float32(0)
		if len(out[b].Timestamps) > 0 {
			tb = out[b].Timestamps[0]
		}
		return ta < tb
	})
	return out
}

// Output returns the handler's contribution to the final JSON (key "power_treads", optional "insights").
func (h *Handler) Output(ctx *common.ParseContext) map[string]interface{} {
	m := map[string]interface{}{
		"pt_switches": h.switches,
	}
	if ins := h.buildInsights(); len(ins) > 0 {
		m["insights"] = ins
	}
	return m
}
