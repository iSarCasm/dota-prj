package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"
	"github.com/golang/snappy"
)

// manta@v1.4.7/string_table.go

type stringTables struct {
	Tables    map[int32]*stringTable
	NameIndex map[string]int32
	nextIndex int32
}

type stringTableItem struct {
	Index int32
	Key   string
	Value []byte
}

type stringTable struct {
	index             int32
	name              string
	Items             map[int32]*stringTableItem
	userDataFixedSize bool
	userDataSizeBits  int32
	flags             int32
	varintBitCounts   bool
}

const (
	stringtableKeyHistorySize = 32
)

// Parse a string table data blob, returning a list of item updates.
func parseStringTable(buf []byte, numUpdates int32, name string, userDataFixed bool, userDataSizeBits int32, flags int32, varintBitCounts bool) (items []*stringTableItem) {
	defer func() {
		if err := recover(); err != nil {
			// _debugf("warning: unable to parse string table %s: %s", name, err)
			log.Printf("warning: unable to parse string table %s: %s", name, err)
			return
		}
	}()

	items = make([]*stringTableItem, 0)

	// Create a reader for the buffer
	r := newReader(buf)

	// Start with an index of -1.
	// If the first item is at index 0 it will use a incr operation.
	index := int32(-1)

	// Maintain a list of key history
	keys := make([]string, 0, stringtableKeyHistorySize)

	// Some tables have no data
	if len(buf) == 0 {
		return items
	}

	// Loop through entries in the data structure
	//
	// Each entry is a tuple consisting of {index, key, value}
	//
	// Index can either be incremented from the previous position or
	// overwritten with a given entry.
	//
	// Key may be omitted (will be represented here as "")
	//
	// Value may be omitted
	for i := 0; i < int(numUpdates); i++ {
		key := ""
		value := []byte{}

		// Read a boolean to determine whether the operation is an increment or
		// has a fixed index position. A fixed index position of zero should be
		// the last data in the buffer, and indicates that all data has been read.
		incr := r.readBoolean()
		if incr {
			index++
		} else {
			index = int32(r.readVarUint32()) + 1
		}

		// Some values have keys, some don't.
		hasKey := r.readBoolean()
		if hasKey {
			// Some entries use reference a position in the key history for
			// part of the key. If referencing the history, read the position
			// and size from the buffer, then use those to build the string
			// combined with an extra string read (null terminated).
			// Alternatively, just read the string.
			useHistory := r.readBoolean()
			if useHistory {
				pos := r.readBits(5)
				size := r.readBits(5)

				if int(pos) >= len(keys) {
					key += r.readString()
				} else {
					s := keys[pos]
					if int(size) > len(s) {
						key += s + r.readString()
					} else {
						key += s[0:size] + r.readString()
					}
				}
			} else {
				key = r.readString()
			}

			if len(keys) >= stringtableKeyHistorySize {
				copy(keys[0:], keys[1:])
				keys[len(keys)-1] = ""
				keys = keys[:len(keys)-1]
			}
			keys = append(keys, key)
		}

		// Some entries have a value.
		hasValue := r.readBoolean()
		if hasValue {
			bitSize := uint32(0)
			isCompressed := false
			if userDataFixed {
				bitSize = uint32(userDataSizeBits)
			} else {
				if (flags & 0x1) != 0 {
					isCompressed = r.readBoolean()
				}
				if varintBitCounts {
					bitSize = r.readUBitVar() * 8
				} else {
					bitSize = r.readBits(17) * 8
				}
			}
			value = r.readBitsAsBytes(bitSize)

			if isCompressed {
				tmp, err := snappy.Decode(nil, value)
				if err != nil {
					// _panicf("unable to decode snappy compressed stringtable item (%s, %d, %s): %s", name, index, key, err)
					log.Printf("unable to decode snappy compressed stringtable item (%s, %d, %s): %s", name, index, key, err)
					return nil
				}
				value = tmp
			}
		}

		items = append(items, &stringTableItem{index, key, value})
	}

	return items
}

// manta@v1.4.7/reader.go
type reader struct {
	buf      []byte
	size     uint32
	pos      uint32
	bitVal   uint64 // value of the remaining bits in the current byte
	bitCount uint32 // number of remaining bits in the current byte
}

