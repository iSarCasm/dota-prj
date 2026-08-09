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

type pathcornerLaneTableRow struct {
	Pathcorner string  `json:"pathcorner"`
	Team       string  `json:"team"`
	NameLane   string  `json:"name_lane"` // lane prefix in EntityNames
	StartX     float32 `json:"start_x"`
	StartY     float32 `json:"start_y"`
	RealLane   string  `json:"real_lane"`
	Spawns     int     `json:"spawns"`
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

	rows := buildPathcornerLaneTable(all)
	switch format {
	case "json":
		writePathcornerLaneTableJSON(out, rows, replayPaths)
	case "table":
		writePathcornerLaneTable(out, rows, replayPaths)
	default:
		writePathcornerLaneSpawnSummary(out, rows, replayPaths, all)
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

type pathcornerAgg struct {
	pathcorner string
	team       string
	lanePrefix string
	n          int
	meanX      float32
	meanY      float32
}

func aggregateSpawns(spawns []spawnPoint) map[string]*pathcornerAgg {
	agg := make(map[string]*pathcornerAgg)
	for _, s := range spawns {
		a := agg[s.pathcorner]
		if a == nil {
			a = &pathcornerAgg{
				pathcorner: s.pathcorner,
				team:       s.team,
				lanePrefix: s.lanePrefix,
			}
			agg[s.pathcorner] = a
		}
		a.n++
		a.meanX += s.x
		a.meanY += s.y
	}
	for _, a := range agg {
		if a.n > 0 {
			a.meanX /= float32(a.n)
			a.meanY /= float32(a.n)
		}
	}
	return agg
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

func inferMidRealLane(a *pathcornerAgg, spawns []spawnPoint) string {
	tBot, tTop := teamReferenceCentroids(spawns, a.team)
	tbx, tby := tBot.avg()
	ttx, tty := tTop.avg()
	hasTeamBot := tBot.n > 0
	hasTeamTop := tTop.n > 0

	if a.meanX > 0 && a.meanY > 0 {
		return "mid"
	}

	switch a.team {
	case "goodguys":
		if hasTeamBot {
			if dist(a.meanX, a.meanY, tbx, tby) < dist(a.meanX, a.meanY, ttx, tty)+500 || !hasTeamTop {
				return "bot"
			}
			return "top"
		}
		if hasTeamTop {
			return "top"
		}
	case "badguys":
		if hasTeamTop {
			if dist(a.meanX, a.meanY, ttx, tty) < dist(a.meanX, a.meanY, tbx, tby)+500 || !hasTeamBot {
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

func buildPathcornerLaneTable(spawns []spawnPoint) []pathcornerLaneTableRow {
	agg := aggregateSpawns(spawns)
	keys := make([]string, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var rows []pathcornerLaneTableRow
	for _, k := range keys {
		a := agg[k]
		realLane := a.lanePrefix
		if a.lanePrefix == "mid" {
			realLane = inferMidRealLane(a, spawns)
		}
		rows = append(rows, pathcornerLaneTableRow{
			Pathcorner: a.pathcorner,
			Team:       a.team,
			NameLane:   a.lanePrefix,
			StartX:     a.meanX,
			StartY:     a.meanY,
			RealLane:   realLane,
			Spawns:     a.n,
		})
	}
	return rows
}

func writePathcornerLaneTable(out io.Writer, rows []pathcornerLaneTableRow, replayPaths []string) {
	fmt.Fprintf(out, "# pathcorner (EntityNames) → start position → real lane\n")
	fmt.Fprintf(out, "# replays: %s\n", strings.Join(replayPaths, ", "))
	fmt.Fprintf(out, "# start_x/start_y: mean world position at first full-HP Created spawn\n")
	fmt.Fprintf(out, "# name_lane: prefix in pathcorner string; real_lane: geographic lane (mid bucket reclassified)\n")
	fmt.Fprintf(out, "pathcorner\tteam\tname_lane\tstart_x\tstart_y\treal_lane\tspawns\n")
	for _, r := range rows {
		fmt.Fprintf(out, "%s\t%s\t%s\t%.0f\t%.0f\t%s\t%d\n",
			r.Pathcorner, r.Team, r.NameLane, r.StartX, r.StartY, r.RealLane, r.Spawns)
	}
}

func writePathcornerLaneTableJSON(out io.Writer, rows []pathcornerLaneTableRow, replayPaths []string) {
	lookup := make(map[string]pathcornerLaneTableRow, len(rows))
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

func writePathcornerLaneSpawnSummary(out io.Writer, rows []pathcornerLaneTableRow, replayPaths []string, spawns []spawnPoint) {
	fmt.Fprintf(out, "# pathcorner spawn lane classification\n")
	fmt.Fprintf(out, "# replays: %s\n", strings.Join(replayPaths, ", "))
	fmt.Fprintf(out, "#\n")
	fmt.Fprintf(out, "# LANE TABLE (pathcorner → start position → real lane)\n")
	fmt.Fprintf(out, "pathcorner | team | name_lane | start_x | start_y | real_lane | spawns\n")
	for _, r := range rows {
		fmt.Fprintf(out, "%s | %s | %s | %.0f | %.0f | %s | %d\n",
			r.Pathcorner, r.Team, r.NameLane, r.StartX, r.StartY, r.RealLane, r.Spawns)
	}
	fmt.Fprintf(out, "# entries: %d\n", len(rows))
}
