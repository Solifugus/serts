package sim

import "testing"

func TestNamesLookLikeNames(t *testing.T) {
	s := newTestSim(5)
	seen := map[string]bool{}
	for i := 0; i < 24 && i < len(s.Chars); i++ {
		n := s.FullName(CharID(i))
		t.Log(n)
		seen[n] = true
	}
	if len(seen) < 20 {
		t.Errorf("only %d distinct names in 24 people; the generator is collapsing", len(seen))
	}
	// A name that costs an allocation per tick would defeat the point of storing seeds.
	if testing.AllocsPerRun(100, func() { _ = GivenName(12345) }) > 4 {
		t.Error("name generation allocates more than expected for a display-only path")
	}
}
