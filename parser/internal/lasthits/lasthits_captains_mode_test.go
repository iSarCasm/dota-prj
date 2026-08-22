package lasthits

import "testing"

func TestReplay8941817475_SlarkGameplayStarted(t *testing.T) {
	h := parseReplayHero(t, "8941817475", "Slark", toTimestamp(11, 0))

	if !h.timeAndPausesHandler.IsGameplayStarted() {
		t.Fatal("expected isGameplayStarted after Captain's Mode draft")
	}
	if h.timeAndPausesHandler.PreGameStartTime() == 0 {
		t.Fatal("expected preGameStartTime calibrated")
	}
	if h.lastHitsLane == 0 && len(h.events) == 0 {
		t.Fatalf("expected last-hit events after calibration, got lane=%d events=%d", h.lastHitsLane, len(h.events))
	}
}
