package matchsummary

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/dotabuff/manta"

	"dota2/internal/common"
	"dota2/internal/timeandpauses"
)

func parseReplay(t *testing.T, p *manta.Parser) {
	t.Helper()

	restore := func() {}
	if os.Getenv("DOTA_TEST_VERBOSE") == "" {
		old := log.Writer()
		log.SetOutput(io.Discard)
		restore = func() { log.SetOutput(old) }
	}
	defer restore()

	if err := p.Start(); err != nil && err.Error() != "EOF" {
		t.Fatal(err)
	}
}

func replayPath(t *testing.T, matchID string) string {
	t.Helper()

	name := matchID + ".dem"
	if dir := os.Getenv("DOTA_REPLAYS_DIR"); dir != "" {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	path := filepath.Join("..", "..", "..", "dota-replays", name)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("replay not found at %s (fetch: ruby ../../../dota-replays/fetch.rb)", path)
	}
	return path
}

func parseReplayHeroMatchSummary(t *testing.T, matchID, heroName string) *Handler {
	t.Helper()

	f, err := os.Open(replayPath(t, matchID))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	p, err := manta.NewStreamParser(f)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &common.ParseContext{HeroName: heroName, TickInterval: 0.033333335}
	tp := timeandpauses.NewHandler()
	if err := tp.Init(ctx); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(tp)
	if err := h.Init(ctx); err != nil {
		t.Fatal(err)
	}

	tp.RegisterCallbacks(p, ctx)
	h.RegisterCallbacks(p, ctx)
	parseReplay(t, p)
	return h
}

func TestReplay8915936762_WarlockKDA(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8915936762", "Warlock")
	got := h.summary.KDA
	want := KDA{Kills: 0, Deaths: 9, Assists: 14}
	if got != want {
		t.Fatalf("KDA = %+v, want %+v", got, want)
	}
}

func TestReplay8934466456_PhantomAssassinKDA(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8934466456", "Phantom Assassin")
	got := h.summary.KDA
	want := KDA{Kills: 4, Deaths: 6, Assists: 4}
	if got != want {
		t.Fatalf("KDA = %+v, want %+v", got, want)
	}
}
