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
	missedLastHitWindowSec   = float32(2.0)
	missedLastHitWindowTicks = uint32(missedLastHitWindowSec * timeandpauses.TickRate)
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
	tick          uint32  // replay tick from manta (combat log ↔ entity correlation key)
	gameTime      float32 // for Event timestamps only
	health        int32   // post-damage health from combat log
	damage        int32   // damage dealt (value); prev health = health + damage
	candidates    []int32 // entity idxs whose health drop matched this line
	closed        bool    // true once candidate collection finished
	entityMatched bool    // true once bound to entity track(s)
}

// creepTrack holds per-entity creep state for correlating combat log with entity updates.
type creepTrack struct {
	creepName       string // npc name from combat log once correlated
	entityName      string // EntityNames from m_iUnitNameIndex (debugging; lane creeps use pathcorner strings)
	className       string // class name from entity
	lane            string // lane from entity name
	side            string // map side from pathcorner spawn geography (good=SW, bad=NE)
	prevHealth      int32
	hasPrevHealth   bool
	heroDamagedTick uint32 // 0 if our hero has not damaged this creep recently
	lastUpdatedTick uint32 // last tick we updated this creep track
	conflictGroupID uint64 // pending damage id when ambiguous; 0 = unique match
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
	lastCombatLogTick     uint32
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
		return h.onCreepEntity(p, e, op)
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
	tick := p.Tick
	gameTime := h.timeAndPausesHandler.CurrentGameTime()
	h.closePendingHeroDamageBeforeTick(tick)
	h.prunePendingEvents(tick)

	h.lastCombatLogTick = tick

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
			tick:      tick,
			gameTime:  gameTime,
			health:    m.GetHealth(),
			damage:    int32(m.GetValue()),
		})

		if tick == 14377 {
			log.Printf("tick %d pendingHeroDamageLogs: %+v", tick, h.pendingHeroDamageLogs)
		}

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
				tick:      tick,
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
				tick:      tick,
				gameTime:  gameTime,
			})
		}
	}
	return nil
}

func entityName(p *manta.Parser, e *manta.Entity) string {
	nameIdx, ok := e.GetInt32("m_iUnitNameIndex")
	if !ok {
		return ""
	}
	name, ok := p.LookupStringByIndex("EntityNames", nameIdx)
	if !ok {
		return ""
	}
	return name
}

func (h *Handler) onCreepEntity(p *manta.Parser, e *manta.Entity, op manta.EntityOp) error {
	entityName := entityName(p, e)
	className := e.GetClassName()
	if !creeps.IsLaneCreep(className) {
		return nil
	}
	x, y := creeps.GetEntityLocation(e)
	isMaxHealth := creeps.IsMaxHealth(e)
	health, ok := e.GetInt32("m_iHealth")
	if !ok {
		return nil
	}
	gameTime := h.timeAndPausesHandler.CurrentGameTime()
	h.onCreepHealthUpdate(e.GetIndex(), health, isMaxHealth, x, y, op, p.Tick, gameTime, entityName, className)
	return nil
}

