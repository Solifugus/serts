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

// Is the payroll problem illiquidity or insolvency?
//
// The distinction decides whether credit would help at all. A business that earns more
// than it pays out across the year but runs dry between receipts is illiquid, and lending
// to it bridges a timing gap that is nobody's fault — which is what agricultural credit
// has always been for, since a farm is paid once at harvest and owes wages every day. A
// business that pays out more than it takes in is insolvent, and lending to it only
// converts a failure into a failure with debt attached.
//
// So: for every employer, the year's inflow, the year's wage bill, and the low-water mark
// of its till.
func TestIsPayrollFailureLiquidityOrInsolvency(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(2 * TicksPerYear)

	type acct struct {
		start, end, low, high float64
		paidOut, tookIn       float64
		dryTicks              int
		typ                   StructType
	}
	accts := map[StructID]*acct{}
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Jobs == 0 {
			continue
		}
		g := float64(st.Gold)
		accts[StructID(i)] = &acct{start: g, low: g, high: g, typ: st.Type}
	}

	prev := map[StructID]float64{}
	for id := range accts {
		prev[id] = float64(s.Structs[id].Gold)
	}

	for i := 0; i < TicksPerYear; i++ {
		s.RunTicks(1)
		for id, a := range accts {
			st := &s.Structs[id]
			if !st.Alive {
				continue
			}
			g := float64(st.Gold)
			if d := g - prev[id]; d > 0 {
				a.tookIn += d
			} else {
				a.paidOut += -d
			}
			prev[id] = g
			if g < a.low {
				a.low = g
			}
			if g > a.high {
				a.high = g
			}
			if st.Wage > 0 && g < float64(st.Wage) && st.Filled > 0 {
				a.dryTicks++
			}
		}
	}
	for id, a := range accts {
		a.end = float64(s.Structs[id].Gold)
	}

	type row struct {
		id StructID
		a  *acct
	}
	var rows []row
	for id, a := range accts {
		rows = append(rows, row{id, a})
	}
	sort.Slice(rows, func(x, y int) bool { return rows[x].a.dryTicks > rows[y].a.dryTicks })

	w := os.Stderr
	fmt.Fprintf(w, "\n=== employer cash flow over one year ===\n")
	fmt.Fprintf(w, "%-14s %9s %9s %9s %9s %9s %8s\n",
		"employer", "start", "end", "tookIn", "paidOut", "lowest", "dry%")
	liquid, insolvent := 0, 0
	for _, r := range rows {
		a := r.a
		net := a.end - a.start
		dry := 100 * float64(a.dryTicks) / float64(TicksPerYear)
		if a.dryTicks == 0 {
			continue
		}
		fmt.Fprintf(w, "%-14s %9.1f %9.1f %9.1f %9.1f %9.2f %7.1f%%\n",
			Defs[a.typ].Name, a.start, a.end, a.tookIn, a.paidOut, a.low, dry)
		if net >= 0 || a.tookIn >= a.paidOut {
			liquid++
		} else {
			insolvent++
		}
	}
	fmt.Fprintf(w, "\nof the employers that ran dry:\n")
	fmt.Fprintf(w, "  took in at least what they paid out (illiquid, not insolvent): %d\n", liquid)
	fmt.Fprintf(w, "  paid out more than they took in (genuinely failing):           %d\n", insolvent)
	fmt.Fprintf(w, "\nCredit bridges the first and merely postpones the second.\n")
}