// newReader creates a new reader object for the given buffer
func newReader(buf []byte) *reader {
	return &reader{buf, uint32(len(buf)), 0, 0, 0}
}

// remBits calculates the number of unread bits in the buffer
func (r *reader) remBits() uint32 {
	return r.remBytes() + r.bitCount
}

func (r *reader) position() string {
	if r.bitCount > 0 {
		return fmt.Sprintf("%d.%d", r.pos-1, 8-r.bitCount)
	}
	return fmt.Sprintf("%d", r.pos)
}

// remBytes calculates the number of unread bytes in the buffer
func (r *reader) remBytes() uint32 {
	return r.size - r.pos
}

// nextByte reads the next byte from the buffer
func (r *reader) nextByte() byte {
	r.pos += 1
	if r.pos > r.size {
		// _panicf("nextByte: insufficient buffer (%d of %d)", r.pos, r.size)
		log.Printf("nextByte: insufficient buffer (%d of %d)", r.pos, r.size)
		return 0
	}
	return r.buf[r.pos-1]
}

// readBits returns the uint32 value for the given number of sequential bits
func (r *reader) readBits(n uint32) uint32 {
	for n > r.bitCount {
		r.bitVal |= uint64(r.nextByte()) << r.bitCount
		r.bitCount += 8
	}

	x := (r.bitVal & ((1 << n) - 1))
	r.bitVal >>= n
	r.bitCount -= n

	return uint32(x)
}

// readByte reads a single byte
func (r *reader) readByte() byte {
	// Fast path if we're byte aligned
	if r.bitCount == 0 {
		return r.nextByte()
	}

	return byte(r.readBits(8))
}

// readBytes reads the given number of bytes
func (r *reader) readBytes(n uint32) []byte {
	// Fast path if we're byte aligned
	if r.bitCount == 0 {
		r.pos += n
		if r.pos > r.size {
			// _panicf("readBytes: insufficient buffer (%d of %d)", r.pos, r.size)
			log.Printf("readBytes: insufficient buffer (%d of %d)", r.pos, r.size)
			return nil
		}
		return r.buf[r.pos-n : r.pos]
	}

	buf := make([]byte, n)
	for i := uint32(0); i < n; i++ {
		buf[i] = byte(r.readBits(8))
	}
	return buf
}

// readLeUint32 reads an little-endian uint32
func (r *reader) readLeUint32() uint32 {
	return binary.LittleEndian.Uint32(r.readBytes(4))
}

// readLeUint64 reads a little-endian uint64
func (r *reader) readLeUint64() uint64 {
	return binary.LittleEndian.Uint64(r.readBytes(8))
}

// readVarUint64 reads an unsigned 32-bit varint
func (r *reader) readVarUint32() uint32 {
	var x, s uint32
	for {
		b := uint32(r.readByte())
		x |= (b & 0x7F) << s
		s += 7
		if ((b & 0x80) == 0) || (s == 35) {
			break
		}
	}

	return x
}

// readVarInt64 reads a signed 32-bit varint
func (r *reader) readVarInt32() int32 {
	ux := r.readVarUint32()
	x := int32(ux >> 1)
	if ux&1 != 0 {
		x = ^x
	}
	return x
}

// readVarUint64 reads an unsigned 64-bit varint
func (r *reader) readVarUint64() uint64 {
	var x, s uint64
	for i := 0; ; i++ {
		b := r.readByte()
		if b < 0x80 {
			if i > 9 || i == 9 && b > 1 {
				// _panicf("read overflow: varint overflows uint64")
				log.Printf("read overflow: varint overflows uint64")
				return 0
			}
			return x | uint64(b)<<s
		}
		x |= uint64(b&0x7f) << s
		s += 7
	}
}

// readVarInt64 reads a signed 64-bit varint
func (r *reader) readVarInt64() int64 {
	ux := r.readVarUint64()
	x := int64(ux >> 1)
	if ux&1 != 0 {
		x = ^x
	}
	return x
}

// readBoolean reads and interprets single bit as true or false
func (r *reader) readBoolean() bool {
	return r.readBits(1) == 1
}

// readFloat reads an IEEE 754 float
func (r *reader) readFloat() float32 {
	return math.Float32frombits(r.readLeUint32())
}

