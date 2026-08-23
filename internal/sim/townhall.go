package sim

import "github.com/solifugus/serts/internal/torus"

// The town hall (§8.1a): where policy becomes a building.
//
// Everything here follows two rules paid for in measurement. Money is conserved — every
// coin the hall holds was levied or escheated, every coin it spends lands in a pocket —
// and relief moves money, never people (Appendix A.5): eight behavioural interventions
// failed by displacing labour hours, so the hall's transfers touch no one's day.
//
// The standing defaults below are the same kind of placeholder PolicyWeight was: a sane
// government's choices, documented, waiting to become the player's dials in Phase 3.

const (
	// LevyFloor is the wealth below which a household member pays nothing. Generous on
	// purpose: subsistence earners and modest savers are exempt; hoards pay. Sized well
	// above the largest personal reserve any behaviour requires.
	LevyFloor = 150.0

	// LevyRatePerYear is the annual rate on wealth above the floor. The levy is the
	// institutionalised alternative to the mob — the civilised form of what every
	// society applies to hoarded wealth one way or another — and at two per cent it
	// erodes a stagnant hoard within a generation without touching working capital.
	LevyRatePerYear = 0.02
	LevyRatePerDay  = LevyRatePerYear / DaysPerYear

	// ReliefShare is the dole against the current subsistence wage: the less-eligibility
	// dial (§8.1a). Two-thirds keeps relief below the worst-paid work, so the dole never
	// outbids a job — the same principle §4.2 applies to panning.
	ReliefShare = 0.67

	// ReliefFloor is how poor an adult must be before the hall pays: less than a few
	// days of eating in hand.
	ReliefFloor = 2.0

	// ClerkWagePremium sets hall wages against subsistence. Like a build site, the hall
	// has a purse rather than a trade, so its wage is declared rather than derived —
	// revenue-derived wages on a treasury would pay clerks like kings.
	ClerkWagePremium = 1.2
)

// stepTownHall runs the civic day: levy, then relief, in that order so a day's take can
// fund a day's dole.
func (s *State) stepTownHall() {
	var halls []StructID
	for i := range s.Structs {
		if s.Structs[i].Alive && s.Structs[i].Type == TownHall {
			halls = append(halls, StructID(i))
		}
	}
	if len(halls) == 0 {
		return
	}
	// Each person answers to their nearest hall (§2.7a brings a second one): a
	// colonist's levy funds the colony's relief, not the mother's.
	nearest := func(pos torus.Vec2) *Structure {
		best, bestD := halls[0], s.T.Dist(pos, s.Structs[halls[0]].Pos)
		for _, hid := range halls[1:] {
			if d := s.T.Dist(pos, s.Structs[hid].Pos); d < bestD {
				best, bestD = hid, d
			}
		}
		return &s.Structs[best]
	}

	// The levy: assessed where wealth actually sits. Measured before the hall existed:
	// a few households holding thousands while the median held tens.
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Gold <= LevyFloor {
			continue
		}
		take := (c.Gold - LevyFloor) * LevyRatePerDay
		c.Gold -= take
		nearest(c.Pos).Gold += take
		s.Led.GoldLevied += take
	}

	// Relief: coin to the destitute, bounded by the local treasury. A hall with no
	// money feeds nobody, which is what keeps conservation honest — and the dole is
	// paid wherever the person stands, because relief that requires a walk is a
	// behavioural intervention wearing charity's clothes.
	dole := s.SubsistenceWage() * WorkTicksPerDay * ReliefShare
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Stage() == Child || c.Gold >= ReliefFloor {
			continue
		}
		h := nearest(c.Pos)
		if h.Gold < dole {
			continue // this treasury is spent; a richer hall cannot reach them
		}
		h.Gold -= dole
		c.Gold += dole
		s.Led.GoldRelieved += dole
		s.diarise(CharID(i), "took relief of %.2f at the town hall (%.2f gold in hand)", dole, c.Gold)
	}
}
