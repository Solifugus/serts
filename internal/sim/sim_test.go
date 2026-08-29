package sim

import (
	"sync"
	"testing"

	"github.com/solifugus/serts/internal/torus"
	"github.com/solifugus/serts/internal/worldgen"
)

// testWorld builds the world the game actually ships with.
//
// It used to halve the dimensions to keep tests quick, and that quietly stopped the suite
// testing the real thing. A smaller world puts everything closer together and hides every
// problem that distance causes — so the suite reported green all day while the standard
// 256-cell village was declining to a handful of survivors.
func testWorld(seed int64) *worldgen.World {
	return worldgen.Generate(worldgen.DefaultParams(seed))
}

// acceptance marks one of the century-scale runs that judge whether the society works at
// all — as opposed to the invariant tests, which judge whether the code does what it says.
//
// The distinction is a cost one. An invariant test simulates a few years and finishes in
// seconds; an acceptance test carries several thousand people through a hundred years,
// several times over, and cannot be made cheap without ceasing to measure what it is for.
// Left in the same tier, the expensive ones set the price of every check, the suite stops
// being run, and the cheap tests stop protecting anything either (docs/method.md, note 15).
//
// So they skip under -short, which is the fast gate, and run everywhere else. This is NOT
// a licence to shorten their horizons or relax their thresholds to make a suite green —
// see note 7. They are the goal, written down.
func acceptance(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("century-scale acceptance run; `make accept` runs it, `make test` does not")
	}
}

func newTestSim(seed int64) *State {
	return New(DefaultConfig(testWorld(seed), seed))
}

// ---- Time ----

func TestClockConversions(t *testing.T) {
	cases := []struct {
		tick Tick
		want Date
	}{
		{0, Date{Year: 0, Day: 0, Hour: 0, Minute: 0}},
		{1, Date{Year: 0, Day: 0, Hour: 0, Minute: 0}}, // 0.6 in-world minutes, truncated
		{50, Date{Year: 0, Day: 0, Hour: 0, Minute: 30}},
		{TicksPerHour, Date{Year: 0, Day: 0, Hour: 1, Minute: 0}},
		{TicksPerDay, Date{Year: 0, Day: 1, Hour: 0, Minute: 0}},
		{TicksPerYear, Date{Year: 1, Day: 0, Hour: 0, Minute: 0}},
		{TicksPerYear + TicksPerDay*5 + TicksPerHour*13, Date{Year: 1, Day: 5, Hour: 13, Minute: 0}},
	}
	for _, c := range cases {
		if got := c.tick.Date(); got != c.want {
			t.Errorf("Tick(%d).Date() = %+v, want %+v", c.tick, got, c.want)
		}
	}
}

// The master constant of the design (§2.10): one real day is one in-world year.
//
// This is the number every other rate is tuned against, so it is asserted directly
// rather than left to be inferred from whatever the constants happen to say.
func TestMasterTimeConstantHolds(t *testing.T) {
	if realHoursPerYear := float64(TicksPerYear) / TickRate / 3600; realHoursPerYear != 24 {
		t.Errorf("an in-world year takes %v real hours, want exactly 24", realHoursPerYear)
	}
	if realMinutesPerDay := float64(TicksPerDay) / TickRate / 60; realMinutesPerDay != 4 {
		t.Errorf("an in-world day takes %v real minutes, want 4", realMinutesPerDay)
	}
	// The tick arithmetic must stay exact, which is why the year is 360 days.
	if TicksPerYear%TicksPerDay != 0 || TicksPerDay%TicksPerHour != 0 {
		t.Error("tick arithmetic does not divide evenly")
	}
	// A full life of 60 in-world years is about two real months.
	if lifeDays := 60 * float64(TicksPerYear) / TickRate / 86400; lifeDays != 60 {
		t.Errorf("a 60-year life takes %.1f real days, want 60", lifeDays)
	}
	// Long enough to actually watch: a villager should cross a few cells in seconds,
	// not teleport. This is what the slower clock buys.
	cellsPerRealSecond := WalkSpeed / float64(TicksPerHour) * TickRate
	if cellsPerRealSecond > 1 {
		t.Errorf("people move %v cells per real second; too fast to follow", cellsPerRealSecond)
	}
}

func TestWorkTimeSpansTheWorkingDay(t *testing.T) {
	var working int
	for h := 0; h < 24; h++ {
		if Tick(h * TicksPerHour).IsWorkTime() {
			working++
		}
	}
	if working != WorkEndHour-WorkStartHour {
		t.Errorf("%d working hours, want %d", working, WorkEndHour-WorkStartHour)
	}
	if Tick(0).IsWorkTime() {
		t.Error("midnight should not be work time")
	}
	if !Tick(12 * TicksPerHour).IsWorkTime() {
		t.Error("midday should be work time")
	}
}

// ---- Determinism (§9.2) ----

// The property the whole design rests on: identical inputs must produce identical
// worlds, or replay, regression tests, and tuning all become guesswork.
func TestSimulationIsDeterministic(t *testing.T) {
	a, b := newTestSim(5), newTestSim(5)
	const ticks = 20000

	a.RunTicks(ticks)
	b.RunTicks(ticks)

	sa, sb := a.Stats(), b.Stats()
	if sa != sb {
		t.Fatalf("same seed diverged:\n  %+v\n  %+v", sa, sb)
	}
	if len(a.Chars) != len(b.Chars) {
		t.Fatalf("character counts differ: %d vs %d", len(a.Chars), len(b.Chars))
	}
	for i := range a.Chars {
		if a.Chars[i] != b.Chars[i] {
			t.Fatalf("character %d differs:\n  %+v\n  %+v", i, a.Chars[i], b.Chars[i])
		}
	}
	for i := range a.Structs {
		if a.Structs[i] != b.Structs[i] {
			t.Fatalf("structure %d differs:\n  %+v\n  %+v", i, a.Structs[i], b.Structs[i])
		}
	}
}

// Running in one long stretch must equal running in pieces — the property that makes the
// development speed multiplier safe (§2.10).
func TestSteppingIsIndependentOfBatchSize(t *testing.T) {
	a, b := newTestSim(6), newTestSim(6)
	a.RunTicks(9000)
	for i := 0; i < 9; i++ {
		b.RunTicks(1000)
	}
	if a.Stats() != b.Stats() {
		t.Errorf("batching changed the outcome:\n  %+v\n  %+v", a.Stats(), b.Stats())
	}
}

