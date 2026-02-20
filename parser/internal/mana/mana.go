package mana

import (
	"log"

	"github.com/dotabuff/manta"

	"dota2/internal/common"
)

type ManaSnapshot struct {
	Tick        uint32
	Time        float32
	Mana        float32
	MaxMana     float32
	ManaPercent float32
}

// Handler implements common.ReplayHandler for mana extraction.
type Handler struct {
	manaTickInterval int
	heroClass        string
	snapshots        []*ManaSnapshot
}

// NewHandler creates a mana handler with the given tick interval (record mana every N ticks).
func NewHandler(tickInterval int) *Handler {
	return &Handler{
		manaTickInterval: tickInterval,
		snapshots:        make([]*ManaSnapshot, 0, 1024),
	}
}

// Init validates config and allocates state.
func (h *Handler) Init(ctx *common.ParseContext) error {
	h.heroClass = common.HeroNameToClass(ctx.HeroName)
	if h.heroClass == "" {
		return common.ErrInvalidHeroName
	}
	return nil
}

// RegisterCallbacks registers mana-specific callbacks.
func (h *Handler) RegisterCallbacks(p *manta.Parser, ctx *common.ParseContext) {
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil {
			return nil
		}
		entityTick := p.Tick
		entityTime := ctx.TickInterval * float32(entityTick)
		cn := e.GetClassName()

		if cn != h.heroClass || !common.IsRealHero(e) {
			return nil
		}

		maxMana, ok := e.GetFloat32("m_flMaxMana")
		if !ok {
			return nil
		}
		if entityTick%uint32(h.manaTickInterval) != 0 {
			return nil
		}

		mana, ok := e.GetFloat32("m_flMana")
		if !ok {
			log.Printf("%s mana: missing m_flMana", h.heroClass)
			return nil
		}

		h.snapshots = append(h.snapshots, &ManaSnapshot{
			Tick:        entityTick,
			Time:        entityTime,
			Mana:        mana,
			MaxMana:     maxMana,
			ManaPercent: mana / maxMana * 100,
		})
		return nil
	})

}

// Output returns the handler's contribution to the final JSON (key "mana").
func (h *Handler) Output(ctx *common.ParseContext) map[string]interface{} {
	manaRows := make([][]interface{}, 0, len(h.snapshots))
	for _, s := range h.snapshots {
		manaRows = append(manaRows, []interface{}{s.Tick, s.Time, s.Mana, s.MaxMana, s.ManaPercent})
	}
	return map[string]interface{}{
		"mana": manaRows,
	}
}
