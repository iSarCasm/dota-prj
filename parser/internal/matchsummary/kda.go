package matchsummary

import (
	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

// KDA is kill/death/assist totals for the analyzed hero.
type KDA struct {
	Kills   int `json:"kills"`
	Deaths  int `json:"deaths"`
	Assists int `json:"assists"`
}

func (h *Handler) onHeroDeath(p *manta.Parser, m *dota.CMsgDOTACombatLogEntry) {
	attackerName, okA := lookupCombatName(p, m.GetAttackerName())
	if !okA {
		return
	}
	targetName, okT := lookupCombatName(p, m.GetTargetName())
	if !okT {
		return
	}

	attackerClass := heroClassFromNPC(attackerName)
	targetClass := heroClassFromNPC(targetName)
	if attackerClass == "" || targetClass == "" {
		return
	}

	if attackerClass == h.heroClass && !m.GetIsAttackerIllusion() {
		h.summary.KDA.Kills++
		h.recordHeroKill(targetName)
	}
	if targetClass == h.heroClass && !m.GetIsTargetIllusion() {
		h.summary.KDA.Deaths++
	}
	if h.hasPlayerID &&
		attackerClass != h.heroClass &&
		targetClass != h.heroClass &&
		m.GetIsAttackerHero() &&
		h.isAssister(m) {
		h.summary.KDA.Assists++
	}
}

func (h *Handler) isAssister(m *dota.CMsgDOTACombatLogEntry) bool {
	assistIndex := int32(h.playerID / 2)
	for _, ap := range m.GetAssistPlayers() {
		if ap == assistIndex {
			return true
		}
	}
	legacy := []uint32{
		m.GetAssistPlayer0(),
		m.GetAssistPlayer1(),
		m.GetAssistPlayer2(),
		m.GetAssistPlayer3(),
	}
	for _, ap := range legacy {
		if ap == uint32(assistIndex) {
			return true
		}
	}
	return false
}
