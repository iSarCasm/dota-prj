package abilities

import (
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/common"
)

const (
	usageTypeAbility = "ability"
	usageTypeItem    = "item"
)

// Usage is a single ability/item use for JSON output. ManaCost is read from the ability/item entity at the moment of use (replay order).
type Usage struct {
	Timestamp   float32 `json:"timestamp"`
	AbilityName string  `json:"ability_name"`
	Type        string  `json:"type"`
	ManaCost    int32   `json:"mana_cost"`
}

// Handler implements common.ReplayHandler for ability and item usage tracking.
type Handler struct {
	heroClass         string
	usages            []Usage
	abilityNameToMana map[string]int32 // combat-log style name -> m_iManaCost (from entity), so combat log lookup matches
}

// NewHandler creates an abilities handler.
func NewHandler() *Handler {
	return &Handler{
		usages:            make([]Usage, 0, 512),
		abilityNameToMana: make(map[string]int32),
	}
}

// Init sets up the handler.
func (h *Handler) Init(ctx *common.ParseContext) error {
	h.heroClass = common.HeroNameToClass(ctx.HeroName)
	return nil
}

// RegisterCallbacks registers ability/item usage callbacks.
// Entity callback must run so abilityClassToMana is updated before combat log entries; replay order ensures we see entity state at or before each use.
func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
	// Entity: keep m_iManaCost by combat-log-style name (derived from entity class) so combat log lookup matches
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil {
			return nil
		}
		cn := e.GetClassName()
		if cn == "" {
			return nil
		}
		if !strings.HasPrefix(cn, "CDOTA_Ability_") && !strings.HasPrefix(cn, "CDOTA_Item_") {
			return nil
		}
		manaCost, ok := e.GetInt32("m_iManaCost")
		if !ok {
			return nil
		}
		key := common.EntityClassToCombatLogName(cn)
		if key != "" {
			h.abilityNameToMana[key] = manaCost
		}
		return nil
	})

	// Combat log: record one Usage per use with mana cost at this moment (from latest entity state)
	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		ctype := m.GetType()
		isAbility := ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ABILITY ||
			ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ABILITY_TRIGGER
		isItem := ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ITEM
		if !isAbility && !isItem {
			return nil
		}

		attackerName := m.GetAttackerName()
		realAttackerName, ok := p.LookupStringByIndex("CombatLogNames", int32(attackerName))
		if !ok {
			return nil
		}
		heroClassName := common.GuessHeroClassFromNPC(realAttackerName)
		if heroClassName != h.heroClass {
			return nil
		}

		inflictorName := m.GetInflictorName()
		abilityName, ok := p.LookupStringByIndex("CombatLogNames", int32(inflictorName))
		if !ok {
			abilityName = ""
		}

		usageType := usageTypeAbility
		if isItem {
			usageType = usageTypeItem
		}

		manaCost := h.abilityNameToMana[abilityName]

		h.usages = append(h.usages, Usage{
			Timestamp:   m.GetTimestamp() - 10,
			AbilityName: abilityName,
			Type:        usageType,
			ManaCost:    manaCost,
		})
		return nil
	})
}

// Output returns the handler's contribution to the final JSON (key "ability_usages").
func (h *Handler) Output(ctx *common.ParseContext) map[string]interface{} {
	return map[string]interface{}{
		"ability_usages": h.usages,
	}
}
