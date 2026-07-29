package common

import (
	"strings"
	"unicode"

	"github.com/dotabuff/manta"
)

type HeroRef struct {
	ClassName string
	EntityIdx uint32
}

// converts npc_dota_hero_* string to CDOTA_Unit_Hero_* class name.
func GetHeroClassName(npc string) string {
	const prefix = "npc_dota_hero_"
	if !strings.HasPrefix(npc, prefix) {
		return ""
	}
	raw := strings.TrimPrefix(npc, prefix) // e.g. "zuus" or "keeper_of_the_light"
	parts := strings.Split(raw, "_")
	var b strings.Builder
	b.WriteString("CDOTA_Unit_Hero_")
	for _, p2 := range parts {
		if p2 == "" {
			continue
		}
		r := []rune(p2)
		r[0] = unicode.ToUpper(r[0])
		b.WriteString(string(r))
	}
	return b.String()
}

// HeroNameToClass maps a hero name (e.g. "Puck", "keeper_of_the_light") to CDOTA_Unit_Hero_* class name.
func HeroNameToClass(heroName string) string {
	heroName = strings.TrimSpace(strings.ReplaceAll(heroName, " ", "_"))
	if heroName == "" {
		return ""
	}
	// parts := strings.Split(heroName, "_")
	// titled := make([]string, 0, len(parts))
	// for _, p := range parts {
	// 	if p == "" {
	// 		continue
	// 	}
	// 	r := []rune(strings.ToLower(p))
	// 	if len(r) > 0 {
	// 		r[0] = unicode.ToUpper(r[0])
	// 		titled = append(titled, string(r))
	// 	}
	// }
	// return "CDOTA_Unit_Hero_" + strings.Join(titled, "_")
	return "CDOTA_Unit_Hero_" + heroName
}

// entityClassToCombatLogName converts an entity class name to the combat-log style name
// so we can key mana cost by the same string the combat log uses.
// e.g. CDOTA_Ability_StormSpirit_StaticRemnant -> storm_spirit_static_remnant, CDOTA_Item_PowerTreads -> item_power_treads.
func EntityClassToCombatLogName(className string) string {
	className = strings.TrimSpace(className)
	if className == "" {
		return ""
	}
	const abilityPrefix = "CDOTA_Ability_"
	const itemPrefix = "CDOTA_Item_"
	var rest string
	if strings.HasPrefix(className, abilityPrefix) {
		rest = strings.TrimPrefix(className, abilityPrefix)
	} else if strings.HasPrefix(className, itemPrefix) {
		rest = strings.TrimPrefix(className, itemPrefix)
		return "item_" + pascalToSnake(rest)
	} else {
		return ""
	}
	// rest is e.g. "StormSpirit_StaticRemnant" - words separated by underscore, each word in PascalCase
	parts := strings.Split(rest, "_")
	var out []string
	for _, word := range parts {
		if word == "" {
			continue
		}
		out = append(out, pascalToSnake(word))
	}
	return strings.Join(out, "_")
}

// pascalToSnake converts PascalCase to snake_case (e.g. StormSpirit -> storm_spirit).
func pascalToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// IsRealHero returns true if the entity is a real hero (not clone/illusion).
// Real heroes are created at the beginning of the game.
// preGameStartTime is the replay's pre-game start time (e.g. from timeandpauses.Handler.PreGameStartTime()).
func IsRealHero(e *manta.Entity, preGameStartTime float32) bool {
	createTime, _ := e.GetFloat32("m_flCreateTime")
	return createTime <= preGameStartTime+10
}
