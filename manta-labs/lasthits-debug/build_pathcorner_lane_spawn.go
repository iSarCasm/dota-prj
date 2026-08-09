package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/dotabuff/manta"
)

type spawnPoint struct {
	pathcorner string
	team       string // goodguys | badguys
	lanePrefix string // bot | mid | top
	x          float32
	y          float32
	tick       uint32
	maxHealth  int32
}

type pathcornerSpawnStats struct {
	Pathcorner string  `json:"pathcorner"`
	Team       string  `json:"team"`
	NameLane   string  `json:"name_lane"`
	Spawns     int     `json:"spawns"`
	MeanX      float32 `json:"mean_x"`
	MeanY      float32 `json:"mean_y"`
	StdX       float32 `json:"std_x"`
	StdY       float32 `json:"std_y"`
	Spread     float32 `json:"spread"` // max distance from mean across spawns
	RangeX     float32 `json:"range_x"`
	RangeY     float32 `json:"range_y"`
	MinX       float32 `json:"min_x"`
	MaxX       float32 `json:"max_x"`
	MinY       float32 `json:"min_y"`
	MaxY       float32 `json:"max_y"`
	RealLane   string  `json:"real_lane"`
}

func entityWorldXY(e interface {
	GetUint64(string) (uint64, bool)
	GetFloat32(string) (float32, bool)
}) (float32, float32, bool) {
	x, okX := e.GetUint64("CBodyComponent.m_cellX")
	y, okY := e.GetUint64("CBodyComponent.m_cellY")
	if !okX || !okY {
		return 0, 0, false
	}
	vx, _ := e.GetFloat32("CBodyComponent.m_vecVelocity.x")
	vy, _ := e.GetFloat32("CBodyComponent.m_vecVelocity.y")
	return float32(x)*128 + vx - 16384, float32(y)*128 + vy - 16384, true
}

func pathcornerLanePrefix(pathcorner string) string {
	p := strings.ToLower(pathcorner)
	switch {
	case strings.Contains(p, "lane_bot_pathcorner"):
		return "bot"
	case strings.Contains(p, "lane_top_pathcorner"):
		return "top"
	case strings.Contains(p, "lane_mid_pathcorner"):
		return "mid"
	default:
		return ""
	}
}

func collectPathcornerSpawns(replayPath string) ([]spawnPoint, error) {
	f, err := os.Open(replayPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	p, err := manta.NewStreamParser(f)
	if err != nil {
		return nil, err
	}

	var spawns []spawnPoint
	seenFirst := make(map[int32]bool)

	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e.GetClassName() != "CDOTA_BaseNPC_Creep_Lane" {
			return nil
		}
		if !op.Flag(manta.EntityOpCreated) && !op.Flag(manta.EntityOpCreatedEntered) {
			return nil
		}

		idx := e.GetIndex()
		if seenFirst[idx] {
			return nil
		}
		seenFirst[idx] = true

		nameIdx, _ := e.GetInt32("m_iUnitNameIndex")
		pathcorner, ok := p.LookupStringByIndex("EntityNames", nameIdx)
		if !ok || !strings.Contains(pathcorner, "pathcorner") {
			return nil
		}

		health, okH := e.GetInt32("m_iHealth")
		maxHealth := health
		if mh, ok := e.GetInt32("m_iMaxHealth"); ok && mh > 0 {
			maxHealth = mh
		}
		if !okH || health <= 0 || health != maxHealth {
			return nil
		}

		x, y, okPos := entityWorldXY(e)
		if !okPos {
			return nil
		}

		spawns = append(spawns, spawnPoint{
			pathcorner: pathcorner,
			team:       pathcornerTeam(pathcorner),
			lanePrefix: pathcornerLanePrefix(pathcorner),
			x:          x,
			y:          y,
			tick:       p.Tick,
			maxHealth:  maxHealth,
		})
		return nil
	})

	if err := p.Start(); err != nil && err != io.EOF {
		return nil, err
	}
	return spawns, nil
}

func runPathcornerLaneSpawn(replayPaths []string, out io.Writer, format string) error {
	var all []spawnPoint
	for _, rp := range replayPaths {
		spawns, err := collectPathcornerSpawns(rp)
		if err != nil {
			return fmt.Errorf("%s: %w", rp, err)
		}
		all = append(all, spawns...)
	}

	rows := buildPathcornerSpawnStats(all)
	switch format {
	case "json":
		writePathcornerSpawnStatsJSON(out, rows, replayPaths)
	case "tsv":
		writePathcornerSpawnStatsTSV(out, rows, replayPaths)
	case "markdown", "md":
		writePathcornerSpawnStatsMarkdown(out, rows, replayPaths)
	case "table":
		writePathcornerSpawnStatsAlignedTable(out, rows, replayPaths)
	default:
		writePathcornerSpawnStatsSummary(out, rows, replayPaths)
	}
	return nil
}

type xyCentroid struct {
	x, y float32
	n    int
}

func (c *xyCentroid) add(x, y float32) {
	c.x += x
	c.y += y
	c.n++
}

