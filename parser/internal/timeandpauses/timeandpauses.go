package timeandpauses

import (
	"log"
	"math"

	"github.com/dotabuff/manta"

	"dota2/internal/common"
)

const defaultTickInterval = float32(0.033333335)
const preGameOffsetSeconds = float32(90)
const gameStartEpsilon = float32(0.05)

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
	preGameStart   float32
	gameStartTime  float32
	gameEndTime    float32
	isGameEnded    bool
	tickInterval   float32
	seenState      bool
}

// NewHandler creates a TimeAndPauses handler.
func NewHandler() *Handler {
	return &Handler{
		intervals:    make([]Interval, 0, 64),
		tickInterval: defaultTickInterval,
	}
}

func (h *Handler) Init(ctx *common.ParseContext) error {
	if ctx.TickInterval > 0 {
		h.tickInterval = ctx.TickInterval
	}
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

// PreGameStartTime returns pre-game start from CDOTAGamerulesProxy.
func (h *Handler) PreGameStartTime() float32 {
	return h.preGameStart
}

// GameStartTime returns game start from CDOTAGamerulesProxy.
func (h *Handler) GameStartTime() float32 {
	return h.gameStartTime
}

// GameEndTime returns game end time from CDOTAGamerulesProxy.
func (h *Handler) GameEndTime() float32 {
	return h.gameEndTime
}

// IsGameEnded reports whether game end was observed in gamerules.
func (h *Handler) IsGameEnded() bool {
	return h.isGameEnded
}

// PauseTimeSoFar returns closed pause duration plus current open pause (if any).
func (h *Handler) PauseTimeSoFar() float32 {
	return h.pauseDurationAtTime(h.lastEntityTime)
}

// CurrentGameTime = currentTickTime - preGameStartTime - 90 - pauseTimeSoFar.
func (h *Handler) CurrentGameTime() float32 {
	return h.CurrentTickTime() - h.PreGameStartTime() - preGameOffsetSeconds - h.PauseTimeSoFar()
}

// PauseDurationAtTick returns total pause duration accumulated up to a given tick.
func (h *Handler) PauseDurationAtTick(tick uint32) float32 {
	tickTime := h.tickInterval * float32(tick)
	return h.pauseDurationAtTime(tickTime)
}

// GameTimeAtTick returns game time at a given tick:
// currentTickTime - preGameStartTime - 90 - pauseDurationAtTick.
func (h *Handler) GameTimeAtTick(tick uint32) float32 {
	tickTime := h.tickInterval * float32(tick)
	return tickTime - h.preGameStart - preGameOffsetSeconds - h.pauseDurationAtTime(tickTime)
}

func (h *Handler) pauseDurationAtTime(t float32) float32 {
	total := float32(0)
	for _, in := range h.intervals {
		if in.End <= t {
			total += in.End - in.Start
			continue
		}
		if in.Start < t {
			total += t - in.Start
		}
		break
	}
	if h.isPaused && t > h.currentPauseAt {
		total += t - h.currentPauseAt
	}
	return total
}

func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil || e.GetClassName() != "CDOTAGamerulesProxy" {
			return nil
		}

		if ctx.TickInterval > 0 && ctx.TickInterval != h.tickInterval {
			log.Printf("timeandpauses: tickInterval set to %.3f", ctx.TickInterval)
			h.tickInterval = ctx.TickInterval
		}

		entityTick := p.Tick
		entityTime := h.tickInterval * float32(entityTick)
		h.lastEntityTime = entityTime

		if h.preGameStart == 0 {
			if v, ok := e.GetFloat32("m_flPreGameStartTime"); ok {
				if v != 0 {
					h.preGameStart = v
					log.Printf("timeandpauses: preGameStartTime set to %.3f", h.preGameStart)
				}
			} else if v, ok := e.GetFloat32("m_pGameRules.m_flPreGameStartTime"); ok {
				if v != 0 {
					h.preGameStart = v
					log.Printf("timeandpauses: preGameStartTime set to %.3f", h.preGameStart)
				}
			}
		}
		if h.gameStartTime == 0 {
			if v, ok := e.GetFloat32("m_flGameStartTime"); ok {
				if v != 0 {
					h.gameStartTime = v
					log.Printf("timeandpauses: gameStartTime set to %.3f", h.gameStartTime)
					if h.preGameStart != 0 {
						expected := h.preGameStart + preGameOffsetSeconds
						if float32(math.Abs(float64(h.gameStartTime-expected))) > gameStartEpsilon {
							log.Printf("WARNING timeandpauses: gameStartTime mismatch: got=%.3f expected=%.3f (preGameStartTime + %.0f)",
								h.gameStartTime, expected, preGameOffsetSeconds)
						}
					}
				}
			} else if v, ok := e.GetFloat32("m_pGameRules.m_flGameStartTime"); ok {
				if v != 0 {
					h.gameStartTime = v
					log.Printf("timeandpauses: gameStartTime set to %.3f", h.gameStartTime)
					if h.preGameStart != 0 {
						expected := h.preGameStart + preGameOffsetSeconds
						if float32(math.Abs(float64(h.gameStartTime-expected))) > gameStartEpsilon {
							log.Printf("WARNING timeandpauses: gameStartTime mismatch: got=%.3f expected=%.3f (preGameStartTime + %.0f)",
								h.gameStartTime, expected, preGameOffsetSeconds)
						}
					}
				}
			}
		}
		if !h.isGameEnded {
			if v, ok := e.GetFloat32("m_flGameEndTime"); ok {
				if v > 0 {
					h.gameEndTime = v
					h.isGameEnded = true
					log.Printf("timeandpauses: gameEndTime set to %.3f", h.gameEndTime)
				}
			} else if v, ok := e.GetFloat32("m_pGameRules.m_flGameEndTime"); ok {
				if v > 0 {
					h.gameEndTime = v
					h.isGameEnded = true
					log.Printf("timeandpauses: gameEndTime set to %.3f", h.gameEndTime)
				}
			}
		}

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
			"currentTickTime":  h.CurrentTickTime(),
			"preGameStartTime": h.PreGameStartTime(),
			"gameStartTime":    h.GameStartTime(),
			"gameEndTime":      h.GameEndTime(),
			"isGameEnded":      h.IsGameEnded(),
			"pauseTimeSoFar":   pauseTimeSoFar,
			"currentGameTime":  h.CurrentGameTime(),
			"pauses": map[string]interface{}{
				"intervals":      intervals,
				"totalPauseTime": totalPauseTime,
			},
		},
	}
}
