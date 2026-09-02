package lasthits

import (
	_ "embed"
	"errors"
	"log"
	"slices"
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/common"
	"dota2/internal/creeps"
	"dota2/internal/slicesx"
	"dota2/internal/timeandpauses"
)

//go:embed lasthit_inflictors.txt
var lasthitInflictorsFile string

const (
	missedLastHitWindowSec   = float32(2.5)
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
	combatLogCreepname string  // npc name from combat log once correlated
	entityName         string  // EntityNames from m_iUnitNameIndex (debugging; lane creeps use pathcorner strings)
	className          string  // class name from entity
	creepKind          string  // kind from entity
	lane               string  // lane from entity name
	side               string  // map side from pathcorner spawn geography (good=SW, bad=NE)
	spawnTimestamp     float32 // game time when the creep spawned
	spawnWave          int     // wave number when the creep spawned
	prevHealth         int32
	currentHealth      int32
	hasPrevHealth      bool
	heroDamagedTick    uint32 // 0 if our hero has not damaged this creep recently
	lastUpdatedTick    uint32 // last tick we updated this creep track
	conflictGroupID    uint64 // pending damage id when ambiguous; 0 = unique match
}

// conflictGroup tracks ambiguous hero-damage correlation across multiple entity idxs.
type conflictGroup struct {
	remainingCombatLogsCount int // hero LH slots left to resolve; decrements on hero kill in group
}

// Handler implements common.ReplayHandler for last-hit, deny, and missed last-hit counting.
type Handler struct {
	heroClass                    string
	lastHitsLane                 int
	lastHitsJungle               int
	denies                       int
	lastCombatLogTick            uint32
	events                       []Event
	missedEvents                 []Event
	pendingHeroDamageLogs        []pendingCLogCreepEvent
	pendingOtherKillLogs         []pendingCLogCreepEvent
	pendingHeroKillLogs          []pendingCLogCreepEvent // combat-log DEATH where our hero was killer
	pendingHealthReducedCreepIds []int32
	pendingDeadCreeps            []int32
	creepsMatchedThisTick        []int32 // used to remove creeps from match pool if they were matched EXACTLY for some other cLog
	creepTracks                  map[int32]*creepTrack
	conflictGroups               map[uint64]*conflictGroup // keyed by pending damage id
	nextUniqueId                 uint64
	timeAndPausesHandler         *timeandpauses.Handler
	lasthitInflictorList         []string
}

// NewHandler creates a lasthits handler.
func NewHandler(timeAndPausesHandler *timeandpauses.Handler) *Handler {
	return &Handler{
		events:                       make([]Event, 0, 256),
		missedEvents:                 make([]Event, 0, 128),
		pendingHeroDamageLogs:        make([]pendingCLogCreepEvent, 0, 64),
		pendingOtherKillLogs:         make([]pendingCLogCreepEvent, 0, 64),
		pendingHeroKillLogs:          make([]pendingCLogCreepEvent, 0, 64),
		pendingHealthReducedCreepIds: make([]int32, 0, 64),
		pendingDeadCreeps:            make([]int32, 0, 64),
		creepTracks:                  make(map[int32]*creepTrack, 256),
		conflictGroups:               make(map[uint64]*conflictGroup, 32),
		creepsMatchedThisTick:        make([]int32, 0, 32),
		timeAndPausesHandler:         timeAndPausesHandler,
		lasthitInflictorList:         loadLasthitInflictors(lasthitInflictorsFile),
	}
}

func loadLasthitInflictors(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, _, _ := strings.Cut(line, "\t")
		out = append(out, name)
	}
	return out
}

