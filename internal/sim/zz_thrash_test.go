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
	"math"
	"os"
	"sort"
	"testing"
)

// Which term of the job score is actually flipping?
//
// A switch requires bestScore >= currentScore * gain, where gain is 1.2 to 2.1 depending
// on Ambition. So every switch is a claim that some job became at least twenty per cent
// better than the one held. Sixty-nine such claims per person-year, with a median tenure
// of one day, is not a labour market responding to anything; something in the comparison
// is moving that should not be.
//
// The score is wage * skillFit * distance * urgency * policy * safety. Urgency depends
// only on the character, so it cancels in the ratio and cannot cause a switch. That
// leaves five candidates, and this decomposes real switches into them: for each one, how
// much did each factor contribute to the new job winning?
//
// Reported as geometric means, because the terms multiply. A factor with a geometric mean
// near 1.0 is not driving switches whatever its variance; the one that drives them will
// sit well above 1.
func TestWhichTermDrivesJobSwitching(t *testing.T) {
	const years = 3
	s := newTestSim(5)
	s.RunTicks(TicksPerYear) // past the founding transient

	type factors struct{ wage, skill, dist, safety, policy float64 }

	// components recomputes the score's parts for one character and one post.
	components := func(id CharID, sid StructID) factors {
		c := &s.Chars[id]
		st := &s.Structs[sid]
		f := factors{wage: float64(st.Wage), skill: efficiency(c.Skill[st.Type]), policy: s.PolicyWeight(st.Type)}
		if sub := s.SubsistenceWageAt(st.Pos); sub > 0 {
			if afford := f.wage / float64(sub); afford < 1 {
				f.wage *= afford * afford
			}
		}
		d := s.T.Dist(c.Pos, st.Pos)
		f.dist = 1 / (1 + d*float64(c.Traits.Rootedness)/12)
		f.safety = 1.0
		if dg := Danger[st.Type]; dg > 0 {
			f.safety = 1 / (1 + dg*DangerAversion*float64(c.Traits.Caution))
		}
		return f
	}

	held := make(map[CharID]StructID)
	prev := make(map[CharID]StructID)
	for i := range s.Chars {
		held[CharID(i)] = s.Chars[i].Job
	}

	var lw, ls, ld, lsf, lp []float64 // log-ratios, so the mean is geometric
	switches, sameStruct, sameType, distDrove := 0, 0, 0, 0
	var sample []string

	for tick := 0; tick < years*TicksPerYear; tick++ {
		s.RunTicks(1)
		for i := range s.Chars {
			c := &s.Chars[i]
			id := CharID(i)
			if !c.Alive || c.Job == NoStruct {
				continue
			}
			was, ok := held[id]
			if !ok || was == c.Job {
				held[id] = c.Job
				continue
			}
			held[id] = c.Job
			if was == NoStruct {
				continue // took work while unemployed; not a switch
			}
			switches++
			if p, ok := prev[id]; ok && p == c.Job {
				sameStruct++
			}
			prev[id] = was
			if s.Structs[was].Type == s.Structs[c.Job].Type {
				sameType++
			}

			old, new := components(id, was), components(id, c.Job)
			ratio := func(a, b float64) float64 {
				if a <= 0 || b <= 0 {
					return math.NaN()
				}
				return math.Log(b / a)
			}
			w, sk, d, sf, p := ratio(old.wage, new.wage), ratio(old.skill, new.skill),
				ratio(old.dist, new.dist), ratio(old.safety, new.safety), ratio(old.policy, new.policy)
			if math.IsNaN(w) || math.IsNaN(sk) || math.IsNaN(d) || math.IsNaN(sf) || math.IsNaN(p) {
				continue
			}
			lw, ls, ld, lsf, lp = append(lw, w), append(ls, sk), append(ld, d), append(lsf, sf), append(lp, p)
			// Which single factor contributed most to the new job winning?
			if d > w && d > sk && d > sf && d > p {
				distDrove++
			}
			if len(sample) < 12 {
				sample = append(sample, fmt.Sprintf(
					"  %-22s -> %-22s  wage x%.2f skill x%.2f dist x%.2f safety x%.2f policy x%.2f",
					Defs[s.Structs[was].Type].Name, Defs[s.Structs[c.Job].Type].Name,
					math.Exp(w), math.Exp(sk), math.Exp(d), math.Exp(sf), math.Exp(p)))
			}
		}
	}

	geo := func(v []float64) float64 {
		if len(v) == 0 {
			return math.NaN()
		}
		sum := 0.0
		for _, x := range v {
			sum += x
		}
		return math.Exp(sum / float64(len(v)))
	}
	// The share of switches in which a factor was above 1 at all.
	share := func(v []float64) float64 {
		n := 0
		for _, x := range v {
			if x > 0 {
				n++
			}
		}
		return 100 * float64(n) / float64(len(v))
	}

	w := os.Stderr
	fmt.Fprintf(w, "\n=== what drives a job switch (%d years, %d switches decomposed) ===\n", years, len(lw))
	fmt.Fprintf(w, "%-10s %12s %14s\n", "factor", "geo-mean", "% above 1")
	for _, f := range []struct {
		name string
		v    []float64
	}{{"wage", lw}, {"skill", ls}, {"distance", ld}, {"safety", lsf}, {"policy", lp}} {
		fmt.Fprintf(w, "%-10s %12.3f %13.0f%%\n", f.name, geo(f.v), share(f.v))
	}
	fmt.Fprintf(w, "\nswitches:              %d\n", switches)
	if switches > 0 {
		fmt.Fprintf(w, "back to the post just left: %.1f%%\n", 100*float64(sameStruct)/float64(switches))
		fmt.Fprintf(w, "same trade:                 %.1f%%\n", 100*float64(sameType)/float64(switches))
	}
	if len(lw) > 0 {
		fmt.Fprintf(w, "distance was the largest factor in %.1f%% of switches\n",
			100*float64(distDrove)/float64(len(lw)))
	}
	fmt.Fprintf(w, "\nsample switches:\n")
	for _, l := range sample {
		fmt.Fprintln(w, l)
	}

	// Distance is computed from where the character is standing at the moment of the
	// decision, and they walk. If that is the driver, the same person re-decides
	// differently as they move — so report how far people are from their own workplace
	// when they reconsider.
	var atWork, away int
	var dists []float64
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Job == NoStruct {
			continue
		}
		d := s.T.Dist(c.Pos, s.Structs[c.Job].Pos)
		dists = append(dists, d)
		if d < 1.5 {
			atWork++
		} else {
			away++
		}
	}
	sort.Float64s(dists)
	fmt.Fprintf(w, "\nemployed people standing at their workplace: %d; elsewhere: %d\n", atWork, away)
	if len(dists) > 0 {
		fmt.Fprintf(w, "distance from own workplace: median %.1f, 90th %.1f cells\n",
			dists[len(dists)/2], dists[len(dists)*9/10])
	}
}

