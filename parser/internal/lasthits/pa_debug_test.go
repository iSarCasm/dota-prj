package lasthits

import (
	"os"
	"testing"

	"github.com/dotabuff/manta"
	"github.com/dotabuff/manta/dota"

	"dota2/internal/common"
	"dota2/internal/timeandpauses"
)

func TestPA_DebugTick11061(t *testing.T) {
	if os.Getenv("DOTA_PA_DEBUG") == "" {
		t.Skip("set DOTA_PA_DEBUG=1")
	}
	f, err := os.Open(replayPath(t, "8934466456"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p, err := manta.NewStreamParser(f)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &common.ParseContext{HeroName: "Phantom Assassin"}
	tp := timeandpauses.NewHandler()
	_ = tp.Init(ctx)
	h := NewHandler(tp)
	_ = h.Init(ctx)
	tp.RegisterCallbacks(p, ctx)

	const wantTick uint32 = 11061
	p.Callbacks.OnCMsgDOTACombatLogEntry(func(m *dota.CMsgDOTACombatLogEntry) error {
		if p.Tick != wantTick {
			return nil
		}
		target, _ := p.LookupStringByIndex("CombatLogNames", int32(m.GetTargetName()))
		if target != "npc_dota_creep_goodguys_melee" {
			return nil
		}
		attacker, _ := p.LookupStringByIndex("CombatLogNames", int32(m.GetAttackerName()))
		t.Logf("COMBAT type=%v attacker=%s inflictor=%d health=%d value=%d",
			m.GetType(), attacker, m.GetInflictorName(), m.GetHealth(), m.GetValue())
		return nil
	})
	p.OnEntity(func(e *manta.Entity, op manta.EntityOp) error {
		if e.GetIndex() != 1536 || p.Tick < 11031 || p.Tick > wantTick {
			return nil
		}
		health, _ := e.GetInt32("m_iHealth")
		t.Logf("tick=%d ENTITY idx=1536 health=%d op=%s", p.Tick, health, op)
		return nil
	})
	h.RegisterCallbacks(p, ctx)
	_ = p.Start()

	t.Logf("heroClass=%q missedEvents=%+v", h.heroClass, h.missedEvents)
	for i, pd := range h.pendingHeroDamageLogs {
		t.Logf("pendingHeroDamage[%d] tick=%d matched=%v closed=%v candidates=%v creep=%q h=%d d=%d",
			i, pd.tick, pd.entityMatched, pd.closed, pd.candidates, pd.creepName, pd.health, pd.damage)
	}
	for i, pd := range h.pendingHeroKillLogs {
		if pd.creepName == "npc_dota_creep_goodguys_melee" {
			t.Logf("pendingHeroKill[%d] tick=%d matched=%v", i, pd.tick, pd.entityMatched)
		}
	}
	for i, pd := range h.pendingOtherKillLogs {
		if pd.creepName == "npc_dota_creep_goodguys_melee" {
			t.Logf("pendingOtherKill[%d] tick=%d matched=%v", i, pd.tick, pd.entityMatched)
		}
	}
}
