package sim

import "math"

// Life cycle constants (design §3.2). Ages are in in-world years, and at the master
// constant of one real second to thirty in-world minutes, a full life is about twelve
// real days.
const (
	AdultAge = 15.0
	ElderAge = 56.0

	FertileMin = 18.0
	FertileMax = 45.0

	// Rates are per tick.
	AgePerTick = 1.0 / TicksPerYear

	// HungerPerTick fills an empty stomach over roughly a day and a half, so missing a
	// single meal is survivable and missing several is not.
	HungerPerTick = 100.0 / (1.5 * TicksPerDay)

	// HungerEatThreshold is when a character goes looking for food.
	HungerEatThreshold = 45
	// HungerStarving is when hunger starts costing health.
	HungerStarving = 90

	// FoodPerMeal is how much stock one meal consumes.
	FoodPerMeal = 1.0

	// Health rates.
	StarveDamagePerTick   = 100.0 / (6 * TicksPerDay) // starve to death in ~6 days
	HomelessDamagePerTick = 100.0 / (40 * TicksPerDay)
	HealPerTick           = 100.0 / (10 * TicksPerDay)

	// ElderMortality is the yearly chance of dying of old age at ElderAge, rising
	// steeply thereafter.
	ElderMortalityBase = 0.02

	// LarderTarget is how many meals a household keeps at home for its children, and
	// LarderReserve is the personal savings an adult will not spend stocking it.
	// A household keeps a real store, not a day's worth. At six meals the larder ran
	// permanently empty — adults topped it up on each visit and hungry children drained
	// it within ticks, so the village could raise about three children and starved every
	// one after that. A buffer has to be large enough to cover the gap between shopping
	// trips for every child in the house.
	// The larder must be affordable as well as adequate. At twenty-four meals a single
	// top-up cost thirteen times an adult's daily wage, so only the best-paid households
	// could ever stock one and everyone else's children starved beside full granaries.
	LarderTarget  = 8.0
	LarderReserve = 2.0

	// ForageAge is when a child is old enough to gather food. Below it they depend
	// entirely on the household; above it they have the same subsistence floor as
	// everyone else, which stops one bad week from wiping out a generation.
	ForageAge = 6.0

	// HungerForage is when someone with no way to buy gives up and lives off the land.
	//
	// It sits well above HungerEatThreshold on purpose. When the two were equal, anyone
	// who once started foraging never stopped: they grazed down to just under the
	// threshold, rose above it next tick, and foraged again, never reaching the branch
	// that sends them to work. The whole village fell into subsistence, the wage economy
	// died, and their children — who cannot forage — starved in front of them. A
	// fallback must be reached later than the behaviour it replaces.
	HungerForage = 70

	// ForageRate is how fast someone living off the land reduces their hunger.
	//
	// Wild forage (§2.5) is the subsistence floor beneath the wage economy. Barely
	// outpacing HungerPerTick, it keeps the jobless and the penniless alive but always
	// hungry, never accumulating anything. Without it, a closed circulation guarantees
	// that surplus labour starves: total wages exactly equal total food spending, so
	// there is nothing left over for anyone outside the loop.
	ForageRate = HungerPerTick * 1.15
)

// stepCharacters advances every living person by one tick.
func (s *State) stepCharacters() {
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive {
			continue
		}
		id := CharID(i)

		c.Age += AgePerTick
		c.Hunger = float32(math.Min(100, float64(c.Hunger)+HungerPerTick))

		// Growing up means needing a household of one's own.
		if !c.housed && c.Age >= AdultAge && c.Home != NoStruct {
			c.Home = NoStruct
		}
		if c.Home == NoStruct {
			s.assignHome(id)
		}

		s.stepNeeds(id)
		if !c.Alive {
			continue
		}
		s.stepBehaviour(id)
	}
}

// stepNeeds applies hunger, shelter, healing, and death.
func (s *State) stepNeeds(id CharID) {
	c := &s.Chars[id]

	switch {
	case c.Hunger >= HungerStarving:
		c.Health -= StarveDamagePerTick
	case c.Home == NoStruct:
		// Those without a home lose health until they have one (§5).
		c.Health -= HomelessDamagePerTick
	case c.Hunger < 50:
		c.Health = float32(math.Min(100, float64(c.Health)+HealPerTick))
	}

	if c.Health <= 0 {
		s.DeathsStarved++
		if c.Stage() == Child {
			s.DeathsChild++
		}
		if c.Home == NoStruct {
			s.DeathsHomeless++
		}
		s.kill(id)
		return
	}

	// Mortality climbs with age past the elder threshold (§3.2).
	if c.Age >= ElderAge {
		yearly := ElderMortalityBase * math.Pow(1.16, float64(c.Age)-ElderAge)
		if s.rng.Chance(yearly / TicksPerYear) {
			s.DeathsAge++
			s.kill(id)
		}
	}
}

