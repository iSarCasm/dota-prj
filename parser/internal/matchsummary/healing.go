package matchsummary

import (
	"sort"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

// HeroHealingRow is one row in the healing-by-hero table.
type HeroHealingRow struct {
	Hero    string `json:"hero"`
	Healing uint32 `json:"healing"`
}

// Healing is hitpoint restoration provided by the analyzed hero (combat-log reconstruction).
type Healing struct {
	Total  uint32           `json:"total"`
	ByHero []HeroHealingRow `json:"by_hero,omitempty"`
}

func (h *Handler) onHeal(p *manta.Parser, m *dota.CMsgDOTACombatLogEntry) {
	sourceName, ok := lookupCombatName(p, m.GetDamageSourceName())
	if !ok {
		sourceName, ok = lookupCombatName(p, m.GetAttackerName())
		if !ok {
			return
		}
	}
	if heroClassFromNPC(sourceName) != h.heroClass {
		return
	}

	targetName, ok := lookupCombatName(p, m.GetTargetName())
	if !ok || !m.GetIsTargetHero() || m.GetIsTargetIllusion() {
		return
	}
	if heroClassFromNPC(targetName) == "" {
		return
	}
	if heroClassFromNPC(targetName) == h.heroClass {
		return
	}

	amount := m.GetValue()
	if amount == 0 {
		return
	}

	h.summary.Healing.Total += amount
	if h.healingByHero == nil {
		h.healingByHero = make(map[string]uint32)
	}
	h.healingByHero[targetName] += amount
}

func (h *Handler) healingByHeroTable() []HeroHealingRow {
	if len(h.healingByHero) == 0 {
		return nil
	}
	rows := make([]HeroHealingRow, 0, len(h.healingByHero))
	for hero, healing := range h.healingByHero {
		rows = append(rows, HeroHealingRow{Hero: hero, Healing: healing})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Healing != rows[j].Healing {
			return rows[i].Healing > rows[j].Healing
		}
		return rows[i].Hero < rows[j].Hero
	})
	return rows
}