func isLasthitInflictorDamage(inflictorName uint32, list []string, lookup func(uint32) (string, bool)) bool {
	if inflictorName == 0 {
		return true
	}
	name, ok := lookup(inflictorName)
	if !ok {
		return false
	}
	// dota_unknown = right click attack
	if name == "dota_unknown" {
		return true
	}
	return slices.Contains(list, name)
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

func (h *Handler) correlateDamagesBy(removeCreepsFromMatchPool bool, correlate func(pd pendingCLogCreepEvent, creep *creepTrack) bool) {
	for i := range h.pendingHeroDamageLogs {
		pd := &h.pendingHeroDamageLogs[i]
		if pd.closed {
			continue
		}

		for _, v := range h.pendingHealthReducedCreepIds {
			creep := h.creepTracks[v]
			if correlate(*pd, creep) && !slices.Contains(h.creepsMatchedThisTick, v) {
				pd.candidates = slicesx.AppendIfMissing(pd.candidates, v)
				pd.closed = true
			}
		}

		// Because some correlation strategies are too greedy (like "heroDamageCorrelatesByCreepKindAndSide")
		// they would match too many candidates even if it was clear some creeps match 1-to-1 with some prior more
		// obvious combat logs. This is the case for JuggMissedMeleeAOE_22_09
		if removeCreepsFromMatchPool {
			if len(pd.candidates) == 1 {
				h.creepsMatchedThisTick = slicesx.AppendIfMissing(h.creepsMatchedThisTick, pd.candidates[0])
			}
		}
	}
}

func (h *Handler) correlateLastTickDamages(tick uint32, gameTime float32) {
	h.correlateDamagesBy(true, func(pd pendingCLogCreepEvent, creep *creepTrack) bool {
		return heroDamageCorrelatesExactly(pd, creep.prevHealth, creep.currentHealth)
	})

	// This may happen if other entity applied damage on the same tick before us
	// the after health will match (if we are the last attacker) but before health will not
	h.correlateDamagesBy(false, func(pd pendingCLogCreepEvent, creep *creepTrack) bool {
		return heroDamageCorrelatesByAfterHealth(pd, creep.currentHealth)
	})

	// Sometimes we deal damage, then another entity (Creep) deals a killing blow. Check PA 2_10 test
	// Correlate by creep kind
	h.correlateDamagesBy(false, func(pd pendingCLogCreepEvent, creep *creepTrack) bool {
		return heroDamageCorrelatesByCreepKindAndSide(pd, creep.creepKind, creep.side) &&
			creep.currentHealth < (pd.health+2) // accounting for creep regen
	})

	if h.lastCombatLogTick == 46148 {
		log.Printf("gameTime %f tick %d pendingHeroDamageLogs: %+v", gameTime, tick, h.pendingHeroDamageLogs)
	}

	// Create conflict groups
	h.closePendingHeroDamageBeforeTick(tick + 1)

	// Resolve died creeps that were damaged by us
	for _, id := range h.pendingDeadCreeps {
		track := h.creepTracks[id]
		h.handleCreepDeath(id, track, tick, gameTime)
		delete(h.creepTracks, id)
	}

	// Tick cleanup
	// Clean up temp tick arrays
	h.pendingHealthReducedCreepIds = make([]int32, 0, 64)
	h.pendingDeadCreeps = make([]int32, 0, 64)
	h.pendingHeroDamageLogs = make([]pendingCLogCreepEvent, 0, 64)
	h.pendingOtherKillLogs = make([]pendingCLogCreepEvent, 0, 64)
	h.pendingHeroKillLogs = make([]pendingCLogCreepEvent, 0, 64)
	h.creepsMatchedThisTick = make([]int32, 0, 32)
	// Rollover health into prevHealth
	for k := range h.creepTracks {
		creep := h.creepTracks[k]
		creep.prevHealth = creep.currentHealth
		creep.hasPrevHealth = true
	}

}

func (h *Handler) onCombatLogEntry(p *manta.Parser, m *dota.CMsgDOTACombatLogEntry) error {
	tick := p.Tick
	gameTime := h.timeAndPausesHandler.CurrentGameTime()

	if tick != h.lastCombatLogTick {
		h.correlateLastTickDamages(h.lastCombatLogTick, h.timeAndPausesHandler.LastTickGameTime())
		h.lastCombatLogTick = tick
	}

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

		inflictorName := m.GetInflictorName()
		if !isLasthitInflictorDamage(inflictorName, h.lasthitInflictorList, func(idx uint32) (string, bool) {
			return p.LookupStringByIndex("CombatLogNames", int32(idx))
		}) {
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
				damage:    int32(m.GetValue()),
				health:    m.GetHealth(),
				gameTime:  gameTime,
			})
			h.pendingHeroDamageLogs = append(h.pendingHeroDamageLogs, pendingCLogCreepEvent{
				creepName: realTargetName,
				tick:      tick,
				gameTime:  gameTime,
				health:    m.GetHealth(),
				damage:    int32(m.GetValue()),
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
	creepKind := creeps.KindFromEntity(e)
	h.onCreepHealthUpdate(e.GetIndex(), health, isMaxHealth, x, y, op, p.Tick, gameTime, entityName, className, creepKind)
	return nil
}

func (h *Handler) onCreepHealthUpdate(entityId int32, health int32, isMaxHealth bool, x float32, y float32, op manta.EntityOp, tick uint32, gameTime float32, entityName string, className string, creepKind string) {
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
			} else {
				track.side = sideCandidate1
			}
			track.spawnTimestamp = gameTime
			track.spawnWave = creeps.GetWaveNumber(gameTime)
		}
	}
	track.entityName = entityName
	track.className = className
	track.creepKind = creepKind
	track.lastUpdatedTick = tick
	track.currentHealth = health

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
		h.pendingHealthReducedCreepIds = slicesx.AppendIfMissing(h.pendingHealthReducedCreepIds, entityId)
	}

	if justDied || op.Flag(manta.EntityOpDeleted) || op.Flag(manta.EntityOpDeletedLeft) {
		h.pendingDeadCreeps = slicesx.AppendIfMissing(h.pendingDeadCreeps, entityId)
	}
}