// How far below subsistence do employers actually sit?
//
// The score discounts a wage that cannot feed you by afford squared, so the effective
// wage is cubic in the wage itself: a post paying one per cent of subsistence scores at
// one millionth of its wage. A switch is a ratio test — best >= current * gain — and a
// ratio test against approximately zero is not a test at all. This measures whether the
// population of employers actually reaches those depths.
func TestHowFarBelowSubsistenceEmployersPay(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(3 * TicksPerYear)

	type row struct {
		name   string
		afford float64
		eff    float64
	}
	var rows []row
	buckets := map[string]int{}
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Jobs == 0 || st.Wage <= 0 {
			continue
		}
		sub := float64(s.SubsistenceWageAt(st.Pos))
		if sub <= 0 {
			continue
		}
		afford := float64(st.Wage) / sub
		eff := float64(st.Wage)
		if afford < 1 {
			eff *= afford * afford
		}
		rows = append(rows, row{Defs[st.Type].Name, afford, eff})
		switch {
		case afford >= 1:
			buckets["at or above subsistence"]++
		case afford >= 0.5:
			buckets["half to full"]++
		case afford >= 0.1:
			buckets["a tenth to half"]++
		case afford >= 0.01:
			buckets["1% to 10%"]++
		default:
			buckets["below 1% of subsistence"]++
		}
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].afford < rows[b].afford })

	w := os.Stderr
	fmt.Fprintf(w, "\n=== employer wages against local subsistence (%d posts) ===\n", len(rows))
	for _, k := range []string{"at or above subsistence", "half to full", "a tenth to half",
		"1% to 10%", "below 1% of subsistence"} {
		fmt.Fprintf(w, "  %-26s %d\n", k, buckets[k])
	}
	fmt.Fprintf(w, "\nthe ten worst-paying posts:\n")
	for i, r := range rows {
		if i >= 10 {
			break
		}
		fmt.Fprintf(w, "  %-14s pays %.4f of subsistence -> effective score weight %.3g\n",
			r.name, r.afford, r.eff)
	}
	if len(rows) > 0 {
		fmt.Fprintf(w, "\nA ratio test against the smallest of these is cleared by anything at all,\n")
		fmt.Fprintf(w, "which is why JobSwitchGain does not bite.\n")
	}
}

