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

// pendingCreepEvent is a combat-log creep event waiting for OnEntity correlation.
type pendingCreepEvent struct {
	creepName string
	gameTime  float32
	health    int32 // post-damage health from combat log
	damage    int32 // damage dealt (value); prev health = health + damage
	consumed  bool
}

// creepTrack holds per-entity creep state for correlating combat log with entity updates.
type creepTrack struct {
	creepName     string // npc name from combat log once correlated
	prevHealth    int32
	hasHealth     bool
	heroDamagedAt float32 // 0 if our hero has not damaged this creep recently
}

// Handler implements common.ReplayHandler for last-hit, deny, and missed last-hit counting.
type Handler struct {
	heroClass            string
	lastHitsLane         int
	lastHitsJungle       int
	denies               int
	events               []Event
	missedEvents         []Event
	pendingHeroDamage    []pendingCreepEvent
	pendingOtherDeath    []pendingCreepEvent
	pendingSelfKill      []pendingCreepEvent
	creepTracks          map[int32]*creepTrack
	timeAndPausesHandler *timeandpauses.Handler
}

// NewHandler creates a lasthits handler.
func NewHandler(timeAndPausesHandler *timeandpauses.Handler) *Handler {
	return &Handler{
		events:               make([]Event, 0, 256),
		missedEvents:         make([]Event, 0, 128),
		pendingHeroDamage:    make([]pendingCreepEvent, 0, 64),
		pendingOtherDeath:    make([]pendingCreepEvent, 0, 64),
		pendingSelfKill:      make([]pendingCreepEvent, 0, 64),
		creepTracks:          make(map[int32]*creepTrack, 256),
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

// registers lasthits callbacks.
func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		if h.timeAndPausesHandler.IsGameEnded() {
			return nil
		}
		gameTime := h.timeAndPausesHandler.CurrentGameTime()
		h.prunePendingEvents(gameTime)

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

		combatLogType := m.GetType()
		switch combatLogType {
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE:
			attackerClass := common.GetHeroClassName(realAttackerName)
			if attackerClass != h.heroClass || m.GetIsAttackerIllusion() {
				return nil
			}
			if creepTypeFromTargetName(realTargetName) == "" {
				return nil
			}
			h.pendingHeroDamage = append(h.pendingHeroDamage, pendingCreepEvent{
				creepName: realTargetName,
				gameTime:  gameTime,
				health:    m.GetHealth(),
				damage:    int32(m.GetValue()),
			})
			return nil

		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DEATH:
			creepType := creepTypeFromTargetName(realTargetName)
			if creepType == "" {
				return nil
			}

			attackerClass := common.GetHeroClassName(realAttackerName)
			weAreKiller := attackerClass == h.heroClass && !m.GetIsAttackerIllusion()

			if weAreKiller {
				h.pendingSelfKill = append(h.pendingSelfKill, pendingCreepEvent{
					creepName: realTargetName,
					gameTime:  gameTime,
				})
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

			h.pendingOtherDeath = append(h.pendingOtherDeath, pendingCreepEvent{
				creepName: realTargetName,
				gameTime:  gameTime,
			})
			return nil
		}
		return nil
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil || h.timeAndPausesHandler.IsGameEnded() {
			return nil
		}
		gameTime := h.timeAndPausesHandler.CurrentGameTime()
		h.onCreepEntity(e, op, gameTime)
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

func isCreepEntityClass(className string) bool {
	return strings.HasPrefix(className, "CDOTA_BaseNPC_Creep")
}

func (h *Handler) prunePendingEvents(gameTime float32) {
	cutoff := gameTime - missedLastHitWindowSec
	h.pendingHeroDamage = prunePendingByTime(h.pendingHeroDamage, cutoff)
	h.pendingOtherDeath = prunePendingByTime(h.pendingOtherDeath, cutoff)
	h.pendingSelfKill = prunePendingByTime(h.pendingSelfKill, cutoff)
}

func prunePendingByTime(events []pendingCreepEvent, cutoff float32) []pendingCreepEvent {
	n := 0
	for _, e := range events {
		if e.consumed || e.gameTime < cutoff {
			continue
		}
		events[n] = e
		n++
	}
	return events[:n]
}

func (h *Handler) pruneConsumedPending() {
	n := 0
	for _, e := range h.pendingHeroDamage {
		if e.consumed {
			continue
		}
		h.pendingHeroDamage[n] = e
		n++
	}
	h.pendingHeroDamage = h.pendingHeroDamage[:n]

	n = 0
	for _, e := range h.pendingOtherDeath {
		if e.consumed {
			continue
		}
		h.pendingOtherDeath[n] = e
		n++
	}
	h.pendingOtherDeath = h.pendingOtherDeath[:n]
}

func (h *Handler) hasPendingSelfKill(creepName string, deathTime float32) bool {
	cutoff := deathTime - missedLastHitWindowSec
	for _, sk := range h.pendingSelfKill {
		if sk.creepName == creepName && sk.gameTime >= cutoff && sk.gameTime <= deathTime+0.05 {
			return true
		}
	}
	return false
}

func (h *Handler) consumePendingSelfKill(creepName string, deathTime float32) {
	cutoff := deathTime - missedLastHitWindowSec
	n := 0
	for _, sk := range h.pendingSelfKill {
		if sk.creepName == creepName && sk.gameTime >= cutoff && sk.gameTime <= deathTime+0.05 {
			continue
		}
		h.pendingSelfKill[n] = sk
		n++
	}
	h.pendingSelfKill = h.pendingSelfKill[:n]
}

func heroDamageCorrelates(pd pendingCreepEvent, prevHealth, health int32, healthReduced bool) bool {
	if pd.health > 0 {
		if health != pd.health || !healthReduced {
			return false
		}
		if pd.damage > 0 && prevHealth != pd.health+pd.damage {
			return false
		}
		return true
	}
	return healthReduced
}

func (h *Handler) correlateHeroDamage(track *creepTrack, health int32, healthReduced bool) {
	for i := range h.pendingHeroDamage {
		pd := &h.pendingHeroDamage[i]
		if pd.consumed {
			continue
		}
		if !heroDamageCorrelates(*pd, track.prevHealth, health, healthReduced) {
			continue
		}
		track.creepName = pd.creepName
		track.heroDamagedAt = pd.gameTime
		pd.consumed = true
		return
	}
}

func (h *Handler) onCreepEntity(e *manta.Entity, op manta.EntityOp, gameTime float32) {
	if !isCreepEntityClass(e.GetClassName()) {
		return
	}
	health, ok := e.GetInt32("m_iHealth")
	if !ok {
		return
	}
	h.onCreepHealthUpdate(e.GetIndex(), health, op, gameTime)
}

func (h *Handler) onCreepHealthUpdate(idx int32, health int32, op manta.EntityOp, gameTime float32) {
	track := h.creepTracks[idx]
	if track == nil {
		track = &creepTrack{}
		h.creepTracks[idx] = track
	}

	healthReduced := track.hasHealth && health < track.prevHealth
	justDied := (track.hasHealth && health <= 0 && track.prevHealth > 0) ||
		op.Flag(manta.EntityOpDeleted) || op.Flag(manta.EntityOpDeletedLeft)

	h.correlateHeroDamage(track, health, healthReduced)

	track.prevHealth = health
	track.hasHealth = true

	if justDied {
		if h.hasPendingSelfKill(track.creepName, gameTime) {
			track.heroDamagedAt = 0
			h.consumePendingSelfKill(track.creepName, gameTime)
		} else if track.creepName != "" && track.heroDamagedAt > 0 {
			for i := range h.pendingOtherDeath {
				pd := &h.pendingOtherDeath[i]
				if pd.consumed || pd.creepName != track.creepName {
					continue
				}
				if pd.gameTime-track.heroDamagedAt > missedLastHitWindowSec {
					continue
				}
				h.missedEvents = append(h.missedEvents, Event{
					Timestamp: gameTime,
					Type:      "missed_last_hit",
					CreepName: track.creepName,
				})
				track.heroDamagedAt = 0
				pd.consumed = true
				break
			}
		}
	}

	h.pruneConsumedPending()

	if op.Flag(manta.EntityOpDeleted) || op.Flag(manta.EntityOpDeletedLeft) {
		delete(h.creepTracks, idx)
	}
}
