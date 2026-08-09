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
// npc_dota_hero_phantom_assassin -> CDOTA_Unit_Hero_PhantomAssassin
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

// HeroNameToClass maps a display hero name (e.g. "Puck", "Phantom Assassin") to the same
// CDOTA_Unit_Hero_* class name that GetHeroClassName returns for the combat-log npc name.
func HeroNameToClass(heroName string) string {
	heroName = strings.TrimSpace(heroName)
	if heroName == "" {
		return ""
	}
	npc := "npc_dota_hero_" + strings.ToLower(strings.ReplaceAll(heroName, " ", "_"))
	return GetHeroClassName(npc)
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
