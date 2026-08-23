package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"dota2/internal/lasthits"
)

func evaluateCase(c qualityCase, misses []lasthits.Event) caseResult {
	matched := eventsInWindow(misses, c.From, c.To, c.CreepContains)
	gotMiss := len(matched) > 0

	var pass bool
	var detail string
	switch {
	case c.ExpectMiss && gotMiss:
		pass = true
		detail = fmt.Sprintf("miss @ %s on %s", formatGameClock(matched[0].Timestamp), shortCreepName(matched[0].CreepName))
	case c.ExpectMiss && !gotMiss:
		pass = false
		detail = "expected miss in window — none found"
	case !c.ExpectMiss && !gotMiss:
		pass = true
		detail = "no miss in window"
	default:
		pass = false
		parts := make([]string, len(matched))
		for i, e := range matched {
			parts[i] = fmt.Sprintf("%s @ %s", shortCreepName(e.CreepName), formatGameClock(e.Timestamp))
		}
		detail = "unexpected miss: " + strings.Join(parts, ", ")
	}
	return caseResult{Case: c, Pass: pass, Detail: detail}
}

func eventsInWindow(events []lasthits.Event, from, to float32, creepContains string) []lasthits.Event {
	var matched []lasthits.Event
	for _, e := range events {
		if e.Timestamp < from || e.Timestamp > to {
			continue
		}
		if creepContains != "" && !strings.Contains(e.CreepName, creepContains) {
			continue
		}
		matched = append(matched, e)
	}
	return matched
}

func formatGameClock(seconds float32) string {
	m := int(seconds) / 60
	s := seconds - float32(m*60)
	return fmt.Sprintf("%d:%05.2f", m, s)
}

func shortCreepName(name string) string {
	return strings.TrimPrefix(name, "npc_dota_creep_")
}

func formatTags(tags []CaseTag) string {
	if len(tags) == 0 {
		return "—"
	}
	parts := make([]string, len(tags))
	for i, t := range tags {
		parts[i] = string(t)
	}
	return strings.Join(parts, ", ")
}

func expectedLabel(expectMiss bool) string {
	if expectMiss {
		return "should miss"
	}
	return "should not miss"
}

func formatWindow(from, to float32) string {
	return formatGameClock(from) + "-" + formatGameClock(to)
}

