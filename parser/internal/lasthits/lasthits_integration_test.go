package lasthits

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

// parseReplay runs p.Start(), discarding standard log output unless DOTA_TEST_VERBOSE=1.
// Manta and timeandpauses use log.Printf during replay parsing, which floods test output.
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

// replayPath resolves a match replay from DOTA_REPLAYS_DIR or repo-root dota-replays/.
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

func parseWarlock8915936762(t *testing.T) *Handler {
	t.Helper()

	f, err := os.Open(replayPath(t, "8915936762"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	p, err := manta.NewStreamParser(f)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &common.ParseContext{HeroName: "Warlock", TickInterval: 0.033333335}
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

func TestReplay8915936762_WarlockMissedFlagbearerAround_2_45(t *testing.T) {
	h := parseWarlock8915936762(t)

	var foundFlagbearer bool
	for _, e := range h.missedEvents {
		if e.CreepName == "npc_dota_creep_badguys_flagbearer" && e.Timestamp >= toTimestamp(2, 44) && e.Timestamp <= toTimestamp(2, 46) {
			foundFlagbearer = true
		}
	}
	if !foundFlagbearer {
		t.Fatalf("expected flagbearer miss around 2:45, got %d total misses", len(h.missedEvents))
	}
}

// Spell/ambient damage on allied creeps around 2:51–2:59 must not register as missed CS.
func TestReplay8915936762_WarlockNoFalseMisses_2_51_to_2_59(t *testing.T) {
	h := parseWarlock8915936762(t)

	for _, e := range h.missedEvents {
		if e.Timestamp >= toTimestamp(2, 51) && e.Timestamp <= toTimestamp(2, 59) {
			t.Fatalf("unexpected missed last hit at %.2fs on %s", e.Timestamp, e.CreepName)
		}
	}
}

func toTimestamp(min int, sec int) float32 {
	return float32(min*60 + sec)
}
