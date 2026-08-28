//go:build probe

// A probe, not a test.
//
// This file measures rather than asserts: it runs the simulation for a long horizon and
// prints what it found, so that a question about the world can be answered with evidence.
// It is excluded from the default build because probes and tests want opposite things
// from a test suite. A test must be cheap enough to run on every change and must fail
// when something breaks; a probe is deliberately expensive and cannot fail, because it
// has no opinion about what the answer should be.
//
// Run one with:
//
//	go test -tags probe ./internal/sim -run TestName -timeout 3h -v
//
// See docs/method.md, note 15.

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
