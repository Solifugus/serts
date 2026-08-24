package sim

import "github.com/solifugus/serts/internal/torus"

// Daughter settlements (§2.7a): how growth crosses the map.
//
// Everything here is built under Appendix A's constraints. The party must clear critical
// mass, because a colony founded small is founded dying (A.1). It leaves equipped —
// provisions bought with real gold from real granaries, a treasury raised from the hall
// and the wealthy — because the founding transient is survivable only with capital, and
// because conservation is not negotiable: nothing here conjures a coin or a meal.
//
// Two abstractions are inherited from the original founding and flagged rather than
// hidden. Colony structures are granted at arrival exactly as the mother village's were
// at world-start — the construction economy exists and should eventually raise them
// plank by plank, but a re-founding uses the founding's own abstraction until haulage is
// employment (§5). And the party arrives rather than treks: group travel with feeding en
// route is real design work that belongs with roads. Both debts are recorded here so
// they are owed, not forgotten.

const (
	// ColonyParty is the founding party size: above the measured critical mass (A.1),
	// below the mother's own viability.
	ColonyParty = 60

	// ColonyMinPop is the smallest mother that can send a party and remain above
	// critical mass herself.
	ColonyMinPop = 130

	// ColonyPressurePop is the population at which a village colonises even with food in
	// hand — the valley is simply full. Seed 5 reached 401 in one valley; long before
	// that, the land is the constraint.
	ColonyPressurePop = 200

	// ColonyCooldown is the least time between colonies from one world: a generation to
	// recover the people and the purse.
	ColonyCooldown = 15 * TicksPerYear

	// ColonyMinPurse is the treasury a colony must raise before it may leave: enough to
	// endow a granary and pay the first years' wages. A party that cannot raise it
	// waits — leaving broke is the measured founding disaster.
	ColonyMinPurse = 2500.0

	// ColonyMinDist keeps the daughter out of the mother's working radius — new valley,
	// not a suburb.
	ColonyMinDist = 40.0

	// ExogamyRange is how far the marriage market reaches between settlements. Two
	// villages within it pool their eligible singles — the direct answer to the Allee
	// threshold (A.1), and historically exactly why isolated hamlets exchanged spouses.
	ExogamyRange = 120.0
)

// considerColony weighs founding a daughter settlement, on the post-harvest cadence:
// the one moment of the year the stores speak honestly (the same reasoning as the
// harvest-shortfall farm trigger before it).
func (s *State) considerColony() {
	if d := s.Tick.Date().Day; d < HarvestDay {
		return
	}
	if s.Tick-s.lastColonyAt < ColonyCooldown && s.lastColonyAt > 0 {
		return
	}
	pop := s.Population()
	if pop < ColonyMinPop {
		return
	}
	// Two reasons the restless go: the stores cannot cover the year even at their
	// annual peak — the valley cannot feed its people — or the sheer press of numbers.
	// Either way, sixty fewer mouths is relief for the mother and a future for the
	// daughter.
	need := float32(pop) * MealsPerDay * DaysPerYear
	shortfall := s.MarketFood() < need*0.6
	if !shortfall && pop < ColonyPressurePop {
		return
	}

	site, ok := s.colonySite()
	if !ok {
		return
	}
	party := s.selectParty()
	if len(party) < ColonyParty {
		return // not enough willing feet; the restless are not yet numerous
	}
	purse, provisions := s.raiseColonyFunds(party)
	if purse < ColonyMinPurse {
		return // cannot yet afford to leave properly; wait and grow richer
	}

	s.foundColony(site, party, purse, provisions)
	s.lastColonyAt = s.Tick
	s.Colonies++
}

// colonySite finds a new valley: the same scoring the world's first village used,
// excluding everything near an existing settlement.
func (s *State) colonySite() (torus.Cell, bool) {
	best, bestScore := torus.Cell{}, -1.0
	const stride = 4
	for y := 0; y < s.T.CY; y += stride {
		for x := 0; x < s.T.CX; x += stride {
			c := torus.Cell{X: x, Y: y}
			i := s.T.Index(c)
			w := s.World
			if !w.Walkable(i) || w.FreshDist[i] > Defs[Farm].MaxFreshDist {
				continue
			}
			// Not in anyone's valley already.
			tooClose := false
			for j := range s.Structs {
				st := &s.Structs[j]
				if st.Alive && (st.Type == Granary || st.Type == TownHall) &&
					s.T.Dist(s.T.Center(c), st.Pos) < ColonyMinDist {
					tooClose = true
					break
				}
			}
			if tooClose {
				continue
			}
			var soil float64
			usable := 0
			for dy := -4; dy <= 4; dy++ {
				for dx := -4; dx <= 4; dx++ {
					n := s.T.Index(s.T.WrapCell(torus.Cell{X: x + dx, Y: y + dy}))
					if w.Walkable(n) {
						usable++
						soil += float64(w.Soil[n])
					}
				}
			}
			if usable < 60 {
				continue
			}
			score := soil / float64(usable)
			// Water counts as land does. A shore with poor ground can still feed a
			// settlement — year round, which farmland cannot — so scoring on soil
			// alone was refusing every coast and lakeside on the map.
			if fish := waterWithin(w, c, FisheryReach); fish > 0 {
				bonus := fish / 40
				if bonus > 0.35 {
					bonus = 0.35
				}
				score += bonus
			}
			if score > bestScore {
				best, bestScore = c, score
			}
		}
	}
	return best, bestScore > 0.35 // decent farmland or nothing: a colony on scrub starves
}

