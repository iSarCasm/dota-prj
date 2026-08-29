package strategic_states

import (
	"errors"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/common"
	"dota2/internal/creeps"
	"dota2/internal/timeandpauses"
)

const (
	StateDead     = "dead"
	StateRoaming  = "roaming"
	StateFarming  = "farming"
	StateFighting = "fighting"

	SubstateJungle = "jungle"
	SubstateLane   = "lane"

	// farmingTimeoutSec: after this many seconds without creep damage, state goes back to roaming
	farmingTimeoutSec = 10.0
	// fightingTimeoutSec: after this many seconds without hero damage (or buff to/from hero in combat), state leaves fighting
	fightingTimeoutSec = 10.0
)

// Snapshot is a single strategic state at a point in time.
type Snapshot struct {
	TickTime float32 `json:"tick_time"`
	GameTime float32 `json:"game_time"`
	State    string  `json:"state"`
	Substate string  `json:"substate,omitempty"` // "jungle" or "lane" when state is "farming"
}

// Handler implements common.ReplayHandler for strategic state extraction (dead / roaming / farming / fighting).
type Handler struct {
	heroClass        string
	snapshots        []Snapshot
	lastState        string
	lastSubstate     string
	lastFarmingTime  float32
	lastFightingTime float32
	// lastHeroDamageTime: for each hero class, last game time they dealt or received hero damage (damage only, not deaths). Used to define "in combat" for buff propagation.
	lastHeroDamageTime   map[string]float32
	timeAndPausesHandler *timeandpauses.Handler
}

// NewHandler creates a strategic_states handler.
func NewHandler(timeAndPausesHandler *timeandpauses.Handler) *Handler {
	return &Handler{
		snapshots:            make([]Snapshot, 0, 256),
		lastState:            "",
		lastHeroDamageTime:   make(map[string]float32, 32),
		timeAndPausesHandler: timeAndPausesHandler,
	}
}

// Init validates config and allocates state.
func (h *Handler) Init(ctx *common.ParseContext) error {
	h.heroClass = common.HeroNameToClass(ctx.HeroName)
	if h.heroClass == "" {
		return common.ErrInvalidHeroName
	}
	if h.timeAndPausesHandler == nil {
		return errors.New("strategic_states handler requires timeandpauses dependency")
	}
	return nil
}

// creepSubstateFromTargetName returns "jungle", "lane", or "" if target is not a creep we track.
// Combat log target names: npc_dota_neutral_*, npc_dota_creep_goodguys_*, npc_dota_creep_badguys_*,
// npc_dota_creep_siege*, npc_dota_goodguys_siege, npc_dota_badguys_siege.
func creepSubstateFromTargetName(targetName string) string {
	return creeps.TypeFromTargetName(targetName)
}

// isHeroInCombat returns true if the hero had dealt or received hero damage within fightingTimeoutSec. Buffs do not set this; only damage does (buffs are not contagious).
func (h *Handler) isHeroInCombat(heroClass string, gameTime float32) bool {
	t, ok := h.lastHeroDamageTime[heroClass]
	if !ok {
		return false
	}
	return gameTime-t <= fightingTimeoutSec
}

