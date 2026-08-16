package sim

import "testing"

// The point of traits is that people differ. If a wiring mistake ever left everyone on the
// default the simulation would still run, still pass every other test, and quietly be the
// monoculture the traits were added to break — so the variance itself needs asserting.
func TestPersonalityActuallyVaries(t *testing.T) {
	s := newTestSim(11)

	var lo, hi Traits
	first := true
	for i := range s.Chars {
		if !s.Chars[i].Alive {
			continue
		}
		tr := s.Chars[i].Traits
		if first {
			lo, hi, first = tr, tr, false
			continue
		}
		lo = Traits{minf(lo.Patience, tr.Patience), minf(lo.Caution, tr.Caution),
			minf(lo.Diligence, tr.Diligence), minf(lo.Rootedness, tr.Rootedness),
			minf(lo.Ambition, tr.Ambition)}
		hi = Traits{maxf(hi.Patience, tr.Patience), maxf(hi.Caution, tr.Caution),
			maxf(hi.Diligence, tr.Diligence), maxf(hi.Rootedness, tr.Rootedness),
			maxf(hi.Ambition, tr.Ambition)}
	}
	if first {
		t.Fatal("no settlers to inspect")
	}

	for _, c := range []struct {
		name   string
		lo, hi float32
	}{
		{"patience", lo.Patience, hi.Patience},
		{"caution", lo.Caution, hi.Caution},
		{"diligence", lo.Diligence, hi.Diligence},
		{"rootedness", lo.Rootedness, hi.Rootedness},
		{"ambition", lo.Ambition, hi.Ambition},
	} {
		if c.hi-c.lo < 0.2 {
			t.Errorf("%s barely varies across the settlers: %.2f to %.2f", c.name, c.lo, c.hi)
		}
		if c.lo < 1-TraitSpread-1e-4 || c.hi > 1+TraitSpread+1e-4 {
			t.Errorf("%s left the founding range: %.2f to %.2f", c.name, c.lo, c.hi)
		}
	}
}

// Children must resemble their parents without being clones of them, and must stay inside
// the bounds however many generations pass.
func TestTraitsAreHeritableAndBounded(t *testing.T) {
	r := NewRand(3, 9)
	parent := Traits{Patience: 1.6, Caution: 1.6, Diligence: 1.6, Rootedness: 1.6, Ambition: 1.6}

	var sum float32
	const n = 400
	for i := 0; i < n; i++ {
		c := inheritTraits(r, parent, parent)
		if c.Patience < TraitMin || c.Patience > TraitMax {
			t.Fatalf("child out of bounds: %.2f", c.Patience)
		}
		if d := c.Patience - parent.Patience; d > TraitDrift+1e-4 || d < -TraitDrift-1e-4 {
			t.Fatalf("child fell %.2f from the parental mean, further than TraitDrift", d)
		}
		sum += c.Patience
	}
	// Two identical parents should produce children centred on them, not drifting toward
	// the population mean — otherwise selection could never move anything.
	if mean := sum / n; mean < 1.55 || mean > 1.65 {
		t.Errorf("children of patient parents averaged %.2f, not near the parental 1.60", mean)
	}

	// And many generations of drift must not escape the bounds.
	cur := parent
	for i := 0; i < 5000; i++ {
		cur = inheritTraits(r, cur, cur)
	}
	if cur.Patience < TraitMin || cur.Patience > TraitMax {
		t.Errorf("drift escaped the bounds after 5000 generations: %.2f", cur.Patience)
	}
}

// Patience is a duration, not a threshold. The distinction is the whole reason the earlier
// attempt failed: a dip in the wage must be survivable, and only a wage that stays below
// subsistence should cost somebody their job.
func TestPatienceIsSpentOverTimeNotAtOnce(t *testing.T) {
	s := newTestSim(12)
	s.RunTicks(TicksPerDay * 5)

	// Find somebody in work.
	var id CharID = NoChar
	for i := range s.Chars {
		if s.Chars[i].Alive && s.Chars[i].Job != NoStruct {
			id = CharID(i)
			break
		}
	}
	if id == NoChar {
		t.Skip("nobody in work to test")
	}

	job := s.Chars[id].Job
	sub := s.SubsistenceWage()
	s.Structs[job].Wage = sub / 4 // a wage that plainly cannot feed anyone

	// One review must not cost the job, however bad the wage.
	s.reviewWage(id, JobStagger)
	if s.Chars[id].Job != job {
		t.Fatal("a single bad review cost somebody their job; patience is behaving as a threshold")
	}

	// Sustained, it must — and within a season or so of their own tolerance.
	tol := s.leanTolerance(id)
	var spent Tick
	for spent < tol*2 && s.Chars[id].Job == job {
		s.Structs[job].Wage = sub / 4
		s.reviewWage(id, JobStagger)
		spent += JobStagger
	}
	if s.Chars[id].Job == job {
		t.Fatalf("still in unpayable work after %d ticks, past a tolerance of %d", spent, tol)
	}
	if spent < tol/2 {
		t.Errorf("gave up after %d ticks, well short of a tolerance of %d", spent, tol)
	}
}

// A trade that comes good again keeps the people who stuck with it.
func TestGoodYearsRestorePatience(t *testing.T) {
	s := newTestSim(13)
	s.RunTicks(TicksPerDay * 5)

	var id CharID = NoChar
	for i := range s.Chars {
		if s.Chars[i].Alive && s.Chars[i].Job != NoStruct {
			id = CharID(i)
			break
		}
	}
	if id == NoChar {
		t.Skip("nobody in work to test")
	}
	job := s.Chars[id].Job
	sub := s.SubsistenceWage()

	// Alternate lean and good spells for far longer than anyone's patience. Nobody should
	// lose a job to a trade that pays properly half the time.
	tol := s.leanTolerance(id)
	for spent := Tick(0); spent < tol*4; spent += JobStagger * 2 {
		s.Structs[job].Wage = sub / 4
		s.reviewWage(id, JobStagger)
		s.Structs[job].Wage = sub * 2
		s.reviewWage(id, JobStagger)
	}
	if s.Chars[id].Job != job {
		t.Error("quit a trade that paid well half the time; recovery is not clearing the lean years")
	}
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
