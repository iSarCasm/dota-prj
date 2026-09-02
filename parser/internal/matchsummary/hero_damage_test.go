package matchsummary

import "testing"

func TestDamageTypeLabel(t *testing.T) {
	tests := map[uint32]string{
		1: "physical",
		2: "magical",
		4: "pure",
		0: "unknown",
		3: "unknown",
	}
	for in, want := range tests {
		if got := damageTypeLabel(in); got != want {
			t.Fatalf("damageTypeLabel(%d) = %q, want %q", in, got, want)
		}
	}
}