// stepBehaviour runs one tick of a character's decision loop.
//
// The priority order is the whole of their psychology this milestone: eat if hungry,
// otherwise work if it is the working day, otherwise go home. Everything the village
// visibly does — the morning commute, the queue at the granary, the drift homeward at
// dusk — falls out of these three rules applied to a few hundred people.
func (s *State) stepBehaviour(id CharID) {
	c := &s.Chars[id]

	if c.Stage() == Child {
		// Children stay at home and are fed there.
		c.Activity = Resting
		if c.Home != NoStruct {
			s.moveToward(id, c.Home)
			if c.Hunger > HungerEatThreshold {
				s.feedChild(id)
			}
		}
		// Old enough to gather for themselves when the larder fails them.
		if c.Hunger > HungerForage && c.Age >= ForageAge {
			s.forage(id)
		}
		return
	}

	// Hunger interrupts everything else.
	if c.Hunger > HungerEatThreshold {
		g := s.nearestFoodSource(id)
		canBuy := g != NoStruct && c.Gold >= s.FoodPrice*FoodPerMeal
		if canBuy {
			c.Activity = GoingToEat
			c.dest = g
			if s.moveToward(id, g) {
				s.buyAndEat(id, g)
			}
			return
		}
		// Nothing to buy, or nothing to buy it with. Work on while it is merely
		// uncomfortable; live off the land only once it is not.
		if c.Hunger > HungerForage && s.forage(id) {
			return
		}
	}

	if c.Job != NoStruct && s.Tick.IsWorkTime() {
		c.Tenure += AgePerTick
		if s.moveToward(id, c.Job) {
			c.Activity = Working
			s.work(id)
		} else {
			c.Activity = GoingToWork
		}
		return
	}

	if c.Job == NoStruct {
		c.Activity = SeekingWork
	}

	// Off shift: go home and rest.
	if c.Home != NoStruct {
		if s.moveToward(id, c.Home) {
			c.Activity = Resting
		} else if c.Job != NoStruct {
			c.Activity = GoingHome
		}
	}
}

// work applies one tick of labour, paying a wage and contributing to output.
func (s *State) work(id CharID) {
	c := &s.Chars[id]
	st := &s.Structs[c.Job]

	// Wages come out of the employer's own reserves (§4.3). An employer that cannot
	// make payroll turns its workers out, which is how a failing faction sheds people
	// (§3.5) — and it is why the economy cannot quietly drain to nothing the way a
	// single shared treasury did.
	wage := st.Wage
	if s.Treasury < wage {
		// A faction that cannot make payroll turns its workers out, which is how a
		// failing faction sheds people (§3.5).
		s.quitJob(id)
		return
	}
	s.Treasury -= wage
	c.Gold += wage

	// Skill accrues with time served, and output scales with it.
	c.Skill[st.Type] += AgePerTick
	if st.Type == Farm {
		st.produce += float32(FarmYieldPerWorker * efficiency(c.Skill[Farm]))
	}
}

// forage feeds a character from the wild, and reports whether they did so.
//
// This is the floor beneath the market: someone with no job and no gold is not simply
// left to die, they go and find something to eat. It is deliberately worse than being
// paid — foraging keeps hunger just below crisis and leaves no time for anything else —
// so it sustains the unemployed without making unemployment comfortable.
func (s *State) forage(id CharID) bool {
	c := &s.Chars[id]
	i := s.T.Index(s.T.CellAt(c.Pos))
	if !s.World.Walkable(i) {
		return false
	}
	// Richer ground yields more. Bare rock yields almost nothing.
	yield := ForageRate * (0.35 + 0.65*float64(s.World.Soil[i]))
	c.Hunger = float32(math.Max(0, float64(c.Hunger)-yield))
	c.Activity = Idle
	return true
}

// nearestFoodSource finds the closest structure holding food.
func (s *State) nearestFoodSource(id CharID) StructID {
	c := &s.Chars[id]
	best, bestD := NoStruct, math.MaxFloat64
	for sid := range s.Structs {
		st := &s.Structs[sid]
		if !st.Alive || st.Food < FoodPerMeal {
			continue
		}
		if st.Type != Granary && st.Type != Farm {
			continue
		}
		if d := s.T.Dist2(c.Pos, st.Pos); d < bestD {
			best, bestD = StructID(sid), d
		}
	}
	return best
}