// MissedEvents returns detected missed last-hit events (for tooling / quality reports).
func (h *Handler) MissedEvents() []Event {
	return h.missedEvents
}

func replayTicksWithinWindow(fromTick, toTick uint32) bool {
	if toTick < fromTick {
		return false
	}
	return toTick-fromTick <= missedLastHitWindowTicks
}

func (h *Handler) hasPendingHeroKill(creepName string, currentTick uint32) bool {
	for _, cLog := range h.pendingHeroKillLogs {
		if cLog.entityMatched || cLog.creepName != creepName {
			continue
		}
		if cLog.tick == currentTick {
			return true
		}
	}
	return false
}

func (h *Handler) hasPendingOtherKill(creepName string, heroDamagedTick uint32, currentTick uint32) bool {
	if heroDamagedTick == 0 {
		return false
	}
	for i := range h.pendingOtherKillLogs {
		pendingOtherKillLog := &h.pendingOtherKillLogs[i]
		if pendingOtherKillLog.entityMatched || pendingOtherKillLog.creepName != creepName {
			continue
		}
		if !replayTicksWithinWindow(heroDamagedTick, currentTick) {
			continue
		}
		return true
	}
	return false
}

func (h *Handler) matchPendingHeroKill(creepName string, currentTick uint32) {
	for i := range h.pendingHeroKillLogs {
		cLog := &h.pendingHeroKillLogs[i]
		if cLog.entityMatched || cLog.creepName != creepName {
			continue
		}
		if cLog.tick == currentTick {
			cLog.entityMatched = true
			return
		}
	}
}

func heroDamageCorrelates(pd pendingCLogCreepEvent, prevHealth, health int32, entityTick uint32) bool {
	if prevHealth != pd.health+pd.damage {
		return false
	}
	if health == pd.health {
		return true
	}
	// Same replay tick: entity may jump to 0/death without reporting post-damage HP.
	return health <= 0 && entityTick == pd.tick
}

func heroDamageCorrelatesExactly(pd pendingCLogCreepEvent, prevHealth, health int32) bool {
	return prevHealth == pd.health+pd.damage && health == pd.health
}

func heroDamageCorrelatesByAfterHealth(pd pendingCLogCreepEvent, health int32) bool {
	return health == pd.health
}

func heroDamageCorrelatesByCreepKindAndSide(pd pendingCLogCreepEvent, entityCreepKind string, entitySide string) bool {
	pdCreepKind := creeps.KindFromTargetName(pd.creepName)
	pdSide := creeps.GetCreepSide(pd.creepName)
	return pdCreepKind == entityCreepKind && pdSide == entitySide
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
	track.combatLogCreepname = creepName
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
		if track.currentHealth > 0 {
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
func (h *Handler) handleCreepDeath(idx int32, track *creepTrack, currentTick uint32, gameTime float32) {
	// Both enemyKill and heroKill can be true at the same if there are 2+ combat logs for creep death on the same tick
	// in this case we assume there is no miss
	enemyKill := h.hasPendingOtherKill(track.combatLogCreepname, track.heroDamagedTick, currentTick)
	heroKill := h.hasPendingHeroKill(track.combatLogCreepname, currentTick)

	if track.conflictGroupID == 0 {
		h.handleCreepDeathWithoutConflict(heroKill, enemyKill, track, currentTick, gameTime)
	} else {
		h.handleCreepDeathWithConflict(idx, heroKill, enemyKill, track, currentTick, gameTime)
	}
}

func (h *Handler) handleCreepDeathWithoutConflict(heroKill bool, enemyKill bool, track *creepTrack, currentTick uint32, gameTime float32) {
	if heroKill {
		h.matchPendingHeroKill(track.combatLogCreepname, currentTick)
		track.heroDamagedTick = 0
		return
	}

	if enemyKill && track.heroDamagedTick > 0 {
		h.missedEvents = append(h.missedEvents, Event{
			Timestamp: gameTime,
			Type:      "missed_last_hit",
			CreepName: track.combatLogCreepname,
		})
		track.heroDamagedTick = 0
	}
}

func (h *Handler) handleCreepDeathWithConflict(entityIdx int32, heroKill bool, enemyKill bool, track *creepTrack, currentTick uint32, gameTime float32) {
	groupID := track.conflictGroupID
	group := h.conflictGroups[groupID]

	if heroKill {
		h.matchPendingHeroKill(track.combatLogCreepname, currentTick)
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
				CreepName: track.combatLogCreepname,
			})
		}
	}
}
