package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

type combatLogCollector struct {
	maxPerType       int
	maxPerReplayType int
	lines            map[dota.DOTA_COMBATLOG_TYPES][]string
	total            map[dota.DOTA_COMBATLOG_TYPES]int
	perReplay        map[string]map[dota.DOTA_COMBATLOG_TYPES]int
}

func newCombatLogCollector(maxPerType, maxPerReplayType int) *combatLogCollector {
	return &combatLogCollector{
		maxPerType:       maxPerType,
		maxPerReplayType: maxPerReplayType,
		lines:            make(map[dota.DOTA_COMBATLOG_TYPES][]string),
		total:            make(map[dota.DOTA_COMBATLOG_TYPES]int),
		perReplay:        make(map[string]map[dota.DOTA_COMBATLOG_TYPES]int),
	}
}

func (c *combatLogCollector) register(p *manta.Parser, replayName string) {
	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		t := m.GetType()
		if t == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_INVALID {
			return nil
		}
		if c.total[t] >= c.maxPerType {
			return nil
		}
		if c.perReplay[replayName] != nil && c.perReplay[replayName][t] >= c.maxPerReplayType {
			return nil
		}
		line := formatCombatLogEntry(p, m, replayName, p.Tick)
		c.add(replayName, t, line)
		return nil
	})
}

func (c *combatLogCollector) add(replay string, t dota.DOTA_COMBATLOG_TYPES, line string) {
	if c.perReplay[replay] == nil {
		c.perReplay[replay] = make(map[dota.DOTA_COMBATLOG_TYPES]int)
	}
	if c.perReplay[replay][t] >= c.maxPerReplayType {
		return
	}
	c.lines[t] = append(c.lines[t], line)
	c.total[t]++
	c.perReplay[replay][t]++
}

func (c *combatLogCollector) write(outDir string) (int, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return 0, err
	}
	written := 0
	for _, t := range sortedCombatLogTypes(c.lines) {
		if err := writeCombatLogTypeFile(outDir, t, c.lines[t]); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

func combatLogTypeFileName(t dota.DOTA_COMBATLOG_TYPES) string {
	name := strings.TrimPrefix(t.String(), "DOTA_COMBATLOG_")
	return strings.ToLower(name) + ".txt"
}

func sortedCombatLogTypes(lines map[dota.DOTA_COMBATLOG_TYPES][]string) []dota.DOTA_COMBATLOG_TYPES {
	types := make([]dota.DOTA_COMBATLOG_TYPES, 0, len(lines))
	for t := range lines {
		types = append(types, t)
	}
	for i := 0; i < len(types); i++ {
		for j := i + 1; j < len(types); j++ {
			if types[j] < types[i] {
				types[i], types[j] = types[j], types[i]
			}
		}
	}
	return types
}

func writeCombatLogTypeFile(outDir string, t dota.DOTA_COMBATLOG_TYPES, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	path := filepath.Join(outDir, combatLogTypeFileName(t))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "# combat log type: %s\n", t.String())
	fmt.Fprintf(f, "# examples: %d\n", len(lines))
	for _, line := range lines {
		fmt.Fprintln(f, line)
	}
	return nil
}

func writeCombatLogSummary(w io.Writer, counts map[dota.DOTA_COMBATLOG_TYPES]int) {
	fmt.Fprintln(w, "# combat log type summary")
	for _, t := range sortedCombatLogTypesFromCounts(counts) {
		fmt.Fprintf(w, "%s\t%d\t%s\n", t.String(), counts[t], combatLogTypeFileName(t))
	}
}

func sortedCombatLogTypesFromCounts(counts map[dota.DOTA_COMBATLOG_TYPES]int) []dota.DOTA_COMBATLOG_TYPES {
	types := make([]dota.DOTA_COMBATLOG_TYPES, 0, len(counts))
	for t := range counts {
		types = append(types, t)
	}
	for i := 0; i < len(types); i++ {
		for j := i + 1; j < len(types); j++ {
			if types[j] < types[i] {
				types[i], types[j] = types[j], types[i]
			}
		}
	}
	return types
}

func lookupCombatName(p *manta.Parser, idx uint32) string {
	if idx == 0 {
		return ""
	}
	s, ok := p.LookupStringByIndex("CombatLogNames", int32(idx))
	if !ok {
		return fmt.Sprintf("idx:%d", idx)
	}
	return s
}

func formatCombatLogEntry(p *manta.Parser, m *dota.CMsgDOTACombatLogEntry, replay string, tick uint32) string {
	var b strings.Builder
	appendField(&b, "replay", replay)
	appendUint(&b, "tick", tick)
	appendFloat(&b, "timestamp", m.GetTimestamp())
	appendField(&b, "type", m.GetType().String())
	appendField(&b, "attacker", lookupCombatName(p, m.GetAttackerName()))
	appendField(&b, "target", lookupCombatName(p, m.GetTargetName()))
	appendField(&b, "inflictor", lookupCombatName(p, m.GetInflictorName()))
	appendField(&b, "damage_source", lookupCombatName(p, m.GetDamageSourceName()))
	appendInt(&b, "health", m.GetHealth())
	appendUint(&b, "value", m.GetValue())
	appendUint(&b, "ability_level", m.GetAbilityLevel())
	appendUint(&b, "gold_reason", m.GetGoldReason())
	appendUint(&b, "xp_reason", m.GetXpReason())
	appendInt(&b, "attacker_team", int32(m.GetAttackerTeam()))
	appendInt(&b, "target_team", int32(m.GetTargetTeam()))
	appendFloat(&b, "location_x", m.GetLocationX())
	appendFloat(&b, "location_y", m.GetLocationY())
	appendUint(&b, "last_hits", m.GetLastHits())
	appendUint(&b, "stack_count", m.GetStackCount())
	appendUint(&b, "neutral_camp_type", m.GetNeutralCampType())
	appendUint(&b, "rune_type", m.GetRuneType())
	appendUint(&b, "damage_type", m.GetDamageType())
	appendUint(&b, "damage_category", m.GetDamageCategory())
	appendBool(&b, "is_attacker_hero", m.GetIsAttackerHero())
	appendBool(&b, "is_target_hero", m.GetIsTargetHero())
	appendBool(&b, "is_attacker_illusion", m.GetIsAttackerIllusion())
	appendBool(&b, "is_target_illusion", m.GetIsTargetIllusion())
	appendBool(&b, "is_target_building", m.GetIsTargetBuilding())
	return b.String()
}

func appendField(b *strings.Builder, key, val string) {
	if val == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	fmt.Fprintf(b, "%s=%s", key, val)
}

func appendInt(b *strings.Builder, key string, val int32) {
	if val == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	fmt.Fprintf(b, "%s=%d", key, val)
}

func appendUint(b *strings.Builder, key string, val uint32) {
	if val == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	fmt.Fprintf(b, "%s=%d", key, val)
}

func appendFloat(b *strings.Builder, key string, val float32) {
	if val == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	fmt.Fprintf(b, "%s=%.3f", key, val)
}

func appendBool(b *strings.Builder, key string, val bool) {
	if !val {
		return
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	fmt.Fprintf(b, "%s=true", key)
}