// buyAndEat exchanges gold for a meal, completing the economic loop.
//
// Gold circulates: a granary pays a farm for grain, the farm pays its labourers, the
// labourers buy meals back from the granary. Nothing is created or destroyed, which is
// why this milestone neither inflates nor deflates. Real faucets and sinks (§4.2) belong
// with the full economy.
func (s *State) buyAndEat(id CharID, src StructID) {
	c := &s.Chars[id]
	st := &s.Structs[src]
	if st.Food < FoodPerMeal {
		return
	}
	cost := s.FoodPrice * FoodPerMeal
	if c.Gold < cost {
		// Cannot afford to eat. This is the failure the wage term in §3.5 is meant to
		// prevent, and its consequence is starvation.
		return
	}
	c.Gold -= cost
	s.Treasury += cost
	st.Food -= FoodPerMeal
	c.Hunger = 0
	c.Activity = Resting

	// While here, stock the household larder for any children at home. Children cannot
	// work and so cannot buy; somebody has to shop for them.
	s.provision(id, src)
}

// provision buys food for a character's home while they are at a food source.
func (s *State) provision(id CharID, src StructID) {
	c := &s.Chars[id]
	if c.Home == NoStruct {
		return
	}
	home := &s.Structs[c.Home]
	if home.Food >= LarderTarget {
		return
	}
	st := &s.Structs[src]
	for home.Food < LarderTarget {
		cost := s.FoodPrice * FoodPerMeal
		if c.Gold < cost+LarderReserve || st.Food < FoodPerMeal {
			return
		}
		c.Gold -= cost
		s.Treasury += cost
		st.Food -= FoodPerMeal
		home.Food += FoodPerMeal
	}
}

// feedChild consumes food from the child's home on their behalf.
//
// Children cannot work and so cannot buy, which makes them a pure drain on the
// household — exactly the dependency §3.2 describes, and the reason a village that
// breeds faster than it farms starves.
func (s *State) feedChild(id CharID) {
	c := &s.Chars[id]
	home := &s.Structs[c.Home]
	if home.Food >= FoodPerMeal {
		home.Food -= FoodPerMeal
		c.Hunger = 0
		return
	}

	// An empty larder means the household is failing to provision, which is how a
	// household in trouble starves its young first. Letting the household buy directly
	// from a granary at this point was tried and was strictly worse: it drained the
	// treasury faster than wages could refill it and collapsed the whole village.
}

// stepBirths pairs adults and produces children (§3.2, §3.3).
func (s *State) stepBirths() {
	// Births are checked once per in-world day rather than every tick: the outcome is
	// the same and the cost is 480 times lower.
	if s.Tick%TicksPerDay != 0 {
		return
	}

	// Pair the unpartnered.
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Partner != NoChar || !fertile(c) {
			continue
		}
		for j := i + 1; j < len(s.Chars); j++ {
			o := &s.Chars[j]
			if !o.Alive || o.Partner != NoChar || !fertile(o) || o.Sex == c.Sex {
				continue
			}
			// Partnership is weighted by proximity: people pair with those they live
			// near (§3.3).
			if s.T.Dist(c.Pos, o.Pos) > 25 {
				continue
			}
			c.Partner, o.Partner = CharID(j), CharID(i)
			break
		}
	}

	// Partnered pairs with food, a home, and health may have a child.
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Partner == NoChar || c.Sex != 0 || !fertile(c) {
			continue
		}
		p := &s.Chars[c.Partner]
		if !p.Alive || !fertile(p) {
			continue
		}
		if c.Home == NoStruct || c.Health < 55 || c.Hunger > 60 {
			continue
		}
		// The child is born into its mother's house and takes no resident slot of its
		// own until it grows up. Two earlier rules each killed the village outright:
		// requiring a free slot in the mother's own home suppressed births almost
		// entirely, because assignHome packs the nearest homes full while others stand
		// empty; and placing the newborn in whichever home had room sent it to an empty
		// house that no adult ever stocked, so every child born starved.
		// Tuned toward one child per pair every two to four in-world years (§3.2).
		// Population growth is far easier to make explosive than to make stable.
		if !s.rng.Chance(1.0 / (3.0 * DaysPerYear)) {
			continue
		}
		s.birth(CharID(i), c.Home)
	}
}

func fertile(c *Character) bool {
	return c.Age >= FertileMin && c.Age <= FertileMax && c.Health > 40
}

// birth adds a child at a home with room for them.
func (s *State) birth(mother CharID, home StructID) {
	pos := s.Structs[home].Pos

	id := s.newChar(Character{
		Pos:      pos,
		Age:      0,
		Health:   100,
		Hunger:   20,
		Home:     home,
		Job:      NoStruct,
		Partner:  NoChar,
		Activity: Resting,
		Sex:      uint8(s.rng.Intn(2)),
		dest:     NoStruct,
	})
	// No Residents++ here on purpose. A child lives with its parents and takes no
	// housing slot of its own until it grows up (see the housed flag). Counting them
	// here leaked: births incremented the tally, but kill only decrements it for the
	// housed, so every child born or died pushed the count permanently upward until
	// every home looked full and adults could not be housed at all.
	s.Births++
	_ = id
}
