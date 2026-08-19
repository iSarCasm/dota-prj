package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

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

func writeReport(w io.Writer, results []caseResult) {
	const width = 78

	fmt.Fprintln(w, strings.Repeat("=", width))
	fmt.Fprintln(w, "  Missed Last-Hit Quality Report")
	fmt.Fprintln(w, strings.Repeat("=", width))
	fmt.Fprintln(w)

	for _, r := range results {
		c := r.Case
		mark := "✓"
		if !r.Pass {
			if c.ExpectMiss {
				mark = "✗ FN"
			} else {
				mark = "✗ FP"
			}
		}
		fmt.Fprintf(w, "  %s  %s\n", mark, c.Label)
		fmt.Fprintf(w, "       replay: %s   hero: %s   role: %s   tags: %s\n",
			c.Replay, c.Hero, c.HeroRole.Label(), formatTags(c.Tags))
		fmt.Fprintf(w, "       %s\n", c.Description)
		fmt.Fprintf(w, "       expect: %-16s  actual: %s\n", expectedLabel(c.ExpectMiss), r.Detail)
		fmt.Fprintln(w)
	}

	totals, groupBuckets, tagBuckets := summarize(results)

	fmt.Fprintln(w, strings.Repeat("=", width))
	fmt.Fprintln(w, "  Summary")
	fmt.Fprintln(w, strings.Repeat("-", width))
	fmt.Fprintf(w, "  Total cases passed: %d/%d\n", totals.Passed, totals.Total)
	fmt.Fprintf(w, "  False positives:    %d  (unexpected miss)\n", totals.FP)
	fmt.Fprintf(w, "  False negatives:    %d  (expected miss not found)\n", totals.FN)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  By replay and hero:")
	for _, line := range formatGroupLines(groupBuckets) {
		fmt.Fprintf(w, "    %s\n", line)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  By role and tag:")
	for _, line := range formatTagBucketLines(tagBuckets) {
		fmt.Fprintf(w, "    %s\n", line)
	}
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

func summarize(results []caseResult) (bucketStat, map[groupKey]bucketStat, map[bucketKey]bucketStat) {
	var totals bucketStat
	groupBuckets := make(map[groupKey]bucketStat)
	tagBuckets := make(map[bucketKey]bucketStat)
	for _, r := range results {
		totals = addResult(totals, r)

		gkey := groupKey{Replay: r.Case.Replay, Hero: r.Case.Hero}
		groupBuckets[gkey] = addResult(groupBuckets[gkey], r)

		for _, tag := range r.Case.Tags {
			key := bucketKey{Role: r.Case.HeroRole, Tag: tag}
			tagBuckets[key] = addResult(tagBuckets[key], r)
		}
	}
	return totals, groupBuckets, tagBuckets
}

func formatGroupLines(buckets map[groupKey]bucketStat) []string {
	keys := make([]groupKey, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Replay != keys[j].Replay {
			return keys[i].Replay < keys[j].Replay
		}
		return keys[i].Hero < keys[j].Hero
	})

	lines := make([]string, len(keys))
	for i, k := range keys {
		stat := buckets[k]
		lines[i] = fmt.Sprintf("%s / %s: %d/%d passed",
			k.Replay, k.Hero, stat.Passed, stat.Total)
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