func (c xyCentroid) avg() (float32, float32) {
	if c.n == 0 {
		return 0, 0
	}
	return c.x / float32(c.n), c.y / float32(c.n)
}

func dist(x1, y1, x2, y2 float32) float32 {
	dx := x1 - x2
	dy := y1 - y2
	return float32(math.Hypot(float64(dx), float64(dy)))
}

type pathcornerBucket struct {
	pathcorner string
	team       string
	lanePrefix string
	points     []spawnPoint
}

func bucketSpawns(spawns []spawnPoint) map[string]*pathcornerBucket {
	buckets := make(map[string]*pathcornerBucket)
	for _, s := range spawns {
		b := buckets[s.pathcorner]
		if b == nil {
			b = &pathcornerBucket{
				pathcorner: s.pathcorner,
				team:       s.team,
				lanePrefix: s.lanePrefix,
			}
			buckets[s.pathcorner] = b
		}
		b.points = append(b.points, s)
	}
	return buckets
}

func spawnPositionStats(b *pathcornerBucket) pathcornerSpawnStats {
	n := len(b.points)
	st := pathcornerSpawnStats{
		Pathcorner: b.pathcorner,
		Team:       b.team,
		NameLane:   b.lanePrefix,
		Spawns:     n,
	}
	if n == 0 {
		return st
	}

	var sumX, sumY float64
	st.MinX, st.MaxX = b.points[0].x, b.points[0].x
	st.MinY, st.MaxY = b.points[0].y, b.points[0].y
	for _, p := range b.points {
		sumX += float64(p.x)
		sumY += float64(p.y)
		if p.x < st.MinX {
			st.MinX = p.x
		}
		if p.x > st.MaxX {
			st.MaxX = p.x
		}
		if p.y < st.MinY {
			st.MinY = p.y
		}
		if p.y > st.MaxY {
			st.MaxY = p.y
		}
	}

	st.MeanX = float32(sumX / float64(n))
	st.MeanY = float32(sumY / float64(n))
	st.RangeX = st.MaxX - st.MinX
	st.RangeY = st.MaxY - st.MinY

	if n > 1 {
		var varX, varY float64
		for _, p := range b.points {
			dx := float64(p.x - st.MeanX)
			dy := float64(p.y - st.MeanY)
			varX += dx * dx
			varY += dy * dy
			d := dist(p.x, p.y, st.MeanX, st.MeanY)
			if d > st.Spread {
				st.Spread = d
			}
		}
		st.StdX = float32(math.Sqrt(varX / float64(n)))
		st.StdY = float32(math.Sqrt(varY / float64(n)))
	}

	return st
}

func teamReferenceCentroids(spawns []spawnPoint, team string) (bot, top xyCentroid) {
	for _, s := range spawns {
		if s.team != team {
			continue
		}
		switch s.lanePrefix {
		case "bot":
			bot.add(s.x, s.y)
		case "top":
			top.add(s.x, s.y)
		}
	}
	return bot, top
}

func inferMidRealLane(st pathcornerSpawnStats, spawns []spawnPoint) string {
	tBot, tTop := teamReferenceCentroids(spawns, st.Team)
	tbx, tby := tBot.avg()
	ttx, tty := tTop.avg()
	hasTeamBot := tBot.n > 0
	hasTeamTop := tTop.n > 0

	if st.MeanX > 0 && st.MeanY > 0 {
		return "mid"
	}

	switch st.Team {
	case "goodguys":
		if hasTeamBot {
			if dist(st.MeanX, st.MeanY, tbx, tby) < dist(st.MeanX, st.MeanY, ttx, tty)+500 || !hasTeamTop {
				return "bot"
			}
			return "top"
		}
		if hasTeamTop {
			return "top"
		}
	case "badguys":
		if hasTeamTop {
			if dist(st.MeanX, st.MeanY, ttx, tty) < dist(st.MeanX, st.MeanY, tbx, tby)+500 || !hasTeamBot {
				return "top"
			}
			return "bot"
		}
		if hasTeamBot {
			return "bot"
		}
	}
	return "unknown"
}

func buildPathcornerSpawnStats(spawns []spawnPoint) []pathcornerSpawnStats {
	buckets := bucketSpawns(spawns)
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var rows []pathcornerSpawnStats
	for _, k := range keys {
		st := spawnPositionStats(buckets[k])
		if st.NameLane == "mid" {
			st.RealLane = inferMidRealLane(st, spawns)
		} else {
			st.RealLane = st.NameLane
		}
		rows = append(rows, st)
	}
	return rows
}

