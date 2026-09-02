package matchsummary

import "sort"

// HeroKillRow is one row in the heroes-killed table.
type HeroKillRow struct {
	Hero  string `json:"hero"`
	Kills int    `json:"kills"`
}

func (h *Handler) recordHeroKill(targetNPC string) {
	if h.heroesKilled == nil {
		h.heroesKilled = make(map[string]int)
	}
	h.heroesKilled[targetNPC]++
}

func (h *Handler) heroesKilledTable() []HeroKillRow {
	if len(h.heroesKilled) == 0 {
		return nil
	}
	rows := make([]HeroKillRow, 0, len(h.heroesKilled))
	for hero, kills := range h.heroesKilled {
		rows = append(rows, HeroKillRow{Hero: hero, Kills: kills})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Kills != rows[j].Kills {
			return rows[i].Kills > rows[j].Kills
		}
		return rows[i].Hero < rows[j].Hero
	})
	return rows
}
