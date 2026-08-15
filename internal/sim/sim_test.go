package sim

import (
	"testing"

	"github.com/solifugus/serts/internal/torus"
	"github.com/solifugus/serts/internal/worldgen"
)

func testWorld(seed int64) *worldgen.World {
	p := worldgen.DefaultParams(seed)
	p.CX, p.CY = 128, 128
	return worldgen.Generate(p)
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
			if s.Structs[j].Food < -1e-4 {
				t.Fatalf("structure %d has negative food %v at tick %d", j, s.Structs[j].Food, s.Tick)
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

func TestJobScoringRejectsImpossibleWork(t *testing.T) {
	s := newTestSim(53)
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

// The question this milestone exists to answer. The village is not yet demographically
// stable over a full lifetime — see the note in the package documentation — but it must
// at least run its economy without collapsing.
func TestVillageSurvivesItsFirstDecades(t *testing.T) {
	s := newTestSim(7)
	s.RunTicks(20 * TicksPerYear)

	st := s.Stats()
	if st.Population == 0 {
		t.Fatal("the village died out within twenty years")
	}
	if st.Population < 15 {
		t.Errorf("population fell to %d after twenty years, from 24 settlers", st.Population)
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
			granaryFood += s.Structs[i].Food
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
	s.kill(victim)

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
	s.quitJob(worker)
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

func BenchmarkStep(b *testing.B) {
	s := newTestSim(1)
	s.RunTicks(TicksPerYear) // settle into a steady state first
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Step()
	}
}
