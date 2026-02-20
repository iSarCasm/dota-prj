package mana

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

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
	log.Printf("heroClass: %s", h.heroClass)
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

// WriteOutput writes the collected mana data to JSON.
func (h *Handler) WriteOutput(ctx *common.ParseContext) error {
	manaRows := make([][]interface{}, 0, len(h.snapshots))
	for _, s := range h.snapshots {
		manaRows = append(manaRows, []interface{}{s.Tick, s.Time, s.Mana, s.MaxMana, s.ManaPercent})
	}
	out := map[string]interface{}{
		"mana":          manaRows,
		"gameStartTime": ctx.GameStartTime,
	}

	jsonPath := filepath.Join(ctx.OutputDir, ctx.MatchID+"_output.json")
	f, err := os.Create(jsonPath)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}

	log.Printf("Parse complete: match_id=%s hero=%s -> %s (%d rows)", ctx.MatchID, ctx.HeroName, jsonPath, len(h.snapshots))
	return nil
}