func TestDifferentSeedsDiverge(t *testing.T) {
	a, b := newTestSim(1), newTestSim(2)
	a.RunTicks(5000)
	b.RunTicks(5000)
	if a.Stats() == b.Stats() {
		t.Error("different seeds produced identical outcomes")
	}
}

func TestRandIsStableForASeed(t *testing.T) {
	a, b := NewRand(42, 1), NewRand(42, 1)
	for i := 0; i < 1000; i++ {
		if a.Uint64() != b.Uint64() {
			t.Fatalf("same seed and stream diverged at draw %d", i)
		}
	}
	// Separate streams must not march in lockstep, or unrelated systems perturb each
	// other's sequences.
	c, d := NewRand(42, 1), NewRand(42, 2)
	same := 0
	for i := 0; i < 1000; i++ {
		if c.Uint64() == d.Uint64() {
			same++
		}
	}
	if same > 5 {
		t.Errorf("streams 1 and 2 agreed %d/1000 times", same)
	}
}

func TestRandFloatsStayInRange(t *testing.T) {
	r := NewRand(7, 0)
	for i := 0; i < 10000; i++ {
		if v := r.Float64(); v < 0 || v >= 1 {
			t.Fatalf("Float64 returned %v", v)
		}
		if v := r.Intn(10); v < 0 || v >= 10 {
			t.Fatalf("Intn(10) returned %v", v)
		}
	}
}

// ---- Founding ----

func TestFoundingBuildsWhatWasAskedFor(t *testing.T) {
	s := newTestSim(11)
	cfg := DefaultConfig(s.World, 11)
	if s.BuiltFarms != cfg.Farms {
		t.Errorf("built %d farms, wanted %d", s.BuiltFarms, cfg.Farms)
	}
	if s.BuiltHomes != cfg.Homes {
		t.Errorf("built %d homes, wanted %d", s.BuiltHomes, cfg.Homes)
	}
	if s.BuiltGranaries != cfg.Granaries {
		t.Errorf("built %d granaries, wanted %d", s.BuiltGranaries, cfg.Granaries)
	}
	if got := s.Population(); got != cfg.Settlers {
		t.Errorf("settled %d, wanted %d", got, cfg.Settlers)
	}
}

// Farms must be able to irrigate (§2.8), and nothing may be built on water.
func TestStructuresRespectTerrain(t *testing.T) {
	s := newTestSim(13)
	for i := range s.Structs {
		st := &s.Structs[i]
		idx := s.T.Index(st.Cell)
		if !s.World.Walkable(idx) {
			t.Errorf("%v built on unwalkable terrain at %v", st.Type, st.Cell)
		}
		if st.Type == Farm && s.World.FreshDist[idx] > Defs[Farm].MaxFreshDist {
			t.Errorf("farm at %v is %d cells from fresh water, limit %d",
				st.Cell, s.World.FreshDist[idx], Defs[Farm].MaxFreshDist)
		}
	}
}

func TestStructuresDoNotOverlap(t *testing.T) {
	s := newTestSim(17)
	seen := map[[2]int]bool{}
	for i := range s.Structs {
		k := [2]int{s.Structs[i].Cell.X, s.Structs[i].Cell.Y}
		if seen[k] {
			t.Errorf("two structures share cell %v", k)
		}
		seen[k] = true
	}
}

// ---- Bookkeeping invariants ----
//
// These are the checks that would have caught the resident-count leak that quietly
// starved every child in the village.

func TestFilledMatchesActualWorkers(t *testing.T) {
	s := newTestSim(19)
	s.RunTicks(30000)

	actual := make([]int, len(s.Structs))
	for i := range s.Chars {
		if c := &s.Chars[i]; c.Alive && c.Job != NoStruct {
			actual[c.Job]++
		}
	}
	for i := range s.Structs {
		if s.Structs[i].Filled != actual[i] {
			t.Errorf("structure %d (%v) reports %d staff, %d characters are employed there",
				i, s.Structs[i].Type, s.Structs[i].Filled, actual[i])
		}
	}
}

func TestResidentsMatchHousedCharacters(t *testing.T) {
	s := newTestSim(23)
	s.RunTicks(30000)

	actual := make([]int, len(s.Structs))
	for i := range s.Chars {
		if c := &s.Chars[i]; c.Alive && c.Home != NoStruct && c.housed {
			actual[c.Home]++
		}
	}
	for i := range s.Structs {
		if s.Structs[i].Residents != actual[i] {
			t.Errorf("home %d reports %d residents, %d characters are housed there",
				i, s.Structs[i].Residents, actual[i])
		}
	}
}

func TestNoOneExceedsCapacityOrVacancies(t *testing.T) {
	s := newTestSim(29)
	s.RunTicks(30000)
	for i := range s.Structs {
		st := &s.Structs[i]
		if st.Filled > st.Jobs {
			t.Errorf("structure %d has %d staff in %d jobs", i, st.Filled, st.Jobs)
		}
		if st.Type == Home && st.Residents > Defs[Home].Capacity {
			t.Errorf("home %d has %d residents, capacity %d", i, st.Residents, Defs[Home].Capacity)
		}
	}
}

func TestStocksNeverGoNegative(t *testing.T) {
	s := newTestSim(31)
	for i := 0; i < 30000; i++ {
		s.Step()
		for j := range s.Structs {
			if s.Structs[j].Stock[Food] < -1e-4 {
				t.Fatalf("structure %d has negative food %v at tick %d", j, s.Structs[j].Stock[Food], s.Tick)
			}
		}
	}
	for i := range s.Chars {
		if c := &s.Chars[i]; c.Alive && c.Gold < -1e-4 {
			t.Fatalf("character %d has negative gold %v", i, c.Gold)
		}
	}
}

func TestDeadCharactersReleaseTheirClaims(t *testing.T) {
	s := newTestSim(37)
	s.RunTicks(30000)
	for i := range s.Chars {
		c := &s.Chars[i]
		if c.Alive {
			continue
		}
		if c.Job != NoStruct || c.Home != NoStruct {
			t.Errorf("dead character %d still holds job %d and home %d", i, c.Job, c.Home)
		}
	}
}

// ---- Movement (§9.3) ----

