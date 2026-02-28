package common

import (
	"errors"

	"github.com/dotabuff/manta"
)

var ErrInvalidHeroName = errors.New("invalid hero name")

// Insight is a single insight for the final output (e.g. PT mistake or good play).
type Insight struct {
	Type       string                 `json:"type"`
	Timestamps []float32              `json:"timestamps"`
	Verdict    string                 `json:"verdict"` // "good" or "bad"
	Level      string                 `json:"level"`   // "minor", "medium", "major"
	Details    map[string]interface{} `json:"details"`
}

// ParseContext holds shared state for replay parsing.
// Fields like TickInterval and PlayerIDToHero are populated during parse.
type ParseContext struct {
	TickInterval   float32
	PlayerIDToHero map[uint32]HeroRef

	// Config known before parse (for handlers that need it)
	MatchID    string
	OutputDir  string
	HeroName   string
	ReplayPath string
}

// ReplayHandler is the interface each parsing feature (mana, PT, etc.) implements.
type ReplayHandler interface {
	// Init is called before parsing starts. Use it to validate config, allocate state, create output files.
	Init(ctx *ParseContext) error

	// RegisterCallbacks registers this handler's callbacks on the parser.
	RegisterCallbacks(p *manta.Parser, ctx *ParseContext)

	// Output returns this handler's contribution to the final JSON. Keys should be unique (e.g. "mana", "power_treads").
	// Return nil or empty map if the handler has nothing to output.
	Output(ctx *ParseContext) map[string]interface{}
}
