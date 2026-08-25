package sim

// Migration between settlements (§3.5, §3.7).
//
// The job market reaches forty cells and settlements are founded at least forty apart,
// so a worker in a failing valley cannot see the work in the next one. That gap is why
// per-market pricing was measured as a death spiral: a struggling settlement prices food
// dear, its wages read as unpayable, its payroll empties — and nothing carries those
// people to where food is cheap. Local prices and migration are one feature; this is the
// half that equilibrates.
//
// Built under Appendix A.5, which is the hard-won constraint here: transfers of BEHAVIOUR
// are ruinous, because a subsistence household's scarcest asset is its labour hours.
// Eight interventions failed by spending them. So migration is rare, annual, and open
// only to people whose present situation has already failed — never a standing search
// that costs everyone time.
//
// Who goes is Rootedness doing its defining job (§3.7). The restless leave a dying
// valley; the rooted stay with the graves. That is a real fitness difference in a world
// with shocks, and it is the first mechanism that lets the trait be selected on rather
// than merely drift.
//
// Settlements are allowed to empty. A valley whose people all leave becomes a ghost town,
// which is §2.7's abandonment step arriving through the labour market rather than through
// a decay timer — the design's own preference.

const (
	// MigrationGain is how much better a destination must be before anyone uproots.
	// High on purpose: moving costs a household its home, its jobs and its standing, so
	// only a plain difference is worth it. Friction, exactly like JobSwitchGain.
	MigrationGain = 1.8

	// MigrationRange is how far a household will move. Far enough to reach a neighbouring
	// valley (colonies are founded at least ColonyMinDist away), not so far that people
	// cross the world.
	MigrationRange = 140.0

	// MigrationDesperation is how bad things must be before leaving is considered at all:
	// out of work, or in a settlement with less than this many days of food.
	MigrationDesperation = 60.0
)

// stepMigration lets failing households move to a settlement that is plainly better.
// Once a year, after the harvest, when the year's outcome is known.
func (s *State) stepMigration() {
	if d := s.Tick.Date().Day; d != HarvestDay {
		return
	}
	places := s.Settlements()
	if len(places) < 2 {
		return // nowhere to go
	}

	// What a settlement offers: food per head, and work per adult. Both matter — a full
	// granary with no vacancies feeds nobody who arrives.
	appeal := make([]float64, len(places))
	for i, v := range places {
		if v.Pop == 0 {
			continue
		}
		food := v.FoodDays / 200
		if food > 2 {
			food = 2 // beyond two years' stores, more grain is not more attractive
		}
		open := 0
		for j := range s.Structs {
			st := &s.Structs[j]
			if st.Alive && st.Jobs > st.Filled && s.marketHall(st.Pos) == v.Hall {
				open += st.Jobs - st.Filled
			}
		}
		work := float64(open) / float64(maxInt(v.Adults, 1))
		if work > 1 {
			work = 1
		}
		appeal[i] = food * (0.5 + work)
	}

	// Index each settlement by hall so a character's home can be found cheaply.
	at := func(hall StructID) int {
		for i, v := range places {
			if v.Hall == hall {
				return i
			}
		}
		return -1
	}

	moved := map[CharID]bool{}
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Stage() == Child || moved[CharID(i)] {
			continue
		}
		here := at(s.marketHall(c.Pos))
		if here < 0 {
			continue
		}
		// Has this life already failed where it stands? Out of work, or living in a
		// settlement that cannot feed a year.
		if c.Job != NoStruct && places[here].FoodDays >= MigrationDesperation {
			continue
		}

		// The restless go. Rootedness raises the bar for leaving, so the same failing
		// valley empties of its wanderers first and keeps its stubborn.
		bar := MigrationGain * float64(c.Traits.Rootedness)
		best, bestScore := -1, appeal[here]*bar
		for j, v := range places {
			if j == here || v.Pop == 0 {
				continue
			}
			d := s.T.Dist(c.Pos, s.Structs[v.Hall].Pos)
			if d > MigrationRange {
				continue
			}
			// Distance discounts the prize, so people prefer the near valley.
			score := appeal[j] / (1 + d/MigrationRange)
			if score > bestScore {
				best, bestScore = j, score
			}
		}
		if best < 0 {
			continue
		}

		// Whole households move: partners and children together, never split.
		party := []CharID{CharID(i)}
		if c.Partner != NoChar && s.AliveChar(c.Partner) {
			party = append(party, c.Partner)
		}
		if c.Home != NoStruct {
			for j := range s.Chars {
				k := &s.Chars[j]
				if k.Alive && k.Home == c.Home && k.Stage() == Child {
					party = append(party, CharID(j))
				}
			}
		}

		dest := s.Structs[places[best].Hall].Pos
		for _, id := range party {
			m := &s.Chars[id]
			moved[id] = true
			s.quitJob(id)
			if m.Home != NoStruct && m.housed {
				s.Structs[m.Home].Residents--
			}
			m.housed = false
			m.Home = NoStruct
			m.newHome = NoStruct
			m.dest = NoStruct
			m.Pos = dest
			if m.Stage() != Child {
				s.assignHome(id)
			}
		}
		// Children join whichever parent found a roof.
		for _, id := range party {
			k := &s.Chars[id]
			if k.Stage() != Child {
				continue
			}
			for _, pid := range party {
				if p := &s.Chars[pid]; p.Stage() != Child && p.Home != NoStruct {
					k.Home = p.Home
					break
				}
			}
		}
		s.diarise(CharID(i), "left for the settlement at %v with %d others",
			s.T.CellAt(dest), len(party)-1)
		s.Migrations++
	}
	if s.Migrations > 0 {
		s.countHouseholds()
	}
}