func TestFlowFieldsReachTheirDestination(t *testing.T) {
	s := newTestSim(41)
	for sid := range s.Structs {
		f := s.fieldTo(StructID(sid))
		dest := s.T.Index(s.Structs[sid].Cell)

		// Walk the field from a sample of reachable cells and confirm arrival.
		checked := 0
		for i := 0; i < s.T.Cells() && checked < 200; i += 37 {
			if f.dir[i] < 0 || !s.World.Walkable(i) {
				continue
			}
			checked++
			cur, steps := i, 0
			for cur != dest {
				d := f.dir[cur]
				if d < 0 {
					t.Fatalf("field to structure %d dead-ends at cell %d", sid, cur)
				}
				cur = s.T.Index(s.T.Neighbor8(s.T.CellOf(cur), int(d)))
				if steps++; steps > s.T.Cells() {
					t.Fatalf("field to structure %d loops from cell %d", sid, i)
				}
			}
		}
	}
}

func TestFlowFieldsAreCached(t *testing.T) {
	s := newTestSim(43)
	s.RunTicks(5000)
	st := s.Stats()
	// One field per structure, plus the shared gold field, which is rebuilt at most once
	// an in-world day as deposits are worked out.
	budget := len(s.Structs) + 1 + int(s.Tick/TicksPerDay) + 1
	if st.PathMisses > budget {
		t.Errorf("computed %d fields for %d structures over %d days; cache is not holding",
			st.PathMisses, len(s.Structs), s.Tick/TicksPerDay)
	}
	if st.PathHits < st.PathMisses {
		t.Errorf("cache hits %d below misses %d", st.PathHits, st.PathMisses)
	}
}

func TestCharactersStayOnWalkableGround(t *testing.T) {
	s := newTestSim(47)
	for i := 0; i < 20000; i++ {
		s.Step()
		if i%500 != 0 {
			continue
		}
		for j := range s.Chars {
			c := &s.Chars[j]
			if !c.Alive {
				continue
			}
			if !s.World.Walkable(s.T.Index(s.T.CellAt(c.Pos))) {
				t.Fatalf("character %d is standing in water at %v", j, c.Pos)
			}
		}
	}
}

// ---- The employment market (§3.5) ----

func TestSkillEfficiencyRisesWithDiminishingReturns(t *testing.T) {
	if efficiency(0) != 1 {
		t.Errorf("a novice's efficiency is %v, want 1", efficiency(0))
	}
	prev, prevGain := efficiency(0), 0.0
	for y := float32(1); y <= 40; y++ {
		e := efficiency(y)
		if e <= prev {
			t.Fatalf("efficiency fell from %v to %v at %v years", prev, e, y)
		}
		if gain := e - prev; y > 1 && gain > prevGain {
			t.Fatalf("gain grew from %v to %v at %v years; returns are not diminishing", prevGain, gain, y)
		} else if y > 1 {
			prevGain = gain
		} else {
			prevGain = e - prev
		}
		prev = e
	}
	// A lifetime of work should be worth substantially more than a novice, but not
	// unboundedly so (§3.4).
	if lifetime := efficiency(45); lifetime < 1.5 || lifetime > 3.5 {
		t.Errorf("a lifetime of tenure gives efficiency %v, want between 1.5 and 3.5", lifetime)
	}
}

// intoGrowingSeason advances the clock to a day when the fields are being worked.
//
// Farm work is seasonal, so a farm scores zero as an employer outside the growing season.
// Tests about wages, urgency or reachability are not about seasonality and would otherwise
// be measuring the calendar instead of the thing they name.
func intoGrowingSeason(s *State) {
	for !s.Tick.InGrowingSeason() {
		s.RunTicks(TicksPerDay)
	}
}

func TestJobScoringRejectsImpossibleWork(t *testing.T) {
	s := newTestSim(53)
	intoGrowingSeason(s)
	var farm StructID = NoStruct
	for i := range s.Structs {
		if s.Structs[i].Type == Farm {
			farm = StructID(i)
			break
		}
	}
	if farm == NoStruct {
		t.Fatal("no farm to test against")
	}
	worker := CharID(0)
	// The property under test is the zero-vs-nonzero boundary, so the non-zero side must
	// be set up rather than assumed: at founding scale the first farm can happen to offer
	// no wage yet, or stand farther from character zero than the search radius, and then
	// the test measures village layout instead of scoring rules.
	s.Structs[farm].Wage = 0.01
	s.Structs[farm].Filled = 0
	s.Chars[worker].Pos = s.Structs[farm].Pos

	if s.scoreJob(worker, farm) <= 0 {
		t.Error("a reachable farm with vacancies scored zero")
	}

	// A structure with no vacancy is impossible, not merely unattractive.
	s.Structs[farm].Filled = s.Structs[farm].Jobs
	if got := s.scoreJob(worker, farm); got != 0 {
		t.Errorf("a fully staffed farm scored %v, want 0", got)
	}
	s.Structs[farm].Filled = 0

	// So is one beyond reach.
	s.Chars[worker].Pos = s.T.Add(s.Structs[farm].Pos,
		torus.Vec2{X: JobSearchRadius + 10, Y: 0})
	if got := s.scoreJob(worker, farm); got != 0 {
		t.Errorf("a farm beyond the search radius scored %v, want 0", got)
	}
}

func TestHungerMakesWorkersLessChoosy(t *testing.T) {
	s := newTestSim(59)
	intoGrowingSeason(s)
	var farm StructID = NoStruct
	for i := range s.Structs {
		if s.Structs[i].Type == Farm {
			farm = StructID(i)
			break
		}
	}
	worker := CharID(0)
	s.Chars[worker].Pos = s.Structs[farm].Pos
	s.Structs[farm].Filled = 0

	s.Chars[worker].Hunger = 10
	comfortable := s.scoreJob(worker, farm)
	s.Chars[worker].Hunger = 85
	desperate := s.scoreJob(worker, farm)

	if desperate <= comfortable {
		t.Errorf("a starving character scored the job %v, a comfortable one %v; urgency has no effect",
			desperate, comfortable)
	}
}

// ---- The village, end to end ----

