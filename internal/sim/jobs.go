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
	//
	// It is also a subsistence limit, not merely a performance one. Food is sold at the
	// village and nowhere else, so a job far enough out that a worker cannot get back to
	// restock is a job that kills them. Miners were taking well-paid work forty cells
	// from the only granary and starving at the diggings while the village's average
	// hunger looked perfectly healthy. Feeding people at their work is what the design's
	// mobile kitchen is for (§5); until there is one, the radius stands in for it.
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

	// wage: what the structure offers, funded from what it earns (§4.3).
	wage := float64(st.Wage)
	if wage <= 0 {
		return 0
	}

	// Work that cannot feed you is worth little, and the discount steepens the further
	// below subsistence it falls. This is deliberately a preference and not a rule: the
	// worker declines, the employer is not compelled to sack anybody, so a struggling
	// trade empties gradually through the labour market instead of collapsing in a day.
	if sub := s.SubsistenceWage(); sub > 0 {
		if afford := wage / float64(sub); afford < 1 {
			wage *= afford * afford
		}
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

	// danger: nobody takes a job that is likely to kill them for the same money as one
	// that is not. This is what makes a mine pay more than a granary without anything
	// deciding that it should — the wage has to rise until it offsets the risk.
	safety := 1.0
	if d := Danger[st.Type]; d > 0 {
		safety = 1 / (1 + d*DangerAversion)
	}

	// policy_weight: the player's thumb on the scale (§8.1). No player yet, so the
	// hook exists at neutral rather than being absent.
	policy := s.PolicyWeight(st.Type)

	return wage * skillFit * distance * urgency * policy * safety
}

// DangerAversion is how strongly people discount risky work. At this value a trade with a
// two per cent annual chance of killing you must pay about a third more to be as
// attractive as a safe one.
const DangerAversion = 20.0

// PolicyWeight is the faction's preference for a kind of work — the player's thumb on
// the labour market's scale (§8.1).
//
// There is no player yet, so this stands in for one with the decision any sane government
// would make: when the granaries are running down, put people in the fields. Without it,
// adding trades to the village was actively fatal. Every employer offered the same wage,
// so labour spread evenly across whatever jobs existed, farms ended up half-staffed, and
// a village that had fed itself for a century starved while its people mined iron nobody
// had any use for.
//
// A real economy would signal this through wages — scarce food raises food prices, which
// raises what a farm can pay, which pulls labour back. Wage discovery of that kind was
// tried in the previous milestone and oscillated destructively (§4.3), so for now the
// faction expresses the same judgement directly.
func (s *State) PolicyWeight(t StructType) float64 {
	switch t {
	case Farm, Granary:
		switch days := s.FoodDays(); {
		case days < 20:
			return 6 // hungry: nothing else matters
		case days < 45:
			return 2.5
		case days < 90:
			return 1.3
		}
		return 0.8 // well stocked; the fields can spare a pair of hands
	}
	return 1
}

// FoodDays is how many days the faction could feed itself from what it holds.
func (s *State) FoodDays() float64 {
	pop := s.Population()
	if pop == 0 {
		return 0
	}
	need := float64(pop) * MealsPerDay
	if need <= 0 {
		return 0
	}
	return float64(s.TotalFood()) / need
}

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
		s.seekWork(CharID(i))
	}
}

// JobSwitchGain is how much better an offer must be before somebody already in work will
// leave for it. Below this they stay put.
//
// Some friction is essential or people thrash between near-identical jobs every time a
// price twitches. Too much and the labour market seizes: with switching forbidden
// altogether, a village could have farms offering fifty times subsistence and standing
// empty, because everyone already had a job and nobody was looking. Prices moved, wages
// moved, and not one person did.
const JobSwitchGain = 1.4

// seekWork finds the best job available to a character and takes it if it is worth taking.
//
// Everyone reconsiders, not only the unemployed. This is the step that makes the price
// signal mean anything: a trade whose goods have become dear can pay more, and paying more
// has to actually pull people out of other work or the whole chain from scarcity to price
// to wage to labour ends in a wage nobody responds to.
func (s *State) seekWork(id CharID) {
	c := &s.Chars[id]

	best, bestScore := NoStruct, 0.0
	for sid := range s.Structs {
		sc := s.scoreJob(id, StructID(sid))
		// Ties break toward the lower ID so the outcome does not depend on iteration
		// order changing between runs.
		if sc > bestScore {
			best, bestScore = StructID(sid), sc
		}
	}
	if best == NoStruct {
		if c.Job == NoStruct {
			c.Activity = SeekingWork
		}
		return
	}

	if c.Job != NoStruct {
		// Already in work: only move for a materially better offer. Skill is held per
		// trade (§3.4), so scoreJob already discounts a change of trade by the tenure it
		// would waste — an old hand is genuinely harder to poach than a novice.
		if bestScore < s.currentJobScore(id)*JobSwitchGain {
			return
		}
		s.quitJob(id)
	}

	c.Job = best
	c.Tenure = 0
	s.Structs[best].Filled++
	c.Activity = GoingToWork
	c.dest = best
}

// currentJobScore values the job a character already holds, on the same terms as any
// other offer. Their own post has no vacancy, so it must be scored directly.
func (s *State) currentJobScore(id CharID) float64 {
	c := &s.Chars[id]
	if c.Job == NoStruct {
		return 0
	}
	s.Structs[c.Job].Filled-- // score it as though the post were open
	score := s.scoreJob(id, c.Job)
	s.Structs[c.Job].Filled++
	return score
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
		if !st.Alive || st.Type != Home || st.Residents >= st.Capacity() {
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
		if !st.Alive || st.Type != Home || st.Residents >= st.Capacity() {
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