// Is there idle capital that a lender could mobilise?
//
// Gold is conserved and the suite enforces it, so credit cannot be conjured: a lender must
// hold real reserves. The question is whether the coin exists and is merely sitting still
// while solvent businesses dismiss their staff for want of a few gold.
func TestWhereTheMoneySits(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(3 * TicksPerYear)

	var people, tills, treasury, works float64
	var richest float64
	var idleRich float64 // held by people with more than a year of subsistence
	yearOfFood := float64(s.Prices[Food]) * MealsPerDay * DaysPerYear

	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive {
			continue
		}
		people += float64(c.Gold)
		if float64(c.Gold) > richest {
			richest = float64(c.Gold)
		}
		if float64(c.Gold) > yearOfFood {
			idleRich += float64(c.Gold) - yearOfFood
		}
	}
	var dryShortfall float64
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive {
			continue
		}
		tills += float64(st.Gold)
		works += float64(st.Works)
		if st.Jobs > 0 && st.Wage > 0 && st.Filled > 0 && st.Gold < st.Wage {
			dryShortfall += float64(st.Wage - st.Gold)
		}
	}
	treasury = float64(s.Treasury)

	w := os.Stderr
	fmt.Fprintf(w, "\n=== where the money sits, year 3 ===\n")
	fmt.Fprintf(w, "  in people's purses:      %10.1f\n", people)
	fmt.Fprintf(w, "    of which above a year's food: %.1f\n", idleRich)
	fmt.Fprintf(w, "    richest single purse:         %.1f\n", richest)
	fmt.Fprintf(w, "  in employers' tills:     %10.1f\n", tills)
	fmt.Fprintf(w, "  in public works funds:   %10.1f\n", works)
	fmt.Fprintf(w, "  in the treasury:         %10.1f\n", treasury)
	fmt.Fprintf(w, "\n  a year's food for one person: %.1f\n", yearOfFood)
	fmt.Fprintf(w, "  total shortfall of every dry employer RIGHT NOW: %.4f\n", dryShortfall)
	fmt.Fprintf(w, "\nIf the shortfall is trivial beside the idle capital, the churn is not a\n")
	fmt.Fprintf(w, "shortage of money. It is a shortage of any means of moving it.\n")
}

// Do the hungry die for want of food, or for want of money?
//
// The answer decides whether more food-producing trades — hunting, gathering — would help
// at all. If the granaries are empty when people starve, the village is supply-limited and
// more hands producing food is exactly the fix. If the granaries are full and the dead had
// no coin, it is income-limited, and adding another trade only spreads the same wage bill
// across more jobs — which is how adding trades was measured to be fatal before
// (PolicyWeight, jobs.go).
func TestDoTheHungryDieBesideFood(t *testing.T) {
	s := newTestSim(5)

	type death struct {
		gold, stock, price, larder float32
		employed, hadRations       bool
	}
	var deaths []death
	s.onDeath = func(id CharID, cause DeathCause) {
		if cause != CauseHunger {
			return
		}
		c := &s.Chars[id]
		d := death{gold: float32(c.Gold), price: float32(s.FoodPriceAt(c.Pos)), employed: c.Job != NoStruct,
			hadRations: c.Rations > 0}
		if src := s.NearestFoodSource(c.Pos); src != NoStruct {
			d.stock = s.Structs[src].Stock[Food]
		}
		if c.Home != NoStruct {
			d.larder = s.Structs[c.Home].Stock[Food]
		}
		deaths = append(deaths, d)
	}
	s.RunTicks(25 * TicksPerYear)

	if len(deaths) == 0 {
		t.Skip("nobody starved")
	}
	var foodWasThere, couldAfford, employed, hadLarder int
	for _, d := range deaths {
		if d.stock >= FoodPerMeal {
			foodWasThere++
		}
		if d.price > 0 && d.gold >= d.price*FoodPerMeal {
			couldAfford++
		}
		if d.employed {
			employed++
		}
		if d.larder >= FoodPerMeal {
			hadLarder++
		}
	}
	n := float64(len(deaths))
	w := os.Stderr
	fmt.Fprintf(w, "\n=== %d hunger deaths over 25 years ===\n", len(deaths))
	fmt.Fprintf(w, "  a shop within reach had food:   %5.1f%%\n", 100*float64(foodWasThere)/n)
	fmt.Fprintf(w, "  could afford a meal when they died: %5.1f%%\n", 100*float64(couldAfford)/n)
	fmt.Fprintf(w, "  had a job:                      %5.1f%%\n", 100*float64(employed)/n)
	fmt.Fprintf(w, "  had food in their own larder:   %5.1f%%\n", 100*float64(hadLarder)/n)
	fmt.Fprintf(w, "\nFood on the shelf and no coin in the purse means the village is short of\n")
	fmt.Fprintf(w, "income, not of food, and another trade divides the same wage bill further.\n")

	// And how much of the year can a farm even hire?
	growing := 0
	for d := 0; d < DaysPerYear; d++ {
		if Tick(d * TicksPerDay).InGrowingSeason() {
			growing++
		}
	}
	fmt.Fprintf(w, "\ngrowing season: %d of %d days (%.0f%%) — farms hire nobody outside it\n",
		growing, DaysPerYear, 100*float64(growing)/float64(DaysPerYear))
}

