package lasthits

import (
	"errors"
	"log"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/common"
	"dota2/internal/creeps"
	"dota2/internal/slicesx"
	"dota2/internal/timeandpauses"
)

const (
	missedLastHitWindowSec = 2.0
	dotaTickRate           = float32(30)
	tickDuration           = 1 / dotaTickRate // one server tick; candidate collection window per combat-log line
	// this value is questionable, need more integration testing
	deathCombatLogEpsilon = float32(0.05) // slop when matching entity death time to combat-log DEATH time
)

// Event is a single last-hit, deny, or missed last-hit.
type Event struct {
	Timestamp float32 `json:"timestamp"`
	Type      string  `json:"type"` // "last_hit", "deny", or "missed_last_hit"
	CreepName string  `json:"creep_name"`
}

// pendingCLogCreepEvent is a combat-log creep event waiting for OnEntity correlation.
type pendingCLogCreepEvent struct {
	id            uint64 // shared with conflictGroups key when match is ambiguous
	creepName     string
	gameTime      float32
	health        int32   // post-damage health from combat log
	damage        int32   // damage dealt (value); prev health = health + damage
	candidates    []int32 // entity idxs whose health drop matched this line
	closed        bool    // true once candidate collection finished
	entityMatched bool    // true once bound to entity track(s)
}

// creepTrack holds per-entity creep state for correlating combat log with entity updates.
type creepTrack struct {
	creepName       string // npc name from combat log once correlated
	prevHealth      int32
	hasPrevHealth   bool
	heroDamagedAt   float32 // 0 if our hero has not damaged this creep recently
	conflictGroupID uint64  // pending damage id when ambiguous; 0 = unique match
}

// conflictGroup tracks ambiguous hero-damage correlation across multiple entity idxs.
type conflictGroup struct {
	remainingCombatLogsCount int // hero LH slots left to resolve; decrements on hero kill in group
}

// Handler implements common.ReplayHandler for last-hit, deny, and missed last-hit counting.
type Handler struct {
	heroClass             string
	lastHitsLane          int
	lastHitsJungle        int
	denies                int
	events                []Event
	missedEvents          []Event
	pendingHeroDamageLogs []pendingCLogCreepEvent
	pendingOtherKillLogs  []pendingCLogCreepEvent
	pendingHeroKillLogs   []pendingCLogCreepEvent // combat-log DEATH where our hero was killer
	creepTracks           map[int32]*creepTrack
	conflictGroups        map[uint64]*conflictGroup // keyed by pending damage id
	nextUniqueId          uint64
	timeAndPausesHandler  *timeandpauses.Handler
}

