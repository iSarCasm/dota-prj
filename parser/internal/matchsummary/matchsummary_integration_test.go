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

func TestReplay8934466456_PhantomAssassinHeroesKilled(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8934466456", "Phantom Assassin")
	h.summary.HeroesKilled = h.heroesKilledTable()
	want := []HeroKillRow{
		{Hero: "npc_dota_hero_lich", Kills: 2},
		{Hero: "npc_dota_hero_ancient_apparition", Kills: 1},
		{Hero: "npc_dota_hero_dark_seer", Kills: 1},
	}
	if !heroKillRowsEqual(h.summary.HeroesKilled, want) {
		t.Fatalf("heroes_killed = %+v, want %+v", h.summary.HeroesKilled, want)
	}
}

func TestReplay8915936762_WarlockHeroesKilled(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8915936762", "Warlock")
	h.summary.HeroesKilled = h.heroesKilledTable()
	if len(h.summary.HeroesKilled) != 0 {
		t.Fatalf("heroes_killed = %+v, want empty", h.summary.HeroesKilled)
	}
}

func heroKillRowsEqual(a, b []HeroKillRow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func inflictorMatrixEqual(a, b *InflictorMatrix) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a.Heroes) != len(b.Heroes) {
		return false
	}
	for i := range a.Heroes {
		if a.Heroes[i] != b.Heroes[i] {
			return false
		}
	}
	if len(a.Rows) != len(b.Rows) {
		return false
	}
	for i := range a.Rows {
		if a.Rows[i].Inflictor != b.Rows[i].Inflictor {
			return false
		}
		if len(a.Rows[i].Values) != len(b.Rows[i].Values) {
			return false
		}
		for j := range a.Rows[i].Values {
			if a.Rows[i].Values[j] != b.Rows[i].Values[j] {
				return false
			}
		}
	}
	return true
}

func TestReplay8915936762_WarlockHeroDamage(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8915936762", "Warlock")
	got := h.summary.HeroDamage.Total
	want := uint32(13488)
	if got != want {
		t.Fatalf("hero_damage.total = %d, want %d (by_type=%v)", got, want, h.summary.HeroDamage.ByType)
	}
}

func TestReplay8934466456_PhantomAssassinHeroDamage(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8934466456", "Phantom Assassin")
	got := h.summary.HeroDamage.Total
	want := uint32(10901)
	if got != want {
		t.Fatalf("hero_damage.total = %d, want %d (by_type=%v)", got, want, h.summary.HeroDamage.ByType)
	}
	h.summary.HeroDamage.Matrix = buildInflictorMatrix(h.heroDamageMatrix)
	wantMatrix := &InflictorMatrix{
		Heroes: []string{
			"npc_dota_hero_dark_seer",
			"npc_dota_hero_lich",
			"npc_dota_hero_ancient_apparition",
			"npc_dota_hero_mars",
			"npc_dota_hero_nevermore",
		},
		Rows: []InflictorMatrixRow{
			{Inflictor: "auto_attack", Values: []uint32{4347, 1269, 461, 0, 365}},
			{Inflictor: "phantom_assassin_stifling_dagger", Values: []uint32{1255, 1708, 387, 357, 0}},
			{Inflictor: "item_bfury", Values: []uint32{34, 346, 356, 16, 0}},
		},
	}
	if !inflictorMatrixEqual(h.summary.HeroDamage.Matrix, wantMatrix) {
		t.Fatalf("hero_damage.matrix = %+v, want %+v", h.summary.HeroDamage.Matrix, wantMatrix)
	}
}

func TestReplay8941961575_TerrorbladeTowerDamage(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8941961575", "Terrorblade")
	got := h.summary.TowerDamage.Total
	want := uint32(24794)
	if got != want {
		t.Fatalf("tower_damage.total = %d, want %d", got, want)
	}
}

func TestReplay8941817475_SlarkTowerDamage(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8941817475", "Slark")
	got := h.summary.TowerDamage.Total
	want := uint32(1046)
	if got != want {
		t.Fatalf("tower_damage.total = %d, want %d", got, want)
	}
}

func TestReplay8915936762_WarlockTowerDamage(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8915936762", "Warlock")
	if h.summary.TowerDamage.Total != 0 {
		t.Fatalf("tower_damage.total = %d, want 0", h.summary.TowerDamage.Total)
	}
}

func TestReplay8915936762_WarlockHealing(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8915936762", "Warlock")
	got := h.summary.Healing.Total
	want := uint32(4365)
	if got != want {
		t.Fatalf("healing.total = %d, want %d", got, want)
	}
}

func TestReplay8934466456_DazzleHealing(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8934466456", "Dazzle")
	got := h.summary.Healing.Total
	want := uint32(5197)
	if got != want {
		t.Fatalf("healing.total = %d, want %d", got, want)
	}
	h.summary.Healing.Matrix = buildInflictorMatrix(h.healingMatrix)
	wantMatrix := &InflictorMatrix{
		Heroes: []string{
			"npc_dota_hero_axe",
			"npc_dota_hero_lion",
			"npc_dota_hero_phantom_assassin",
			"npc_dota_hero_obsidian_destroyer",
		},
		Rows: []InflictorMatrixRow{
			{Inflictor: "dazzle_shadow_wave", Values: []uint32{979, 712, 1208, 583}},
			{Inflictor: "item_holy_locket", Values: []uint32{537, 678, 0, 0}},
			{Inflictor: "item_mekansm", Values: []uint32{275, 0, 0, 0}},
			{Inflictor: "dazzle_shallow_grave", Values: []uint32{0, 0, 0, 225}},
		},
	}
	if !inflictorMatrixEqual(h.summary.Healing.Matrix, wantMatrix) {
		t.Fatalf("healing.matrix = %+v, want %+v", h.summary.Healing.Matrix, wantMatrix)
	}
}

func TestReplay8941817475_LycanHealing(t *testing.T) {
	h := parseReplayHeroMatchSummary(t, "8941817475", "Lycan")
	got := h.summary.Healing.Total
	want := uint32(3329)
	if got != want {
		t.Fatalf("healing.total = %d, want %d", got, want)
	}
}
