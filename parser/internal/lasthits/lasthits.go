package lasthits

import (
	"errors"
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/common"
	"dota2/internal/timeandpauses"
)

const missedLastHitWindowSec = 2.0

// Event is a single last-hit, deny, or missed last-hit.
type Event struct {
	Timestamp float32 `json:"timestamp"`
	Type      string  `json:"type"` // "last_hit", "deny", or "missed_last_hit"
	CreepName string  `json:"creep_name"`
}

// damageRecord is our damage to a creep (same name can be multiple creeps).
type damageRecord struct {
	creepName string
	gameTime  float32
}

// Handler implements common.ReplayHandler for last-hit, deny, and missed last-hit counting.
type Handler struct {
	heroClass            string
	lastHitsLane         int
	lastHitsJungle       int
	denies               int
	events               []Event
	missedEvents         []Event
	damagedCreeps        []damageRecord
	timeAndPausesHandler *timeandpauses.Handler
}

// NewHandler creates a lasthits handler.
func NewHandler(timeAndPausesHandler *timeandpauses.Handler) *Handler {
	return &Handler{
		events:               make([]Event, 0, 256),
		missedEvents:         make([]Event, 0, 128),
		damagedCreeps:        make([]damageRecord, 0, 64),
		timeAndPausesHandler: timeAndPausesHandler,
	}
}

// Init validates config.
func (h *Handler) Init(ctx *common.ParseContext) error {
	h.heroClass = common.HeroNameToClass(ctx.HeroName)
	if h.heroClass == "" {
		return common.ErrInvalidHeroName
	}
	if h.timeAndPausesHandler == nil {
		return errors.New("lasthits handler requires timeandpauses dependency")
	}
	return nil
}

// returns "lane", "jungle", or "" if target is not a creep we count.
func creepTypeFromTargetName(targetName string) string {
	targetName = strings.ToLower(strings.TrimSpace(targetName))
	if targetName == "" {
		return ""
	}
	if strings.HasPrefix(targetName, "npc_dota_neutral_") {
		return "jungle"
	}
	if strings.HasPrefix(targetName, "npc_dota_creep_goodguys_") || strings.HasPrefix(targetName, "npc_dota_creep_badguys_") {
		return "lane"
	}
	if strings.HasPrefix(targetName, "npc_dota_creep_siege") {
		return "lane"
	}
	return ""
}

// pruneDamagedCreeps removes entries older than gameTime - missedLastHitWindowSec.
func (h *Handler) pruneDamagedCreeps(gameTime float32) {
	cutoff := gameTime - missedLastHitWindowSec
	n := 0
	for _, r := range h.damagedCreeps {
		if r.gameTime >= cutoff {
			h.damagedCreeps[n] = r
			n++
		}
	}
	h.damagedCreeps = h.damagedCreeps[:n]
}

// removes one recent damage record for the given creep name (within window) and returns true if found.
func (h *Handler) removeRecentDamageForCreep(creepName string, deathTime float32) bool {
	cutoff := deathTime - missedLastHitWindowSec
	for i, r := range h.damagedCreeps {
		if r.creepName == creepName && r.gameTime >= cutoff {
			// remove at i (replace with last, shrink)
			h.damagedCreeps[i] = h.damagedCreeps[len(h.damagedCreeps)-1]
			h.damagedCreeps = h.damagedCreeps[:len(h.damagedCreeps)-1]
			return true
		}
	}
	return false
}

// registers lasthits callbacks.
func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		if h.timeAndPausesHandler.IsGameEnded() {
			return nil
		}
		gameTime := h.timeAndPausesHandler.CurrentGameTime()

		attackerNameIdx := m.GetAttackerName()
		targetNameIdx := m.GetTargetName()
		realAttackerName, okA := p.LookupStringByIndex("CombatLogNames", int32(attackerNameIdx))
		if !okA {
			return nil
		}
		realTargetName, okT := p.LookupStringByIndex("CombatLogNames", int32(targetNameIdx))
		if !okT {
			return nil
		}

		ctype := m.GetType()

		switch ctype {
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE:
			// Record when we deal damage to a lane/neutral creep (to detect missed last hits later).
			attackerClass := common.GetHeroClassName(realAttackerName)
			if attackerClass != h.heroClass || m.GetIsAttackerIllusion() {
				return nil
			}
			if creepTypeFromTargetName(realTargetName) != "" {
				h.damagedCreeps = append(h.damagedCreeps, damageRecord{creepName: realTargetName, gameTime: gameTime})
			}
			return nil

		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DEATH:
			creepType := creepTypeFromTargetName(realTargetName)
			if creepType == "" {
				return nil
			}
			h.pruneDamagedCreeps(gameTime)

			attackerClass := common.GetHeroClassName(realAttackerName)
			weAreKiller := attackerClass == h.heroClass && !m.GetIsAttackerIllusion()

			if weAreKiller {
				h.removeRecentDamageForCreep(realTargetName, gameTime) // consume so we don't count as missed later
				switch creepType {
				case "lane":
					if m.GetAttackerTeam() == m.GetTargetTeam() {
						h.denies++
						h.events = append(h.events, Event{Timestamp: gameTime, Type: "deny", CreepName: realTargetName})
					} else {
						h.lastHitsLane++
						h.events = append(h.events, Event{Timestamp: gameTime, Type: "last_hit", CreepName: realTargetName})
					}
				case "jungle":
					h.lastHitsJungle++
					h.events = append(h.events, Event{Timestamp: gameTime, Type: "last_hit", CreepName: realTargetName})
				}
				return nil
			}

			// Creep died and we didn't kill it: missed last hit if we had damaged it recently.
			if h.removeRecentDamageForCreep(realTargetName, gameTime) {
				h.missedEvents = append(h.missedEvents, Event{Timestamp: gameTime, Type: "missed_last_hit", CreepName: realTargetName})
			}
			return nil
		}
		return nil
	})
}

// Output returns the handler's contribution to the final JSON.
func (h *Handler) Output(ctx *common.ParseContext) map[string]interface{} {
	return map[string]interface{}{
		"last_hits": map[string]interface{}{
			"lane":                   h.lastHitsLane,
			"jungle":                 h.lastHitsJungle,
			"total":                  h.lastHitsLane + h.lastHitsJungle,
			"denies":                 h.denies,
			"last_hits":              h.events,
			"missed_last_hits":       h.missedEvents,
			"missed_last_hits_total": len(h.missedEvents),
		},
	}
}