// How much of the churn is employers failing payroll?
//
// An employer that cannot pay this tick turns its workers out (character.go). That rule is
// load-bearing — note 14 records that wage arrears were tried instead and cost 0.08 of R0
// — but nothing stops a broke employer from advertising the vacancy it just created, so a
// worker can be dismissed and rehired by the same failing post repeatedly. This counts how
// often employers are insolvent at the moment of payroll, and how often a post is
// simultaneously unpayable and open.
func TestHowOftenEmployersMissPayroll(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(TicksPerYear)

	const days = 120
	var samples, insolvent, insolventAndHiring, staffed int
	perStruct := map[StructID]int{}

	for d := 0; d < days; d++ {
		s.RunTicks(TicksPerDay)
		for i := range s.Structs {
			st := &s.Structs[i]
			if !st.Alive || st.Jobs == 0 || st.Wage <= 0 {
				continue
			}
			samples++
			if st.Filled > 0 {
				staffed++
			}
			if st.Gold < st.Wage {
				insolvent++
				perStruct[StructID(i)]++
				if st.Openings() > 0 {
					insolventAndHiring++
				}
			}
		}
	}

	type row struct {
		id StructID
		n  int
	}
	var worst []row
	for id, n := range perStruct {
		worst = append(worst, row{id, n})
	}
	sort.Slice(worst, func(a, b int) bool {
		if worst[a].n != worst[b].n {
			return worst[a].n > worst[b].n
		}
		return worst[a].id < worst[b].id // deterministic
	})

	w := os.Stderr
	fmt.Fprintf(w, "\n=== employer solvency at payroll, %d days ===\n", days)
	fmt.Fprintf(w, "employer-days sampled:        %d\n", samples)
	fmt.Fprintf(w, "  cannot make payroll:        %d  (%.1f%%)\n", insolvent, 100*float64(insolvent)/float64(samples))
	fmt.Fprintf(w, "  cannot pay AND advertising: %d  (%.1f%%)\n", insolventAndHiring, 100*float64(insolventAndHiring)/float64(samples))
	fmt.Fprintf(w, "\nA post that is unpayable and open at the same time dismisses its worker\n")
	fmt.Fprintf(w, "and immediately invites them back, which is a loop with no exit.\n")
	fmt.Fprintf(w, "\nworst offenders:\n")
	for i, r := range worst {
		if i >= 8 {
			break
		}
		st := &s.Structs[r.id]
		fmt.Fprintf(w, "  %-14s (%d) insolvent on %d of %d days, wage %.4f, gold %.2f\n",
			Defs[st.Type].Name, r.id, r.n, days, st.Wage, st.Gold)
	}
}