// The question the project exists to answer: does a village replace itself?
//
// Twenty years cannot answer it, and reading twenty years as though it could was a real
// mistake. A founding settlement grows for a decade or so on its settlers' savings and
// their being uniformly adult and healthy, and that transient looks exactly like health.
// Measured to 120 years across six seeds, every village studied died:
//
//	seed   5:  y20 25  y40  7  y60  3  y80 1  y100 0  y120 0
//	seed   7:  y20 16  y40  8  y60  2  y80 0  y100 0  y120 0
//	seed 108:  y20 32  y40 15  y60  8  y80 7  y100 3  y120 1
//	seed 209:  y20 28  y40 19  y60 11  y80 4  y100 1  y120 0
//	seed 310:  y20 22  y40  4  y60  4  y80 2  y100 2  y120 0
//	seed 411:  y20 16  y40  9  y60 10  y80 4  y100 1  y120 0
//
// Every one of those passed the twenty-year test comfortably. An instrument that reports
// success on a village in terminal decline is the same fault as the half-size test world
// and the single-seed vitals, and it is the third time in this project that the measurement
// rather than the simulation was what needed fixing.
//
// This test is expected to fail until the village genuinely reproduces. That is the point
// of it (see docs/method.md, note 7): it encodes the goal, so it goes red until the goal is
// met and must not be relaxed to make the suite green.
func TestVillageReplacesItself(t *testing.T) {
	acceptance(t)
	seeds := []int64{5, 7, 108, 209}
	const years = 100

	final := make([]int, len(seeds))
	var wg sync.WaitGroup
	for i, seed := range seeds {
		wg.Add(1)
		go func(i int, seed int64) {
			defer wg.Done()
			s := newTestSim(seed)
			s.RunTicks(years * TicksPerYear)
			final[i] = s.Population()
		}(i, seed)
	}
	wg.Wait()

	var died, total int
	for i, seed := range seeds {
		t.Logf("seed %3d: population %d after %d years", seed, final[i], years)
		total += final[i]
		if final[i] == 0 {
			died++
		}
	}
	if died > 0 {
		t.Errorf("%d of %d villages died out within %d years", died, len(seeds), years)
	}
	// A village that has shrunk to a handful has not replaced itself either; it is simply
	// dying more slowly. Founding size is the bar, since anything less is decline.
	settlers := DefaultConfig(testWorld(seeds[0]), seeds[0]).Settlers
	if mean := float64(total) / float64(len(seeds)); mean < float64(settlers) {
		t.Errorf("mean population after %d years is %.1f, below the %d it was founded with",
			years, mean, settlers)
	}
}

// A check that the economy runs at all, kept separate from the demographic tests: it says
// nothing about whether the village thrives, only that it has not seized up. See
// TestVillageReplacesItself for the demographic question.
//
// This is 220 of the 300 seconds the fast gate costs — three quarters of it, against
// about forty-five seconds for every other invariant test combined. It stays in the gate
// anyway, for two reasons.
//
// The first is that it is the test note 7 was written about: it has repeatedly refused
// changes that were individually correct, and a test that only runs before a commit is a
// test that finds the problem after the work is built on top of it.
//
// The second is that its cost is inverted, which makes it cheaper than it looks. Twenty
// years is slow because the village fills with people; a village that seizes up empties
// out, and an empty village simulates almost instantly. The gate is slow exactly when
// everything is fine, and fast whenever it is about to fail. Paying five minutes to learn
// that all is well is a much better trade than it appears on the clock.
func TestVillageSurvivesItsFirstDecades(t *testing.T) {
	s := newTestSim(7)
	s.RunTicks(20 * TicksPerYear)

	st := s.Stats()
	if st.Population == 0 {
		t.Fatal("the village died out within twenty years")
	}
	if st.Population < 15 {
		t.Errorf("population fell to %d after twenty years, from %d settlers", st.Population, DefaultConfig(s.World, 7).Settlers)
	}
	if st.AvgHealth < 80 {
		t.Errorf("average health %v after twenty years", st.AvgHealth)
	}
	if st.Employed == 0 {
		t.Error("nobody is employed; the labour market has collapsed")
	}
	if st.Food <= 0 {
		t.Error("the village holds no food at all")
	}
	if st.Births == 0 {
		t.Error("no children were born in twenty years")
	}
}

func TestPeopleFindWorkAndHousing(t *testing.T) {
	s := newTestSim(61)
	s.RunTicks(TicksPerYear)

	var housed, employed, adults int
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Stage() == Child {
			continue
		}
		adults++
		if c.Home != NoStruct {
			housed++
		}
		if c.Job != NoStruct {
			employed++
		}
	}
	if housed < adults*3/4 {
		t.Errorf("only %d of %d adults are housed after a year", housed, adults)
	}
	if employed < adults/2 {
		t.Errorf("only %d of %d adults found work after a year", employed, adults)
	}
}

func TestFarmsProduceAndFoodReachesGranaries(t *testing.T) {
	s := newTestSim(67)
	s.RunTicks(TicksPerYear)
	var granaryFood float32
	for i := range s.Structs {
		if s.Structs[i].Type == Granary {
			granaryFood += s.Structs[i].Stock[Food]
		}
	}
	if granaryFood <= 0 {
		t.Error("no food reached the granaries in a year of farming")
	}
}

// ---- Money (§4.2) ----

// Gold enters the world only by being panned and leaves only through upkeep and estate
// loss, so the total can fall but must never rise. This is the check that catches an
// economy quietly minting money, which no amount of watching population graphs would.
func TestGoldIsNeverCreated(t *testing.T) {
	s := newTestSim(71)
	start := s.TotalCoin()
	high := start

	for i := 0; i < 40*TicksPerDay; i++ {
		s.Step()
		if i%(TicksPerDay/4) != 0 {
			continue
		}
		if now := s.TotalCoin(); now > high {
			high = now
		}
	}
	// A small tolerance for float32 accumulation across millions of transactions.
	if tol := start * 0.001; high > start+tol {
		t.Errorf("total gold rose from %.2f to %.2f; the economy is creating money", start, high)
	}
}

