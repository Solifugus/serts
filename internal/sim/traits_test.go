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

// Each trait must come intact from one parent or the other, give or take a mutation.
func TestTraitsSegregateFromParents(t *testing.T) {
	r := NewRand(3, 9)
	a := Traits{Patience: 0.7, Caution: 0.7, Diligence: 0.7, Rootedness: 0.7, Ambition: 0.7}
	b := Traits{Patience: 1.3, Caution: 1.3, Diligence: 1.3, Rootedness: 1.3, Ambition: 1.3}

	var fromA, fromB, mutants int
	const n = 2000
	for i := 0; i < n; i++ {
		c := inheritTraits(r, a, b)
		for j := 0; j < NumTraits; j++ {
			v := *c.at(j)
			switch {
			case v == 0.7:
				fromA++
			case v == 1.3:
				fromB++
			default:
				mutants++
			}
			// Blending would put children between the parents. Nothing may land there
			// except by mutation, which cannot exceed MutationSize.
			if v > 0.7+MutationSize+1e-4 && v < 1.3-MutationSize-1e-4 {
				t.Fatalf("child trait %.3f sits between the parents; inheritance is blending", v)
			}
		}
	}
	// Both parents should contribute about equally.
	if fromA < fromB/2 || fromB < fromA/2 {
		t.Errorf("parents contributed unevenly: %d from one, %d from the other", fromA, fromB)
	}
	// Mutation should be present but rare.
	if got := float64(mutants) / float64(n*NumTraits); got < 0.01 || got > 0.10 {
		t.Errorf("mutation rate %.3f per trait is outside the intended range", got)
	}
}

// The property blending destroys, and the reason for changing to segregation: a population
// must not converge on one temperament. Without variance there is nothing for selection to
// act on, however strongly the world rewards one value over another.
func TestInheritancePreservesVariance(t *testing.T) {
	r := NewRand(4, 1)

	pop := make([]Traits, 400)
	for i := range pop {
		pop[i] = rollTraits(r)
	}
	start := spreadOf(pop)

	// Twenty generations of random mating, with no selection at all.
	for gen := 0; gen < 20; gen++ {
		next := make([]Traits, len(pop))
		for i := range next {
			next[i] = inheritTraits(r, pop[r.Intn(len(pop))], pop[r.Intn(len(pop))])
		}
		pop = next
	}
	end := spreadOf(pop)

	// Blending halves the spread every generation and settles near half the founding
	// value; segregation should hold it. Allow real drift, but not collapse.
	if end < start*0.75 {
		t.Errorf("spread fell from %.3f to %.3f over twenty generations; inheritance is losing variation", start, end)
	}
}

// spreadOf is the mean standard deviation across all five traits.
func spreadOf(pop []Traits) float32 {
	var total float32
	for j := 0; j < NumTraits; j++ {
		var sum float32
		for i := range pop {
			sum += *pop[i].at(j)
		}
		mean := sum / float32(len(pop))
		var ss float32
		for i := range pop {
			d := *pop[i].at(j) - mean
			ss += d * d
		}
		total += sqrt32(ss / float32(len(pop)))
	}
	return total / NumTraits
}

func sqrt32(v float32) float32 {
	if v <= 0 {
		return 0
	}
	x := v
	for i := 0; i < 24; i++ {
		x = 0.5 * (x + v/x)
	}
	return x
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
