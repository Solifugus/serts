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

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

// The standing measurement for any change to how people find food.
//
// Food sourcing sits directly on the mechanism that the population recovery rests on, and
// note 14 lists four occasions when a rule that looked plainly wrong turned out to be
// holding something up. So changes here are measured against this, before and after, on
// the same seeds — never argued.
//
// Fifty years rather than a hundred: long enough for a village to pass through the
// founding transient and show whether it grows, short enough to run between edits. The
// hundred-year acceptance tests remain the authority.
//
// THE STANDING BASELINE, twelve seeds, fifty years:
//
//	population     1,248
//	total deaths   1,855
//	hunger deaths    554
//
// Taken after money moved to float64 and five money bugs were fixed. Every figure
// measured before that is void — 1,072/434 and everything compared against it — because
// they were taken on an economy where the ledger counted a mint that never happened and
// the richest characters could not physically receive a wage. The VERDICTS from those
// comparisons stand, none having been marginal, with one exception: clinics measured
// +11% at t = 1.30, which was never established and is now measured in a currency that
// no longer exists. That one is genuinely unknown.
//
// The between-seed spread is what bounds this instrument: 73 to 140 on identical code.
// Nothing smaller than that is visible without many seeds (note 16).
func TestFoodSourcingBaseline(t *testing.T) {
	seeds := []int64{5, 7, 108, 209, 311, 407, 512, 613, 714, 815, 916, 1017}
	const years = 50

	type result struct {
		pop, deaths, hungerDeaths int
		avgHealth                 float32
	}
	out := make([]result, len(seeds))

	var wg sync.WaitGroup
	for i, seed := range seeds {
		wg.Add(1)
		go func(i int, seed int64) {
			defer wg.Done()
			s := newTestSim(seed)
			s.RunTicks(years * TicksPerYear)
			st := s.Stats()
			r := result{pop: s.Population(), avgHealth: st.AvgHealth}
			for _, l := range s.Lives {
				r.deaths++
				if l.Cause == CauseHunger {
					r.hungerDeaths++
				}
			}
			out[i] = r
		}(i, seed)
	}
	wg.Wait()

	w := os.Stderr
	fmt.Fprintf(w, "\n=== food sourcing, %d years ===\n", years)
	total, totalHunger := 0, 0
	for i, seed := range seeds {
		fmt.Fprintf(w, "seed %3d: pop %4d  health %.0f  deaths %4d of which hunger %4d\n",
			seed, out[i].pop, out[i].avgHealth, out[i].deaths, out[i].hungerDeaths)
		total += out[i].pop
		totalHunger += out[i].hungerDeaths
	}
	fmt.Fprintf(w, "TOTAL population %d, hunger deaths %d\n", total, totalHunger)
}
