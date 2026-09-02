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

func heroDamageRowsEqual(a, b []HeroDamageRow) bool {
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

func heroHealingRowsEqual(a, b []HeroHealingRow) bool {
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
	h.summary.HeroDamage.ByHero = h.heroDamageByHeroTable()
	wantRows := []HeroDamageRow{
		{Hero: "npc_dota_hero_dark_seer", Damage: 5636},
		{Hero: "npc_dota_hero_lich", Damage: 3323},
		{Hero: "npc_dota_hero_ancient_apparition", Damage: 1204},
		{Hero: "npc_dota_hero_mars", Damage: 373},
		{Hero: "npc_dota_hero_nevermore", Damage: 365},
	}
	if !heroDamageRowsEqual(h.summary.HeroDamage.ByHero, wantRows) {
		t.Fatalf("hero_damage.by_hero = %+v, want %+v", h.summary.HeroDamage.ByHero, wantRows)
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
	h.summary.Healing.ByHero = h.healingByHeroTable()
	wantRows := []HeroHealingRow{
		{Hero: "npc_dota_hero_axe", Healing: 1791},
		{Hero: "npc_dota_hero_lion", Healing: 1390},
		{Hero: "npc_dota_hero_phantom_assassin", Healing: 1208},
		{Hero: "npc_dota_hero_obsidian_destroyer", Healing: 808},
	}
	if !heroHealingRowsEqual(h.summary.Healing.ByHero, wantRows) {
		t.Fatalf("healing.by_hero = %+v, want %+v", h.summary.Healing.ByHero, wantRows)
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
