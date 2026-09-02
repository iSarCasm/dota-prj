package matchsummary

import (
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

// TowerDamage is building damage dealt by the analyzed hero (combat-log reconstruction).
type TowerDamage struct {
	Total uint32 `json:"total"`
}

func (h *Handler) onTowerDamage(p *manta.Parser, m *dota.CMsgDOTACombatLogEntry) {
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
	if !ok || !isTowerDamageTarget(targetName) {
		return
	}

	amount := m.GetValue()
	if amount == 0 {
		return
	}

	h.summary.TowerDamage.Total += amount
}

// isTowerDamageTarget matches OpenDota tower_damage: towers, barracks, and the ancient.
func isTowerDamageTarget(name string) bool {
	return strings.Contains(name, "_tower") ||
		strings.Contains(name, "_rax") ||
		strings.Contains(name, "_fort")
}