func writePathcornerSpawnStatsTSV(out io.Writer, rows []pathcornerSpawnStats, replayPaths []string) {
	fmt.Fprintf(out, "# pathcorner (EntityNames) → spawn position stats (TSV)\n")
	fmt.Fprintf(out, "# replays: %s\n", strings.Join(replayPaths, ", "))
	fmt.Fprintf(out, "entity_name\tteam\tname_lane\tspawns\tmean_x\tmean_y\tstd_x\tstd_y\tspread\trange_x\trange_y\tmin_x\tmax_x\tmin_y\tmax_y\treal_lane\n")
	for _, r := range rows {
		fmt.Fprintf(out, "%s\t%s\t%s\t%d\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%s\n",
			r.Pathcorner, r.Team, r.NameLane, r.Spawns,
			r.MeanX, r.MeanY, r.StdX, r.StdY, r.Spread,
			r.RangeX, r.RangeY, r.MinX, r.MaxX, r.MinY, r.MaxY, r.RealLane)
	}
}

func writePathcornerSpawnStatsAlignedTable(out io.Writer, rows []pathcornerSpawnStats, replayPaths []string) {
	fmt.Fprintf(out, "# pathcorner (EntityNames) → spawn position stats\n")
	fmt.Fprintf(out, "# replays: %s\n", strings.Join(replayPaths, ", "))
	fmt.Fprintf(out, "# mean_x/y: average world position at first full-HP Created spawn\n")
	fmt.Fprintf(out, "# std_x/y: population std dev; spread: max distance from mean; range_x/y: max-min\n")
	fmt.Fprintf(out, "#\n")

	const (
		cName  = 36
		cTeam  = 9
		cLane  = 5
		cNum   = 7
		cRLane = 5
	)

	header := fmt.Sprintf("%-*s %-*s %-*s %*s %*s %*s %*s %*s %*s %*s %*s %-*s\n",
		cName, "entity_name",
		cTeam, "team",
		cLane, "lane",
		cNum, "spawns",
		cNum, "mean_x",
		cNum, "mean_y",
		cNum, "std_x",
		cNum, "std_y",
		cNum, "spread",
		cNum, "rng_x",
		cNum, "rng_y",
		cRLane, "real",
	)
	sep := strings.Repeat("-", len(header)-1) + "\n"
	fmt.Fprint(out, header, sep)

	for _, r := range rows {
		name := r.Pathcorner
		if len(name) > cName {
			name = name[:cName-1] + "…"
		}
		fmt.Fprintf(out, "%-*s %-*s %-*s %*d %7.0f %7.0f %7.0f %7.0f %7.0f %7.0f %7.0f %-*s\n",
			cName, name,
			cTeam, r.Team,
			cLane, r.NameLane,
			cNum, r.Spawns,
			r.MeanX, r.MeanY, r.StdX, r.StdY, r.Spread, r.RangeX, r.RangeY,
			cRLane, r.RealLane,
		)
	}
	fmt.Fprintf(out, "# entries: %d\n", len(rows))
}

func writePathcornerSpawnStatsMarkdown(out io.Writer, rows []pathcornerSpawnStats, replayPaths []string) {
	fmt.Fprintf(out, "# Pathcorner spawn position stats\n\n")
	fmt.Fprintf(out, "Replays: %s\n\n", strings.Join(replayPaths, ", "))
	fmt.Fprintf(out, "| entity_name | team | lane | spawns | mean_x | mean_y | std_x | std_y | spread | range_x | range_y | real_lane |\n")
	fmt.Fprintf(out, "|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
	for _, r := range rows {
		fmt.Fprintf(out, "| %s | %s | %s | %d | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f | %s |\n",
			r.Pathcorner, r.Team, r.NameLane, r.Spawns,
			r.MeanX, r.MeanY, r.StdX, r.StdY, r.Spread, r.RangeX, r.RangeY, r.RealLane)
	}
}

func writePathcornerSpawnStatsTable(out io.Writer, rows []pathcornerSpawnStats, replayPaths []string) {
	writePathcornerSpawnStatsAlignedTable(out, rows, replayPaths)
}

func writePathcornerSpawnStatsJSON(out io.Writer, rows []pathcornerSpawnStats, replayPaths []string) {
	lookup := make(map[string]pathcornerSpawnStats, len(rows))
	for _, r := range rows {
		lookup[r.Pathcorner] = r
	}
	payload := map[string]interface{}{
		"replays": replayPaths,
		"table":   rows,
		"lookup":  lookup,
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func writePathcornerSpawnStatsSummary(out io.Writer, rows []pathcornerSpawnStats, replayPaths []string) {
	fmt.Fprintf(out, "# pathcorner spawn position stats\n")
	fmt.Fprintf(out, "# replays: %s\n", strings.Join(replayPaths, ", "))
	fmt.Fprintf(out, "#\n")
	fmt.Fprintf(out, "entity_name | team | spawns | mean_x | mean_y | std_x | std_y | spread | range_x | range_y | real_lane\n")
	for _, r := range rows {
		fmt.Fprintf(out, "%s | %s | %d | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f | %.0f | %s\n",
			r.Pathcorner, r.Team, r.Spawns,
			r.MeanX, r.MeanY, r.StdX, r.StdY, r.Spread, r.RangeX, r.RangeY, r.RealLane)
	}
	fmt.Fprintf(out, "# entries: %d\n", len(rows))
}
