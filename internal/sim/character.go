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

	// MealsPerDay is how much one person eats, derived from how fast hunger rises and the
	// point at which they go looking for food.
	MealsPerDay = TicksPerDay * HungerPerTick / HungerEatThreshold

	// PackSize is how many meals a character buys at once and carries with them.
	//
	// Buying a single meal at a time quietly wrecked the working day: a farmhand twenty
	// cells from the granary spent most of it walking there and back for one dinner,
	// arriving hungrier than when they left, and ended up foraging in the fields beside a
	// full barn. Nobody shops like that. They shop for the week.
	PackSize = 6.0

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
	LarderTarget = 8.0
	// LarderReserve is the personal savings an adult will not spend stocking the larder.
	// Set too low it is self-defeating: the parent shops for the household down to their
	// last two coins, then cannot afford their own next meal and ends up foraging while
	// the pantry is full.
	LarderReserve = 8.0

	// ForageAge is when a child is old enough to gather food. Below it they depend
	// entirely on the household; above it they have the same subsistence floor as
	// everyone else, which stops one bad week from wiping out a generation.
	ForageAge = 6.0

	// PanYieldPerDay is what a full day at the riverbed yields in coin.
	//
	// Deliberately well below a wage (§4.2). Panning must never be worth choosing over
	// employment, only worth falling back on — otherwise the money supply expands when
	// the economy is healthy, which is the opposite of what makes it a stabiliser.
	// Set at roughly two thirds of a wage: clearly worse than a job, but enough to live
	// on. Below subsistence it is not a fallback at all — a panner who cannot buy food
	// with a day's panning starves just as surely as one who does nothing.
	PanYieldPerDay = 1.1
	// Divided by the whole day, not the working part of it. A prospector camps on the
	// claim and works it around the clock, so charging the rate against working hours
	// alone doubled their income and made panning pay better than a job — which inverts
	// the entire point of it. Gold poured in, a third of the village stayed jobless
	// deliberately, and the money supply ran away.
	PanYield = PanYieldPerDay / TicksPerDay

	// GardenYieldPerDay is what one adult grows behind the house of an evening.
	//
	// Every household has a garden, not only the jobless, and that turns out to be
	// structurally necessary rather than merely authentic. In a closed circulation the
	// total wage bill exactly equals total spending on food, so there is no aggregate
	// surplus anywhere in the village — and a household with more mouths than earners
	// cannot be fed from wages at all. Children starved in droves for exactly this
	// reason. The garden is food entering the world without passing through money, and
	// it is what feeds dependants.
	//
	// Partial on purpose. It covers roughly half of what one person eats, so households
	// still buy most of their food and the money economy still matters.
	GardenYieldPerDay = 0.75
	GardenYield       = GardenYieldPerDay / (TicksPerDay - WorkTicksPerDay)

	// InheritedShare is how much of a dead character's gold passes to their household.
	// The remainder is lost, and is one of the economy's few sinks (§4.2).
	InheritedShare = 0.7

	// ToolBonus is how much a full kit adds to a worker's output.
	ToolBonus = 0.45
	// ToolWearPerDay is how fast a kit wears out under a day's work. A tool lasts on the
	// order of a season, which makes replacing it a recurring expense rather than a
	// one-off — the property that makes it a money sink at all.
	ToolWearPerDay = 1.0 / 70
	// ToolBuyBelow is the condition at which a worker goes shopping for a new kit.
	ToolBuyBelow = 0.25

	// MaxPanningCost is the furthest a prospector will set out for, in path cost. A few
	// days' walk: far enough to reach the hills, short enough that the journey is not the
	// whole of their life.
	MaxPanningCost = 90

	// HungerForage is when someone with no way to buy gives up and lives off the land.
	//
	// It sits well above HungerEatThreshold on purpose. When the two were equal, anyone
	// who once started foraging never stopped: they grazed down to just under the
	// threshold, rose above it next tick, and foraged again, never reaching the branch
	// that sends them to work. The whole village fell into subsistence, the wage economy
	// died, and their children — who cannot forage — starved in front of them. A
	// fallback must be reached later than the behaviour it replaces.
	HungerForage = 70

	// HungerCritical is when hunger overrides every other consideration, including going
	// to work or earning the money for tomorrow's meal.
	HungerCritical = 85

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
	// Start from a different person each tick.
	//
	// Every character acts on a world the ones before them have already changed: they
	// take the job opening, buy the last sack of grain, work the last of the seam. Walked
	// in array order that hands the same few people first refusal on everything, for
	// every tick of the world's life. Rotating the entry point costs nothing, stays
	// perfectly deterministic, and spreads the advantage evenly.
	//
	// It is a cheap half of the real answer. The design asks for double buffering (§9.4)
	// — read the previous tick, write the next — which removes the ordering effect
	// entirely rather than merely sharing it out.
	n := len(s.Chars)
	if n == 0 {
		return
	}
	offset := int(s.Tick % Tick(n))

	for k := 0; k < n; k++ {
		i := (offset + k) % n
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

	// Hunger interrupts everything else — but eating from what you carry does not
	// interrupt anything, which is the point of carrying it.
	if c.Hunger > HungerEatThreshold && c.Rations >= FoodPerMeal {
		c.Rations -= FoodPerMeal
		c.Hunger = 0
		s.consume(Food, FoodPerMeal)
	}

	// Restock before running out, so the trip happens on the way rather than in a crisis.
	if c.Rations < FoodPerMeal || (c.Hunger > HungerEatThreshold && c.Rations < PackSize/2) {
		g := s.nearestFoodSource(id)
		canBuy := g != NoStruct && c.Gold >= s.Prices[Food]*FoodPerMeal
		if canBuy && (c.Rations < FoodPerMeal || !s.Tick.IsWorkTime()) {
			c.Activity = GoingToEat
			c.dest = g
			if s.moveToward(id, g) {
				s.buyAndEat(id, g)
			}
			return
		}
		// No money, but there may be food at home. Go and eat it.
		//
		// This is the branch whose absence killed people. Eating from the larder used to
		// require already standing next to it, and the rule sending them to work fired
		// first, so a penniless quarryman starved twenty cells from a pantry with six
		// meals in it. Nobody does that. Hunger sends you home.
		if c.Hunger > HungerEatThreshold && c.Home != NoStruct &&
			s.Structs[c.Home].Stock[Food] >= FoodPerMeal {
			c.Activity = GoingToEat
			c.dest = c.Home
			if s.moveToward(id, c.Home) {
				s.Structs[c.Home].Stock[Food] -= FoodPerMeal
				c.Hunger = 0
				s.consume(Food, FoodPerMeal)
			}
			return
		}

		// Starving outright overrides everything: find something to eat now.
		if c.Hunger > HungerCritical && s.forage(id) {
			return
		}
	}

	// A worn-out kit is worth replacing if there is money spare after food. This is the
	// only purchase in the game that is not subsistence, and it is what stops gold from
	// simply accumulating in pockets (§4.2).
	if c.Job != NoStruct && c.Tools < ToolBuyBelow && !s.Tick.IsWorkTime() {
		if shop := s.nearestWith(c.Pos, Store, Tools); shop != NoStruct {
			if c.Gold >= s.Prices[Tools]+LarderReserve && s.T.Dist(c.Pos, s.Structs[shop].Pos) < 30 {
				if s.moveToward(id, shop) {
					s.buyTools(id, shop)
				}
				c.Activity = GoingToEat
				return
			}
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

	// The working day belongs to the unemployed too: they spend it panning for gold
	// (§4.2). This is the economy's faucet, and it is deliberately the day-job of people
	// nobody has hired.
	//
	// It has to come before ordinary hunger, not after. When foraging was reached first,
	// anyone who was jobless and hungry grazed all day, never earned a coin, and so was
	// still jobless and hungry tomorrow — a whole village of people living off the land
	// beside a granary they could not afford. Panning is what buys the next meal;
	// foraging is only what stops today's from killing you.
	// Note this is not gated on working hours. A prospector with a claim twelve cells out
	// cannot commute to it: walking home at dusk gave back every yard gained that day, so
	// they set out each morning, turned round each evening, and in three years reached
	// gold three times between them. Someone working ground that far out camps on it, and
	// comes back only to buy food.
	if c.Job == NoStruct {
		if s.pan(id) {
			return
		}
		c.Activity = SeekingWork
	}

	// Hungry, unable to buy, and with no gold to be had: live off the land.
	if c.Hunger > HungerForage && s.forage(id) {
		return
	}

	// Off shift: go home and work the garden.
	if c.Home != NoStruct {
		if s.moveToward(id, c.Home) {
			c.Activity = Resting
			s.tendGarden(id)
		} else if c.Job != NoStruct {
			c.Activity = GoingHome
		}
	}
}

// pan sends an unemployed character to the nearest gold and works it, reporting whether
// they are engaged in doing so.
//
// This is the mechanism that keeps the money supply alive (§4.2). Gold enters the world
// only through people the economy has failed to employ, which makes the supply
// counter-cyclical without anything deciding that it should be: a village with full
// employment mints nothing, and one that has collapsed floods itself with coin until
// trade restarts and the panners are hired away.
func (s *State) pan(id CharID) bool {
	c := &s.Chars[id]
	f := s.goldField()
	here := s.T.Index(s.T.CellAt(c.Pos))

	if s.World.GoldOre[here] > 0 {
		c.Activity = Panning
		take := float32(PanYield)
		if take > s.World.GoldOre[here] {
			take = s.World.GoldOre[here]
		}
		s.World.GoldOre[here] -= take
		c.Gold += take
		if s.World.GoldOre[here] <= 0 {
			// The seam is worked out, so the nearest gold has moved for everyone.
			s.World.GoldOre[here] = 0
			s.paths.goldDirty = true
		}
		return true
	}

	// Standing on a source cell that holds nothing means the shared field is stale: this
	// seam was worked out since it was last built. Flag it so the next rebuild drops the
	// cell, rather than leaving prospectors standing on bare gravel waiting for a route
	// that already points at their feet.
	if f.dist[here] == 0 {
		s.paths.goldDirty = true
	}

	// Refuse only journeys that are genuinely hopeless. The limit used to be a day's
	// round trip, which was right when prospectors commuted — but they camp on the claim
	// now, so distance is a one-off cost paid out of the rations they carry rather than
	// a toll charged every morning. Set at a day's walk it excluded almost everybody: the
	// village itself sat at two thirds of the limit, so anyone standing at their own
	// front door was ruled out of prospecting.
	if f.dist[here] > MaxPanningCost {
		return false
	}

	// Walk toward the nearest deposit.
	if d := f.dir[here]; d >= 0 {
		next := s.T.Neighbor8(s.T.CellAt(c.Pos), int(d))
		s.stepToward(id, s.T.Center(next))
		c.Activity = Prospecting
		return true
	}
	return false // no reachable gold anywhere
}

// tendGarden lets an unemployed household grow a little of its own food (§4.2).
//
// It is what turns joblessness from a death sentence into poverty, and it feeds children
// whose parents cannot afford the granary — which was the single largest cause of death
// in the previous milestone.
func (s *State) tendGarden(id CharID) {
	c := &s.Chars[id]
	home := &s.Structs[c.Home]
	if home.Stock[Food] >= LarderTarget {
		return
	}
	home.Stock[Food] += float32(GardenYield)
	c.Activity = Gardening
}

// inherit passes a dead character's gold to their household (§4.2).
//
// Gold that simply vanished with its owner would shrink the money supply arbitrarily and
// unpredictably; gold that passes to a partner and children keeps circulating. The share
// that is lost is deliberate, and is one of the economy's few sinks.
func (s *State) inherit(id CharID) {
	c := &s.Chars[id]
	if c.Gold <= 0 {
		return
	}
	estate := c.Gold * InheritedShare
	c.Gold = 0

	// Heirs are the partner and anyone sharing the home.
	var heirs []CharID
	if c.Partner != NoChar && s.AliveChar(c.Partner) {
		heirs = append(heirs, c.Partner)
	}
	if c.Home != NoStruct {
		for i := range s.Chars {
			o := &s.Chars[i]
			if o.Alive && CharID(i) != id && o.Home == c.Home && CharID(i) != c.Partner {
				heirs = append(heirs, CharID(i))
			}
		}
	}
	if len(heirs) == 0 {
		return // no household; the estate is lost with them
	}
	share := estate / float32(len(heirs))
	for _, h := range heirs {
		s.Chars[h].Gold += share
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
	if st.Gold < wage {
		// An employer that cannot make payroll turns its workers out, which is how a
		// failing faction sheds people (§3.5).
		s.quitJob(id)
		return
	}
	st.Gold -= wage
	c.Gold += wage

	// Skill accrues with time served, and output scales with it — as do tools, which is
	// how the material economy pays back into the food economy.
	c.Skill[st.Type] += AgePerTick
	effort := efficiency(c.Skill[st.Type]) * (1 + ToolBonus*float64(c.Tools))

	switch st.Type {
	case Farm:
		st.produce += float32(FarmYieldPerWorker * effort)
	case LumberCamp, Quarry, Mine:
		s.extract(c.Job, extractPerWorkerDay(st.Type)/WorkTicksPerDay*effort)
	case Workshop:
		s.manufacture(c.Job, CraftPerWorkerDay/WorkTicksPerDay*effort)
	case BuildSite:
		s.build(c.Job, effort)
	}

	// Tools wear with use, and only while actually working.
	if c.Tools > 0 {
		c.Tools -= float32(ToolWearPerDay / WorkTicksPerDay)
		if c.Tools < 0 {
			c.Tools = 0
		}
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

// nearestFoodSource finds the closest place a character can buy a meal.
//
// Granaries only. Letting people buy at the farm gate looks harmless and quietly destroys
// the economy: the granary is cut out of the retail trade it exists to conduct, so it
// earns nothing, so it cannot buy the harvest, so farms fill up and stop selling. In
// practice every coin in the village ended up in the one farm nearest the houses — which
// had no way to spend it — while the granary stood at zero beside a full barn.
func (s *State) nearestFoodSource(id CharID) StructID {
	c := &s.Chars[id]
	best, bestD := NoStruct, math.MaxFloat64
	for sid := range s.Structs {
		st := &s.Structs[sid]
		if !st.Alive || st.Stock[Food] < FoodPerMeal {
			continue
		}
		if st.Type != Granary {
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
	if st.Stock[Food] < FoodPerMeal {
		return
	}
	cost := s.Prices[Food] * FoodPerMeal
	if c.Gold < cost {
		// Cannot afford to eat. This is the failure the wage term in §3.5 is meant to
		// prevent, and its consequence is starvation.
		return
	}
	// Fill the pack rather than buying one dinner.
	want := PackSize - c.Rations
	if avail := st.Stock[Food]; want > avail {
		want = avail
	}
	// No reserve is withheld here. LarderReserve exists to stop a parent spending the
	// household's last coins stocking the pantry; applying it to a person's own rations
	// meant anyone poor could only ever buy a single meal, which forced a daily walk back
	// to the granary. For a prospector twelve cells out that consumed the whole working
	// day, so nobody ever panned anything and the jobless starved with a gold field in
	// walking distance.
	if affordable := c.Gold / s.Prices[Food]; want > affordable {
		want = affordable
	}
	if want < FoodPerMeal {
		want = FoodPerMeal // always take at least the meal that brought them here
	}
	if want > st.Stock[Food] || want*s.Prices[Food] > c.Gold {
		return
	}
	c.Gold -= want * s.Prices[Food]
	st.Gold += want * s.Prices[Food]
	st.revenue += want * s.Prices[Food]
	st.Stock[Food] -= want
	c.Rations += want

	if c.Hunger > HungerEatThreshold && c.Rations >= FoodPerMeal {
		c.Rations -= FoodPerMeal
		c.Hunger = 0
	}
	c.Activity = Resting

	// While here, stock the household larder for any children at home. Children cannot
	// work and so cannot buy; somebody has to shop for them.
	s.provision(id, src)
}

// buyTools replaces a worker's kit.
func (s *State) buyTools(id CharID, shop StructID) {
	c := &s.Chars[id]
	st := &s.Structs[shop]
	price := s.Prices[Tools]
	if st.Stock[Tools] < 1 || c.Gold < price {
		return
	}
	c.Gold -= price
	st.Gold += price
	st.revenue += price
	st.Stock[Tools]--
	c.Tools = 1
	s.consume(Tools, 1)
}

// provision buys food for a character's home while they are at a food source.
func (s *State) provision(id CharID, src StructID) {
	c := &s.Chars[id]
	if c.Home == NoStruct {
		return
	}
	home := &s.Structs[c.Home]
	if home.Stock[Food] >= LarderTarget {
		return
	}
	st := &s.Structs[src]
	for home.Stock[Food] < LarderTarget {
		cost := s.Prices[Food] * FoodPerMeal
		if c.Gold < cost+LarderReserve || st.Stock[Food] < FoodPerMeal {
			return
		}
		c.Gold -= cost
		st.Gold += cost
		st.revenue += cost
		st.Stock[Food] -= FoodPerMeal
		home.Stock[Food] += FoodPerMeal
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
	if home.Stock[Food] >= FoodPerMeal {
		home.Stock[Food] -= FoodPerMeal
		c.Hunger = 0
		s.consume(Food, FoodPerMeal)
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