// Which path actually produces the job churn?
//
// Two plausible diagnoses have already failed: distance (geometric mean 0.922, pulling the
// other way) and employers failing payroll (1.4% of employer-days). Rather than reason
// about a third, this counts every quit by its cause.
func TestWhyPeopleLeaveTheirJobs(t *testing.T) {
	const years = 5
	s := newTestSim(5)
	s.RunTicks(years * TicksPerYear)

	adults := 0
	for i := range s.Chars {
		if s.Chars[i].Alive && s.Chars[i].Stage() != Child {
			adults++
		}
	}

	total := 0
	for _, n := range s.Quits {
		total += n
	}

	w := os.Stderr
	fmt.Fprintf(w, "\n=== why people left their jobs (%d years, %d adults at the end) ===\n", years, adults)
	fmt.Fprintf(w, "%-18s %10s %8s\n", "reason", "count", "share")
	for r := QuitReason(0); r < NumQuitReasons; r++ {
		share := 0.0
		if total > 0 {
			share = 100 * float64(s.Quits[r]) / float64(total)
		}
		fmt.Fprintf(w, "%-18s %10d %7.1f%%\n", r.String(), s.Quits[r], share)
	}
	fmt.Fprintf(w, "%-18s %10d\n", "TOTAL", total)
}

// How long does an employer stay unable to pay?
//
// 54% of all quits are dismissals for non-payment, and payroll is tested every working
// tick. That makes the question of magnitude a question of DURATION: an employer broke for
// a week is genuinely failing and turning its staff out is right, while one broke for a
// tick is experiencing cash-flow jitter and a hair trigger is converting that jitter into
// permanent churn.
//
// Sampled per tick, because the earlier version of this sampled per day and got an answer
// that pointed away from the truth — payroll is a per-tick event and a daily snapshot
// cannot see it.
func TestHowLongEmployersStayInsolvent(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(2 * TicksPerYear)

	const days = 40
	broke := map[StructID]int{} // ticks insolvent so far
	var spells []float64
	tickSamples, insolventSamples := 0, 0

	for i := 0; i < days*TicksPerDay; i++ {
		s.RunTicks(1)
		if !s.Tick.IsWorkTime() {
			continue // payroll only runs in working hours
		}
		for j := range s.Structs {
			st := &s.Structs[j]
			if !st.Alive || st.Jobs == 0 || st.Wage <= 0 || st.Filled == 0 {
				continue
			}
			tickSamples++
			id := StructID(j)
			if st.Gold < st.Wage {
				insolventSamples++
				broke[id]++
				continue
			}
			if n, ok := broke[id]; ok {
				spells = append(spells, float64(n))
				delete(broke, id)
			}
		}
	}
	sort.Float64s(spells)

	w := os.Stderr
	fmt.Fprintf(w, "\n=== how long employers cannot pay (%d days, per working tick) ===\n", days)
	if tickSamples > 0 {
		fmt.Fprintf(w, "staffed employer-ticks sampled: %d\n", tickSamples)
		fmt.Fprintf(w, "  of which unable to pay:       %d  (%.2f%%)\n",
			insolventSamples, 100*float64(insolventSamples)/float64(tickSamples))
	}
	if len(spells) > 0 {
		fmt.Fprintf(w, "\ninsolvency spells: %d\n", len(spells))
		fmt.Fprintf(w, "  median  %.0f ticks (%.1f working minutes)\n",
			spells[len(spells)/2], spells[len(spells)/2]*MinutesPerTick)
		fmt.Fprintf(w, "  90th    %.0f ticks\n", spells[len(spells)*9/10])
		fmt.Fprintf(w, "  worst   %.0f ticks\n", spells[len(spells)-1])
		under := 0
		for _, v := range spells {
			if v <= TicksPerHour {
				under++
			}
		}
		fmt.Fprintf(w, "  resolved within one in-world hour: %.1f%%\n", 100*float64(under)/float64(len(spells)))
	}
	fmt.Fprintf(w, "\nEvery tick of insolvency turns the staff out. If these spells are short,\n")
	fmt.Fprintf(w, "the dismissal rule is not shedding failing employers -- it is firing\n")
	fmt.Fprintf(w, "people over a momentary empty till.\n")
}
