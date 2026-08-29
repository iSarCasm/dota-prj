package creeps

import (
	"math"
	"strings"

	"github.com/dotabuff/manta"
)

func GetEntityLocation(e *manta.Entity) (float32, float32) {
	x, _ := e.GetUint64("CBodyComponent.m_cellX")
	y, _ := e.GetUint64("CBodyComponent.m_cellY")
	vx, _ := e.GetFloat32("CBodyComponent.m_vecVelocity.x")
	vy, _ := e.GetFloat32("CBodyComponent.m_vecVelocity.y")
	locX := float32(x)*128 + vx - 16384
	locY := float32(y)*128 + vy - 16384
	return locX, locY
}

func IsMaxHealth(e *manta.Entity) bool {
	health, ok := e.GetInt32("m_iHealth")
	if !ok {
		return false
	}
	maxHealth, ok := e.GetInt32("m_iMaxHealth")
	if !ok {
		return false
	}
	return health >= maxHealth
}

func GetWaveNumber(gameTime float32) int {
	// 1st wave starts at 0:00
	// 2nd wave is 0:30
	return int(gameTime/30) + 1
}

// TypeFromTargetName returns "lane", "jungle", or "" for combat-log NPC names.
func TypeFromTargetName(targetName string) string {
	targetName = strings.ToLower(strings.TrimSpace(targetName))
	if targetName == "" {
		return ""
	}
	if strings.HasPrefix(targetName, "npc_dota_neutral_") {
		return "jungle"
	}
	if strings.HasPrefix(targetName, "npc_dota_creep_goodguys_") || strings.HasPrefix(targetName, "npc_dota_creep_badguys_") {
		return "lane"
	}
	// Siege units: legacy npc_dota_creep_siege* and current npc_dota_{good,bad}guys_siege.
	if strings.HasPrefix(targetName, "npc_dota_creep_siege") ||
		targetName == "npc_dota_goodguys_siege" || targetName == "npc_dota_badguys_siege" {
		return "lane"
	}
	return ""
}

// IsLaneCreep reports whether className is a creep entity class.
func IsLaneCreep(className string) bool {
	return className == "CDOTA_BaseNPC_Creep_Lane" || className == "CDOTA_BaseNPC_Creep_Siege"
}

func IsCreep(className string) bool {
	return strings.HasPrefix(className, "CDOTA_BaseNPC_Creep")
}

// Lane-creep m_iAttackRange (8934466456). Flagbearer shares melee range (100);
// distinguish via m_flMagicalResistanceValue == MagicalResistanceFlagbearer.
const (
	AttackRangeMelee  int32 = 100
	AttackRangeRanged int32 = 500
	AttackRangeSiege  int32 = 690
)

const (
	MagicalResistanceMelee       float32 = 0
	MagicalResistanceRanged      float32 = 0
	MagicalResistanceFlagbearer  float32 = 40
	MagicalResistanceSiege       float32 = 80
)

const (
	KindFlagbearer = "flagbearer"
	KindMelee      = "melee"
	KindRanged     = "ranged"
	KindSiege      = "siege"
)

// KindFromAttackRange returns "melee", "ranged", "siege", or "".
// Melee and flagbearer both use AttackRangeMelee — use KindFromEntity for that split.
func KindFromAttackRange(attackRange int32) string {
	switch attackRange {
	case AttackRangeMelee:
		return KindMelee
	case AttackRangeRanged:
		return KindRanged
	case AttackRangeSiege:
		return KindSiege
	default:
		return ""
	}
}

// GetAttackRange reads m_iAttackRange from a creep entity.
func GetAttackRange(e *manta.Entity) (int32, bool) {
	if e == nil {
		return 0, false
	}
	return e.GetInt32("m_iAttackRange")
}

// GetMagicalResistance reads m_flMagicalResistanceValue from a creep entity.
func GetMagicalResistance(e *manta.Entity) (float32, bool) {
	if e == nil {
		return 0, false
	}
	return e.GetFloat32("m_flMagicalResistanceValue")
}

// KindFromEntity returns creep kind from attack range + magical resistance.
// Flagbearer: range 100 and resist 40; plain melee: range 100 and resist 0.
func KindFromEntity(e *manta.Entity) string {
	ar, ok := GetAttackRange(e)
	if !ok {
		return ""
	}
	switch ar {
	case AttackRangeRanged:
		return KindRanged
	case AttackRangeSiege:
		return KindSiege
	case AttackRangeMelee:
		if mr, ok := GetMagicalResistance(e); ok && mr == MagicalResistanceFlagbearer {
			return KindFlagbearer
		}
		return KindMelee
	default:
		return ""
	}
}

