package timeandpauses

import (
	"github.com/dotabuff/manta"

	"dota2/internal/common"
)

// Interval is a single replay pause range [start, end].
type Interval struct {
	Start float32 `json:"start"`
	End   float32 `json:"end"`
}

// Handler implements common.ReplayHandler for pause tracking and time derivations.
type Handler struct {
	intervals      []Interval
	totalPauseTime float32
	isPaused       bool
	currentPauseAt float32
	lastEntityTime float32
	seenState      bool
}

// NewHandler creates a TimeAndPauses handler.
func NewHandler() *Handler {
	return &Handler{
		intervals: make([]Interval, 0, 64),
	}
}

func (h *Handler) Init(ctx *common.ParseContext) error {
	return nil
}

// IsPaused reports whether replay is currently paused at the latest processed event.
func (h *Handler) IsPaused() bool {
	return h.isPaused
}

// Intervals returns all known pause intervals.
func (h *Handler) Intervals() []Interval {
	return h.intervals
}

// CurrentTickTime returns latest observed replay time from entity ticks.
func (h *Handler) CurrentTickTime() float32 {
	return h.lastEntityTime
}

// GameStartTime returns game start from shared parse context.
func (h *Handler) GameStartTime(ctx *common.ParseContext) float32 {
	return ctx.GameStartTime
}

// PauseTimeSoFar returns closed pause duration plus current open pause (if any).
func (h *Handler) PauseTimeSoFar() float32 {
	total := h.totalPauseTime
	if h.isPaused && h.lastEntityTime > h.currentPauseAt {
		total += h.lastEntityTime - h.currentPauseAt
	}
	return total
}

// CurrentGameTime = currentTickTime - gameStartTime - pauseTimeSoFar.
func (h *Handler) CurrentGameTime(ctx *common.ParseContext) float32 {
	return h.CurrentTickTime() - h.GameStartTime(ctx) - h.PauseTimeSoFar()
}

func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil || e.GetClassName() != "CDOTAGamerulesProxy" {
			return nil
		}

		entityTick := p.Tick
		entityTime := ctx.TickInterval * float32(entityTick)
		h.lastEntityTime = entityTime

		paused, ok := e.GetBool("m_bGamePaused")
		if !ok {
			paused, ok = e.GetBool("m_pGameRules.m_bGamePaused")
		}
		if !ok {
			return nil
		}

		if !h.seenState {
			h.seenState = true
			h.isPaused = paused
			if paused {
				h.currentPauseAt = entityTime
			}
			return nil
		}

		if paused == h.isPaused {
			return nil
		}

		if paused {
			h.isPaused = true
			h.currentPauseAt = entityTime
			return nil
		}

		h.isPaused = false
		interval := Interval{Start: h.currentPauseAt, End: entityTime}
		h.intervals = append(h.intervals, interval)
		if interval.End > interval.Start {
			h.totalPauseTime += interval.End - interval.Start
		}
		return nil
	})
}

func (h *Handler) Output(ctx *common.ParseContext) map[string]interface{} {
	intervals := make([]Interval, len(h.intervals))
	copy(intervals, h.intervals)

	totalPauseTime := h.totalPauseTime
	pauseTimeSoFar := h.PauseTimeSoFar()
	if h.isPaused && h.lastEntityTime > h.currentPauseAt {
		intervals = append(intervals, Interval{
			Start: h.currentPauseAt,
			End:   h.lastEntityTime,
		})
		totalPauseTime = pauseTimeSoFar
	}

	return map[string]interface{}{
		"timeAndPauses": map[string]interface{}{
			"currentTickTime": h.CurrentTickTime(),
			"gameStartTime":   h.GameStartTime(ctx),
			"pauseTimeSoFar":  pauseTimeSoFar,
			"currentGameTime": h.CurrentGameTime(ctx),
			"pauses": map[string]interface{}{
				"intervals":      intervals,
				"totalPauseTime": totalPauseTime,
			},
		},
	}
}
