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
			mark = "✗"
		}
		fmt.Fprintf(w, "  %s  %s\n", mark, c.Label)
		fmt.Fprintf(w, "       replay: %s   hero: %s   role: %s   type: %s\n",
			c.Replay, c.Hero, c.HeroRole.Label(), c.CaseType.Label())
		fmt.Fprintf(w, "       %s\n", c.Description)
		fmt.Fprintf(w, "       expect: %-16s  actual: %s\n", expectedLabel(c.ExpectMiss), r.Detail)
		fmt.Fprintln(w)
	}

	pass, buckets := summarize(results)

	fmt.Fprintln(w, strings.Repeat("=", width))
	fmt.Fprintln(w, "  Summary")
	fmt.Fprintln(w, strings.Repeat("-", width))
	fmt.Fprintf(w, "  Total cases passed: %d/%d\n", pass, len(results))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  By role and case type:")
	for _, line := range formatBucketLines(buckets) {
		fmt.Fprintf(w, "    %s\n", line)
	}
	fmt.Fprintln(w, strings.Repeat("=", width))
}

func summarize(results []caseResult) (int, map[bucketKey]bucketStat) {
	pass := 0
	buckets := make(map[bucketKey]bucketStat)
	for _, r := range results {
		if r.Pass {
			pass++
		}
		key := bucketKey{Role: r.Case.HeroRole, Type: r.Case.CaseType}
		stat := buckets[key]
		stat.Total++
		if r.Pass {
			stat.Passed++
		}
		buckets[key] = stat
	}
	return pass, buckets
}

func formatBucketLines(buckets map[bucketKey]bucketStat) []string {
	keys := make([]bucketKey, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Role != keys[j].Role {
			return keys[i].Role < keys[j].Role
		}
		return keys[i].Type < keys[j].Type
	})

	lines := make([]string, len(keys))
	for i, k := range keys {
		stat := buckets[k]
		lines[i] = fmt.Sprintf("%s / %s: %d/%d passed",
			k.Role.Label(), k.Type.Label(), stat.Passed, stat.Total)
	}
	return lines
}
