package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dotabuff/manta"
)

type entityCollector struct {
	maxPerClass       int
	maxPerReplayClass int
	blocks            map[string][]string
	total             map[string]int
	perReplay         map[string]map[string]int
	seenIdx           map[string]map[int32]struct{}
}

func newEntityCollector(maxPerClass, maxPerReplayClass int) *entityCollector {
	return &entityCollector{
		maxPerClass:       maxPerClass,
		maxPerReplayClass: maxPerReplayClass,
		blocks:            make(map[string][]string),
		total:             make(map[string]int),
		perReplay:         make(map[string]map[string]int),
		seenIdx:           make(map[string]map[int32]struct{}),
	}
}

func (c *entityCollector) register(p *manta.Parser, replayName string) {
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil {
			return nil
		}
		className := e.GetClassName()
		if className == "" {
			return nil
		}
		idx := e.GetIndex()
		if c.hasSeen(className, idx) {
			return nil
		}
		if c.total[className] >= c.maxPerClass {
			return nil
		}
		if c.perReplay[replayName] != nil && c.perReplay[replayName][className] >= c.maxPerReplayClass {
			return nil
		}
		block := formatEntityBlock(p, e, op, replayName, p.Tick)
		c.add(replayName, className, idx, block)
		return nil
	})
}

func (c *entityCollector) hasSeen(className string, idx int32) bool {
	if c.seenIdx[className] == nil {
		return false
	}
	_, ok := c.seenIdx[className][idx]
	return ok
}

func (c *entityCollector) add(replay, className string, idx int32, block string) {
	if c.perReplay[replay] == nil {
		c.perReplay[replay] = make(map[string]int)
	}
	if c.perReplay[replay][className] >= c.maxPerReplayClass {
		return
	}
	if c.seenIdx[className] == nil {
		c.seenIdx[className] = make(map[int32]struct{})
	}
	c.seenIdx[className][idx] = struct{}{}
	c.blocks[className] = append(c.blocks[className], block)
	c.total[className]++
	c.perReplay[replay][className]++
}

func (c *entityCollector) write(outDir string) (int, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return 0, err
	}
	written := 0
	for _, className := range sortedStrings(stringMapKeys(c.blocks)) {
		if err := writeEntityClassFile(outDir, className, c.blocks[className]); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

func entityClassFileName(className string) string {
	return strings.ToLower(className) + ".txt"
}

func writeEntityClassFile(outDir, className string, blocks []string) error {
	if len(blocks) == 0 {
		return nil
	}
	path := filepath.Join(outDir, entityClassFileName(className))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "# entity class: %s\n", className)
	fmt.Fprintf(f, "# examples: %d\n", len(blocks))
	for i, block := range blocks {
		if i > 0 {
			fmt.Fprintln(f)
		}
		fmt.Fprint(f, block)
		if !strings.HasSuffix(block, "\n") {
			fmt.Fprintln(f)
		}
	}
	return nil
}

func writeEntitySummary(w io.Writer, counts map[string]int) {
	fmt.Fprintln(w, "# entity class summary")
	for _, className := range sortedStrings(keysOf(counts)) {
		fmt.Fprintf(w, "%s\t%d\t%s\n", className, counts[className], entityClassFileName(className))
	}
}

func formatEntityBlock(p *manta.Parser, e *manta.Entity, op manta.EntityOp, replay string, tick uint32) string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== replay=%s tick=%d idx=%d op=%s class=%s ===\n", replay, tick, e.GetIndex(), op.String(), e.GetClassName())

	fields := e.Map()
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := fields[k]
		if v == nil {
			continue
		}
		fmt.Fprintf(&b, "%s=%s", k, formatEntityValue(v))
		if name := resolveEntityName(p, k, v); name != "" {
			fmt.Fprintf(&b, "  # EntityNames: %s", name)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func formatEntityValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		if strings.ContainsAny(x, " \t\n\r") {
			return fmt.Sprintf("%q", x)
		}
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float32:
		return fmt.Sprintf("%.6g", x)
	case int32:
		return fmt.Sprintf("%d", x)
	case uint32:
		return fmt.Sprintf("%d", x)
	case uint64:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case []float32:
		parts := make([]string, len(x))
		for i, f := range x {
			parts[i] = fmt.Sprintf("%.6g", f)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case []byte:
		if len(x) == 0 {
			return "[]"
		}
		if len(x) <= 64 {
			return fmt.Sprintf("0x%x", x)
		}
		return fmt.Sprintf("0x%x...(+%d bytes)", x[:64], len(x)-64)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func resolveEntityName(p *manta.Parser, key string, v interface{}) string {
	idx, ok := int32FromInterface(v)
	if !ok || idx == 0 {
		return ""
	}
	if key == "m_iUnitNameIndex" || key == "m_pEntity.m_nameStringTableIndex" || strings.HasSuffix(key, "StringTableIndex") {
		if name, ok := p.LookupStringByIndex("EntityNames", idx); ok && name != "" {
			return name
		}
	}
	return ""
}

func int32FromInterface(v interface{}) (int32, bool) {
	switch x := v.(type) {
	case int32:
		return x, true
	case uint32:
		return int32(x), true
	case int64:
		return int32(x), true
	case uint64:
		return int32(x), true
	default:
		return 0, false
	}
}

func stringMapKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keysOf(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func sortedStrings(ss []string) []string {
	out := append([]string(nil), ss...)
	sort.Strings(out)
	return out
}
