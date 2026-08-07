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
	retroactiveDropMaxLag  = float32(0.1)   // entity game time can trail combat log on the same tick
	deathCombatLogEpsilon  = float32(0.05)  // combat-log DEATH can trail entity death on the same tick
)

// Event is a single last-hit, deny, or missed last-hit.
type Event struct {
	Timestamp float32 `json:"timestamp"`
	Type      string  `json:"type"` // "last_hit", "deny", or "missed_last_hit"
	CreepName string  `json:"creep_name"`
}

// pendingCLogCreepEvent is a combat-log creep event waiting for OnEntity correlation.
type pendingCLogCreepEvent struct {
	id         uint64 // shared with conflictGroups key when match is ambiguous
	creepName  string
	gameTime   float32
	health     int32   // post-damage health from combat log
	damage     int32   // damage dealt (value); prev health = health + damage
	candidates []int32 // entity idxs whose health drop matched this line
	closed     bool    // true once candidate collection finished
	consumed   bool    // true once bound to entity track(s)
}

// creepTrack holds per-entity creep state for correlating combat log with entity updates.
type creepTrack struct {
	creepName              string // npc name from combat log once correlated
	prevHealth             int32
	hasHealth              bool
	heroDamagedAt          float32 // 0 if our hero has not damaged this creep recently
	conflictGroupID        uint64  // pending damage id when ambiguous; 0 = unique match
	healthBeforeLastDrop   int32   // prev HP when last drop was observed (for retroactive match)
	lastDropAt             float32 // game time of last health drop
	awaitingDeathCombatLog bool    // entity died before combat-log DEATH line on same tick
}

// conflictGroup tracks ambiguous hero-damage correlation across multiple entity idxs.
type conflictGroup struct {
	remainingCombatLogsCount int  // hero LH slots left to resolve; decrements on hero kill in group
	heroLastHitInGroup       bool // any hero LH in this group suppresses miss on enemy kills
}

// Handler implements common.ReplayHandler for last-hit, deny, and missed last-hit counting.
type Handler struct {
	heroClass            string
	lastHitsLane         int
	lastHitsJungle       int
	denies               int
	events               []Event
	missedEvents         []Event
	pendingHeroDamage    []pendingCLogCreepEvent
	pendingOtherDeath    []pendingCLogCreepEvent
	pendingHeroKills     []pendingCLogCreepEvent // combat-log DEATH where our hero was killer
	creepTracks          map[int32]*creepTrack
	conflictGroups       map[uint64]*conflictGroup // keyed by pending damage id
	nextPendingDamageID  uint64
	timeAndPausesHandler *timeandpauses.Handler
}