// Dead characters must not take their coin out of existence wholesale (§4.2).
func TestInheritanceKeepsMostOfAnEstate(t *testing.T) {
	s := newTestSim(73)

	// Give a character a household and a fortune, then kill them.
	var heir, victim CharID = NoChar, NoChar
	for i := range s.Chars {
		if !s.Chars[i].Alive || s.Chars[i].Home == NoStruct {
			continue
		}
		if victim == NoChar {
			victim = CharID(i)
			continue
		}
		if s.Chars[i].Home == s.Chars[victim].Home {
			heir = CharID(i)
			break
		}
	}
	if victim == NoChar || heir == NoChar {
		t.Skip("no two characters share a home in this world")
	}

	s.Chars[victim].Gold = 100
	before := s.Chars[heir].Gold
	s.kill(victim, CauseAge)

	if gained := s.Chars[heir].Gold - before; gained <= 0 {
		t.Errorf("heir inherited %v of a 100 gold estate", gained)
	}
	if s.Chars[victim].Gold != 0 {
		t.Errorf("the dead still hold %v gold", s.Chars[victim].Gold)
	}
}

// Panning is the faucet, and it must draw down the ground it draws from (§4.2).
func TestPanningDepletesTheGround(t *testing.T) {
	s := newTestSim(79)

	// Find a gold cell and stand an unemployed adult on it.
	var cell = -1
	for i := 0; i < s.T.Cells(); i++ {
		if s.World.GoldOre[i] > 1 && s.World.Walkable(i) {
			cell = i
			break
		}
	}
	if cell < 0 {
		t.Skip("this world has no reachable gold")
	}

	worker := CharID(0)
	s.quitJob(worker, QuitForBetterWork)
	s.Chars[worker].Pos = s.T.Center(s.T.CellOf(cell))
	beforeGround := s.World.GoldOre[cell]
	beforePurse := s.Chars[worker].Gold

	for i := 0; i < 100; i++ {
		s.pan(worker)
	}

	if s.World.GoldOre[cell] >= beforeGround {
		t.Error("panning did not deplete the deposit")
	}
	if s.Chars[worker].Gold <= beforePurse {
		t.Error("panning yielded no coin")
	}
	// What came out of the ground is exactly what went into the purse.
	got := s.Chars[worker].Gold - beforePurse
	lost := beforeGround - s.World.GoldOre[cell]
	if diff := got - lost; diff > 1e-3 || diff < -1e-3 {
		t.Errorf("purse gained %v but the ground lost %v", got, lost)
	}
}

// Panning must never pay better than working, or the money supply expands when the
// economy is healthy — the opposite of the stabiliser it is meant to be.
func TestPanningPaysWorseThanWork(t *testing.T) {
	if PanYieldPerDay >= BaseWagePerDay {
		t.Errorf("panning pays %v a day against a wage of %v; nobody would take a job",
			PanYieldPerDay, BaseWagePerDay)
	}
}

// ---- Movement (§2.2) ----

// The corner-clipping deadlock, which cost a great deal to find and would cost as much
// again. A diagonal step of a fraction of a cell crosses one axis boundary before the
// other, so a character aiming at a walkable diagonal neighbour gets tested against the
// cell beside it. Where that one is water, every step is rejected and they stand
// motionless while believing they are walking.
func TestCharactersSlideAlongObstacles(t *testing.T) {
	s := newTestSim(83)

	// Put someone against water with a walkable diagonal beyond it, and check they move.
	moved := 0
	tested := 0
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive {
			continue
		}
		cell := s.T.CellAt(c.Pos)
		// Look for a spot with a blocked orthogonal neighbour.
		blocked := false
		for k := 0; k < 8; k++ {
			if !s.World.Walkable(s.T.Index(s.T.Neighbor8(cell, k))) {
				blocked = true
			}
		}
		if !blocked {
			continue
		}
		tested++
		before := c.Pos
		// Aim at every direction in turn; at least one must be achievable.
		for k := 0; k < 8; k++ {
			s.stepToward(CharID(i), s.T.Center(s.T.Neighbor8(cell, k)))
		}
		if s.T.Dist(before, c.Pos) > 0 {
			moved++
		}
	}
	if tested > 0 && moved == 0 {
		t.Error("no character beside an obstacle was able to move at all")
	}
}

// Nobody may be permanently stuck: over a long run, characters must actually get around.
func TestCharactersDoNotGetStuck(t *testing.T) {
	s := newTestSim(89)
	s.RunTicks(2000)

	start := make(map[CharID]struct{ x, y float64 })
	for i := range s.Chars {
		if s.Chars[i].Alive {
			start[CharID(i)] = struct{ x, y float64 }{s.Chars[i].Pos.X, s.Chars[i].Pos.Y}
		}
	}
	s.RunTicks(4 * TicksPerDay)

	var stuck, alive int
	for id, p := range start {
		c := &s.Chars[id]
		if !c.Alive || c.Stage() == Child {
			continue
		}
		alive++
		if c.Pos.X == p.x && c.Pos.Y == p.y {
			stuck++
		}
	}
	// A few standing still for four days is plausible; most of them is a deadlock.
	if alive > 0 && stuck*2 > alive {
		t.Errorf("%d of %d adults have not moved a millimetre in four days", stuck, alive)
	}
}

// ---- The material economy (§4.1) ----

// Labour has to follow need, or adding trades starves the village: every employer offers
// the same wage, so workers spread evenly and the fields empty (§8.1).
func TestHungerPrioritisesFarmLabour(t *testing.T) {
	s := newTestSim(97)

	// A newly founded village holds only days of food, so stock it properly first.
	for i := range s.Structs {
		if s.Structs[i].Type == Granary {
			s.Structs[i].Stock[Food] = 100000
		}
	}
	full := s.PolicyWeight(Farm)

	// Empty the granaries and see whether farm work becomes more attractive.
	for i := range s.Structs {
		s.Structs[i].Stock[Food] = 0
	}
	starving := s.PolicyWeight(Farm)

	if starving <= full {
		t.Errorf("farm work weighted %v when starving against %v when stocked; "+
			"labour has no reason to return to the fields", starving, full)
	}
	if s.PolicyWeight(Workshop) != 1 {
		t.Error("non-food work should not be reweighted by hunger")
	}
}

func TestExtractionDepletesTheGround(t *testing.T) {
	s := newTestSim(101)
	var camp StructID = NoStruct
	for i := range s.Structs {
		if s.Structs[i].Type == LumberCamp {
			camp = StructID(i)
			break
		}
	}
	if camp == NoStruct {
		t.Skip("no lumber camp was founded in this world")
	}

	before := s.World.TotalWoodland()
	for i := 0; i < 500; i++ {
		s.extract(camp, 0.01)
	}
	after := s.World.TotalWoodland()

	if after >= before {
		t.Error("felling timber did not reduce the standing woodland")
	}
	if s.Structs[camp].Stock[Wood] <= 0 {
		t.Error("the camp gained no timber from felling")
	}
	// What left the forest is what arrived at the camp.
	if cut, got := before-after, float64(s.Structs[camp].Stock[Wood]); cut-got > 0.01 || got-cut > 0.01 {
		t.Errorf("forest lost %v but the camp gained %v", cut, got)
	}
}

