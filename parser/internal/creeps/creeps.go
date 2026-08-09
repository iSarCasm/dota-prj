package creeps

import "strings"

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
	if strings.HasPrefix(targetName, "npc_dota_creep_siege") {
		return "lane"
	}
	return ""
}

// IsLaneCreep reports whether className is a creep entity class.
func IsLaneCreep(className string) bool {
	return className == "CDOTA_BaseNPC_Creep_Lane"
}

func IsCreep(className string) bool {
	return strings.HasPrefix(className, "CDOTA_BaseNPC_Creep")
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
