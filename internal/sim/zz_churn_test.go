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
	"sort"
	"testing"
)

// Is the job churn seasonal work, or is it thrash?
//
// The diaries record villagers changing work thousands of times in a life — one man 7,753
// times across fourteen years. The generous reading, and a historically real one, is that
// this is a pre-industrial workforce rotating with the calendar: the farm at harvest,
// the quarry in winter, back again in spring. If so the churn is not a fault at all.
//
// That reading makes predictions sharp enough to falsify:
//
//   - Seasonal rotation clusters in time. Switches should pile up around harvest and
//     planting and go quiet between. Thrash is uniform across the year.
//   - Seasonal spells last a season. A rotation means months of work; the median spell
//     should be measured in weeks at least.
//   - Rotation moves between DIFFERENT trades. Leaving a post and retaking the same post
//     is not a rotation under any reading.
//
// Nothing here is asserted. It reports the three numbers and lets them decide.
//
// The simulation is stepped a tick at a time and employment sampled directly, so the
// measurement needs no instrumentation inside the simulation itself and cannot perturb
// what it measures.
func TestJobChurnIsSeasonalOrNot(t *testing.T) {
	const years = 5

	s := newTestSim(5)

	type spell struct {
		job   StructID
		start Tick
	}
	current := make(map[CharID]spell)

	var (
		switches   int
		sameJob    int // left a post and took the very same one again
		sameType   int // moved between posts of the same trade
		durations  []float64
		byDay      [DaysPerYear]int
		personYear float64
	)
	last := make(map[CharID]StructID)

	for tick := 0; tick < years*TicksPerYear; tick++ {
		s.RunTicks(1)
		if tick%TicksPerHour != 0 {
			continue // sampling hourly is finer than any real spell and 100x cheaper
		}
		for i := range s.Chars {
			c := &s.Chars[i]
			id := CharID(i)
			if !c.Alive || c.Stage() == Child {
				continue
			}
			personYear += float64(TicksPerHour) / float64(TicksPerYear)
			if c.Job == NoStruct {
				continue
			}
			prev, had := current[id]
			if had && prev.job == c.Job {
				continue
			}
			if had {
				switches++
				durations = append(durations, float64(s.Tick-prev.start)/float64(TicksPerYear))
				byDay[s.Tick.Date().Day]++
				if l, ok := last[id]; ok && l == c.Job {
					sameJob++
				}
				if s.Structs[prev.job].Type == s.Structs[c.Job].Type {
					sameType++
				}
				last[id] = prev.job
			}
			current[id] = spell{job: c.Job, start: s.Tick}
		}
	}

	sort.Float64s(durations)
	median, p90 := 0.0, 0.0
	if len(durations) > 0 {
		median = durations[len(durations)/2]
		p90 = durations[len(durations)*9/10]
	}

	// Seasonality: compare the busiest month of switching against the quietest. A calendar
	// rotation should show a large ratio; uniform thrash sits near one.
	var month [12]int
	for d, n := range byDay {
		month[d*12/DaysPerYear] += n
	}
	lo, hi := month[0], month[0]
	for _, m := range month {
		if m < lo {
			lo = m
		}
		if m > hi {
			hi = m
		}
	}
	ratio := 0.0
	if lo > 0 {
		ratio = float64(hi) / float64(lo)
	}

	w := os.Stderr
	fmt.Fprintf(w, "\n=== job churn over %d years ===\n", years)
	fmt.Fprintf(w, "switches observed:        %d\n", switches)
	fmt.Fprintf(w, "adult person-years:       %.0f\n", personYear)
	if personYear > 0 {
		fmt.Fprintf(w, "switches per person-year: %.1f\n", float64(switches)/personYear)
	}
	fmt.Fprintf(w, "\nspell length (in-world days)\n")
	fmt.Fprintf(w, "  median: %.2f\n  90th:   %.2f\n", median*DaysPerYear, p90*DaysPerYear)
	fmt.Fprintf(w, "\nwhere they went\n")
	if switches > 0 {
		fmt.Fprintf(w, "  straight back to the post just left: %.1f%%\n", 100*float64(sameJob)/float64(switches))
		fmt.Fprintf(w, "  to another post of the same trade:   %.1f%%\n", 100*float64(sameType)/float64(switches))
	}
	fmt.Fprintf(w, "\nseasonality\n")
	fmt.Fprintf(w, "  switches by month: %v\n", month)
	fmt.Fprintf(w, "  busiest:quietest = %.1f:1\n", ratio)
	fmt.Fprintf(w, "\nA calendar rotation predicts a high month ratio, spells of weeks or\n")
	fmt.Fprintf(w, "months, and few same-post returns. Thrash predicts the opposite of each.\n")
}