func TestWorkshopNeedsMaterials(t *testing.T) {
	s := newTestSim(103)
	var shop StructID = NoStruct
	for i := range s.Structs {
		if s.Structs[i].Type == Workshop {
			shop = StructID(i)
			break
		}
	}
	if shop == NoStruct {
		t.Skip("no workshop founded")
	}

	s.Structs[shop].Stock = Stock{}
	s.manufacture(shop, 1)
	if s.Structs[shop].Stock[Tools] > 0 {
		t.Error("a workshop with no materials produced tools from nothing")
	}

	s.Structs[shop].Stock[Wood] = 100
	s.Structs[shop].Stock[Iron] = 100
	s.manufacture(shop, 1)
	if s.Structs[shop].Stock[Tools] <= 0 {
		t.Error("a stocked workshop produced nothing")
	}
	if s.Structs[shop].Stock[Wood] >= 100 {
		t.Error("making tools consumed no timber")
	}
}

// Trade must not create goods or coin; every hop is an exchange.
func TestTradeConservesGoodsAndCoin(t *testing.T) {
	s := newTestSim(107)
	if len(s.Structs) < 2 {
		t.Skip("not enough structures")
	}
	a, b := StructID(0), StructID(1)
	s.Structs[a].Stock[Wood] = 50
	s.Structs[a].Gold = 10
	s.Structs[b].Gold = 100
	s.Structs[b].Stock[Wood] = 0

	goodsBefore := s.Structs[a].Stock[Wood] + s.Structs[b].Stock[Wood]
	coinBefore := s.Structs[a].Gold + s.Structs[b].Gold

	s.transact(a, b, Wood, 30, 1.5)

	goodsAfter := s.Structs[a].Stock[Wood] + s.Structs[b].Stock[Wood]
	coinAfter := s.Structs[a].Gold + s.Structs[b].Gold

	if diff := goodsAfter - goodsBefore; diff > 1e-4 || diff < -1e-4 {
		t.Errorf("trade changed total goods by %v", diff)
	}
	if diff := coinAfter - coinBefore; diff > 1e-4 || diff < -1e-4 {
		t.Errorf("trade changed total coin by %v", diff)
	}
	if s.Structs[b].Stock[Wood] <= 0 {
		t.Error("the buyer received nothing")
	}
}

func TestConstructionFinishesIntoTheRightBuilding(t *testing.T) {
	s := newTestSim(109)
	site := s.Structs[0].Cell
	sid := s.Build(Home, s.T.WrapCell(torus.Cell{X: site.X + 20, Y: site.Y + 20}))

	if s.Structs[sid].Type != BuildSite {
		t.Fatalf("Build produced a %v rather than a site", s.Structs[sid].Type)
	}
	// Hand it the materials and the labour it needs.
	s.Structs[sid].Stock = Defs[Home].BuildCost
	for i := 0; i < 200000 && s.Structs[sid].Type == BuildSite; i++ {
		s.build(sid, 1)
	}
	if s.Structs[sid].Type != Home {
		t.Errorf("site never finished; still %v at progress %v",
			s.Structs[sid].Type, s.Structs[sid].Progress)
	}
	if s.Built == 0 {
		t.Error("completion was not counted")
	}
}

// ---- Supply and demand (§4.3) ----

// Scarcity must raise a price and glut must lower it. This is the mechanism that
// allocates labour, and PolicyWeight is only a stand-in for a government that would reach
// the same conclusion by other means.
func TestPricesRespondToScarcityAndGlut(t *testing.T) {
	s := newTestSim(113)

	// Establish a demand for food, then take the stores away.
	s.demand[Food] = 40
	for i := range s.Structs {
		s.Structs[i].Stock[Food] = 0
	}
	before := s.Prices[Food]
	for d := 0; d < 20; d++ {
		s.adjustPrices()
	}
	scarce := s.Prices[Food]
	if scarce <= before {
		t.Errorf("food price fell from %v to %v while the granaries stood empty", before, scarce)
	}

	// Now flood the market.
	for i := range s.Structs {
		if s.Structs[i].Type == Granary {
			s.Structs[i].Stock[Food] = 100000
		}
	}
	s.demand[Food] = 40
	for d := 0; d < 20; d++ {
		s.demand[Food] = 40 // hold demand steady against the smoothing
		s.adjustPrices()
	}
	if s.Prices[Food] >= scarce {
		t.Errorf("food price held at %v with a hundred thousand units in store (was %v when scarce)",
			s.Prices[Food], scarce)
	}
}

// Prices must stay inside their band however extreme the shortage, or a single famine
// sends the whole economy to infinity.
func TestPricesStayWithinTheirBand(t *testing.T) {
	s := newTestSim(127)
	s.demand[Food] = 1000
	for i := range s.Structs {
		s.Structs[i].Stock[Food] = 0
	}
	for d := 0; d < 5000; d++ {
		s.adjustPrices()
	}
	if hi := s.basePrices[Food] * PriceCeiling; s.Prices[Food] > hi+1e-4 {
		t.Errorf("price ran to %v, above the ceiling %v", s.Prices[Food], hi)
	}

	for i := range s.Structs {
		s.Structs[i].Stock[Food] = 1e6
	}
	for d := 0; d < 5000; d++ {
		s.demand[Food] = 1
		s.adjustPrices()
	}
	if lo := s.basePrices[Food] * PriceFloor; s.Prices[Food] < lo-1e-4 {
		t.Errorf("price fell to %v, below the floor %v", s.Prices[Food], lo)
	}
}

// Prices move slowly. The design warns that this loop oscillates if the gain is high
// (§4.2), and a price that can double in a day is a price that will.
func TestPricesMoveSlowly(t *testing.T) {
	s := newTestSim(131)
	s.demand[Food] = 1000
	for i := range s.Structs {
		s.Structs[i].Stock[Food] = 0
	}
	before := s.Prices[Food]
	s.adjustPrices()
	if change := s.Prices[Food]/before - 1; change > PriceMaxStep+1e-6 {
		t.Errorf("price moved %.1f%% in one day against a cap of %.1f%%",
			change*100, PriceMaxStep*100)
	}
}

