package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/dotabuff/manta"
)

// Returns the name of the calling function
func _caller(n int) string {
	if pc, _, _, ok := runtime.Caller(n); ok {
		fns := strings.Split(runtime.FuncForPC(pc).Name(), "/")
		return fns[len(fns)-1]
	}

	return "unknown"
}

// dump named object
func _dump(w io.Writer, label string, args ...interface{}) {
	fmt.Fprintf(w, "%s: %s", _caller(2), label)
	// spew.Dump(args...)
	spew.Fdump(w, args...)
}

// Dump prints the current entity state to standard output
func fDump(w io.Writer, e *manta.Entity) {
	_dump(w, e.String(), e.Map())
}

func main() {
	unitDumps := [...]string{"CDOTA_Item_PowerTreads"}

	f, err := os.Open("../replay1.dem")
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer f.Close()

	// Quick sanity: file size
	if st, err := f.Stat(); err == nil {
		log.Printf("Replay size: %d bytes", st.Size())
	}

	p, err := manta.NewStreamParser(f)
	if err != nil {
		log.Fatalf("NewStreamParser: %v", err)
	}

	// Init Entity Stats
	entityStats := map[string]int{}
	fEntityStats, err := os.Create("entity_stats.log")
	if err != nil {
		log.Fatalf("create entity_stats.log: %v", err)
	}
	defer fEntityStats.Close()

	// Init Entity Dumps
	unitDumpLogs := make(map[string]*os.File)
	logNum := 0
	for _, unit := range unitDumps {
		f, err := os.Create(fmt.Sprintf("entity_dump%d.log", logNum))
		if err != nil {
			log.Fatalf("create %s.log: %v", unit, err)
		}
		unitDumpLogs[unit] = f
		logNum++
	}

	// Maintain our best-effort hero lookup index as entity updates stream in.
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e == nil {
			return nil
		}
		cn := e.GetClassName()
		// log.Printf("Entity: %s", cn)
		entityStats[cn]++

		if _, ok := unitDumpLogs[cn]; ok {
			fDump(unitDumpLogs[cn], e)
		}

		return nil
	})

	// IMPORTANT: actually check Start() error
	if err := p.Start(); err != nil && err != io.EOF {
		log.Fatalf("parse error: %v", err)
	}

	// Write to Log Files
	// Write Entity Stats (sorted by count)
	sortedEntityStats := make([]string, 0, len(entityStats))
	for cn := range entityStats {
		sortedEntityStats = append(sortedEntityStats, cn)
	}
	sort.Slice(sortedEntityStats, func(i, j int) bool {
		return entityStats[sortedEntityStats[i]] > entityStats[sortedEntityStats[j]]
	})
	for _, cn := range sortedEntityStats {
		fEntityStats.WriteString(fmt.Sprintf("%s: %d\n", cn, entityStats[cn]))
	}

	log.Printf("Parse Complete!")
}
