package sim

import "testing"

// The national accounts, year by year — the instrument that would have caught every
// hoard and leak of the last two days on the day it began.
func TestLedgerReport(t *testing.T) {
	s := newTestSim(5)
	for y := 0; y < 8; y++ {
		s.ResetLedger()
		s.RunTicks(TicksPerYear)
		t.Logf("y%d pop %d\n  %s", y+1, s.Population(), s.LedgerReport())
	}
}
