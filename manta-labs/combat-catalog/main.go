// Command combat-catalog collects unique hero, item, and spell names from
// combat-log entries across one or more replays.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
)

type nameHit struct {
	Name    string   `json:"name"`
	Count   int      `json:"count"`
	Replays []string `json:"replays"`
}

type catalog struct {
	heroes              map[string]*nameHit
	items               map[string]*nameHit
	spells              map[string]*nameHit
	spellDamageInflictor map[string]*nameHit
	itemDamageInflictor  map[string]*nameHit
}

func newCatalog() *catalog {
	return &catalog{
		heroes:              make(map[string]*nameHit),
		items:               make(map[string]*nameHit),
		spells:              make(map[string]*nameHit),
		spellDamageInflictor: make(map[string]*nameHit),
		itemDamageInflictor:  make(map[string]*nameHit),
	}
}

func (c *catalog) add(bucket map[string]*nameHit, name, replay string) {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "idx:") {
		return
	}
	h, ok := bucket[name]
	if !ok {
		h = &nameHit{Name: name}
		bucket[name] = h
	}
	h.Count++
	for _, r := range h.Replays {
		if r == replay {
			return
		}
	}
	h.Replays = append(h.Replays, replay)
}

func lookup(p *manta.Parser, idx uint32) string {
	if idx == 0 {
		return ""
	}
	s, ok := p.LookupStringByIndex("CombatLogNames", int32(idx))
	if !ok {
		return ""
	}
	return s
}

func isSpellInflictor(name string) bool {
	return name != "" && name != "dota_unknown" &&
		!strings.HasPrefix(name, "item_") &&
		!strings.HasPrefix(name, "npc_dota_")
}

func classifyAndAdd(c *catalog, name, replay string, asSpell bool) {
	switch {
	case strings.HasPrefix(name, "npc_dota_hero_"):
		c.add(c.heroes, name, replay)
	case strings.HasPrefix(name, "item_"):
		c.add(c.items, name, replay)
	case asSpell && isSpellInflictor(name):
		c.add(c.spells, name, replay)
	}
}

func (c *catalog) addSpellInflictor(inflictor, replay string) {
	if !isSpellInflictor(inflictor) {
		return
	}
	c.add(c.spells, inflictor, replay)
}

func (c *catalog) addItemInflictor(inflictor, replay string) {
	if !strings.HasPrefix(inflictor, "item_") {
		return
	}
	c.add(c.items, inflictor, replay)
}

func (c *catalog) addDamageInflictors(inflictor, replay string) {
	if isSpellInflictor(inflictor) {
		c.add(c.spellDamageInflictor, inflictor, replay)
	}
	if strings.HasPrefix(inflictor, "item_") {
		c.add(c.itemDamageInflictor, inflictor, replay)
	}
}

func (c *catalog) register(p *manta.Parser, replay string) {
	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		t := m.GetType()
		if t == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_INVALID {
			return nil
		}

		attacker := lookup(p, m.GetAttackerName())
		target := lookup(p, m.GetTargetName())
		inflictor := lookup(p, m.GetInflictorName())
		damageSource := lookup(p, m.GetDamageSourceName())

		classifyAndAdd(c, attacker, replay, false)
		classifyAndAdd(c, target, replay, false)
		classifyAndAdd(c, damageSource, replay, false)

		switch t {
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ABILITY,
			dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ABILITY_TRIGGER:
			c.addSpellInflictor(inflictor, replay)
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_ITEM:
			c.addItemInflictor(inflictor, replay)
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_PURCHASE:
			c.addItemInflictor(lookup(p, m.GetValue()), replay)
		case dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE:
			c.addSpellInflictor(inflictor, replay)
			c.addItemInflictor(inflictor, replay)
			c.addDamageInflictors(inflictor, replay)
		default:
			// Heal, modifier, death, etc. may carry spell/item inflictors.
			c.addSpellInflictor(inflictor, replay)
			c.addItemInflictor(inflictor, replay)
		}
		return nil
	})
}

func sortedHits(bucket map[string]*nameHit) []nameHit {
	out := make([]nameHit, 0, len(bucket))
	for _, h := range bucket {
		cp := *h
		sort.Strings(cp.Replays)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func writeNameList(path string, hits []nameHit) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "# count=%d\n", len(hits))
	for _, h := range hits {
		fmt.Fprintf(f, "%s\t%d\t%d\n", h.Name, h.Count, len(h.Replays))
	}
	return nil
}

