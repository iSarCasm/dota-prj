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

// IsEntityClass reports whether className is a creep entity class.
func IsEntityClass(className string) bool {
	return strings.HasPrefix(className, "CDOTA_BaseNPC_Creep")
}