func writeReport(w io.Writer, results []caseResult, elapsed time.Duration) {
	const width = 78

	fmt.Fprintln(w, strings.Repeat("=", width))
	fmt.Fprintln(w, "  Missed Last-Hit Quality Report")
	fmt.Fprintln(w, strings.Repeat("=", width))
	fmt.Fprintln(w)

	for _, r := range results {
		c := r.Case
		if r.Pass {
			fmt.Fprintf(w, "  ✓  %s  %s  %s\n",
				c.ReplayHero.Replay.ID, c.ReplayHero.Hero, formatWindow(c.From, c.To))
			continue
		}

		mark := "✗ FN"
		if !c.ExpectMiss {
			mark = "✗ FP"
		}
		fmt.Fprintf(w, "  %s  %s\n", mark, c.Label)
		fmt.Fprintf(w, "       replay: %s   skill: %s   hero: %s   role: %s   tags: %s\n",
			c.ReplayHero.Replay.ID, c.ReplayHero.Replay.SkillLevel.Label(),
			c.ReplayHero.Hero, c.ReplayHero.Role.Label(), formatTags(c.Tags))
		if c.Description != "" {
			fmt.Fprintf(w, "       %s\n", c.Description)
		}
		fmt.Fprintf(w, "       window: %s   target: %s\n", formatWindow(c.From, c.To), c.CreepContains)
		fmt.Fprintf(w, "       expect: %-16s  actual: %s\n", expectedLabel(c.ExpectMiss), r.Detail)
		fmt.Fprintln(w)
	}

	totals, groupBuckets, tagBuckets, tagPairBuckets, skillBuckets := summarize(results)

	fmt.Fprintln(w, strings.Repeat("=", width))
	fmt.Fprintln(w, "  Summary")
	fmt.Fprintln(w, strings.Repeat("-", width))
	fmt.Fprintln(w, "  By replay and hero:")
	for _, line := range formatGroupLines(groupBuckets) {
		fmt.Fprintf(w, "    %s\n", line)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  By skill:")
	for _, line := range formatSkillLines(skillBuckets) {
		fmt.Fprintf(w, "    %s\n", line)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  By role and tag:")
	for _, line := range formatTagBucketLines(tagBuckets) {
		fmt.Fprintf(w, "    %s\n", line)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  By role and tag pair:")
	for _, line := range formatTagPairBucketLines(tagPairBuckets) {
		fmt.Fprintf(w, "    %s\n", line)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Total cases passed: %d/%d\n", totals.Passed, totals.Total)
	fmt.Fprintf(w, "  False positives:    %d  (unexpected miss)\n", totals.FP)
	fmt.Fprintf(w, "  False negatives:    %d  (expected miss not found)\n", totals.FN)
	fmt.Fprintf(w, "  Elapsed:            %s\n", elapsed.Round(time.Millisecond))
	fmt.Fprintln(w, strings.Repeat("=", width))
}

func addResult(stat bucketStat, r caseResult) bucketStat {
	stat.Total++
	if r.Pass {
		stat.Passed++
		return stat
	}
	if r.Case.ExpectMiss {
		stat.FN++
	} else {
		stat.FP++
	}
	return stat
}

var reportTagPairs = []struct {
	Label string
	Tags  []CaseTag
}{
	{Label: "auto-attack / too-early", Tags: []CaseTag{TagAutoAttack, TagTooEarly}},
	{Label: "spell / too-early", Tags: []CaseTag{TagSpell, TagTooEarly}},
}

func hasAllTags(tags []CaseTag, required ...CaseTag) bool {
	for _, req := range required {
		found := false
		for _, tag := range tags {
			if tag == req {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func summarize(results []caseResult) (bucketStat, map[*ReplayHero]bucketStat, map[bucketKey]bucketStat, map[tagPairKey]bucketStat, map[SkillLevel]bucketStat) {
	var totals bucketStat
	groupBuckets := make(map[*ReplayHero]bucketStat)
	tagBuckets := make(map[bucketKey]bucketStat)
	tagPairBuckets := make(map[tagPairKey]bucketStat)
	skillBuckets := make(map[SkillLevel]bucketStat)
	for _, r := range results {
		totals = addResult(totals, r)

		rh := r.Case.ReplayHero
		groupBuckets[rh] = addResult(groupBuckets[rh], r)
		skillBuckets[rh.Replay.SkillLevel] = addResult(skillBuckets[rh.Replay.SkillLevel], r)

		for _, tag := range r.Case.Tags {
			key := bucketKey{Role: rh.Role, Tag: tag}
			tagBuckets[key] = addResult(tagBuckets[key], r)
		}

		for _, pair := range reportTagPairs {
			if !hasAllTags(r.Case.Tags, pair.Tags...) {
				continue
			}
			key := tagPairKey{Role: rh.Role, Label: pair.Label}
			tagPairBuckets[key] = addResult(tagPairBuckets[key], r)
		}
	}
	return totals, groupBuckets, tagBuckets, tagPairBuckets, skillBuckets
}

func formatGroupLines(buckets map[*ReplayHero]bucketStat) []string {
	keys := make([]*ReplayHero, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Replay.ID != keys[j].Replay.ID {
			return keys[i].Replay.ID < keys[j].Replay.ID
		}
		return keys[i].Hero < keys[j].Hero
	})

	lines := make([]string, len(keys))
	for i, k := range keys {
		stat := buckets[k]
		lines[i] = fmt.Sprintf("%s / %s: %d/%d passed",
			k.Replay.ID, k.Hero, stat.Passed, stat.Total)
	}
	return lines
}

func formatSkillLines(buckets map[SkillLevel]bucketStat) []string {
	keys := make([]SkillLevel, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].order() < keys[j].order()
	})

	lines := make([]string, len(keys))
	for i, k := range keys {
		stat := buckets[k]
		lines[i] = fmt.Sprintf("%s: %d/%d passed", k.Label(), stat.Passed, stat.Total)
	}
	return lines
}

func formatTagPairBucketLines(buckets map[tagPairKey]bucketStat) []string {
	keys := make([]tagPairKey, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Role != keys[j].Role {
			return keys[i].Role < keys[j].Role
		}
		return keys[i].Label < keys[j].Label
	})

	lines := make([]string, len(keys))
	for i, k := range keys {
		stat := buckets[k]
		lines[i] = fmt.Sprintf("%s / %s: %d/%d passed",
			k.Role.Label(), k.Label, stat.Passed, stat.Total)
	}
	return lines
}

func formatTagBucketLines(buckets map[bucketKey]bucketStat) []string {
	keys := make([]bucketKey, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Role != keys[j].Role {
			return keys[i].Role < keys[j].Role
		}
		return keys[i].Tag < keys[j].Tag
	})

	lines := make([]string, len(keys))
	for i, k := range keys {
		stat := buckets[k]
		lines[i] = fmt.Sprintf("%s / %s: %d/%d passed",
			k.Role.Label(), k.Tag, stat.Passed, stat.Total)
	}
	return lines
}
