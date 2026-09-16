package matchsummary

import (
	"sort"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

const autoAttackInflictor = "auto_attack"

// InflictorMatrixRow is one inflictor row in a hero-column matrix.
type InflictorMatrixRow struct {
	Inflictor string   `json:"inflictor"`
	Values    []uint32 `json:"values"`
}

// InflictorMatrix is damage/healing broken down by inflictor (rows) and hero (columns).
type InflictorMatrix struct {
	Heroes []string             `json:"heroes"`
	Rows   []InflictorMatrixRow `json:"rows"`
}

type inflictorHeroAccumulator map[string]map[string]uint32

func (a inflictorHeroAccumulator) add(inflictor, hero string, amount uint32) {
	if inflictor == "" || hero == "" || amount == 0 {
		return
	}
	if a[inflictor] == nil {
		a[inflictor] = make(map[string]uint32)
	}
	a[inflictor][hero] += amount
}

func inflictorKey(p *manta.Parser, m *dota.CMsgDOTACombatLogEntry) string {
	idx := m.GetInflictorName()
	if idx == 0 {
		return autoAttackInflictor
	}
	name, ok := lookupCombatName(p, idx)
	if !ok || name == "dota_unknown" {
		return autoAttackInflictor
	}
	return name
}

func buildInflictorMatrix(acc inflictorHeroAccumulator) *InflictorMatrix {
	if len(acc) == 0 {
		return nil
	}

	heroTotals := make(map[string]uint32)
	for _, byHero := range acc {
		for hero, amount := range byHero {
			heroTotals[hero] += amount
		}
	}

	heroes := make([]string, 0, len(heroTotals))
	for hero := range heroTotals {
		heroes = append(heroes, hero)
	}
	sort.Slice(heroes, func(i, j int) bool {
		if heroTotals[heroes[i]] != heroTotals[heroes[j]] {
			return heroTotals[heroes[i]] > heroTotals[heroes[j]]
		}
		return heroes[i] < heroes[j]
	})

	heroIndex := make(map[string]int, len(heroes))
	for i, hero := range heroes {
		heroIndex[hero] = i
	}

	inflictors := make([]string, 0, len(acc))
	inflictorTotals := make(map[string]uint32, len(acc))
	for inflictor, byHero := range acc {
		inflictors = append(inflictors, inflictor)
		for _, amount := range byHero {
			inflictorTotals[inflictor] += amount
		}
	}
	sort.Slice(inflictors, func(i, j int) bool {
		if inflictorTotals[inflictors[i]] != inflictorTotals[inflictors[j]] {
			return inflictorTotals[inflictors[i]] > inflictorTotals[inflictors[j]]
		}
		return inflictors[i] < inflictors[j]
	})

	rows := make([]InflictorMatrixRow, 0, len(inflictors))
	for _, inflictor := range inflictors {
		values := make([]uint32, len(heroes))
		for hero, amount := range acc[inflictor] {
			values[heroIndex[hero]] = amount
		}
		rows = append(rows, InflictorMatrixRow{Inflictor: inflictor, Values: values})
	}

	return &InflictorMatrix{Heroes: heroes, Rows: rows}
}