// Is unemployment seasonal?
//
// Nobody starves for want of food — 100% of hunger deaths had a stocked shop within reach
// and none could afford a meal, and none had a job. So the binding constraint is income.
// Farms hire nobody outside the 240-day growing season, which leaves a third of the year
// with the village's largest employer shut. If unemployment spikes in those months, then
// counter-seasonal food work (hunting, gathering) is not "another trade dividing the same
// wage bill" — it is employing labour that is currently idle, producing food, in exactly
// the months when nothing else will hire.
func TestIsUnemploymentSeasonal(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(5 * TicksPerYear)

	var byMonth [12]struct{ employed, adults, samples int }
	for d := 0; d < 3*DaysPerYear; d++ {
		s.RunTicks(TicksPerDay)
		m := s.Tick.Date().Day * 12 / DaysPerYear
		byMonth[m].samples++
		for i := range s.Chars {
			c := &s.Chars[i]
			if !c.Alive || c.Stage() == Child {
				continue
			}
			byMonth[m].adults++
			if c.Job != NoStruct {
				byMonth[m].employed++
			}
		}
	}

	w := os.Stderr
	fmt.Fprintf(w, "\n=== employment by month (3 years) ===\n")
	fmt.Fprintf(w, "growing season is days %d-%d, months %d-%d\n",
		SowDay, HarvestDay, SowDay*12/DaysPerYear, HarvestDay*12/DaysPerYear)
	for m := 0; m < 12; m++ {
		b := byMonth[m]
		if b.adults == 0 {
			continue
		}
		rate := 100 * float64(b.employed) / float64(b.adults)
		inSeason := " "
		if m >= SowDay*12/DaysPerYear && m < HarvestDay*12/DaysPerYear {
			inSeason = "*"
		}
		bar := ""
		for i := 0; i < int(rate/2); i++ {
			bar += "#"
		}
		fmt.Fprintf(w, "  month %2d %s  %5.1f%% employed  %s\n", m, inSeason, rate, bar)
	}
	fmt.Fprintf(w, "\n(* = growing season, when farms hire)\n")
}

// Is the population capped by the number of jobs?
//
// Employment runs at 92-100% all year, so there is no idle labour to put to work — and yet
// everyone who starves is unemployed and penniless beside a stocked shop. That points at a
// hard ceiling: the village supports exactly as many adults as it has posts, and every
// adult beyond that number is destitute regardless of how much food is on the shelf.
//
// If so, the lever is not wages, not credit, and not seasonality. It is the number of
// posts the land can support — which is what a new food source (hunting, gathering) would
// raise, by opening a resource base the village cannot currently exploit at all.
func TestIsPopulationCappedByJobs(t *testing.T) {
	s := newTestSim(5)

	w := os.Stderr
	fmt.Fprintf(w, "\n=== adults against posts ===\n")
	fmt.Fprintf(w, "%5s %8s %7s %7s %9s %9s\n", "year", "pop", "adults", "posts", "unemp", "food/day")
	for y := 0; y <= 50; y += 5 {
		if y > 0 {
			s.RunTicks(5 * TicksPerYear)
		}
		adults, unemployed := 0, 0
		for i := range s.Chars {
			c := &s.Chars[i]
			if !c.Alive || c.Stage() == Child {
				continue
			}
			adults++
			if c.Job == NoStruct {
				unemployed++
			}
		}
		posts := 0
		for i := range s.Structs {
			st := &s.Structs[i]
			if st.Alive && st.Jobs > 0 {
				posts += st.Jobs
			}
		}
		fmt.Fprintf(w, "%5d %8d %7d %7d %9d %9.0f\n",
			y, s.Population(), adults, posts, unemployed, s.FoodDays())
	}
	fmt.Fprintf(w, "\nIf adults track posts and the surplus is the unemployed, the ceiling is\n")
	fmt.Fprintf(w, "the post count, and only new kinds of work can raise it.\n")
}