// readUBitVar reads a variable length uint32 with encoding in last to bits of 6 bit group
func (r *reader) readUBitVar() uint32 {
	ret := r.readBits(6)

	switch ret & 0x30 {
	case 16:
		ret = (ret & 15) | (r.readBits(4) << 4)
		break
	case 32:
		ret = (ret & 15) | (r.readBits(8) << 4)
		break
	case 48:
		ret = (ret & 15) | (r.readBits(28) << 4)
		break
	}

	return ret
}

// readUBitVarFP reads a variable length uint32 encoded using fieldpath encoding
func (r *reader) readUBitVarFP() uint32 {
	if r.readBoolean() {
		return r.readBits(2)
	}
	if r.readBoolean() {
		return r.readBits(4)
	}
	if r.readBoolean() {
		return r.readBits(10)
	}
	if r.readBoolean() {
		return r.readBits(17)
	}
	return r.readBits(31)
}

func (r *reader) readUBitVarFieldPath() int {
	return int(r.readUBitVarFP())
}

// readStringN reads a string of a given length
func (r *reader) readStringN(n uint32) string {
	return string(r.readBytes(n))
}

// readString reads a null terminated string
func (r *reader) readString() string {
	buf := make([]byte, 0)
	for {
		b := r.readByte()
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}

	return string(buf)
}

// readCoord reads a coord as a float32
func (r *reader) readCoord() float32 {
	value := float32(0.0)

	intval := r.readBits(1)
	fractval := r.readBits(1)
	signbit := false

	if intval != 0 || fractval != 0 {
		signbit = r.readBoolean()

		if intval != 0 {
			intval = r.readBits(14) + 1
		}

		if fractval != 0 {
			fractval = r.readBits(5)
		}

		value = float32(intval) + float32(fractval)*(1.0/(1<<5))

		// Fixup the sign if negative.
		if signbit {
			value = -value
		}
	}

	return value
}

// readAngle reads a bit angle of the given size
func (r *reader) readAngle(n uint32) float32 {
	return float32(r.readBits(n)) * 360.0 / float32(int(1<<n))
}

// readNormal reads a normalized float vector
func (r *reader) readNormal() float32 {
	isNeg := r.readBoolean()
	len := r.readBits(11)
	ret := float32(len) * float32(1.0/(float32(1<<11)-1.0))

	if isNeg {
		return -ret
	} else {
		return ret
	}
}

// read3BitNormal reads a normalized float vector
func (r *reader) read3BitNormal() []float32 {
	ret := []float32{0.0, 0.0, 0.0}

	hasX := r.readBoolean()
	haxY := r.readBoolean()

	if hasX {
		ret[0] = r.readNormal()
	}

	if haxY {
		ret[1] = r.readNormal()
	}

	negZ := r.readBoolean()
	prodsum := ret[0]*ret[0] + ret[1]*ret[1]

	if prodsum < 1.0 {
		ret[2] = float32(math.Sqrt(float64(1.0 - prodsum)))
	} else {
		ret[2] = 0.0
	}

	if negZ {
		ret[2] = -ret[2]
	}

	return ret
}

// readBitsAsBytes reads the given number of bits in groups of bytes
func (r *reader) readBitsAsBytes(n uint32) []byte {
	tmp := make([]byte, 0)
	for n >= 8 {
		tmp = append(tmp, r.readByte())
		n -= 8
	}
	if n > 0 {
		tmp = append(tmp, byte(r.readBits(n)))
	}
	return tmp
}

// manta@v1.4.7/lzss.go

func unlzss(buf []byte) ([]byte, error) {
	r := newReader(buf)

	if s := r.readStringN(4); s != "LZSS" {
		return nil, fmt.Errorf("expected LZSS header, got %s", s)
	}

	size := int(r.readLeUint32())
	out := make([]byte, 0)

	var cmdByte, getCmdByte byte

	for {
		if getCmdByte == 0 {
			cmdByte = r.readByte()
		}

		getCmdByte = (getCmdByte + 1) & 0x07

		if (cmdByte & 0x01) != 0 {
			a := r.readByte()
			b := r.readByte()

			position := (int(a) << 4) | (int(b) >> 4)
			count := int((b & 0x0F) + 1)
			if count == 1 {
				break
			}
			source := len(out) - int(position) - 1
			for i := 0; i < count; i++ {
				out = append(out, out[source+i])
			}
		} else {
			out = append(out, r.readByte())
		}
		cmdByte = cmdByte >> 1
	}

	if len(out) != size {
		return nil, fmt.Errorf("expected %d bytes, got %d", size, len(out))
	}

	return out, nil
}

