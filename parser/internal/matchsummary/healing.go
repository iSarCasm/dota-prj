package matchsummary

import (
	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

// Healing is hitpoint restoration provided by the analyzed hero (combat-log reconstruction).
type Healing struct {
	Total uint32 `json:"total"`
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
}