// Who actually dies, and at what age?
//
// Every labour-market reading has come back saying the village is fine: 147 posts for 36
// adults, zero unemployment, 544 days of food in store. Yet adults fall from 68 to 36 over
// fifty years while the headcount holds, which means the village is filling with children
// who do not become adults. And the "unemployed" who starve cannot be unemployed adults if
// unemployment is zero — children hold no jobs by definition.
//
// So: the age at every death, by cause.
func TestWhoActuallyDies(t *testing.T) {
	s := newTestSim(5)

	type rec struct {
		age    float32
		cause  DeathCause
		gold   float32
		larder float32
		fedBy  int // adults in the household
	}
	var recs []rec
	s.onDeath = func(id CharID, cause DeathCause) {
		c := &s.Chars[id]
		r := rec{age: c.Age, cause: cause, gold: float32(c.Gold)}
		if c.Home != NoStruct {
			r.larder = s.Structs[c.Home].Stock[Food]
			for j := range s.Chars {
				k := &s.Chars[j]
				if k.Alive && k.Home == c.Home && k.Stage() != Child && CharID(j) != id {
					r.fedBy++
				}
			}
		}
		recs = append(recs, r)
	}
	s.RunTicks(40 * TicksPerYear)

	w := os.Stderr
	fmt.Fprintf(w, "\n=== %d deaths over 40 years ===\n", len(recs))

	// Age bands, by cause.
	bands := []struct {
		lo, hi float32
		name   string
	}{{0, 1, "infant (<1)"}, {1, 5, "1-5"}, {5, 15, "5-15"}, {15, 45, "15-45"}, {45, 200, "45+"}}
	fmt.Fprintf(w, "%-14s %8s %8s %8s %8s\n", "age band", "hunger", "disease", "age", "other")
	for _, b := range bands {
		var hunger, disease, old, other int
		for _, r := range recs {
			if r.age < b.lo || r.age >= b.hi {
				continue
			}
			switch r.cause {
			case CauseHunger:
				hunger++
			case CauseDisease:
				disease++
			default:
				if r.age >= 45 {
					old++
				} else {
					other++
				}
			}
		}
		fmt.Fprintf(w, "%-14s %8d %8d %8d %8d\n", b.name, hunger, disease, old, other)
	}

	// For the young dead: was there food in the house, and an adult to give it?
	var young, withLarder, withAdult int
	for _, r := range recs {
		if r.age >= 15 || r.cause != CauseHunger {
			continue
		}
		young++
		if r.larder >= FoodPerMeal {
			withLarder++
		}
		if r.fedBy > 0 {
			withAdult++
		}
	}
	if young > 0 {
		fmt.Fprintf(w, "\nof %d children who starved:\n", young)
		fmt.Fprintf(w, "  had food in the family larder:  %d (%.0f%%)\n", withLarder, 100*float64(withLarder)/float64(young))
		fmt.Fprintf(w, "  had a living adult at home:     %d (%.0f%%)\n", withAdult, 100*float64(withAdult)/float64(young))
	}
}