// NewHandler creates a lasthits handler.
func NewHandler(timeAndPausesHandler *timeandpauses.Handler) *Handler {
	return &Handler{
		events:                make([]Event, 0, 256),
		missedEvents:          make([]Event, 0, 128),
		pendingHeroDamageLogs: make([]pendingCLogCreepEvent, 0, 64),
		pendingOtherKillLogs:  make([]pendingCLogCreepEvent, 0, 64),
		pendingHeroKillLogs:   make([]pendingCLogCreepEvent, 0, 64),
		creepTracks:           make(map[int32]*creepTrack, 256),
		conflictGroups:        make(map[uint64]*conflictGroup, 32),
		timeAndPausesHandler:  timeAndPausesHandler,
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

func (h *Handler) GetNextUniqueId() uint64 {
	h.nextUniqueId++
	return h.nextUniqueId
}

// registers lasthits callbacks.
func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		if h.timeAndPausesHandler.IsGameEnded() {
			return nil
		}
		return h.onCombatLogEntry(p, m)
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil || h.timeAndPausesHandler.IsGameEnded() {
			return nil
		}
		return h.onCreepEntity(e, op)
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

func (h *Handler) onCombatLogEntry(p *manta.Parser, m *dota.CMsgDOTACombatLogEntry) error {
	gameTime := h.timeAndPausesHandler.CurrentGameTime()
	h.closePendingHeroDamageBefore(gameTime)
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

	switch m.GetType() {
	case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE:
		attackerClass := common.GetHeroClassName(realAttackerName)
		if attackerClass != h.heroClass || m.GetIsAttackerIllusion() {
			return nil
		}
		if creeps.TypeFromTargetName(realTargetName) == "" {
			return nil
		}
		// Only right-click (auto-attack) damage counts toward missed CS; spells/items set inflictor_name.
		// TODO: handle other inflictors, right now its problematic because fatal bonds, etc. would
		// be counted as missed CS
		if m.GetInflictorName() != 0 {
			return nil
		}
		h.pendingHeroDamageLogs = append(h.pendingHeroDamageLogs, pendingCLogCreepEvent{
			id:        h.GetNextUniqueId(),
			creepName: realTargetName,
			gameTime:  gameTime,
			health:    m.GetHealth(),
			damage:    int32(m.GetValue()),
		})
		return nil

	case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DEATH:
		creepType := creeps.TypeFromTargetName(realTargetName)
		if creepType == "" {
			return nil
		}

		attackerClass := common.GetHeroClassName(realAttackerName)
		weAreKiller := attackerClass == h.heroClass && !m.GetIsAttackerIllusion()

		if weAreKiller {
			h.pendingHeroKillLogs = append(h.pendingHeroKillLogs, pendingCLogCreepEvent{
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
		} else {
			h.pendingOtherKillLogs = append(h.pendingOtherKillLogs, pendingCLogCreepEvent{
				creepName: realTargetName,
				gameTime:  gameTime,
			})
		}
	}
	return nil
}

func (h *Handler) onCreepEntity(e *manta.Entity, op manta.EntityOp) error {
	gameTime := h.timeAndPausesHandler.CurrentGameTime()
	if !creeps.IsEntityClass(e.GetClassName()) {
		return nil
	}
	health, ok := e.GetInt32("m_iHealth")
	if !ok {
		return nil
	}
	h.onCreepHealthUpdate(e.GetIndex(), health, op, gameTime)
	return nil
}

func (h *Handler) onCreepHealthUpdate(entityId int32, health int32, op manta.EntityOp, gameTime float32) {
	track := h.creepTracks[entityId]
	if track == nil {
		track = &creepTrack{}
		h.creepTracks[entityId] = track
	}

	healthReduced := track.hasPrevHealth && health < track.prevHealth
	justDied := (track.hasPrevHealth && health <= 0 && track.prevHealth > 0) ||
		op.Flag(manta.EntityOpDeleted) || op.Flag(manta.EntityOpDeletedLeft)

	if healthReduced {
		h.correlateHeroDamage(entityId, track, health, gameTime)
	}

	track.prevHealth = health
	track.hasPrevHealth = true

	h.closePendingHeroDamageBefore(gameTime)

	if justDied || op.Flag(manta.EntityOpDeleted) || op.Flag(manta.EntityOpDeletedLeft) {
		h.handleCreepDeath(entityId, track, gameTime)
		delete(h.creepTracks, entityId)
	}

	h.pruneMatchedCombatLogs()
}

// MissedEvents returns detected missed last-hit events (for tooling / quality reports).
func (h *Handler) MissedEvents() []Event {
	return h.missedEvents
}

func (h *Handler) prunePendingEvents(gameTime float32) {
	cutoff := gameTime - missedLastHitWindowSec
	// sometimes hero damage does not follow by creep death within the cutoff window
	h.pendingHeroDamageLogs = prunePendingByTime(h.pendingHeroDamageLogs, cutoff)
	h.pendingOtherKillLogs = prunePendingByTime(h.pendingOtherKillLogs, cutoff)
	h.pendingHeroKillLogs = prunePendingByTime(h.pendingHeroKillLogs, cutoff)
}

func prunePendingByTime(events []pendingCLogCreepEvent, cutoff float32) []pendingCLogCreepEvent {
	n := 0
	for _, e := range events {
		if e.entityMatched || e.gameTime < cutoff {
			continue
		}
		events[n] = e
		n++
	}
	return events[:n]
}

func (h *Handler) pruneMatchedCombatLogs() {
	n := 0
	for _, e := range h.pendingHeroDamageLogs {
		if e.entityMatched {
			continue
		}
		h.pendingHeroDamageLogs[n] = e
		n++
	}
	h.pendingHeroDamageLogs = h.pendingHeroDamageLogs[:n]

	n = 0
	for _, e := range h.pendingOtherKillLogs {
		if e.entityMatched {
			continue
		}
		h.pendingOtherKillLogs[n] = e
		n++
	}
	h.pendingOtherKillLogs = h.pendingOtherKillLogs[:n]

	n = 0
	for _, e := range h.pendingHeroKillLogs {
		if e.entityMatched {
			continue
		}
		h.pendingHeroKillLogs[n] = e
		n++
	}
	h.pendingHeroKillLogs = h.pendingHeroKillLogs[:n]
}

func (h *Handler) hasPendingHeroKill(creepName string, deathTime float32) bool {
	cutoff := deathTime - missedLastHitWindowSec
	for _, cLog := range h.pendingHeroKillLogs {
		if cLog.entityMatched || cLog.creepName != creepName {
			continue
		}
		if cLog.gameTime >= cutoff && cLog.gameTime <= deathTime+deathCombatLogEpsilon {
			return true
		}
	}
	return false
}

func (h *Handler) hasPendingOtherKill(creepName string, heroDamagedAt float32) bool {
	for i := range h.pendingOtherKillLogs {
		pendingOtherKillLog := &h.pendingOtherKillLogs[i]
		if pendingOtherKillLog.entityMatched || pendingOtherKillLog.creepName != creepName {
			continue
		}
		if heroDamagedAt <= 0 || pendingOtherKillLog.gameTime-heroDamagedAt > missedLastHitWindowSec {
			continue
		}
		return true
	}
	return false
}

func (h *Handler) matchPendingHeroKill(creepName string, deathTime float32) {
	cutoff := deathTime - missedLastHitWindowSec
	for i := range h.pendingHeroKillLogs {
		cLog := &h.pendingHeroKillLogs[i]
		if cLog.entityMatched || cLog.creepName != creepName {
			continue
		}
		if cLog.gameTime >= cutoff && cLog.gameTime <= deathTime+deathCombatLogEpsilon {
			cLog.entityMatched = true
			return
		}
	}
}

func (h *Handler) matchPendingOtherKill(creepName string, deathTime float32) {
	cutoff := deathTime - missedLastHitWindowSec
	for i := range h.pendingOtherKillLogs {
		cLog := &h.pendingOtherKillLogs[i]
		if cLog.entityMatched || cLog.creepName != creepName {
			continue
		}
		if cLog.gameTime >= cutoff && cLog.gameTime <= deathTime+deathCombatLogEpsilon {
			cLog.entityMatched = true
			return
		}
	}
}

func heroDamageCorrelates(pd pendingCLogCreepEvent, prevHealth, health int32, dropGameTime float32) bool {
	if dropGameTime < pd.gameTime {
		return false
	}
	return health == pd.health && prevHealth == pd.health+pd.damage
}

func (h *Handler) closePendingHeroDamageBefore(gameTime float32) {
	// Stop collecting candidates for old damage lines, then bind unique or conflict group.
	for i := range h.pendingHeroDamageLogs {
		pd := &h.pendingHeroDamageLogs[i]
		if pd.closed {
			continue
		}
		if gameTime > pd.gameTime+tickDuration {
			pd.closed = true
		}
	}
	h.finalizeClosedPendingHeroDamage()
}

type pendingBatchKey struct {
	gameTime  float32
	creepName string
	health    int32
	damage    int32 // batches same-tick identical damage lines into one conflict group
}

func (pd pendingCLogCreepEvent) batchKey() pendingBatchKey {
	return pendingBatchKey{
		gameTime:  pd.gameTime,
		creepName: pd.creepName,
		health:    pd.health,
		damage:    pd.damage,
	}
}

// Group closed pending logs by signature, then bind each batch.
// We can have multiple same combat log events on the same tick (e.g. aoe damage)
func (h *Handler) finalizeClosedPendingHeroDamage() {
	batches := make(map[pendingBatchKey][]*pendingCLogCreepEvent)
	var order []pendingBatchKey

	for i := range h.pendingHeroDamageLogs {
		pd := &h.pendingHeroDamageLogs[i]
		if !pd.closed || pd.entityMatched {
			continue
		}
		key := pd.batchKey()
		if _, ok := batches[key]; !ok {
			order = append(order, key)
		}
		batches[key] = append(batches[key], pd)
	}

	for _, key := range order {
		h.finalizePendingBatch(batches[key])
	}
}

func (h *Handler) finalizePendingBatch(batch []*pendingCLogCreepEvent) {
	// One matching entity → unique bind; multiple → shared conflict group.
	if len(batch) == 0 {
		return
	}

	primary := batch[0]
	candidates := primary.candidates

	// Abnormal case: pending was closed before any entity correlated. Reopen and keep collecting.
	if len(candidates) == 0 {
		ids := make([]uint64, len(batch))
		for i, pd := range batch {
			ids[i] = pd.id
		}
		log.Printf(
			"WARNING lasthits: finalizePendingBatch with ZERO entity candidates (not normal)\n"+
				"  creep=%q gameTime=%.3f postHealth=%d damage=%d pendingLines=%d ids=%v\n"+
				"  action=reopening closed pending lines; no entity health drop matched yet",
			primary.creepName, primary.gameTime, primary.health, primary.damage, len(batch), ids,
		)
		for _, pd := range batch {
			pd.closed = false
		}
		return
	}

	// Abnormal case: less entity candidates than batch size.
	if len(candidates) < len(batch) {
		ids := make([]uint64, len(batch))
		for i, pd := range batch {
			ids[i] = pd.id
		}
		log.Printf(
			"WARNING lasthits: finalizePendingBatch with LESS entity candidates than batch size (not normal)\n"+
				"  creep=%q gameTime=%.3f postHealth=%d damage=%d pendingLines=%d ids=%v\n"+
				"  action=reopening closed pending lines; no entity health drop matched yet",
			primary.creepName, primary.gameTime, primary.health, primary.damage, len(batch), ids,
		)
	}

	for _, pd := range batch {
		pd.entityMatched = true
	}

	if len(candidates) == 1 && len(batch) == 1 {
		h.bindUniqueCandidate(candidates[0], primary.creepName, primary.gameTime)
		return
	}

	groupID := primary.id
	h.conflictGroups[groupID] = &conflictGroup{
		remainingCombatLogsCount: len(batch), // this will be >1 if there are multiple identical combat logs on same tick
	}
	for _, entityId := range candidates {
		h.bindConflictCandidate(entityId, groupID, primary.creepName, primary.gameTime)
	}
}

func (h *Handler) bindUniqueCandidate(entityId int32, creepName string, gameTime float32) {
	h.bindConflictCandidate(entityId, 0, creepName, gameTime)
}

func (h *Handler) bindConflictCandidate(entityId int32, groupID uint64, creepName string, gameTime float32) {
	track := h.creepTracks[entityId]
	if track == nil {
		track = &creepTrack{}
		h.creepTracks[entityId] = track
	}
	track.creepName = creepName
	track.heroDamagedAt = gameTime
	track.conflictGroupID = groupID
}

func (h *Handler) correlateHeroDamage(entityId int32, track *creepTrack, health int32, dropGameTime float32) {
	for i := range h.pendingHeroDamageLogs {
		pd := &h.pendingHeroDamageLogs[i]
		if pd.closed {
			continue
		}

		if heroDamageCorrelates(*pd, track.prevHealth, health, dropGameTime) {
			pd.candidates = slicesx.AppendIfMissing(pd.candidates, entityId)
		}
	}
}

func (h *Handler) resolveConflictGroupHeroLastHit(groupID uint64, entityIdx int32) {
	// One hero LH slot consumed; only unmark the killed creep so siblings stay correlated.
	group := h.conflictGroups[groupID]
	if group == nil {
		return
	}
	group.remainingCombatLogsCount--
}

// Count alive creeps still in the group (defer enemy-kill miss while ambiguous).
func (h *Handler) aliveConflictGroupMembers(groupID uint64, excludeIdx int32) int {
	n := 0
	for idx, track := range h.creepTracks {
		if idx == excludeIdx || track.conflictGroupID != groupID {
			continue
		}
		if track.hasPrevHealth && track.prevHealth > 0 {
			n++
		}
	}
	return n
}

// Consume matching other death pending events.
// Returns true if an event was matched to a log.
func (h *Handler) consumeMatchingOtherDeath(creepName string, heroDamagedAt float32) bool {
	for i := range h.pendingOtherKillLogs {
		pendingOtherKillLog := &h.pendingOtherKillLogs[i]
		if pendingOtherKillLog.entityMatched || pendingOtherKillLog.creepName != creepName {
			continue
		}
		if heroDamagedAt <= 0 || pendingOtherKillLog.gameTime-heroDamagedAt > missedLastHitWindowSec {
			continue
		}
		pendingOtherKillLog.entityMatched = true
		return true
	}
	return false
}

func (h *Handler) resolveConflictGroup(groupID uint64) {
	for i := range h.creepTracks {
		creepTrack := h.creepTracks[i]
		if creepTrack.conflictGroupID == groupID {
			creepTrack.conflictGroupID = 0
			creepTrack.heroDamagedAt = 0
		}
	}
}

// Creep tracks with single candidate: immediate miss on enemy steal;
// Creep tracks with multiple candidates: defer until resolved.
func (h *Handler) handleCreepDeath(idx int32, track *creepTrack, gameTime float32) {
	// Both enemyKill and heroKill can be true at the same if there are 2+ combat logs for creep death on the same tick
	// in this case we assume there is no miss
	enemyKill := h.hasPendingOtherKill(track.creepName, track.heroDamagedAt)
	heroKill := h.hasPendingHeroKill(track.creepName, gameTime)

	log.Printf("Creep %s died at %f; enemyKill=%v heroKill=%v", track.creepName, gameTime, enemyKill, heroKill)
	if track.conflictGroupID == 0 {
		log.Printf("handleCreepDeathWithoutConflict")
		h.handleCreepDeathWithoutConflict(heroKill, enemyKill, track, gameTime)
	} else {
		log.Printf("handleCreepDeathWithConflict")
		h.handleCreepDeathWithConflict(idx, heroKill, enemyKill, track, gameTime)
	}
}

func (h *Handler) handleCreepDeathWithoutConflict(heroKill bool, enemyKill bool, track *creepTrack, gameTime float32) {
	if heroKill {
		track.heroDamagedAt = 0
		h.matchPendingHeroKill(track.creepName, gameTime)
		return
	}

	if enemyKill && track.heroDamagedAt > 0 {
		h.missedEvents = append(h.missedEvents, Event{
			Timestamp: gameTime,
			Type:      "missed_last_hit",
			CreepName: track.creepName,
		})
		track.heroDamagedAt = 0
	}
}

func (h *Handler) handleCreepDeathWithConflict(entityIdx int32, heroKill bool, enemyKill bool, track *creepTrack, gameTime float32) {
	groupID := track.conflictGroupID
	group := h.conflictGroups[groupID]

	// if enemyKill {
	// if h.aliveConflictGroupMembers(groupID, entityIdx) > 0 {
	// 	log.Printf("	No missed event for enemy kill because there are still alive creeps in the group")
	// 	return
	// }
	// if group.remainingCombatLogsCount > 0 {
	// 	log.Printf("	Adding missed event for enemy kill")
	// 	h.missedEvents = append(h.missedEvents, Event{
	// 		Timestamp: gameTime,
	// 		Type:      "missed_last_hit",
	// 		CreepName: track.creepName,
	// 	})
	// } else {
	// 	log.Printf("	No missed event for enemy kill because there are no remaining combat logs in the group")
	// }
	// 	delete(h.conflictGroups, groupID)
	// } else if heroKill {
	// h.matchPendingHeroKill(track.creepName, gameTime)
	// h.resolveConflictGroupHeroLastHit(groupID, entityIdx)
	// }

	if heroKill {
		h.matchPendingHeroKill(track.creepName, gameTime)
		group.remainingCombatLogsCount--
		log.Printf("	Remaining combat logs count: %d", group.remainingCombatLogsCount)
		if group.remainingCombatLogsCount == 0 {
			log.Printf("	Resolving conflict group %d", groupID)
			h.resolveConflictGroup(groupID)
		}
	} else if enemyKill {
		if h.aliveConflictGroupMembers(groupID, entityIdx) > 0 {
			log.Printf("	No missed event for enemy kill because there are still alive creeps in the group")
			return
		}
		if group.remainingCombatLogsCount > 0 {
			log.Printf("	Adding missed event for enemy kill")
			h.missedEvents = append(h.missedEvents, Event{
				Timestamp: gameTime,
				Type:      "missed_last_hit",
				CreepName: track.creepName,
			})
		} else {
			log.Printf("	No missed event for enemy kill because there are no remaining combat logs in the group")
		}
	}
}