// func (p *manta.Parser) GetStringTables() *manta.stringTables { return p.stringTables }

func creepTypeFromTargetName(targetName string) string {
	targetName = strings.ToLower(strings.TrimSpace(targetName))
	if targetName == "" {
		return ""
	}
	if strings.HasPrefix(targetName, "npc_dota_neutral_") {
		return "jungle"
	}
	if strings.HasPrefix(targetName, "npc_dota_creep_goodguys_") || strings.HasPrefix(targetName, "npc_dota_creep_badguys_") {
		return "lane"
	}
	if strings.HasPrefix(targetName, "npc_dota_creep_siege") {
		return "lane"
	}
	return ""
}

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
	// f, err := os.Open("../replay1.dem")
	f, err := os.Open("/Users/igortsykalo/workspace/dota2/dota-web/storage/replays/8676648471.dem")
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

	nextIndex := int32(0)
	stringTables := &stringTables{
		Tables:    make(map[int32]*stringTable),
		NameIndex: make(map[string]int32),
		nextIndex: 0,
	}
	p.Callbacks.OnCSVCMsg_CreateStringTable(func(m *dota.CSVCMsg_CreateStringTable) error {
		// log.Printf("CreateStringTable: %s", m.String())

		t := &stringTable{
			index:             nextIndex,
			name:              m.GetName(),
			Items:             make(map[int32]*stringTableItem),
			userDataFixedSize: m.GetUserDataFixedSize(),
			userDataSizeBits:  m.GetUserDataSizeBits(),
			flags:             m.GetFlags(),
			varintBitCounts:   m.GetUsingVarintBitcounts(),
		}

		// Increment the index
		nextIndex += 1

		// Decompress the data if necessary
		buf := m.GetStringData()
		if m.GetDataCompressed() {
			// old replays = lzss
			// new replays = snappy

			r := newReader(buf)
			var err error

			if s := r.readStringN(4); s != "LZSS" {
				if buf, err = snappy.Decode(nil, buf); err != nil {
					return err
				}
			} else {
				if buf, err = unlzss(buf); err != nil {
					return err
				}
			}
		}

		// Parse the items out of the string table data
		items := parseStringTable(buf, m.GetNumEntries(), t.name, t.userDataFixedSize, t.userDataSizeBits, t.flags, t.varintBitCounts)

		// Insert the items into the table
		for _, item := range items {
			t.Items[item.Index] = item
			log.Printf("String Table created: %s, Index: %d, Key: %s, Value: %s", t.name, item.Index, item.Key, string(item.Value))
		}

		stringTables.Tables[t.index] = t
		stringTables.NameIndex[t.name] = t.index

		return nil
	})

	p.Callbacks.OnCSVCMsg_UpdateStringTable(func(m *dota.CSVCMsg_UpdateStringTable) error {
		t, ok := stringTables.Tables[m.GetTableId()]
		if !ok {
			// _panicf("missing string table %d", m.GetTableId())
			log.Printf("missing string table %d", m.GetTableId())
			return nil
		}

		// if v(5) {
		// 	// _debugf("tick=%d name=%s changedEntries=%d size=%d", p.Tick, t.name, m.GetNumChangedEntries(), len(m.GetStringData()))
		// 	log.Printf("tick=%d name=%s changedEntries=%d size=%d", p.Tick, t.name, m.GetNumChangedEntries(), len(m.GetStringData()))
		// }

		// Parse the updates out of the string table data
		items := parseStringTable(m.GetStringData(), m.GetNumChangedEntries(), t.name, t.userDataFixedSize, t.userDataSizeBits, t.flags, t.varintBitCounts)

		// Apply the updates to the parser state
		for _, item := range items {
			index := item.Index
			if _, ok := t.Items[index]; ok {
				if item.Key != "" && item.Key != t.Items[index].Key {
					t.Items[index].Key = item.Key
				}
				if len(item.Value) > 0 {
					t.Items[index].Value = item.Value
				}
			} else {
				t.Items[index] = item
			}
		}

		return nil
	})

	// map entityId -> [health, name]
	entityIdToHealthName := make(map[uint64]map[string]interface{}, 0)
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		return nil
		if e == nil {
			return nil
		}

		className := e.GetClassName()

		if className != "CDOTA_BaseNPC_Creep_Lane" {
			return nil
		}

		// entityId := e.GetIndex()
		entityId2, _ := e.GetUint64("m_nEntityId")
		// log.Printf("Entity: %s, EntityId: %d, EntityId2: %d", className, entityId, entityId2)
		health, _ := e.GetInt32("m_iHealth")
		nameIndex, _ := e.GetInt32("m_pEntity.m_nameStringableIndex")
		nameIndex2, _ := e.GetInt32("m_iUnitNameIndex")
		name, ok := p.LookupStringByIndex("EntityNames", nameIndex)
		name2, ok2 := p.LookupStringByIndex("EntityNames", nameIndex2)

		// all string tables
		// for k, v := range p.GetStringTables() {
		// 	log.Printf("String Table: %s, Index: %d", k, v.GetIndex())
		// }

		if !ok && !ok2 {
			// log.Printf("Failed to lookup name for entityId: %d", entityId2)
			return nil
		}

		log.Printf("Name: %s, Name2: %s, className: %s, index1: %d, index2: %d", name, name2, className, nameIndex, nameIndex2)

		if !strings.HasPrefix(name, "npc_dota_creep_") {
			return nil
		}

		if rand.Intn(2) == 0 {
			os.Exit(1)
		}

		entityIdToHealthName[entityId2] = map[string]interface{}{
			"health": health,
			"name":   name,
		}
		return nil
	})

	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		return nil
		attackerNameIdx := m.GetAttackerName()
		targetNameIdx := m.GetTargetName()
		realAttackerName, okA := p.LookupStringByIndex("CombatLogNames", int32(attackerNameIdx))
		if !okA {
			return nil
		}
		realTargetName, okT := p.LookupStringByIndex("CombatLogNames", int32(targetNameIdx))
		if !okT {
			return nil
		}

		ctype := m.GetType()

		if ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DAMAGE {
			if creepTypeFromTargetName(realTargetName) == "" {
				return nil
			}

			entityId := uint64(0)
			for k, v := range entityIdToHealthName {
				vHealth, ok := v["health"].(int32)
				if !ok {
					log.Printf("Failed to get health for entityId: %d", k)
					continue
				}
				if vHealth == m.GetHealth() {
					entityId = k
					break
				}
			}

			if entityId == 0 {
				log.Printf("Failed to find entityId for damage: %s -> %s (timestamp: %f, health: %d, value: %d)", realAttackerName, realTargetName, m.GetTimestamp(), m.GetHealth(), m.GetValue())
				for k, v := range entityIdToHealthName {
					log.Printf("EntityId: %d, Health: %d, Name: %s", k, v["health"], v["name"])
				}
				return nil
			}

			log.Printf("Damage: %s -> %s (timestamp: %f, health: %d, value: %d) = %d", realAttackerName, realTargetName, m.GetTimestamp(), m.GetHealth(), m.GetValue(), entityId)
		}

		if ctype == dota.DOTA_COMBATLOG_TYPES_DOTA_COMBATLOG_DEATH {
			if creepTypeFromTargetName(realTargetName) == "" {
				return nil
			}

			log.Printf("Death: %s -> %s (timestamp: %f, health: %d, value: %d)", realAttackerName, realTargetName, m.GetTimestamp(), m.GetHealth(), m.GetValue())
		}

		return nil
	})

	// IMPORTANT: actually check Start() error
	if err := p.Start(); err != nil && err != io.EOF {
		log.Fatalf("parse error: %v", err)
	}
	log.Printf("Parse Complete!")

	// Save all string tables to a file
	for _, t := range stringTables.Tables {
		data, err := json.Marshal(t)
		if err != nil {
			log.Printf("Failed to marshal string table %s: %v", t.name, err)
			continue
		}
		os.WriteFile(fmt.Sprintf("string_table_%s.json", t.name), data, 0644)
	}
}