func writeSummary(w io.Writer, heroes, items, spells, damageSpells, damageItems []nameHit, replays int) {
	fmt.Fprintf(w, "# combat-catalog summary\n")
	fmt.Fprintf(w, "replays\t%d\n", replays)
	fmt.Fprintf(w, "heroes\t%d\n", len(heroes))
	fmt.Fprintf(w, "items\t%d\n", len(items))
	fmt.Fprintf(w, "spells\t%d\n", len(spells))
	fmt.Fprintf(w, "damage_spells\t%d\n", len(damageSpells))
	fmt.Fprintf(w, "damage_items\t%d\n", len(damageItems))
}

func writeJSON(path string, heroes, items, spells, damageSpells, damageItems []nameHit, replays []string) error {
	payload := map[string]any{
		"replays":        replays,
		"heroes":         heroes,
		"items":          items,
		"spells":         spells,
		"damage_spells":  damageSpells,
		"damage_items":   damageItems,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func processReplay(path string, c *catalog) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	p, err := manta.NewStreamParser(f)
	if err != nil {
		return err
	}
	c.register(p, filepath.Base(path))
	return p.Start()
}

func discoverExplored(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".dem") {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

type replayList []string

func (r *replayList) String() string { return strings.Join(*r, ",") }
func (r *replayList) Set(v string) error {
	*r = append(*r, v)
	return nil
}

func main() {
	log.SetOutput(os.Stderr)

	out := flag.String("out", "output", "output directory")
	explored := flag.String("explored", "", "scan directory for *.dem (default: ../../dota-replays/explored)")
	limit := flag.Int("limit", 0, "parse at most N dems (0 = all)")
	var replays replayList
	flag.Var(&replays, "replay", "replay .dem path (repeatable)")
	flag.Parse()

	paths := append([]string(nil), replays...)
	paths = append(paths, flag.Args()...)

	if len(paths) == 0 {
		root := *explored
		if root == "" {
			root = filepath.Join("..", "..", "dota-replays", "explored")
		}
		found, err := discoverExplored(root)
		if err != nil {
			log.Fatalf("scan explored: %v", err)
		}
		paths = found
		log.Printf("discovered %d dem(s) under %s", len(paths), root)
	}

	if len(paths) == 0 {
		fmt.Fprintf(os.Stderr, "usage: %s [-explored dir] [-replay path.dem ...] [path.dem ...]\n", os.Args[0])
		os.Exit(2)
	}

	if *limit > 0 && len(paths) > *limit {
		paths = paths[:*limit]
	}

	if err := os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}

	c := newCatalog()
	parsed := make([]string, 0, len(paths))
	for i, path := range paths {
		log.Printf("[%d/%d] parsing %s", i+1, len(paths), path)
		if err := processReplay(path, c); err != nil {
			log.Printf("warn: skip %s: %v", path, err)
			continue
		}
		parsed = append(parsed, filepath.Base(path))
	}

	heroes := sortedHits(c.heroes)
	items := sortedHits(c.items)
	spells := sortedHits(c.spells)
	damageSpells := sortedHits(c.spellDamageInflictor)
	damageItems := sortedHits(c.itemDamageInflictor)

	if err := writeNameList(filepath.Join(*out, "heroes.txt"), heroes); err != nil {
		log.Fatal(err)
	}
	if err := writeNameList(filepath.Join(*out, "items.txt"), items); err != nil {
		log.Fatal(err)
	}
	if err := writeNameList(filepath.Join(*out, "spells.txt"), spells); err != nil {
		log.Fatal(err)
	}
	if err := writeNameList(filepath.Join(*out, "damage_spells.txt"), damageSpells); err != nil {
		log.Fatal(err)
	}
	if err := writeNameList(filepath.Join(*out, "damage_items.txt"), damageItems); err != nil {
		log.Fatal(err)
	}

	sf, err := os.Create(filepath.Join(*out, "summary.txt"))
	if err != nil {
		log.Fatal(err)
	}
	writeSummary(sf, heroes, items, spells, damageSpells, damageItems, len(parsed))
	sf.Close()

	if err := writeJSON(filepath.Join(*out, "catalog.json"), heroes, items, spells, damageSpells, damageItems, parsed); err != nil {
		log.Fatal(err)
	}

	log.Printf("done: heroes=%d items=%d spells=%d damage_spells=%d damage_items=%d replays=%d -> %s",
		len(heroes), len(items), len(spells), len(damageSpells), len(damageItems), len(parsed), *out)
}