// RegisterCallbacks registers strategic_states callbacks.
func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		if h.timeAndPausesHandler.IsGameEnded() {
			return nil
		}
		gameTime := h.timeAndPausesHandler.CurrentGameTime()
		ctype := m.GetType()

		switch ctype {
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE:
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
			attackerClass := common.GetHeroClassName(realAttackerName)
			targetClass := common.GetHeroClassName(realTargetName)

			// Hero-vs-hero damage (no illusions, don't count deaths — only DAMAGE)
			if m.GetIsAttackerHero() && m.GetIsTargetHero() && !m.GetIsAttackerIllusion() && !m.GetIsTargetIllusion() {
				if attackerClass != "" {
					h.lastHeroDamageTime[attackerClass] = gameTime
				}
				if targetClass != "" {
					h.lastHeroDamageTime[targetClass] = gameTime
				}
				weInvolved := attackerClass == h.heroClass || targetClass == h.heroClass
				if weInvolved && h.lastState != StateDead {
					h.lastFightingTime = gameTime
					if h.lastState != StateFighting {
						h.lastState = StateFighting
						h.lastSubstate = ""
						h.snapshots = append(h.snapshots, Snapshot{
							TickTime: h.timeAndPausesHandler.CurrentTickTime(),
							GameTime: gameTime,
							State:    StateFighting,
						})
					}
				}
				return nil
			}

			// Farming: our hero dealt damage to a creep (lane or jungle). Don't override fighting.
			if attackerClass != h.heroClass {
				return nil
			}
			substate := creepSubstateFromTargetName(realTargetName)
			if substate == "" {
				return nil
			}
			h.lastFarmingTime = gameTime
			if h.lastState == StateDead || h.lastState == StateFighting {
				return nil
			}
			if h.lastState != StateFarming || h.lastSubstate != substate {
				h.lastState = StateFarming
				h.lastSubstate = substate
				h.snapshots = append(h.snapshots, Snapshot{
					TickTime: h.timeAndPausesHandler.CurrentTickTime(),
					GameTime: gameTime,
					State:    StateFarming,
					Substate: substate,
				})
			}
			return nil

		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_MODIFIER_ADD:
			// Buff-based fighting: we mark our hero as fighting only if we buff someone in combat, or are buffed by someone in combat. Buffs are not contagious.
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
			sourceClass := common.GetHeroClassName(realAttackerName)
			targetClass := common.GetHeroClassName(realTargetName)
			if sourceClass == "" || targetClass == "" {
				return nil
			}
			weAreSource := sourceClass == h.heroClass
			weAreTarget := targetClass == h.heroClass
			if !weAreSource && !weAreTarget {
				return nil
			}
			if h.lastState == StateDead {
				return nil
			}
			otherClass := targetClass
			if weAreTarget {
				otherClass = sourceClass
			}
			if !h.isHeroInCombat(otherClass, gameTime) {
				return nil
			}
			h.lastFightingTime = gameTime
			if h.lastState != StateFighting {
				h.lastState = StateFighting
				h.lastSubstate = ""
				h.snapshots = append(h.snapshots, Snapshot{
					TickTime: h.timeAndPausesHandler.CurrentTickTime(),
					GameTime: gameTime,
					State:    StateFighting,
				})
			}
			return nil
		}
		return nil
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil {
			return nil
		}
		if h.timeAndPausesHandler.IsGameEnded() {
			return nil
		}
		gameTime := h.timeAndPausesHandler.CurrentGameTime()
		tickTime := h.timeAndPausesHandler.CurrentTickTime()
		cn := e.GetClassName()

		if cn != h.heroClass || !common.IsRealHero(e, h.timeAndPausesHandler.PreGameStartTime()) {
			return nil
		}

		health, ok := e.GetInt32("m_iHealth")
		if !ok {
			return nil
		}

		if health <= 0 {
			if h.lastState != StateDead {
				h.lastState = StateDead
				h.lastSubstate = ""
				h.snapshots = append(h.snapshots, Snapshot{
					TickTime: tickTime,
					GameTime: gameTime,
					State:    StateDead,
				})
			}
			return nil
		}

		// Alive: state priority dead > fighting > farming > roaming
		if h.lastState == StateFighting {
			if gameTime-h.lastFightingTime >= fightingTimeoutSec {
				h.lastState = StateRoaming
				h.lastSubstate = ""
				h.snapshots = append(h.snapshots, Snapshot{
					TickTime: tickTime,
					GameTime: gameTime,
					State:    StateRoaming,
				})
			}
			return nil
		}
		if h.lastState == StateFarming {
			if gameTime-h.lastFarmingTime >= farmingTimeoutSec {
				h.lastState = StateRoaming
				h.lastSubstate = ""
				h.snapshots = append(h.snapshots, Snapshot{
					TickTime: tickTime,
					GameTime: gameTime,
					State:    StateRoaming,
				})
			}
			return nil
		}

		// First time we see hero alive: emit initial roaming
		if h.lastState == "" {
			h.lastState = StateRoaming
			h.snapshots = append(h.snapshots, Snapshot{
				TickTime: tickTime,
				GameTime: gameTime,
				State:    StateRoaming,
			})
		}
		return nil
	})
}

// Output returns the handler's contribution to the final JSON (key "strategic_states").
func (h *Handler) Output(ctx *common.ParseContext) map[string]interface{} {
	return map[string]interface{}{"strategic_states": h.snapshots}
}
