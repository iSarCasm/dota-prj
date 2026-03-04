package strategic_states

import (
	"errors"

	"github.com/dotabuff/manta"

	"dota2/internal/common"
	"dota2/internal/timeandpauses"
)

const (
	StateDead    = "dead"
	StateRoaming = "roaming"
)

// Snapshot is a single strategic state at a point in time.
type Snapshot struct {
	TickTime float32 `json:"tick_time"`
	GameTime float32 `json:"game_time"`
	State    string  `json:"state"`
}

// Handler implements common.ReplayHandler for strategic state extraction (dead / roaming).
type Handler struct {
	heroClass            string
	snapshots            []Snapshot
	lastState            string
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

// RegisterCallbacks registers strategic_states callbacks.
func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
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

		if cn != h.heroClass || !common.IsRealHero(e) {
			return nil
		}

		health, ok := e.GetInt32("m_iHealth")
		if !ok {
			return nil
		}

		state := StateRoaming
		if health <= 0 {
			state = StateDead
		}

		if state != h.lastState {
			h.lastState = state
			h.snapshots = append(h.snapshots, Snapshot{
				TickTime: tickTime,
				GameTime: gameTime,
				State:    state,
			})
		}
		return nil
	})
}

// Output returns the handler's contribution to the final JSON (key "strategic_states").
func (h *Handler) Output(ctx *common.ParseContext) map[string]interface{} {
	return map[string]interface{}{"strategic_states": h.snapshots}
}
