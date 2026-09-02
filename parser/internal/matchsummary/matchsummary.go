package matchsummary

import (
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/common"
	"dota2/internal/timeandpauses"
)

// Summary is the match summary output for the analyzed hero.
type Summary struct {
	KDA         KDA         `json:"kda"`
	HeroDamage  HeroDamage  `json:"hero_damage"`
	TowerDamage TowerDamage `json:"tower_damage"`
	Healing      Healing       `json:"healing"`
	HeroesKilled []HeroKillRow `json:"heroes_killed"`
}

// Handler implements common.ReplayHandler for end-game match summary stats.
type Handler struct {
	timeAndPausesHandler *timeandpauses.Handler
	heroClass            string
	playerID             uint32
	hasPlayerID          bool
	heroesKilled         map[string]int
	summary              Summary
}

// NewHandler creates a match summary handler.
func NewHandler(timeAndPausesHandler *timeandpauses.Handler) *Handler {
	return &Handler{
		timeAndPausesHandler: timeAndPausesHandler,
		summary: Summary{
			HeroDamage: newHeroDamage(),
		},
	}
}

func (h *Handler) Init(ctx *common.ParseContext) error {
	h.heroClass = common.HeroNameToClass(ctx.HeroName)
	if h.heroClass == "" {
		return common.ErrInvalidHeroName
	}
	return nil
}

func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil || h.hasPlayerID {
			return nil
		}
		if e.GetClassName() != h.heroClass {
			return nil
		}
		pid, ok := e.GetUint32("m_iPlayerID")
		if !ok {
			return nil
		}
		h.playerID = pid
		h.hasPlayerID = true
		return nil
	})

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		switch m.GetType() {
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_HEAL:
			h.onHeal(p, m)
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE:
			h.onHeroDamage(p, m)
			h.onTowerDamage(p, m)
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DEATH:
			if !m.GetIsTargetHero() {
				return nil
			}
			h.onHeroDeath(p, m)
		}
		return nil
	})
}

func (h *Handler) Output(ctx *common.ParseContext) map[string]interface{} {
	h.summary.HeroesKilled = h.heroesKilledTable()
	return map[string]interface{}{
		"match_summary": h.summary,
	}
}

func lookupCombatName(p *manta.Parser, idx uint32) (string, bool) {
	if idx == 0 {
		return "", false
	}
	s, ok := p.LookupStringByIndex("CombatLogNames", int32(idx))
	if !ok || strings.TrimSpace(s) == "" {
		return "", false
	}
	return s, true
}

func heroClassFromNPC(npc string) string {
	return common.GetHeroClassName(npc)
}
