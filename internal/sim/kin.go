package sim

// Kin.
//
// Who somebody's parents were, and what follows from knowing it.
//
// The diaries were the argument for this. A death used to appear only in the dead
// person's own record, so a life read as a sequence of things that happened to one
// individual with other people as scenery: a man remarried twice and the diary never said
// why, because the two funerals in between were written in somebody else's file. Adding
// the spouse link fixed half of that at the cost of one branch. Parents and children are
// the other half, and they are the half that makes a village feel inhabited — being
// orphaned at seven, outliving your own children, a line that ends.
//
// Nothing here changes what anyone does. It is recording, not behaviour, and the two are
// deliberately kept apart: a change that alters the simulation has to be measured against
// the seeds (note 16), while a change that only writes things down cannot move a number
// and can be judged by reading. Kin links will eventually feed real behaviour — a
// marriage market that declines to match siblings is the obvious one — and that day it
// becomes a measured change like any other.

// parentOf returns a parent's ID, or NoChar when there is none on record or the slot has
// since been reissued.
//
// The guard is the whole point. Character slots are recycled when somebody dies, so a
// stored parent ID can come to name a completely different person born years later; a
// diary trusting it would announce the death of a mother to a stranger's child. A real
// parent is necessarily born strictly before their child, and a recycled slot necessarily
// holds someone born after — so comparing birth ticks separates them exactly.
func (s *State) parentOf(child, parent CharID) CharID {
	if parent == NoChar || parent < 0 || int(parent) >= len(s.Chars) {
		return NoChar
	}
	if child < 0 || int(child) >= len(s.Chars) {
		return NoChar
	}
	if s.Chars[parent].bornAt >= s.Chars[child].bornAt {
		return NoChar // the slot has been reissued to somebody younger
	}
	return parent
}

// MotherOf and FatherOf are the checked parents of a character, NoChar if unknown. They
// say nothing about whether that parent is still alive — the dead are still your parents.
func (s *State) MotherOf(id CharID) CharID {
	if id < 0 || int(id) >= len(s.Chars) {
		return NoChar
	}
	return s.parentOf(id, s.Chars[id].Mother)
}

func (s *State) FatherOf(id CharID) CharID {
	if id < 0 || int(id) >= len(s.Chars) {
		return NoChar
	}
	return s.parentOf(id, s.Chars[id].Father)
}

// noteBereavements writes a death into the diaries of the people it happened to.
//
// Called from kill while the dying character's links are all still intact, and before
// inherit dissolves the marriage. Costs a scan of the population per death, which is
// affordable because it happens once per death and only when diaries are recording — the
// nil check in diarise means a normal run does not pay for any of this.
func (s *State) noteBereavements(id CharID, cause DeathCause) {
	if s.diaries == nil {
		return
	}
	c := &s.Chars[id]
	name, age := s.FullName(id), c.Age

	// The widow. Recorded before inherit(), which is what ends the marriage.
	if p := c.Partner; p != NoChar && s.AliveChar(p) {
		s.diarise(p, "was widowed; %s died at %.0f, after %.0f years married",
			name, age, s.Chars[p].Age-s.Chars[p].marriedAt)
	}

	// The parents, who have outlived a child. Worth distinguishing from ordinary
	// bereavement: losing a grown son at fifty is a different sentence from losing an
	// infant, and both are different from being widowed.
	for _, parent := range []CharID{s.MotherOf(id), s.FatherOf(id)} {
		if parent == NoChar || !s.AliveChar(parent) {
			continue
		}
		switch {
		case age < 1:
			s.diarise(parent, "lost the baby %s", GivenName(c.NameSeed))
		case age < AdultAge:
			s.diarise(parent, "buried %s, still a child at %.0f", name, age)
		default:
			s.diarise(parent, "outlived %s, who died at %.0f", name, age)
		}
	}

	// The children, who have lost a parent. One pass over the population: the links point
	// from child to parent, so there is no list of somebody's children to walk.
	for i := range s.Chars {
		k := &s.Chars[i]
		if !k.Alive || CharID(i) == id {
			continue
		}
		if s.MotherOf(CharID(i)) != id && s.FatherOf(CharID(i)) != id {
			continue
		}
		// Whether this leaves them with anyone is the fact that matters to a child.
		other := s.MotherOf(CharID(i))
		if other == id {
			other = s.FatherOf(CharID(i))
		}
		orphaned := other == NoChar || !s.AliveChar(other)
		switch {
		case orphaned && k.Age < AdultAge:
			s.diarise(CharID(i), "was orphaned at %.0f; %s died at %.0f", k.Age, name, age)
		case k.Age < AdultAge:
			s.diarise(CharID(i), "lost a parent at %.0f; %s died at %.0f", k.Age, name, age)
		default:
			s.diarise(CharID(i), "%s died at %.0f", name, age)
		}
	}
}
