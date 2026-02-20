package common

import (
	"errors"

	"github.com/dotabuff/manta"
)

var ErrInvalidHeroName = errors.New("invalid hero name")

// ParseContext holds shared state for replay parsing.
// Fields like TickInterval and PlayerIDToHero are populated during parse.
type ParseContext struct {
	TickInterval   float32
	PlayerIDToHero map[uint32]HeroRef
	GameStartTime  float32

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
}
