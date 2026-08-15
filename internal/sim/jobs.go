package sim

import (
	"math"

	"github.com/solifugus/serts/internal/torus"
)

// The employment market (design §3.5).
//
// This is the engine of the whole simulation and deserves more care than anything else
// in the package. Labour shortages, wage competition, economic migration, and the slow
// bleed of an employer who underpays are not written anywhere: they are all consequences
// of characters repeatedly answering one question — of the work I can reach, which is
// worth taking?

const (
	// JobSearchRadius bounds how far someone will look for work, in cells. Without it
	// the market is O(unemployed x structures) across the whole map, which §9.4 names as
	// the worst hot spot in the design.
	JobSearchRadius = 40

	// JobStagger spreads re-evaluation across ticks. A character reconsiders their
	// employment only when tick % JobStagger == id % JobStagger, which cuts the cost by
	// this factor and stays perfectly deterministic — unlike anything keyed to the
	// camera or the wall clock. Sized in ticks per in-world hour so the in-world cadence
	// of job-hunting does not change when the clock does.
	JobStagger = TicksPerHour * 2
)

// SkillGain is the tenure constant from §3.4: efficiency = 1 + k*ln(1 + tenure/T).
const (
	SkillK = 0.55
	SkillT = 6.0 // in-world years
)

// efficiency converts accumulated tenure at a structure type into an output multiplier.
//
// The logarithm is the point: most of the gain arrives in the first few years and then
// tails off, so a veteran is markedly better than a novice but not unboundedly so, and
// losing one hurts in a way that is felt rather than announced (§3.4).
func efficiency(tenure float32) float64 {
	return 1 + SkillK*math.Log(1+float64(tenure)/SkillT)
}

// scoreJob evaluates one structure as an employer for one character.
//
// The terms are multiplied rather than summed so that a zero in any of them rules the
// job out entirely: unreachable work, or work with no vacancy, is not merely unattractive
// but impossible.
func (s *State) scoreJob(id CharID, sid StructID) float64 {
	c := &s.Chars[id]
	st := &s.Structs[sid]

	if !st.Alive || st.Openings() <= 0 || Defs[st.Type].Jobs == 0 {
		return 0
	}

	// wage: what the structure offers, funded from the treasury.
	wage := float64(st.Wage)
	if wage <= 0 {
		return 0
	}

	// skill_fit: accumulated efficiency at this kind of work.
	skillFit := efficiency(c.Skill[st.Type])

	// distance_penalty: decays with travel, measured on the torus.
	d := s.T.Dist(c.Pos, st.Pos)
	if d > JobSearchRadius {
		return 0
	}
	distance := 1 / (1 + d/12)

	// need_urgency: a starving character takes worse work; a comfortable one holds out.
	urgency := 1.0
	if c.Hunger > 55 {
		urgency = 1 + float64(c.Hunger-55)/45
	}

	// policy_weight: the player's thumb on the scale (§8.1). No player yet, so the
	// hook exists at neutral rather than being absent.
	policy := s.PolicyWeight(st.Type)

	return wage * skillFit * distance * urgency * policy
}

// PolicyWeight is the player's preference for a kind of work. Returns 1 until there is a
// player to express one.
func (s *State) PolicyWeight(StructType) float64 { return 1 }

// stepJobs lets a staggered slice of the unemployed look for work.
func (s *State) stepJobs() {
	phase := int(s.Tick) % JobStagger

	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Stage() == Child {
			continue
		}
		if i%JobStagger != phase {
			continue
		}
		// Only the unemployed look. Poaching the employed — the mechanism by which a
		// better offer pulls people across factions (§3.6) — needs rival employers to
		// be meaningful, and arrives with them.
		if c.Job != NoStruct {
			continue
		}
		s.seekWork(CharID(i))
	}
}

// seekWork finds the best available job for a character and takes it.
func (s *State) seekWork(id CharID) {
	best, bestScore := NoStruct, 0.0
	for sid := range s.Structs {
		sc := s.scoreJob(id, StructID(sid))
		// Ties break toward the lower ID so the outcome does not depend on iteration
		// order changing between runs.
		if sc > bestScore {
			best, bestScore = StructID(sid), sc
		}
	}
	c := &s.Chars[id]
	if best == NoStruct {
		c.Activity = SeekingWork
		return
	}
	c.Job = best
	c.Tenure = 0
	s.Structs[best].Filled++
	c.Activity = GoingToWork
	c.dest = best
}

// quitJob releases a character's employment.
func (s *State) quitJob(id CharID) {
	c := &s.Chars[id]
	if c.Job == NoStruct {
		return
	}
	s.Structs[c.Job].Filled--
	c.Job = NoStruct
	c.Tenure = 0
	c.Activity = SeekingWork
}

// findHomeWithRoom returns the nearest home that can take another resident.
func (s *State) findHomeWithRoom(pos torus.Vec2) StructID {
	best, bestD := NoStruct, math.MaxFloat64
	for sid := range s.Structs {
		st := &s.Structs[sid]
		if !st.Alive || st.Type != Home || st.Residents >= Defs[Home].Capacity {
			continue
		}
		if d := s.T.Dist2(pos, st.Pos); d < bestD {
			best, bestD = StructID(sid), d
		}
	}
	return best
}

// assignHome puts a character in the nearest home with room (§5).
//
// Unhoused characters lose health, so this is not a cosmetic assignment: a village that
// builds too few homes kills people slowly.
func (s *State) assignHome(id CharID) {
	c := &s.Chars[id]
	if c.Home != NoStruct {
		return
	}
	best, bestD := NoStruct, math.MaxFloat64
	for sid := range s.Structs {
		st := &s.Structs[sid]
		if !st.Alive || st.Type != Home || st.Residents >= Defs[Home].Capacity {
			continue
		}
		if d := s.T.Dist2(c.Pos, st.Pos); d < bestD {
			best, bestD = StructID(sid), d
		}
	}
	if best != NoStruct {
		c.Home = best
		c.housed = true
		s.Structs[best].Residents++
	}
}

// Unemployment returns the share of working-age adults without a job.
//
// Unemployment is a first-class event in this design, not an edge case: it is what a
// collapsing faction actually looks like (§3.5).
func (s *State) Unemployment() float64 {
	var adults, jobless int
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Stage() == Child {
			continue
		}
		adults++
		if c.Job == NoStruct {
			jobless++
		}
	}
	if adults == 0 {
		return 0
	}
	return float64(jobless) / float64(adults)
}
