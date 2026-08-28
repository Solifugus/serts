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
	"sync"
	"testing"
)

// Decompose the failure: is the village short of births, or short of survivors?
//
// The decisive quantity is R0, the net reproduction rate — daughters born per female birth
// who themselves survive to bear. Every birth credits both parents, so total Children
// across all lives is twice the number of births; with L lives born and half of births
// female, R0 works out as (mean children per life) / 2, taken over ALL lives including
// those who died as infants counted as zero. Vitals.ChildrenPerLife conditions on reaching
// 45 and so cannot see this.
//
// R0 >= 1 is growth. Anything less is extinction, however slow.
func TestDemographicDecomposition(t *testing.T) {
	seeds := []int64{5, 7, 108, 209}
	const years = 120
	pooled := make([][]Life, len(seeds))
	var wg sync.WaitGroup
	for i, seed := range seeds {
		wg.Add(1)
		go func(i int, seed int64) {
			defer wg.Done()
			s := newTestSim(seed)
			s.RunTicks(years * TicksPerYear)
			pooled[i] = s.Lives
		}(i, seed)
	}
	wg.Wait()

	var lives []Life
	for _, ls := range pooled {
		lives = append(lives, ls...)
	}

	var born, totalChildren int
	var reached [8]int // survival to 1,5,10,15,18,25,35,45
	bands := []float32{1, 5, 10, 15, 18, 25, 35, 45}
	byBandCause := map[string]map[DeathCause]int{}
	bandName := func(a float32) string {
		switch {
		case a < 1:
			return "00-01"
		case a < 5:
			return "01-05"
		case a < 10:
			return "05-10"
		case a < 15:
			return "10-15"
		case a < 25:
			return "15-25"
		case a < 45:
			return "25-45"
		case a < 56:
			return "45-56"
		}
		return "56+"
	}
	for _, l := range lives {
		if l.Settler {
			continue
		}
		born++
		totalChildren += l.Children
		for i, b := range bands {
			if l.Age >= b {
				reached[i]++
			}
		}
		bn := bandName(l.Age)
		if byBandCause[bn] == nil {
			byBandCause[bn] = map[DeathCause]int{}
		}
		byBandCause[bn][l.Cause]++
	}
	if born == 0 {
		t.Skip("no lives")
	}

	meanChildren := float64(totalChildren) / float64(born)
	t.Logf("pooled %d villages over %d years: %d lives born in the village", len(seeds), years, born)
	t.Logf("mean children ever born per life (all lives, infant deaths as zero): %.3f", meanChildren)
	t.Logf("NET REPRODUCTION RATE R0 = %.3f   (1.00 is replacement)", meanChildren/2)

	t.Log("survival:")
	for i, b := range bands {
		t.Logf("  to age %2.0f: %5.1f%%", b, 100*float64(reached[i])/float64(born))
	}

	t.Log("deaths by age band and cause:")
	for _, bn := range []string{"00-01", "01-05", "05-10", "10-15", "15-25", "25-45", "45-56", "56+"} {
		m := byBandCause[bn]
		if len(m) == 0 {
			continue
		}
		line, tot := "", 0
		for c := DeathCause(0); c < numCauses; c++ {
			if m[c] > 0 {
				line += fmt.Sprintf("%s %d  ", c, m[c])
				tot += m[c]
			}
		}
		t.Logf("  %s n=%3d  %s", bn, tot, line)
	}

	// How much of the fertile window is actually spent married?
	var everMarried, reachedAdult int
	var marriedAt, diedAt float32
	for _, l := range lives {
		if l.Settler || l.Age < AdultAge {
			continue
		}
		reachedAdult++
		if l.Married {
			everMarried++
			marriedAt += l.MarriedAt
			diedAt += l.Age
		}
	}
	if everMarried > 0 {
		t.Logf("of %d reaching adulthood, %d married (%.0f%%) at mean age %.1f, dying at mean %.1f: about %.1f married years",
			reachedAdult, everMarried, 100*float64(everMarried)/float64(reachedAdult),
			marriedAt/float32(everMarried), diedAt/float32(everMarried),
			(diedAt-marriedAt)/float32(everMarried))
	}
}