// The demographic bottom line, on a cohort that has finished.
//
// Everything economic reads healthy: 147 posts for 36 adults, zero unemployment, 544 days
// of food. Deaths are 47 disease, 32 old age, 14 hunger. So whether this village grows is
// not an economic question at all — it is whether enough children reach an age to have
// children of their own.
func TestCohortSurvival(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(60 * TicksPerYear)

	// Only lives BEGUN in the first twenty years: a near-complete cohort, everyone dead or
	// old by the horizon. Judging all completed lives right-censors in a growing
	// population and manufactures pessimism (see TestVitalStatisticsArePlausible).
	var lives []Life
	for _, l := range s.Lives {
		if l.Born <= Tick(20*TicksPerYear) && !l.Settler {
			lives = append(lives, l)
		}
	}
	if len(lives) < 10 {
		t.Skipf("only %d completed native lives", len(lives))
	}
	v := VitalsOf(lives)
	w := os.Stderr
	fmt.Fprintf(w, "\n=== cohort born in the first 20 years, %d completed lives ===\n", len(lives))
	fmt.Fprintf(w, "%s\n", v.String())

	// R0 the right way. Vitals.ChildrenPerLife is COMPLETED fertility — conditioned on
	// reaching 45 — so dividing it by two gives a number four times too large and says a
	// village with a flat headcount is quadrupling. Every birth credits both parents, so
	// total children is twice the number of births; over ALL lives, infant deaths counted
	// as the zeroes they are, R0 is mean children per life halved.
	var kids int
	for _, l := range lives {
		kids += l.Children
	}
	r0 := float64(kids) / float64(len(lives)) / 2
	fmt.Fprintf(w, "\nchildren per life over ALL lives: %.3f  (completed-fertility figure above: %.2f)\n",
		float64(kids)/float64(len(lives)), v.ChildrenPerLife)
	fmt.Fprintf(w, "R0 = %.3f  — replacement is 1.0\n", r0)
}

// Where does adult time actually go, and how good are their tools?
//
// At full employment output can only rise through productivity, so this asks whether
// there is productivity to be had. Two candidates: tools, which already multiply output
// by 1 + ToolBonus*Tools, and carrying capacity, since the record says fetching food once
// ate half of all adult time and PackSize is what decides how often the trip is made.
func TestWhereAdultTimeGoes(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(5 * TicksPerYear)

	var acts [numActivities]int
	var samples int
	var toolSum, toolWorst float64
	var toolCount, toolless int
	toolWorst = 1

	const days = 120
	for d := 0; d < days; d++ {
		for i := 0; i < TicksPerDay; i += 20 { // sample, rather than every tick
			s.RunTicks(20)
			for j := range s.Chars {
				c := &s.Chars[j]
				if !c.Alive || c.Stage() == Child {
					continue
				}
				samples++
				acts[c.Activity]++
			}
		}
	}
	for j := range s.Chars {
		c := &s.Chars[j]
		if !c.Alive || c.Stage() == Child || c.Job == NoStruct {
			continue
		}
		toolCount++
		toolSum += float64(c.Tools)
		if float64(c.Tools) < toolWorst {
			toolWorst = float64(c.Tools)
		}
		if c.Tools < 0.05 {
			toolless++
		}
	}

	w := os.Stderr
	fmt.Fprintf(w, "\n=== adult time budget over %d days ===\n", days)
	type row struct {
		a Activity
		n int
	}
	var rows []row
	for a := Activity(0); a < numActivities; a++ {
		rows = append(rows, row{a, acts[a]})
	}
	sort.Slice(rows, func(x, y int) bool { return rows[x].n > rows[y].n })
	for _, r := range rows {
		if r.n == 0 {
			continue
		}
		fmt.Fprintf(w, "  %-24v %5.1f%%\n", r.a, 100*float64(r.n)/float64(samples))
	}

	if toolCount > 0 {
		fmt.Fprintf(w, "\ntools among the employed (%d people):\n", toolCount)
		fmt.Fprintf(w, "  mean condition      %.2f  (output multiplier %.2f of a possible %.2f)\n",
			toolSum/float64(toolCount), 1+ToolBonus*toolSum/float64(toolCount), 1+ToolBonus)
		fmt.Fprintf(w, "  working with none   %d (%.0f%%)\n", toolless, 100*float64(toolless)/float64(toolCount))
		fmt.Fprintf(w, "  worst               %.2f\n", toolWorst)
	}
	fmt.Fprintf(w, "\nPackSize is %.0f meals, so a full pack is %.1f days of eating.\n",
		PackSize, PackSize/MealsPerDay)
}

