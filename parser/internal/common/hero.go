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

// GuessHeroClassFromNPC converts npc_dota_hero_* string to CDOTA_Unit_Hero_* class name.
func GuessHeroClassFromNPC(npc string) string {
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

// IsRealHero returns true if the entity is a real hero (not clone/illusion).
// Real heroes are created at the beginning of the game.
func IsRealHero(e *manta.Entity) bool {
	m_flCreateTime, _ := e.GetFloat32("m_flCreateTime")
	return m_flCreateTime < 200
}