// KindFromTargetName returns "flagbearer", "melee", "ranged", "siege", or "" from a combat-log NPC name.
func KindFromTargetName(targetName string) string {
	name := strings.ToLower(strings.TrimSpace(targetName))
	switch {
	case strings.Contains(name, "_flagbearer"):
		return KindFlagbearer
	case strings.Contains(name, "_siege"):
		return KindSiege
	case strings.Contains(name, "_ranged"):
		return KindRanged
	case strings.Contains(name, "_melee"):
		return KindMelee
	default:
		return ""
	}
}

func GetCreepLane(entityName string) string {
	if strings.Contains(entityName, "_mid_") {
		return "mid"
	}
	if strings.Contains(entityName, "_bot_") {
		return "bot"
	}
	if strings.Contains(entityName, "_top_") {
		return "top"
	}
	return ""
}

// pathcornerMapSide maps EntityNames pathcorners to map side from spawn geography
// (proof: manta-labs/proofs/pathcorner-lane-spawn).
// Rule: mean spawn in top-right (x>0,y>0) → "bad"; bottom-left otherwise → "good".
// Do not use the goodguys/badguys suffix — it is routing metadata, not team/side.
var pathcornerMapSide = map[string]string{
	"lane_bot_pathcorner_badguys_1":   "good",
	"lane_bot_pathcorner_badguys_4":   "good",
	"lane_bot_pathcorner_badguys_5":   "good",
	"lane_bot_pathcorner_goodguys_2":  "good",
	"lane_bot_pathcorner_goodguys_3":  "good",
	"lane_mid_pathcorner_badguys_7":   "good",
	"lane_top_pathcorner_goodguys_2b": "good",
	"lane_top_pathcorner_badguys_2b":  "good",
	"lane_top_pathcorner_badguys_4":   "good",
	"lane_bot_pathcorner_badguys_3":   "bad",
	"lane_mid_pathcorner_badguys_4":   "bad",
	"lane_mid_pathcorner_badguys_5":   "bad",
	"lane_mid_pathcorner_goodguys_1":  "bad",
	"lane_mid_pathcorner_goodguys_3":  "bad",
	"lane_mid_pathcorner_goodguys_4":  "bad",
}

// GetCreepSide returns "good" or "bad" for a lane-creep EntityNames pathcorner
// based on spawn map corner (bottom-left = good/Radiant, top-right = bad/Dire).
// For combat-log npc_dota_creep_* names, falls back to the name suffix.
func GetCreepSide(entityName string) string {
	name := strings.ToLower(strings.TrimSpace(entityName))
	if side, ok := pathcornerMapSide[name]; ok {
		return side
	}
	if strings.Contains(name, "_goodguys_") {
		return "good"
	}
	if strings.Contains(name, "_badguys_") {
		return "bad"
	}
	return ""
}

type xy struct{ x, y float32 }

// Spawn lane centroids from manta-labs/proofs/spawn-lane-clusters (7 replays).
// Regenerate: ./manta-labs/proofs/spawn-lane-clusters/run.sh
var spawnLaneCentroids = map[string]map[string]xy{
	"good": {
		"top": {-6720.7, -4100.7},
		"mid": {-5121.4, -4609.1},
		"bot": {-3834.3, -6217.5},
	},
	"bad": {
		"top": {3070.9, 5634.1},
		"mid": {4001.4, 3495.1},
		"bot": {6143.8, 3567.4},
	},
}

// Max distance to accept a point as a spawn-cluster hit.
// Proof max intra-cluster ~360; min inter-cluster ~1678.
const spawnClusterMaxDist = 512

// nearestSpawnCluster returns side+lane of the nearest of the 6 spawn centroids.
// ok is false when the point is not near any known spawn slot (creep already walking,
// default cells, etc.) — do not treat that as a spawn location.
func nearestSpawnCluster(x, y float32) (side, lane string, ok bool) {
	bestD := float32(math.MaxFloat32)
	for s, lanes := range spawnLaneCentroids {
		for l, c := range lanes {
			dx := x - c.x
			dy := y - c.y
			d := dx*dx + dy*dy
			if d < bestD {
				bestD = d
				side, lane = s, l
			}
		}
	}
	if float32(math.Sqrt(float64(bestD))) > spawnClusterMaxDist {
		return "", "", false
	}
	return side, lane, true
}

// GetCreepSideFromSpawnLocation returns "good"/"bad" when (x,y) is near a known
// spawn cluster; otherwise "".
func GetCreepSideFromSpawnLocation(x, y float32) string {
	side, _, ok := nearestSpawnCluster(x, y)
	if !ok {
		return ""
	}
	return side
}

// GetCreepLaneFromSpawnLocation returns "top"/"mid"/"bot" when (x,y) is near a
// known spawn cluster; otherwise "". Use at first full-HP create, not mid-path.
func GetCreepLaneFromSpawnLocation(x, y float32) string {
	_, lane, ok := nearestSpawnCluster(x, y)
	if !ok {
		return ""
	}
	return lane
}