// Why do two in five workers have no tools?
//
// Mean condition is 0.34 against a possible 1.0, and the output multiplier the village
// actually realises is 1.15 of an available 1.45. Either they cannot afford a set, or
// nobody is selling one. The answer decides the fix.
func TestWhyWorkersHaveNoTools(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(5 * TicksPerYear)

	price := s.Prices[Tools]
	var stocked, shops int
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || (st.Type != Store && st.Type != Workshop) {
			continue
		}
		shops++
		if st.Stock[Tools] >= 1 {
			stocked++
		}
	}

	var canAfford, toolless, tooPoor, employed int
	var meanGold float64
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Stage() == Child || c.Job == NoStruct {
			continue
		}
		employed++
		meanGold += float64(c.Gold)
		if c.Gold >= price {
			canAfford++
		}
		if c.Tools < 0.05 {
			toolless++
			if c.Gold < price {
				tooPoor++
			}
		}
	}

	w := os.Stderr
	fmt.Fprintf(w, "\n=== why tools are missing ===\n")
	fmt.Fprintf(w, "  a set costs            %.2f gold\n", price)
	fmt.Fprintf(w, "  a day's food costs     %.2f\n", s.Prices[Food]*MealsPerDay)
	fmt.Fprintf(w, "  shops holding a whole set: %d of %d\n", stocked, shops)
	if employed > 0 {
		fmt.Fprintf(w, "  employed workers:      %d, mean purse %.1f\n", employed, meanGold/float64(employed))
		fmt.Fprintf(w, "  could buy a set today: %d (%.0f%%)\n", canAfford, 100*float64(canAfford)/float64(employed))
		fmt.Fprintf(w, "  working with none:     %d\n", toolless)
		if toolless > 0 {
			fmt.Fprintf(w, "    of whom cannot afford one: %d (%.0f%%)\n", tooPoor, 100*float64(tooPoor)/float64(toolless))
		}
	}
	fmt.Fprintf(w, "\nToolBuyBelow is %.2f, so a worker runs a set down to a quarter before\n", ToolBuyBelow)
	fmt.Fprintf(w, "replacing it, and only off-shift, and only if a shop has a whole one.\n")
}

// Are the workshops idle for want of materials?
//
// One worker finishes 0.32 tools a day and the whole village wears out about 0.16, so
// craft capacity is not the ceiling. But manufacture() returns early without wood or iron,
// and a workshop with an empty bin makes nothing however many hands it has.
func TestAreWorkshopsStarvedOfMaterials(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(5 * TicksPerYear)

	const days = 90
	var samples, noWood, noIron, noneAtAll, staffed int
	for d := 0; d < days; d++ {
		s.RunTicks(TicksPerDay)
		for i := range s.Structs {
			st := &s.Structs[i]
			if !st.Alive || st.Type != Workshop {
				continue
			}
			samples++
			if st.Filled > 0 {
				staffed++
			}
			w := st.Stock[Wood] < ToolWoodCost
			ir := st.Stock[Iron] < ToolIronCost
			if w {
				noWood++
			}
			if ir {
				noIron++
			}
			if w || ir {
				noneAtAll++
			}
		}
	}
	w := os.Stderr
	fmt.Fprintf(w, "\n=== workshop supply over %d days ===\n", days)
	if samples > 0 {
		fmt.Fprintf(w, "  workshop-days sampled:  %d\n", samples)
		fmt.Fprintf(w, "  staffed:                %.0f%%\n", 100*float64(staffed)/float64(samples))
		fmt.Fprintf(w, "  short of wood:          %.0f%%\n", 100*float64(noWood)/float64(samples))
		fmt.Fprintf(w, "  short of iron:          %.0f%%\n", 100*float64(noIron)/float64(samples))
		fmt.Fprintf(w, "  cannot make a tool:     %.0f%%\n", 100*float64(noneAtAll)/float64(samples))
	}
	// And what the upstream trades are holding.
	for _, ty := range []StructType{Mine, LumberCamp, Storehouse, Store} {
		var gold, wood, iron, tools float64
		n := 0
		for i := range s.Structs {
			if st := &s.Structs[i]; st.Alive && st.Type == ty {
				n++
				gold += st.Gold
				wood += float64(st.Stock[Wood])
				iron += float64(st.Stock[Iron])
				tools += float64(st.Stock[Tools])
			}
		}
		fmt.Fprintf(w, "  %-12v x%d  gold %7.1f wood %7.1f iron %7.1f tools %6.1f\n",
			ty, n, gold, wood, iron, tools)
	}
}