// NewHandler creates a lasthits handler.
func NewHandler(timeAndPausesHandler *timeandpauses.Handler) *Handler {
	return &Handler{
		events:               make([]Event, 0, 256),
		missedEvents:         make([]Event, 0, 128),
		pendingHeroDamage:    make([]pendingCLogCreepEvent, 0, 64),
		pendingOtherDeath:    make([]pendingCLogCreepEvent, 0, 64),
		pendingHeroKills:     make([]pendingCLogCreepEvent, 0, 64),
		creepTracks:          make(map[int32]*creepTrack, 256),
		conflictGroups:       make(map[uint64]*conflictGroup, 32),
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
		return h.onCombatLogEntry(p, m)
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil || h.timeAndPausesHandler.IsGameEnded() {
			return nil
		}
		return h.onCreepEntity(e, op)
	})
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
		if m.GetInflictorName() != 0 {
			return nil
		}
		h.nextPendingDamageID++
		h.pendingHeroDamage = append(h.pendingHeroDamage, pendingCLogCreepEvent{
			id:        h.nextPendingDamageID,
			creepName: realTargetName,
			gameTime:  gameTime,
			health:    m.GetHealth(),
			damage:    int32(m.GetValue()),
		})
		h.retroactiveCorrelateOpenPending()
		return nil

	case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DEATH:
		creepType := creeps.TypeFromTargetName(realTargetName)
		if creepType == "" {
			return nil
		}

		attackerClass := common.GetHeroClassName(realAttackerName)
		weAreKiller := attackerClass == h.heroClass && !m.GetIsAttackerIllusion()

		if weAreKiller {
			h.pendingHeroKills = append(h.pendingHeroKills, pendingCLogCreepEvent{
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
			h.pendingOtherDeath = append(h.pendingOtherDeath, pendingCLogCreepEvent{
				creepName: realTargetName,
				gameTime:  gameTime,
			})
			h.resolveAwaitingDeathCombatLog(realTargetName, gameTime)
		}
	}
	return nil
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

// MissedEvents returns detected missed last-hit events (for tooling / quality reports).
func (h *Handler) MissedEvents() []Event {
	return h.missedEvents
}

func (h *Handler) prunePendingEvents(gameTime float32) {
	cutoff := gameTime - missedLastHitWindowSec
	// sometimes hero damage does not follow by creep death within the cutoff window
	h.pendingHeroDamage = prunePendingByTime(h.pendingHeroDamage, cutoff)
	h.pendingOtherDeath = prunePendingByTime(h.pendingOtherDeath, cutoff)
	h.pendingHeroKills = prunePendingByTime(h.pendingHeroKills, cutoff)
}

func prunePendingByTime(events []pendingCLogCreepEvent, cutoff float32) []pendingCLogCreepEvent {
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

func (h *Handler) hasPendingLasthit(creepName string, deathTime float32) bool {
	cutoff := deathTime - missedLastHitWindowSec
	for _, sk := range h.pendingHeroKills {
		if sk.creepName == creepName && sk.gameTime >= cutoff && sk.gameTime <= deathTime+deathCombatLogEpsilon {
			return true
		}
	}
	return false
}

func (h *Handler) consumePendingLasthit(creepName string, deathTime float32) {
	cutoff := deathTime - missedLastHitWindowSec
	n := 0
	for _, sk := range h.pendingHeroKills {
		if sk.creepName == creepName && sk.gameTime >= cutoff && sk.gameTime <= deathTime+deathCombatLogEpsilon {
			continue
		}
		h.pendingHeroKills[n] = sk
		n++
	}
	h.pendingHeroKills = h.pendingHeroKills[:n]
}

func heroDamageCorrelates(pd pendingCLogCreepEvent, prevHealth, health int32, healthReduced bool, dropGameTime float32) bool {
	if dropGameTime < pd.gameTime {
		return false
	}
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

func (h *Handler) closePendingHeroDamageBefore(gameTime float32) {
	// Stop collecting candidates for old damage lines, then bind unique or conflict group.
	for i := range h.pendingHeroDamage {
		pd := &h.pendingHeroDamage[i]
		if pd.consumed || pd.closed {
			continue
		}
		if gameTime > pd.gameTime+tickDuration {
			pd.closed = true
		}
	}
	h.finalizeClosedPendingHeroDamage()
}

// closePendingHeroDamageForDeadCreep closes pending lines that matched the dying entity.
// Unrelated pending lines (other combat-log damage still collecting candidates) stay open.
func (h *Handler) closePendingHeroDamageForDeadCreep(deadIdx int32) {
	for i := range h.pendingHeroDamage {
		pd := &h.pendingHeroDamage[i]
		if pd.consumed || pd.closed {
			continue
		}
		for _, c := range pd.candidates {
			if c == deadIdx {
				pd.closed = true
				break
			}
		}
	}
	h.finalizeClosedPendingHeroDamage()
}

// finalizeAmbiguousPending closes a pending line once multiple entity idxs match it.
// When several combat-log lines share the same signature on one tick, wait one tick
// so each line can bind a distinct creep before finalizing the batch.
func (h *Handler) finalizeAmbiguousPending() {
	openByKey := make(map[pendingBatchKey]int)
	for i := range h.pendingHeroDamage {
		pd := &h.pendingHeroDamage[i]
		if pd.consumed || pd.closed {
			continue
		}
		key := pendingBatchKey{
			gameTime:  pd.gameTime,
			creepName: pd.creepName,
			health:    pd.health,
			damage:    pd.damage,
		}
		openByKey[key]++
	}

	for i := range h.pendingHeroDamage {
		pd := &h.pendingHeroDamage[i]
		if pd.consumed || pd.closed {
			continue
		}
		if len(slicesx.Unique(pd.candidates)) < 2 {
			continue
		}
		key := pendingBatchKey{
			gameTime:  pd.gameTime,
			creepName: pd.creepName,
			health:    pd.health,
			damage:    pd.damage,
		}
		if openByKey[key] > 1 {
			continue
		}
		pd.closed = true
	}
	h.finalizeClosedPendingHeroDamage()
}

type pendingBatchKey struct {
	gameTime  float32
	creepName string
	health    int32
	damage    int32 // batches same-tick identical damage lines into one conflict group
}

func (h *Handler) finalizeClosedPendingHeroDamage() {
	// Group closed pendings by signature, then bind each batch.
	// We can have multiple same combat log events on the same tick (e.g. aoe damage)
	batches := make(map[pendingBatchKey][]*pendingCLogCreepEvent)
	var order []pendingBatchKey

	for i := range h.pendingHeroDamage {
		pd := &h.pendingHeroDamage[i]
		if !pd.closed || pd.consumed {
			continue
		}
		if pd.id == 0 {
			h.nextPendingDamageID++
			pd.id = h.nextPendingDamageID
		}
		key := pendingBatchKey{
			gameTime:  pd.gameTime,
			creepName: pd.creepName,
			health:    pd.health,
			damage:    pd.damage,
		}
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

	var allCandidates []int32
	for _, pd := range batch {
		allCandidates = append(allCandidates, pd.candidates...)
	}
	candidates := slicesx.Unique(allCandidates)

	if len(candidates) == 0 {
		// Abnormal: pending was closed before any entity idx correlated. Reopen and keep collecting.
		primary := batch[0]
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

	primary := batch[0]
	for _, pd := range batch {
		pd.consumed = true
	}

	if len(candidates) == 1 {
		h.bindUniqueCandidate(candidates[0], primary.creepName, primary.gameTime)
		return
	}

	groupID := primary.id
	h.conflictGroups[groupID] = &conflictGroup{
		remainingCombatLogsCount: len(batch),
	}
	for _, idx := range candidates {
		h.bindConflictCandidate(idx, groupID, primary.creepName, primary.gameTime)
	}
}

func (h *Handler) bindUniqueCandidate(idx int32, creepName string, gameTime float32) {
	track := h.creepTracks[idx]
	if track == nil {
		track = &creepTrack{}
		h.creepTracks[idx] = track
	}
	track.creepName = creepName
	track.heroDamagedAt = gameTime
	track.conflictGroupID = 0
}

func (h *Handler) bindConflictCandidate(idx int32, groupID uint64, creepName string, gameTime float32) {
	track := h.creepTracks[idx]
	if track == nil {
		track = &creepTrack{}
		h.creepTracks[idx] = track
	}
	track.creepName = creepName
	track.heroDamagedAt = gameTime
	track.conflictGroupID = groupID
}

func (h *Handler) correlateHeroDamage(idx int32, track *creepTrack, health int32, healthReduced bool, dropGameTime float32) {
	// Bind to the most recent open combat-log line at or before this drop; same-tick
	// batches share gameTime and all receive the candidate.
	var bestTime float32 = -1
	for i := range h.pendingHeroDamage {
		pd := &h.pendingHeroDamage[i]
		if pd.consumed || pd.closed {
			continue
		}
		if !heroDamageCorrelates(*pd, track.prevHealth, health, healthReduced, dropGameTime) {
			continue
		}
		if pd.gameTime > bestTime {
			bestTime = pd.gameTime
		}
	}
	if bestTime < 0 {
		return
	}

	for i := range h.pendingHeroDamage {
		pd := &h.pendingHeroDamage[i]
		if pd.consumed || pd.closed || pd.gameTime != bestTime {
			continue
		}
		if !heroDamageCorrelates(*pd, track.prevHealth, health, healthReduced, dropGameTime) {
			continue
		}
		if pd.id == 0 {
			h.nextPendingDamageID++
			pd.id = h.nextPendingDamageID
		}
		found := false
		for _, c := range pd.candidates {
			if c == idx {
				found = true
				break
			}
		}
		if found {
			continue
		}
		pd.candidates = append(pd.candidates, idx)
	}
}

func (h *Handler) retroactiveCorrelateOpenPending() {
	// Manta dispatches PacketEntities before combat log on the same tick, so entity
	// health drops may already be recorded when the DAMAGE line arrives.
	for i := range h.pendingHeroDamage {
		pd := &h.pendingHeroDamage[i]
		if pd.consumed || pd.closed {
			continue
		}
		for idx, track := range h.creepTracks {
			if !retroactiveTrackMatchesPending(*pd, track) {
				continue
			}
			found := false
			for _, c := range pd.candidates {
				if c == idx {
					found = true
					break
				}
			}
			if !found {
				pd.candidates = append(pd.candidates, idx)
			}
		}
	}
}

func retroactiveTrackMatchesPending(pd pendingCLogCreepEvent, track *creepTrack) bool {
	if !track.hasHealth {
		return false
	}
	dropTimeOK := dropTimeMatchesPending(track.lastDropAt, pd.gameTime)
	if track.prevHealth == pd.health && dropTimeOK {
		if pd.damage <= 0 || track.healthBeforeLastDrop == pd.health+pd.damage {
			return true
		}
	}
	// Same-tick kill: entity dropped to post-damage HP then died before combat log ran.
	if pd.damage > 0 && track.healthBeforeLastDrop == pd.health+pd.damage && dropTimeOK {
		return true
	}
	return false
}

func dropTimeMatchesPending(dropAt, pendingAt float32) bool {
	d := dropAt - pendingAt
	if d < 0 {
		d = -d
	}
	return d <= retroactiveDropMaxLag
}

func (h *Handler) resolveAwaitingDeathCombatLog(creepName string, gameTime float32) {
	for idx, track := range h.creepTracks {
		if !track.awaitingDeathCombatLog || track.creepName != creepName {
			continue
		}
		track.awaitingDeathCombatLog = false
		h.handleCreepDeath(idx, track, gameTime, true)
	}
}

func (h *Handler) clearConflictTrack(idx int32) {
	track := h.creepTracks[idx]
	if track == nil {
		return
	}
	track.conflictGroupID = 0
	track.heroDamagedAt = 0
	track.creepName = ""
}

func (h *Handler) resolveConflictGroupHeroLastHit(groupID uint64, idx int32) {
	// One hero LH slot consumed; only unmark the killed creep so siblings stay correlated.
	group := h.conflictGroups[groupID]
	if group == nil {
		return
	}
	group.heroLastHitInGroup = true
	group.remainingCombatLogsCount--
	h.clearConflictTrack(idx)
}

func (h *Handler) aliveConflictGroupMembers(groupID uint64, excludeIdx int32) int {
	// Count live creeps still in the group (defer enemy-kill miss while ambiguous).
	n := 0
	for idx, track := range h.creepTracks {
		if idx == excludeIdx || track.conflictGroupID != groupID {
			continue
		}
		if track.hasHealth && track.prevHealth > 0 {
			n++
		}
	}
	return n
}

func (h *Handler) consumeMatchingOtherDeath(creepName string, heroDamagedAt float32, gameTime float32) bool {
	for i := range h.pendingOtherDeath {
		pd := &h.pendingOtherDeath[i]
		if pd.consumed || pd.creepName != creepName {
			continue
		}
		if heroDamagedAt <= 0 || pd.gameTime-heroDamagedAt > missedLastHitWindowSec {
			continue
		}
		pd.consumed = true
		return true
	}
	return false
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

func (h *Handler) onCreepHealthUpdate(idx int32, health int32, op manta.EntityOp, gameTime float32) {
	track := h.creepTracks[idx]
	if track == nil {
		track = &creepTrack{}
		h.creepTracks[idx] = track
	}

	healthReduced := track.hasHealth && health < track.prevHealth
	justDied := (track.hasHealth && health <= 0 && track.prevHealth > 0) ||
		op.Flag(manta.EntityOpDeleted) || op.Flag(manta.EntityOpDeletedLeft)

	if healthReduced {
		track.healthBeforeLastDrop = track.prevHealth
		track.lastDropAt = gameTime
	}

	h.correlateHeroDamage(idx, track, health, healthReduced, gameTime)

	track.prevHealth = health
	track.hasHealth = true

	h.finalizeAmbiguousPending()
	h.closePendingHeroDamageBefore(gameTime)

	if justDied {
		h.closePendingHeroDamageForDeadCreep(idx)
		h.handleCreepDeath(idx, track, gameTime, false)
	}

	h.pruneConsumedPending()

	if op.Flag(manta.EntityOpDeleted) || op.Flag(manta.EntityOpDeletedLeft) {
		delete(h.creepTracks, idx)
	}
}

func (h *Handler) handleCreepDeath(idx int32, track *creepTrack, gameTime float32, fromCombatLog bool) {
	// Unique tracks: immediate miss on enemy steal; conflict groups defer until resolved.
	enemyKill := fromCombatLog || h.consumeMatchingOtherDeath(track.creepName, track.heroDamagedAt, gameTime)
	heroKill := h.hasPendingLasthit(track.creepName, gameTime)

	if !fromCombatLog && !enemyKill && !heroKill && track.heroDamagedAt > 0 && track.creepName != "" {
		// Entity death arrived before combat-log DEATH on the same tick.
		track.awaitingDeathCombatLog = true
		return
	}

	if track.conflictGroupID != 0 {
		groupID := track.conflictGroupID
		group := h.conflictGroups[groupID]
		if enemyKill {
			if group != nil && h.aliveConflictGroupMembers(groupID, idx) > 0 {
				return
			}
			if group != nil && !group.heroLastHitInGroup && group.remainingCombatLogsCount > 0 {
				h.missedEvents = append(h.missedEvents, Event{
					Timestamp: gameTime,
					Type:      "missed_last_hit",
					CreepName: track.creepName,
				})
			}
			h.clearConflictTrack(idx)
			if group != nil && h.aliveConflictGroupMembers(groupID, idx) == 0 {
				delete(h.conflictGroups, groupID)
			}
			return
		}
		if heroKill {
			h.consumePendingLasthit(track.creepName, gameTime)
			h.resolveConflictGroupHeroLastHit(groupID, idx)
		}
		return
	}

	if heroKill {
		track.heroDamagedAt = 0
		h.consumePendingLasthit(track.creepName, gameTime)
		return
	}
	if enemyKill && track.creepName != "" && track.heroDamagedAt > 0 {
		h.missedEvents = append(h.missedEvents, Event{
			Timestamp: gameTime,
			Type:      "missed_last_hit",
			CreepName: track.creepName,
		})
		track.heroDamagedAt = 0
	}
}