// selectParty picks who goes: the restless first — Rootedness doing the job it was
// defined for (§3.7) — whole households at a time, so no child is left behind and no
// couple is split. Deterministic order throughout.
func (s *State) selectParty() []CharID {
	// Rank adult households by the minimum Rootedness of their adults: the household
	// leaves when its most restless member wins the argument.
	type cand struct {
		id   CharID
		root float32
	}
	var cands []cand
	taken := map[CharID]bool{}
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Stage() == Child || taken[CharID(i)] {
			continue
		}
		r := c.Traits.Rootedness
		if c.Partner != NoChar && s.AliveChar(c.Partner) {
			if pr := s.Chars[c.Partner].Traits.Rootedness; pr < r {
				r = pr
			}
			taken[c.Partner] = true
		}
		taken[CharID(i)] = true
		cands = append(cands, cand{CharID(i), r})
	}
	// Stable order: rootedness ascending, ID ascending on ties (§9.2).
	for i := 1; i < len(cands); i++ {
		for j := i; j > 0 && (cands[j].root < cands[j-1].root ||
			(cands[j].root == cands[j-1].root && cands[j].id < cands[j-1].id)); j-- {
			cands[j], cands[j-1] = cands[j-1], cands[j]
		}
	}

	var party []CharID
	add := func(id CharID) {
		if id != NoChar && s.AliveChar(id) {
			party = append(party, id)
		}
	}
	for _, cd := range cands {
		if len(party) >= ColonyParty {
			break
		}
		c := &s.Chars[cd.id]
		add(cd.id)
		add(c.Partner)
		// Children of the household come with their parents.
		if c.Home != NoStruct {
			for j := range s.Chars {
				k := &s.Chars[j]
				if k.Alive && k.Home == c.Home && k.Stage() == Child {
					add(CharID(j))
				}
			}
		}
	}
	return party
}

// raiseColonyFunds gathers the treasury and buys the provisions, all with real money.
//
// The hall gives what it holds beyond a working float; the wealthy sponsor the rest,
// proportionally above their own reserves. Provisions are bought from the granaries at
// the market price — food and coin both genuinely move.
func (s *State) raiseColonyFunds(party []CharID) (purse float32, provisions float32) {
	// The hall's contribution.
	for i := range s.Structs {
		st := &s.Structs[i]
		if st.Alive && st.Type == TownHall && st.Gold > 200 {
			purse += st.Gold - 200
			st.Gold = 200
		}
	}
	// Sponsors: everyone wealthy, whether they go or stay — the stayers buy their
	// kin's future; the leavers carry their own.
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Gold <= LevyFloor {
			continue
		}
		give := (c.Gold - LevyFloor) * 0.5
		c.Gold -= give
		purse += give
	}

	// Provisions to first harvest, bought where the grain is.
	want := float32(len(party)) * MealsPerDay * 90 // a season's food in the wagons
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || provisions >= want {
			continue
		}
		if st.Type != Granary && st.Type != Farm {
			continue
		}
		take := want - provisions
		if take > st.Stock[Food] {
			take = st.Stock[Food]
		}
		price := s.FoodPriceAt(st.Pos)
		cost := take * price
		if cost > purse {
			take = purse / price
			cost = take * price
		}
		if take <= 0 {
			continue
		}
		st.Stock[Food] -= take
		st.Gold += cost
		st.revenue += cost
		purse -= cost
		provisions += take
	}
	return purse, provisions
}

// foundColony raises the daughter settlement and moves the party in.
func (s *State) foundColony(site torus.Cell, party []CharID, purse, provisions float32) {
	// The structures are granted as the mother's were at world-start: the founding's own
	// abstraction, reused for a re-founding, debt recorded at the top of this file.
	cfg := Config{
		World: s.World, Seed: s.Seed,
		Homes: 14, Farms: 5, Granaries: 2, Camps: 3, Quarries: 1, Mines: 1,
		Industry: true,
		Treasury: purse,
		// The provisions actually bought — never a granted number. If the wagons are
		// light, the colony's first winter is hard, exactly as it should be.
		StartingFood: provisions,
		FoodPrice:    s.Prices[Food],
	}
	s.buildVillage(site, cfg)

	// The party arrives. Old ties are released properly: jobs quit, home slots freed.
	centre := s.T.Center(site)
	for _, id := range party {
		c := &s.Chars[id]
		s.quitJob(id)
		if c.Home != NoStruct && c.housed {
			s.Structs[c.Home].Residents--
		}
		c.housed = false
		c.Home = NoStruct
		c.newHome = NoStruct
		c.dest = NoStruct
		c.Pos = centre
		s.diarise(id, "left with the founding party for %v", site)
		if c.Stage() != Child {
			s.assignHome(id)
		}
	}
	// Children move in with a parent once the adults are housed.
	for _, id := range party {
		c := &s.Chars[id]
		if c.Stage() != Child {
			continue
		}
		// The mother if she went; else the father; else any adult of the party.
		for _, pid := range party {
			p := &s.Chars[pid]
			if p.Stage() != Child && p.Home != NoStruct {
				c.Home = p.Home
				break
			}
		}
	}
	s.countHouseholds()
}
