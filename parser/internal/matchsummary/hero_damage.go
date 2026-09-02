package matchsummary

import (
	"sort"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

// HeroDamageRow is one row in the damage-by-hero table.
type HeroDamageRow struct {
	Hero   string `json:"hero"`
	Damage uint32 `json:"damage"`
}

// HeroDamage is hero-vs-hero damage dealt by the analyzed hero (combat-log reconstruction).
type HeroDamage struct {
	Total      uint32            `json:"total"`
	ByType     map[string]uint32 `json:"by_type"`
	ByHero     []HeroDamageRow   `json:"by_hero,omitempty"`
}

func newHeroDamage() HeroDamage {
	return HeroDamage{ByType: make(map[string]uint32)}
}

func (h *Handler) onHeroDamage(p *manta.Parser, m *dota.CMsgDOTACombatLogEntry) {
	if !m.GetIsTargetHero() || m.GetIsTargetIllusion() {
		return
	}

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
	if !ok || heroClassFromNPC(targetName) == "" {
		return
	}
	if heroClassFromNPC(targetName) == h.heroClass {
		return
	}

	amount := m.GetValue()
	if amount == 0 {
		return
	}

	h.summary.HeroDamage.Total += amount
	damageType := damageTypeLabel(m.GetDamageType())
	h.summary.HeroDamage.ByType[damageType] += amount
	if h.heroDamageByHero == nil {
		h.heroDamageByHero = make(map[string]uint32)
	}
	h.heroDamageByHero[targetName] += amount
}

func (h *Handler) heroDamageByHeroTable() []HeroDamageRow {
	if len(h.heroDamageByHero) == 0 {
		return nil
	}
	rows := make([]HeroDamageRow, 0, len(h.heroDamageByHero))
	for hero, damage := range h.heroDamageByHero {
		rows = append(rows, HeroDamageRow{Hero: hero, Damage: damage})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Damage != rows[j].Damage {
			return rows[i].Damage > rows[j].Damage
		}
		return rows[i].Hero < rows[j].Hero
	})
	return rows
}

func damageTypeLabel(t uint32) string {
	switch t {
	case 1:
		return "physical"
	case 2:
		return "magical"
	case 4:
		return "pure"
	default:
		return "unknown"
	}
}
