package main

import (
	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

// gameClock mirrors parser/internal/timeandpauses game-time math so this lab
// builds standalone (cannot import dota2/internal/* from outside parser).
type gameClock struct {
	intervals        []interval
	totalPauseTime   float32
	isPaused         bool
	currentPauseAt   float32
	currentTickTime  float32
	preGameStartTick uint32
	tickInterval     float32
	seenState        bool
}

type interval struct {
	start float32
	end   float32
}

const (
	defaultTickInterval  = float32(0.033333335)
	preGameOffsetSeconds = float32(90)
)

func newGameClock() *gameClock {
	return &gameClock{
		intervals:    make([]interval, 0, 64),
		tickInterval: defaultTickInterval,
	}
}

func (c *gameClock) currentGameTime() float32 {
	return c.currentTickTime - c.tickInterval*float32(c.preGameStartTick) - preGameOffsetSeconds - c.pauseTimeSoFar()
}

func (c *gameClock) pauseTimeSoFar() float32 {
	total := c.totalPauseTime
	t := c.currentTickTime
	for _, in := range c.intervals {
		if in.end <= t {
			continue
		}
		if in.start < t {
			total += t - in.start
		}
		break
	}
	if c.isPaused && t > c.currentPauseAt {
		total += t - c.currentPauseAt
	}
	return total
}

func (c *gameClock) register(p *manta.Parser) {
	p.Callbacks.OnCSVCMsg_ServerInfo(func(m *dota.CSVCMsg_ServerInfo) error {
		if ti := m.GetTickInterval(); ti > 0 {
			c.tickInterval = ti
		}
		return nil
	})

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil || e.GetClassName() != "CDOTAGamerulesProxy" {
			return nil
		}
		entityTick := p.Tick
		entityTime := c.tickInterval * float32(entityTick)
		c.currentTickTime = entityTime

		if c.preGameStartTick == 0 {
			if v, ok := e.GetFloat32("m_flPreGameStartTime"); ok && v != 0 {
				c.preGameStartTick = entityTick
			} else if v, ok := e.GetFloat32("m_pGameRules.m_flPreGameStartTime"); ok && v != 0 {
				c.preGameStartTick = entityTick
			}
		}

		paused, ok := e.GetBool("m_bGamePaused")
		if !ok {
			paused, ok = e.GetBool("m_pGameRules.m_bGamePaused")
		}
		if !ok {
			return nil
		}

		if !c.seenState {
			c.seenState = true
			c.isPaused = paused
			if paused {
				c.currentPauseAt = entityTime
			}
			return nil
		}
		if paused == c.isPaused {
			return nil
		}
		if paused {
			c.isPaused = true
			c.currentPauseAt = entityTime
			return nil
		}
		c.isPaused = false
		if entityTime > c.currentPauseAt {
			c.intervals = append(c.intervals, interval{start: c.currentPauseAt, end: entityTime})
			c.totalPauseTime += entityTime - c.currentPauseAt
		}
		return nil
	})
}