// Wages must differ between trades, or labour has no reason to go anywhere in particular
// and the fields empty while people mine ore nobody wants.
func TestWagesFollowRevenue(t *testing.T) {
	s := newTestSim(137)
	if len(s.Structs) < 2 {
		t.Skip("not enough structures")
	}
	var rich, poor StructID = NoStruct, NoStruct
	for i := range s.Structs {
		if Defs[s.Structs[i].Type].Jobs > 0 {
			if rich == NoStruct {
				rich = StructID(i)
			} else if poor == NoStruct {
				poor = StructID(i)
			}
		}
	}
	if poor == NoStruct {
		t.Skip("not enough employers")
	}

	s.Structs[rich].Filled, s.Structs[poor].Filled = 2, 2
	s.Structs[rich].Gold, s.Structs[poor].Gold = 0, 0
	s.Structs[rich].revenue = 500
	s.Structs[poor].revenue = 1

	for d := 0; d < 300; d++ {
		s.Structs[rich].revenue = 500
		s.Structs[poor].revenue = 1
		s.setWages()
	}
	if s.Structs[rich].Wage <= s.Structs[poor].Wage {
		t.Errorf("a trade earning 500 pays %v while one earning 1 pays %v",
			s.Structs[rich].Wage, s.Structs[poor].Wage)
	}
}

// A wage that cannot buy food should repel workers — but as a preference, never as a rule
// forcing the employer to sack anybody. Mass redundancies are what turned a price signal
// into a death spiral last time.
func TestStarvationWagesRepelWorkers(t *testing.T) {
	s := newTestSim(139)
	intoGrowingSeason(s)
	var farm StructID = NoStruct
	for i := range s.Structs {
		if s.Structs[i].Type == Farm {
			farm = StructID(i)
			break
		}
	}
	if farm == NoStruct {
		t.Skip("no farm")
	}
	worker := CharID(0)
	s.Chars[worker].Pos = s.Structs[farm].Pos
	s.Structs[farm].Filled = 0

	s.Structs[farm].Wage = s.SubsistenceWage() * 2
	good := s.scoreJob(worker, farm)
	s.Structs[farm].Wage = s.SubsistenceWage() / 4
	bad := s.scoreJob(worker, farm)

	if bad <= 0 {
		t.Error("a poorly paid job scored zero; it should be unattractive, not forbidden")
	}
	if bad >= good {
		t.Errorf("starvation wages scored %v against a living wage's %v", bad, good)
	}
	// The penalty must bite hard enough to actually move people.
	if bad > good/4 {
		t.Errorf("quarter-subsistence pay scored %v against %v; too weak to redirect labour", bad, good)
	}
}

func TestConsumptionIsRecorded(t *testing.T) {
	s := newTestSim(149)
	s.consumed = [NumResources]float32{}
	s.RunTicks(3 * TicksPerDay)
	if s.demand[Food] <= 0 {
		t.Error("three days passed and no food was recorded as eaten")
	}
}

// ---- Feeding people at their work (§5) ----

// Extraction sites go where the ore is, which may be nowhere near the granary. A kitchen
// has to follow the work, or the only way to stop people starving at remote sites is to
// forbid them the job — which caps how far a settlement can ever reach.
func TestRemoteWorkSitesGetAKitchen(t *testing.T) {
	s := newTestSim(151)
	centre := s.Structs[0].Pos

	for i := range s.Structs {
		st := &s.Structs[i]
		switch st.Type {
		case LumberCamp, Quarry, Mine:
		default:
			continue
		}
		if s.T.Dist(st.Pos, centre) <= DiningHallRange {
			continue // eats at home
		}
		hall := s.nearestOfType(st.Pos, DiningHall)
		if hall == NoStruct {
			t.Errorf("%v sits %.0f cells out with no kitchen anywhere",
				st.Type, s.T.Dist(st.Pos, centre))
			continue
		}
		if d := s.T.Dist(st.Pos, s.Structs[hall].Pos); d > DiningHallRange {
			t.Errorf("%v is %.0f cells from the nearest kitchen, limit %d",
				st.Type, d, DiningHallRange)
		}
	}
}

func TestDiningHallsSellFood(t *testing.T) {
	s := newTestSim(157)
	// Build one rather than depending on this world having founded a remote work site.
	// Whether kitchens get placed is TestRemoteWorkSitesGetAKitchen's business; this test
	// is about whether one can feed anybody, and it should not quietly skip.
	var hall StructID = NoStruct
	for i := range s.Structs {
		if s.Structs[i].Type == DiningHall {
			hall = StructID(i)
			break
		}
	}
	if hall == NoStruct {
		c, ok := s.freeCellNear(s.Structs[0].Cell, DiningHall, 6)
		if !ok {
			t.Fatal("nowhere to build a kitchen near the village")
		}
		hall = s.addStructure(DiningHall, c)
	}

	// Stand someone at a stocked kitchen and check it counts as somewhere to eat.
	s.Structs[hall].Stock[Food] = 50
	worker := CharID(0)
	s.Chars[worker].Pos = s.Structs[hall].Pos
	// Empty every granary so the kitchen is the only place selling.
	for i := range s.Structs {
		if s.Structs[i].Type == Granary {
			s.Structs[i].Stock[Food] = 0
		}
	}
	if got := s.nearestFoodSource(worker); got != hall {
		t.Errorf("nearest food source is %v, not the kitchen the worker is standing in", got)
	}

	// And a hungry worker beside it should actually be able to buy a meal.
	s.Chars[worker].Hunger = 90
	s.Chars[worker].Rations = 0
	s.Chars[worker].Gold = 100
	before := s.Structs[hall].Stock[Food]
	s.buyAndEat(worker, hall)
	if s.Structs[hall].Stock[Food] >= before {
		t.Error("the kitchen sold nothing to a starving worker standing in it")
	}
	if s.Chars[worker].Rations <= 0 {
		t.Error("the worker left the kitchen with no food")
	}
}

