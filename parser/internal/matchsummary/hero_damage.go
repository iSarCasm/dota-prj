package matchsummary

import (
	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

// HeroDamage is hero-vs-hero damage dealt by the analyzed hero (combat-log reconstruction).
type HeroDamage struct {
	Total      uint32            `json:"total"`
	ByType     map[string]uint32 `json:"by_type"`
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