func (h *Handler) onCreepHealthUpdate(entityId int32, health int32, isMaxHealth bool, x float32, y float32, op manta.EntityOp, tick uint32, gameTime float32, entityName string, className string) {
	track := h.creepTracks[entityId]
	if track == nil {
		track = &creepTrack{}
		h.creepTracks[entityId] = track
		// Sometimes we see entity for the first time in some random positions along the lanes
		if isMaxHealth {
			track.lane = creeps.GetCreepLaneFromSpawnLocation(x, y)
			sideCandidate1 := creeps.GetCreepSideFromSpawnLocation(x, y)
			sideCandidate2 := creeps.GetCreepSide(entityName)
			if sideCandidate1 != sideCandidate2 {
				log.Printf("ERROR: sideCandidate1 != sideCandidate2: %s != %s", sideCandidate1, sideCandidate2)
				// log.Printf("entityName: %s", entityName)
				// log.Printf("x: %f y: %f", x, y)
				// log.Printf("lane: %s", track.lane)
				// log.Printf("sideCandidate1: %s", sideCandidate1)
				// log.Printf("sideCandidate2: %s", sideCandidate2)
			} else {
				track.side = sideCandidate1
			}
		}
	}
	track.entityName = entityName
	track.className = className
	track.lastUpdatedTick = tick

	healthReduced := track.hasPrevHealth && health < track.prevHealth
	justDied := (track.hasPrevHealth && health <= 0 && track.prevHealth > 0) ||
		op.Flag(manta.EntityOpDeleted) || op.Flag(manta.EntityOpDeletedLeft)

	// Debug for PA 4:10
	// if (tick >= 14377 && tick <= 14380) && track.prevHealth != 550 && track.prevHealth != 300 && track.side == "bad" && track.lane == "top" {
	// 	log.Printf("game time %f (m = %f, s = %f)", gameTime, math.Floor(float64(gameTime/60)), gameTime-float32(60*math.Floor(float64(gameTime/60))))
	// 	// {id:33 creepName:npc_dota_creep_badguys_ranged tick:14377 gameTime:240.93335 health:16 damage:73 candidates:[] closed:false entityMatched:false}
	// 	log.Printf("tick %d entityId %d creepTrack: %+v", tick, entityId, track)
	// 	log.Printf("health=%d prevHealth=%d", health, track.prevHealth)
	// }

	if healthReduced {
		h.correlateHeroDamage(entityId, track, health, tick)
	}

	track.prevHealth = health
	track.hasPrevHealth = true

	if justDied || op.Flag(manta.EntityOpDeleted) || op.Flag(manta.EntityOpDeletedLeft) {
		h.finalizePendingHeroDamageForTick(tick)
		h.handleCreepDeath(entityId, track, tick, gameTime)
		delete(h.creepTracks, entityId)
	}

	h.pruneMatchedCombatLogs()
}

// MissedEvents returns detected missed last-hit events (for tooling / quality reports).
func (h *Handler) MissedEvents() []Event {
	return h.missedEvents
}

func (h *Handler) prunePendingEvents(currentTick uint32) {
	cutoff := currentTick - missedLastHitWindowTicks
	h.pendingHeroDamageLogs = prunePendingByTick(h.pendingHeroDamageLogs, cutoff)
	h.pendingOtherKillLogs = prunePendingByTick(h.pendingOtherKillLogs, cutoff)
	h.pendingHeroKillLogs = prunePendingByTick(h.pendingHeroKillLogs, cutoff)
}