// The village eats first. A kitchen drawing on the granary must leave a reserve behind,
// or outlying sites starve the people at home to feed themselves.
func TestKitchensLeaveTheVillageAReserve(t *testing.T) {
	s := newTestSim(163)
	var hall, granary StructID = NoStruct, NoStruct
	for i := range s.Structs {
		switch s.Structs[i].Type {
		case DiningHall:
			if hall == NoStruct {
				hall = StructID(i)
			}
		case Granary:
			if granary == NoStruct {
				granary = StructID(i)
			}
		}
	}
	if hall == NoStruct || granary == NoStruct {
		t.Skip("no kitchen or no granary in this world")
	}

	// Put the granary just at its reserve and give the kitchen money to spend.
	s.Structs[granary].Stock[Food] = GranaryCapacity * GranaryReserveShare
	s.Structs[hall].Stock[Food] = 0
	s.Structs[hall].Gold = 10000
	before := s.Structs[granary].Stock[Food]

	for i := 0; i < 50; i++ {
		s.supplyDiningHalls()
	}
	if s.Structs[granary].Stock[Food] < before-1e-3 {
		t.Errorf("granary fell from %v to %v; kitchens ate into the village's reserve",
			before, s.Structs[granary].Stock[Food])
	}

	// With a surplus, supply should flow.
	s.Structs[granary].Stock[Food] = GranaryCapacity
	for i := 0; i < 50; i++ {
		s.supplyDiningHalls()
	}
	if s.Structs[hall].Stock[Food] <= 0 {
		t.Error("kitchen drew nothing from a full granary")
	}
	if s.Structs[hall].Stock[Food] > DiningHallStock+1e-3 {
		t.Errorf("kitchen stocked %v against a limit of %d; it is behaving like a second granary",
			s.Structs[hall].Stock[Food], DiningHallStock)
	}
}

// A forward store is a few days of meals for its crew, not a rival granary. Getting this
// wrong once reduced the village to a single survivor.
func TestKitchenStockIsSizedToItsCrew(t *testing.T) {
	crew := Defs[Mine].Jobs
	days := DiningHallStock / (float64(crew) * MealsPerDay)
	if days < 2 {
		t.Errorf("a kitchen holds %.1f days for a crew of %d; too thin to be worth building",
			days, crew)
	}
	if days > 12 {
		t.Errorf("a kitchen holds %.1f days for a crew of %d; that is a granary, not a canteen",
			days, crew)
	}
}

func BenchmarkStep(b *testing.B) {
	s := newTestSim(1)
	s.RunTicks(TicksPerYear) // settle into a steady state first
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Step()
	}
}

// ---- Vital statistics (§10, Phase 2) ----

// The Phase 2 goal is a population stable across generations. A headcount cannot tell you
// whether that is happening — it passed for hours while no child born in this village had
// ever reached adulthood, the settlement coasting entirely on its founding settlers.
//
// These thresholds are deliberately generous. A pre-industrial village might lose a third
// of its children before five and still thrive; these ask only that it does not lose
// nearly all of them, that somebody born here lives to marry, and that the village
// replaces itself. Failing them does not mean the balance wants adjusting. It means the
// society does not work.
// VitalSeeds is how many independent villages the figures are pooled over.
//
// One village produces far too few completed lives to judge. Across four variants of the
// same code the share reaching adulthood read 44%, 22%, 21% and 30% and the share ever
// marrying 29%, 60%, 25% and 0% — swings large enough to swamp any change being tested, so
// every reading was as likely to mislead as inform. Pooling several seeds is the whole
// difference between an instrument and a coin toss.
//
// The runs are independent, so they go concurrently and cost roughly one village's
// wall-clock on any machine with cores to spare. Each has its own State and its own PRNG
// stream, so determinism is untouched.
const VitalSeeds = 6

func TestVitalStatisticsArePlausible(t *testing.T) {
	acceptance(t)
	pooled := make([][]Life, VitalSeeds)
	var wg sync.WaitGroup
	for i := 0; i < VitalSeeds; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := newTestSim(int64(7 + i*101))
			s.RunTicks(100 * TicksPerYear)
			pooled[i] = s.Lives
		}(i)
	}
	wg.Wait()

	// Collected in seed order rather than completion order, so the pool is identical from
	// run to run however the goroutines happen to be scheduled.
	//
	// Only lives BEGUN in the first twenty years are judged — a near-complete cohort,
	// everyone dead or past eighty by the horizon. Judging all completed lives was
	// right-censoring: once the population began genuinely growing, most people ever
	// born were alive and uncounted, the dead skewed young, and this test read a booming
	// village as one where "adults die young" — pessimism manufactured by success, the
	// exact bias recorded when R0 was retired as the growth metric.
	var lives []Life
	for _, ls := range pooled {
		for _, l := range ls {
			if l.Born <= Tick(20*TicksPerYear) {
				lives = append(lives, l)
			}
		}
	}
	v := VitalsOf(lives)
	t.Logf("pooled over %d villages, first-twenty-years cohort", VitalSeeds)

	// Only skip when there is genuinely nothing to judge. A high bar here would hide the
	// very failure this test exists to catch: a village where almost nobody born lives
	// long enough to complete a life produces few records precisely because it is failing.
	if v.Lives < 4 {
		t.Skipf("only %d completed lives born here after sixty years, which is itself the "+
			"finding: the village is not producing lives to measure", v.Lives)
	}
	t.Log("\n" + v.String())

	if v.ReachedAdulthood < 0.35 {
		t.Errorf("only %.0f%% of those born here reach adulthood; a village losing two "+
			"thirds of its children cannot sustain itself", v.ReachedAdulthood*100)
	}
	if v.ExpectancyAtBirth < 15 {
		t.Errorf("life expectancy at birth is %.1f years", v.ExpectancyAtBirth)
	}
	if v.ExpectancyAt15 < 35 {
		t.Errorf("those who reach fifteen live to only %.1f; adults are dying young",
			v.ExpectancyAt15)
	}
	if v.EverMarried < 0.4 {
		t.Errorf("only %.0f%% of adults born here ever marry", v.EverMarried*100)
	}
	if v.ChildrenPerLife < 2 {
		t.Errorf("%.2f children per completed life; below replacement, so the village "+
			"lives on its founders and dies with them", v.ChildrenPerLife)
	}
	// The dominant cause of death says which system is failing.
	if v.ByCause[CauseHunger] > v.AllDeaths/2 {
		t.Errorf("hunger killed %d of %d; the village cannot feed the people it produces",
			v.ByCause[CauseHunger], v.AllDeaths)
	}
}
