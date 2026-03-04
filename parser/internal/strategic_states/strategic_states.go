package strategic_states

import (
	"errors"
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/common"
	"dota2/internal/timeandpauses"
)

const (
	StateDead    = "dead"
	StateRoaming = "roaming"
	StateFarming = "farming"

	SubstateJungle = "jungle"
	SubstateLane   = "lane"

	// farmingTimeoutSec: after this many seconds without creep damage, state goes back to roaming
	farmingTimeoutSec = 10.0
)

// Snapshot is a single strategic state at a point in time.
type Snapshot struct {
	TickTime float32 `json:"tick_time"`
	GameTime float32 `json:"game_time"`
	State    string  `json:"state"`
	Substate string  `json:"substate,omitempty"` // "jungle" or "lane" when state is "farming"
}

// Handler implements common.ReplayHandler for strategic state extraction (dead / roaming / farming).
type Handler struct {
	heroClass            string
	snapshots            []Snapshot
	lastState            string
	lastSubstate         string
	lastFarmingTime      float32
	timeAndPausesHandler *timeandpauses.Handler
}

// NewHandler creates a strategic_states handler.
func NewHandler(timeAndPausesHandler *timeandpauses.Handler) *Handler {
	return &Handler{
		snapshots:            make([]Snapshot, 0, 256),
		lastState:            "",
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
// Combat log target names: npc_dota_neutral_*, npc_dota_creep_goodguys_*, npc_dota_creep_badguys_*, npc_dota_creep_siege.
func creepSubstateFromTargetName(targetName string) string {
	targetName = strings.ToLower(strings.TrimSpace(targetName))
	if targetName == "" {
		return ""
	}
	if strings.HasPrefix(targetName, "npc_dota_neutral_") {
		return SubstateJungle
	}
	if strings.HasPrefix(targetName, "npc_dota_creep_goodguys_") || strings.HasPrefix(targetName, "npc_dota_creep_badguys_") {
		return SubstateLane
	}
	if strings.HasPrefix(targetName, "npc_dota_creep_siege") {
		return SubstateLane
	}
	return ""
}

// RegisterCallbacks registers strategic_states callbacks.
func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
	// Combat log: when our hero damages or kills a creep, enter farming with jungle/lane substate
	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		if h.timeAndPausesHandler.IsGameEnded() {
			return nil
		}
		ctype := m.GetType()
		isDamage := ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE
		isDeath := ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DEATH
		if !isDamage && !isDeath {
			return nil
		}

		attackerName := m.GetAttackerName()
		realAttackerName, ok := p.LookupStringByIndex("CombatLogNames", int32(attackerName))
		if !ok {
			return nil
		}
		heroClassName := common.GuessHeroClassFromNPC(realAttackerName)
		if heroClassName != h.heroClass {
			return nil
		}

		targetNameIdx := m.GetTargetName()
		realTargetName, ok := p.LookupStringByIndex("CombatLogNames", int32(targetNameIdx))
		if !ok {
			return nil
		}
		substate := creepSubstateFromTargetName(realTargetName)
		if substate == "" {
			return nil
		}

		gameTime := m.GetTimestamp() - h.timeAndPausesHandler.GameStartTime()
		if gameTime < 0 {
			gameTime = 0
		}
		h.lastFarmingTime = gameTime

		// If we're dead we don't override with farming
		if h.lastState == StateDead {
			return nil
		}

		if h.lastState != StateFarming || h.lastSubstate != substate {
			h.lastState = StateFarming
			h.lastSubstate = substate
			tickTime := h.timeAndPausesHandler.CurrentTickTime()
			h.snapshots = append(h.snapshots, Snapshot{
				TickTime: tickTime,
				GameTime: gameTime,
				State:    StateFarming,
				Substate: substate,
			})
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

		if cn != h.heroClass || !common.IsRealHero(e, h.timeAndPausesHandler) {
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

		// Alive: if we were farming and timeout elapsed, transition to roaming
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
