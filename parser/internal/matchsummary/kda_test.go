package matchsummary

import (
	"testing"

	"github.com/dotabuff/manta/dota"
)

func TestIsAssister(t *testing.T) {
	h := &Handler{playerID: 16, hasPlayerID: true}

	m := &dota.CMsgDOTACombatLogEntry{}
	assist := int32(8)
	m.AssistPlayers = []int32{3, assist, 0}
	if !h.isAssister(m) {
		t.Fatal("expected assist index 8 for playerID 16")
	}

	m2 := &dota.CMsgDOTACombatLogEntry{}
	ap := uint32(8)
	m2.AssistPlayer1 = &ap
	if !h.isAssister(m2) {
		t.Fatal("expected assist index 8 in AssistPlayer1")
	}

	m3 := &dota.CMsgDOTACombatLogEntry{}
	m3.AssistPlayers = []int32{2, 4}
	if h.isAssister(m3) {
		t.Fatal("did not expect assist")
	}
}
