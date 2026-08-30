package sim

import "testing"

// Roads must not create or destroy coin. The hall pays its works fund to the quarry for
// the stone, which is a transfer — and TotalCoin counts Works, so a leak here would show.
func TestPavingConservesGoldAndStone(t *testing.T) {
	s := newTestSim(5)
	start := s.TotalCoin()
	s.RunTicks(6 * TicksPerYear)
	now := s.TotalCoin()
	if tol := start * 0.001; now > start+tol || now < start-tol {
		t.Errorf("total coin moved from %.2f to %.2f while paving", start, now)
	}
}

// Traffic must actually accumulate where people walk, or paving has nothing to go on.
func TestTrafficRecordsWhereTheyWalk(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(2 * TicksPerYear)
	var busy, crossed int
	for _, n := range s.traffic {
		if n > 0 {
			crossed++
		}
		if n > RoadTrafficFloor {
			busy++
		}
	}
	if crossed == 0 {
		t.Fatal("two years of walking left no traffic anywhere")
	}
	t.Logf("%d cells crossed, %d above the paving floor", crossed, busy)
}

// A road makes the person standing on it faster, and only them.
func TestRoadsSpeedTravel(t *testing.T) {
	s := newTestSim(5)
	if got := s.roadFactor(0); got != 1 {
		t.Errorf("unpaved ground gave a factor of %v", got)
	}
	// Road holds a CONDITION now, not a flag: fresh stone is RoadNew and anything at or
	// below RoadWorn has gone back to being a track.
	s.Road[0] = RoadNew
	if got := s.roadFactor(0); got != RoadSpeed {
		t.Errorf("fresh paving gave %v, want %v", got, RoadSpeed)
	}
	s.Road[0] = RoadWorn
	if got := s.roadFactor(0); got != 1 {
		t.Errorf("a worn-out surface still gave %v; it should be no better than grass", got)
	}
	// And in between, the advantage tapers rather than switching off.
	s.Road[0] = (RoadNew + RoadWorn) / 2
	if got := s.roadFactor(0); got <= 1 || got >= RoadSpeed {
		t.Errorf("a half-worn road gave %v, want something between 1 and %v", got, RoadSpeed)
	}
	s.Road[0] = RoadNew
	if got := s.roadFactor(1); got != 1 {
		t.Errorf("paving one cell sped up the next: %v", got)
	}
}

// Determinism is the property the whole design rests on, and paving sorts a list of
// hundreds of cells by traffic. Ties must break on index or two identical worlds diverge.
func TestPavingIsDeterministic(t *testing.T) {
	a, b := newTestSim(9), newTestSim(9)
	a.RunTicks(4 * TicksPerYear)
	b.RunTicks(4 * TicksPerYear)
	if a.RoadsLaid != b.RoadsLaid {
		t.Fatalf("same seed paved %d and %d cells", a.RoadsLaid, b.RoadsLaid)
	}
	for i := range a.Road {
		if a.Road[i] != b.Road[i] {
			t.Fatalf("same seed paved different ground at cell %d", i)
		}
	}
}

// Roads must wear out, or the works fund buys a permanent asset for a one-off price and
// a network is never a burden. They must also come back to grass when abandoned.
func TestRoadsWearAndRevertWhenAbandoned(t *testing.T) {
	s := newTestSim(5)
	// A paved cell nobody uses should decay to nothing on weather alone.
	idx := 0
	s.Road[idx] = RoadNew
	s.traffic[idx] = 0
	before := s.Road[idx]

	years := 0
	for ; years < 60 && s.Road[idx] > 0; years++ {
		s.Tick += TicksPerYear
		s.stepRoads()
	}
	if s.Road[idx] != 0 {
		t.Errorf("after %d years of neglect the surface is still %d (started %d)",
			years, s.Road[idx], before)
	}
	if years < 5 {
		t.Errorf("paving crumbled in %d years, which is not a road", years)
	}
	t.Logf("an unused road returned to grass in %d years", years)
}
