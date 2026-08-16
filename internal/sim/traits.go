package sim

// Personality.
//
// Until now every character was identical. Given the same wage, the same distance and the
// same larder they made the same choice, which meant the village had one mind wearing
// thirty bodies — and it failed the way a monoculture fails. The recorded instance: food
// is cheapest when the barns are full, so a rule that sent people out of underpaid work
// emptied the farms in exactly the season farming mattered. Not because the rule was
// wrong, but because it fired for every farmer on the same afternoon.
//
// Traits are the fix. They are not decoration and they are not a personality system in the
// sense of a questionnaire: each one is a per-character multiplier on a term the decision
// already contained (§3.5), so a trait can only make somebody weigh something they were
// already weighing. Nothing new enters the model; the population simply stops agreeing.
//
// The Big Five was considered and rejected for this. It is a factor model of how
// personality is *described*, not of how decisions are *made*, and three of its five
// dimensions have nothing in this simulation to attach to — Extraversion needs a social
// graph and Agreeableness needs cooperation and defection, neither of which exists yet. A
// field with nothing behind it is worse than an absent one, because it reads as
// implemented. These five come the other way round: from the terms in the utility
// function, outward.
//
// Traits are heritable. That makes them an instrument as well as a mechanism — if the
// village is failing and traits are under selection, the lineages that last say what the
// viable strategy was, which is a better source of constants than another confident guess.
type Traits struct {
	// Patience is how long a wage that cannot feed you is tolerated before you walk away
	// from the job entirely. See leanFor.
	Patience float32
	// Caution is how heavily danger is discounted when judging work. A cautious man wants
	// more money to go down a mine; a reckless one barely notices the difference. This is
	// where compensating differentials stop being uniform.
	Caution float32
	// Diligence is how much slack a household keeps: how deep a larder is stocked and how
	// hard the garden is worked of an evening.
	Diligence float32
	// Rootedness is how sharply distance counts against a job. It is what decides who
	// leaves a dying settlement and who stays until the last (§3.7).
	Rootedness float32
	// Ambition is how small an improvement will move somebody already in work.
	Ambition float32
}

// TraitSpread is how far personality varies around the mean of one.
//
// Deliberately modest. The traits multiply terms that are themselves multiplied together,
// so the spread compounds; at a third either way the most and least cautious characters
// already differ by about a factor of two in what they demand to take dangerous work,
// which is as wide as any real population.
const TraitSpread = 0.33

// TraitMin and TraitMax bound a trait after inheritance, so that drift across many
// generations cannot produce someone with no caution at all — which would divide by
// something near zero downstream.
const (
	TraitMin = 0.35
	TraitMax = 2.0
)

// rollTraits draws a fresh personality, for the founding settlers who have no parents.
func rollTraits(r *Rand) Traits {
	draw := func() float32 { return float32(r.Range(1-TraitSpread, 1+TraitSpread)) }
	return Traits{
		Patience:   draw(),
		Caution:    draw(),
		Diligence:  draw(),
		Rootedness: draw(),
		Ambition:   draw(),
	}
}

// inheritTraits draws a child's personality near the average of its parents.
//
// Blending inheritance with fresh noise, which is not genetics but is the right shape: a
// child resembles its parents without being either of them, and a trait under selection
// can move across generations without the population collapsing onto a single value.
func inheritTraits(r *Rand, a, b Traits) Traits {
	mix := func(x, y float32) float32 {
		mid := (x + y) / 2
		return clampTrait(mid + float32(r.Range(-TraitDrift, TraitDrift)))
	}
	return Traits{
		Patience:   mix(a.Patience, b.Patience),
		Caution:    mix(a.Caution, b.Caution),
		Diligence:  mix(a.Diligence, b.Diligence),
		Rootedness: mix(a.Rootedness, b.Rootedness),
		Ambition:   mix(a.Ambition, b.Ambition),
	}
}

// TraitDrift is how far a child may fall from the parental mean. Small against the
// founding spread, so a lineage has a recognisable character across several generations
// rather than being redrawn every birth.
const TraitDrift = 0.12

func clampTrait(v float32) float32 {
	if v < TraitMin {
		return TraitMin
	}
	if v > TraitMax {
		return TraitMax
	}
	return v
}

// PatienceDays is how long a wage below subsistence is borne, for someone of average
// patience, before they give the job up.
//
// The unit matters more than the number. An earlier attempt tested the *current* wage and
// sent people out of work the moment it dipped, which emptied the farms every time a good
// harvest made food cheap. What distinguishes a job that pays badly this week from one
// that does not pay is how long it has gone on, so the measure is duration and not level.
// A season and a half is long enough to sit out a glut and short enough that a genuinely
// dead trade does not hold its workers until they starve in it.
const PatienceDays = 45

// leanTolerance is how long this character in particular will bear it.
func (s *State) leanTolerance(id CharID) Tick {
	return Tick(float32(PatienceDays*TicksPerDay) * s.Chars[id].Traits.Patience)
}

// reviewWage updates how long a character's pay has been failing to feed them, and gives
// the job up once their patience is spent.
//
// Quitting rather than merely preferring other work is the point. The design's money
// faucet opens on unemployment — the jobless pan for gold (§4.2) — and until now that
// faucet could not see the person it was invented for, because a man holding a job that
// pays nothing is not unemployed. Patience is what converts one into the other.
func (s *State) reviewWage(id CharID, elapsed Tick) {
	c := &s.Chars[id]
	if c.Job == NoStruct {
		c.leanFor = 0
		return
	}
	sub := s.SubsistenceWage()
	if sub <= 0 {
		return
	}
	if s.Structs[c.Job].Wage >= sub {
		// Recovery is faster than decline. A trade that comes good again keeps the people
		// who stayed with it, rather than losing them to a debt of past bad years.
		c.leanFor -= elapsed * 2
		if c.leanFor < 0 {
			c.leanFor = 0
		}
		return
	}
	c.leanFor += elapsed
	if c.leanFor >= s.leanTolerance(id) {
		s.quitJob(id)
		c.leanFor = 0
	}
}