func prunePendingByTick(events []pendingCLogCreepEvent, cutoffTick uint32) []pendingCLogCreepEvent {
	n := 0
	for _, e := range events {
		if e.entityMatched || e.tick < cutoffTick {
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

func replayTicksWithinWindow(fromTick, toTick uint32) bool {
	if toTick < fromTick {
		return false
	}
	return toTick-fromTick <= missedLastHitWindowTicks
}

func (h *Handler) hasPendingHeroKill(creepName string, deathTick uint32) bool {
	for _, cLog := range h.pendingHeroKillLogs {
		if cLog.entityMatched || cLog.creepName != creepName {
			continue
		}
		if cLog.tick <= deathTick && replayTicksWithinWindow(cLog.tick, deathTick) {
			return true
		}
	}
	return false
}

func (h *Handler) hasPendingOtherKill(creepName string, heroDamagedTick uint32) bool {
	if heroDamagedTick == 0 {
		return false
	}
	for i := range h.pendingOtherKillLogs {
		pendingOtherKillLog := &h.pendingOtherKillLogs[i]
		if pendingOtherKillLog.entityMatched || pendingOtherKillLog.creepName != creepName {
			continue
		}
		if !replayTicksWithinWindow(heroDamagedTick, pendingOtherKillLog.tick) {
			continue
		}
		return true
	}
	return false
}

func (h *Handler) matchPendingHeroKill(creepName string, deathTick uint32) {
	for i := range h.pendingHeroKillLogs {
		cLog := &h.pendingHeroKillLogs[i]
		if cLog.entityMatched || cLog.creepName != creepName {
			continue
		}
		if cLog.tick <= deathTick && replayTicksWithinWindow(cLog.tick, deathTick) {
			cLog.entityMatched = true
			return
		}
	}
}

func heroDamageCorrelates(pd pendingCLogCreepEvent, prevHealth, health int32, entityTick uint32) bool {
	if entityTick < pd.tick {
		return false
	}
	if !replayTicksWithinWindow(pd.tick, entityTick) {
		return false
	}
	if prevHealth != pd.health+pd.damage {
		return false
	}
	if health == pd.health {
		return true
	}
	// Same replay tick: entity may jump to 0/death without reporting post-damage HP.
	return health <= 0 && entityTick == pd.tick
}

// closePendingHeroDamageBeforeTick closes pending damage from earlier replay ticks (entity phase done).
func (h *Handler) closePendingHeroDamageBeforeTick(currentTick uint32) {
	for i := range h.pendingHeroDamageLogs {
		pd := &h.pendingHeroDamageLogs[i]
		if pd.closed {
			continue
		}
		if currentTick > pd.tick {
			pd.closed = true
		}
	}
	h.finalizeClosedPendingHeroDamage()
}

// finalizePendingHeroDamageForTick binds pending damage queued on this replay tick before creep death.
func (h *Handler) finalizePendingHeroDamageForTick(tick uint32) {
	for i := range h.pendingHeroDamageLogs {
		pd := &h.pendingHeroDamageLogs[i]
		if pd.closed || pd.tick != tick {
			continue
		}
		pd.closed = true
	}
	h.finalizeClosedPendingHeroDamage()
}

type pendingBatchKey struct {
	tick      uint32
	creepName string
	health    int32
	damage    int32 // batches same-tick identical damage lines into one conflict group
}

func (pd pendingCLogCreepEvent) batchKey() pendingBatchKey {
	return pendingBatchKey{
		tick:      pd.tick,
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
		// log.Printf(
		// 	"WARNING lasthits: finalizePendingBatch with ZERO entity candidates (not normal)\n"+
		// 		"  creep=%q tick=%d postHealth=%d damage=%d pendingLogs=%d\n"+
		// 		"  action=reopening closed pending lines; no entity health drop matched yet",
		// 	primary.creepName, primary.tick, primary.health, primary.damage, len(batch),
		// )
		// Display all creeps with matching name
		// for i := range h.creepTracks {
		// 	creepTrack := h.creepTracks[i]
		// 	if creepTrack.creepName == primary.creepName {
		// 		log.Printf("Creep track: %+v", creepTrack)
		// 	}
		// }
		// for i := range batch {
		// 	pd := batch[i]
		// 	pd.closed = false
		// }
		return
	}

	// Abnormal case: less entity candidates than batch size. This can happen when both logs hit same entity on same tick.
	if len(candidates) < len(batch) {
		ids := make([]uint64, len(batch))
		for i, pd := range batch {
			ids[i] = pd.id
		}
		// log.Printf(
		// 	"WARNING lasthits: finalizePendingBatch with LESS entity candidates than batch size (not normal)\n"+
		// 		"  creep=%q tick=%d postHealth=%d damage=%d pendingLines=%d\n"+
		// 		"  action=reopening closed pending lines; no entity health drop matched yet",
		// 	primary.creepName, primary.tick, primary.health, primary.damage, len(batch),
		// )
	}

	for _, pd := range batch {
		pd.entityMatched = true
	}

	if len(candidates) == 1 && len(batch) == 1 {
		h.bindUniqueCandidate(candidates[0], primary.creepName, primary.tick)
		return
	}

	groupID := primary.id
	h.conflictGroups[groupID] = &conflictGroup{
		remainingCombatLogsCount: len(batch), // this will be >1 if there are multiple identical combat logs on same tick
	}
	for _, entityId := range candidates {
		h.bindConflictCandidate(entityId, groupID, primary.creepName, primary.tick)
	}
}

func (h *Handler) bindUniqueCandidate(entityId int32, creepName string, damageTick uint32) {
	h.bindConflictCandidate(entityId, 0, creepName, damageTick)
}

func (h *Handler) bindConflictCandidate(entityId int32, groupID uint64, creepName string, damageTick uint32) {
	track := h.creepTracks[entityId]
	if track == nil {
		track = &creepTrack{}
		h.creepTracks[entityId] = track
	}
	track.creepName = creepName
	track.heroDamagedTick = damageTick
	track.conflictGroupID = groupID
}

func (h *Handler) correlateHeroDamage(entityId int32, track *creepTrack, health int32, entityTick uint32) {
	for i := range h.pendingHeroDamageLogs {
		pd := &h.pendingHeroDamageLogs[i]
		if pd.closed {
			continue
		}

		if heroDamageCorrelates(*pd, track.prevHealth, health, entityTick) {
			pd.candidates = slicesx.AppendIfMissing(pd.candidates, entityId)
		}
	}
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

func (h *Handler) resolveConflictGroup(groupID uint64) {
	for i := range h.creepTracks {
		creepTrack := h.creepTracks[i]
		if creepTrack.conflictGroupID == groupID {
			creepTrack.conflictGroupID = 0
			creepTrack.heroDamagedTick = 0
		}
	}
}

// Creep tracks with single candidate: immediate miss on enemy steal;
// Creep tracks with multiple candidates: defer until resolved.
func (h *Handler) handleCreepDeath(idx int32, track *creepTrack, deathTick uint32, gameTime float32) {
	// Both enemyKill and heroKill can be true at the same if there are 2+ combat logs for creep death on the same tick
	// in this case we assume there is no miss
	enemyKill := h.hasPendingOtherKill(track.creepName, track.heroDamagedTick)
	heroKill := h.hasPendingHeroKill(track.creepName, deathTick)

	if track.conflictGroupID == 0 {
		h.handleCreepDeathWithoutConflict(heroKill, enemyKill, track, deathTick, gameTime)
	} else {
		h.handleCreepDeathWithConflict(idx, heroKill, enemyKill, track, deathTick, gameTime)
	}
}

func (h *Handler) handleCreepDeathWithoutConflict(heroKill bool, enemyKill bool, track *creepTrack, deathTick uint32, gameTime float32) {
	if heroKill {
		h.matchPendingHeroKill(track.creepName, deathTick)
		track.heroDamagedTick = 0
		return
	}

	if enemyKill && track.heroDamagedTick > 0 {
		h.missedEvents = append(h.missedEvents, Event{
			Timestamp: gameTime,
			Type:      "missed_last_hit",
			CreepName: track.creepName,
		})
		track.heroDamagedTick = 0
	}
}

func (h *Handler) handleCreepDeathWithConflict(entityIdx int32, heroKill bool, enemyKill bool, track *creepTrack, deathTick uint32, gameTime float32) {
	groupID := track.conflictGroupID
	group := h.conflictGroups[groupID]

	if heroKill {
		h.matchPendingHeroKill(track.creepName, deathTick)
		group.remainingCombatLogsCount--
		if group.remainingCombatLogsCount == 0 {
			h.resolveConflictGroup(groupID)
		}
	} else if enemyKill {
		if h.aliveConflictGroupMembers(groupID, entityIdx) > 0 {
			return
		}
		if group.remainingCombatLogsCount > 0 {
			h.missedEvents = append(h.missedEvents, Event{
				Timestamp: gameTime,
				Type:      "missed_last_hit",
				CreepName: track.creepName,
			})
		}
	}
}
