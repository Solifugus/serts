package sim

import (
	"math"

	"github.com/solifugus/serts/internal/torus"
)

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

	// MinShoppingTrip is how many meals must be affordable before an off-shift trip to the
	// granary is worth making at all. Below it a character stays home and gardens instead.
	MinShoppingTrip = 3.0

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

	// Disease (§3.2).
	//
	// The baseline mortality that has been missing. With starvation as the only way to
	// die, the population was a knife edge: either collapsing or, once fed, effectively
	// immortal. Real populations lose people steadily to illness whatever the harvest,
	// and it was historically the dominant killer — most of all of the very young.
	//
	// DiseaseBase is the yearly hazard for a healthy, well-fed adult in an uncrowded
	// house. Everything else multiplies it.
	DiseaseBase = 0.010
	// The very young are far more vulnerable, tapering as they grow.
	//
	// Calibrated to lose roughly three in ten before the age of five, which is about right
	// for a pre-industrial village. Nine times the base rate sounded plausible and
	// compounded with crowding to better than half, which no population can replace: of
	// twenty-six children born, nineteen died. A multiplier has to be checked against the
	// cumulative survival it implies, not against whether it sounds severe.
	//
	// Lowered to meet the three-in-ten spec — for the second time, and this time with the
	// food constraint addressed first. The first attempt (at the 34-person baseline) was
	// measured and reverted: disease deaths fell and hunger took every child saved,
	// because food per child was the binding constraint and disease merely reached some
	// children first. The revert note said to re-apply these the day the food constraint
	// broke. That day: the scale + seasonal-servants + garden stack measured expectancy
	// at 18.3 and R0 at 0.736, both records, with the child-food channels the fuller by
	// every ledger line. If survival converts this time, the substitution chain is over.
	InfantRisk = 2.2
	InfantAge  = 5.0
	ChildRisk  = 1.25
	// Hunger and poor health tell badly on the sick.
	MalnutritionRisk = 3.0
	// Crowding spreads it: each occupant of a house above the first adds this much.
	CrowdingRisk = 0.09

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
	// LarderPerPerson is how many meals a household keeps in store for each of its
	// members.
	LarderPerPerson = 6.0
	LarderTarget    = 8.0 // retained for provisioning decisions made away from home
	// LarderReserve is the personal savings an adult will not spend stocking the larder.
	// Set too low it is self-defeating: the parent shops for the household down to their
	// last two coins, then cannot afford their own next meal and ends up foraging while
	// the pantry is full.
	//
	// Stated in gold, deliberately, after denominating it in meals was tried and measured.
	// Meals is the principled unit — an absolute gold figure breaks if the price level
	// moves — but a meal-indexed reserve moves WITH the price, and when the famine signal
	// spiked prices the reserve spiked too, so parents stopped provisioning larders in
	// exactly the famine. Fixed-in-gold is wrong under deflation; indexed is wrong under
	// spikes; fixed wins while the price floor holds the level roughly steady.
	LarderReserve = 8.0

	// ForageAge is when a child is old enough to gather food. Below it they depend
	// entirely on the household; above it they have the same subsistence floor as
	// everyone else, which stops one bad week from wiping out a generation.
	ForageAge = 6.0

	// PanYieldPerDay is what a full day at the riverbed yields in coin.
	//
	// It must stay below what a day's food costs, and it did not. At 1.1 gold against a
	// food cost of 0.467 panning paid better than two days of subsistence work — more than
	// most jobs in the village were offering — which inverts the whole point of it (§4.2).
	// Panning is a fallback, never a career: the moment it out-earns employment the money
	// supply expands when the economy is healthy, which is the opposite of the stabiliser
	// it is meant to be.
	//
	// The error was invisible for a long time because nobody ever reached it. With about
	// as many jobs as adults there was no unemployment, so in twelve measured years not one
	// villager panned and not one coin entered the world.
	//
	// Set to roughly two thirds of a day's food. A panner cannot live on it alone, and is
	// not meant to — the household garden feeds them (§4.2 and the GardenYieldPerDay note),
	// while panning is what puts coin in their hand for everything a garden cannot grow.
	PanYieldPerDay = 0.3
	// Divided by the whole day, not the working part of it. A prospector camps on the
	// claim and works it around the clock, so charging the rate against working hours
	// alone doubled their income and made panning pay better than a job — which inverts
	// the entire point of it. Gold poured in, a third of the village stayed jobless
	// deliberately, and the money supply ran away.
	PanYield = PanYieldPerDay / TicksPerDay

	// GardenAge is when a child is old enough to be useful in the garden. They cannot do
	// wage work, but weeding, watering, gathering and minding animals is exactly what
	// children did, and it is the labour that feeds a large household.
	GardenAge = 6.0
	// ChildGardenShare is how much a child contributes against an adult.
	ChildGardenShare = 0.5

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
	// Raised roughly threefold, because the model had the household economy backwards. In
	// a subsistence village the garden and smallholding were the majority of what a family
	// ate; wage labour bought what could not be grown. Here wages were the main source of
	// food and the garden an eighth of it, so a household of six with one earner could
	// never be fed at all — total wages in a closed circulation equal total food spending
	// exactly, leaving no surplus anywhere for a dependant.
	//
	// Raised again, from 2.1, on the day's accumulated evidence. Every attempt to
	// redistribute existing food — reserves, errands, decanted packs, cheap prices —
	// measured worse, because each broke a rhythm the economy balances on; the only
	// interventions that ever helped ADDED food. Child survival is pinned at half by
	// food-per-child across every configuration tried, and the garden is the one channel
	// that adds food exactly where children eat, in evenings that are otherwise idle,
	// already capped by the larder target so it cannot glut. The design's own record
	// (above) says real smallholdings fed most of a family; at 2.1 ours fed a quarter.
	// Still short of self-sufficiency: a household should still buy a share of its food,
	// so that wages, prices and trade continue to matter.
	GardenYieldPerDay = 3.2
	GardenYield       = GardenYieldPerDay / (TicksPerDay - WorkTicksPerDay)

	// InheritedShare is how much of a dead character's gold passes to their household.
	// The remainder is lost, and is one of the economy's few sinks (§4.2).
	InheritedShare = 0.7

	// ToolBonus is how much a full kit adds to a worker's output.
	ToolBonus = 0.45
	// ToolWearPerDay is how fast a kit wears out under a day's work.
	//
	// Was 1/70 — a tool lasting ten weeks — chosen so that replacing it would be a
	// recurring expense and gold would have somewhere to go. The monetary effect was
	// checked and the material cost was not: twenty-six adults each replacing a kit five
	// times a year is 187 units of timber annually, against a woodlot yielding between one
	// and three. Tools consumed sixty times the forest's entire output, so no timber ever
	// reached construction and fourteen to seventeen houses stood on order, unbuilt, for
	// as long as the village lasted.
	//
	// A hand tool lasts years, not weeks. At a year it is still a recurring expense and
	// still a money sink, at a twentieth of the timber.
	ToolWearPerDay = 1.0 / 360
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

		// Age is recomputed from the birth date, never accumulated.
		//
		// Adding 1/864000 to a float32 age is a no-op once the age passes about thirty
		// two: the increment falls below half the gap between representable values at
		// that magnitude, so every addition rounds straight back to where it started.
		// Nobody in this world had aged past thirty-two, ever. The people in their forties
		// were founders frozen at the age they were created, the mortality curve past
		// fifty-six could never fire, and the oldest villager was reported as fifty in
		// every run because that was the eldest settler, stuck there since the first tick.
		c.Age = float32(float64(s.Tick-c.bornAt) / TicksPerYear)
		c.Hunger = float32(math.Min(100, float64(c.Hunger)+HungerPerTick))

		// Growing up means needing a household of one's own — but not being turned out of
		// the family home to find it.
		//
		// Written on the theory that eviction at fifteen was killing everyone born here —
		// the age histogram shows children, teenagers, and then nobody at all in their
		// twenties. It made no difference whatsoever: assignHome already tended to put
		// them back in the house they were standing in. The theory was wrong and the gap
		// in the twenties remains unexplained. Kept because saying "stay with your family
		// if there is room" outright is clearer than relying on that coincidence.
		if !c.housed && c.Age >= AdultAge {
			s.comeOfAge(id)
		}
		if c.Home == NoStruct {
			s.assignHome(id)
		}

		if s.dbgWatch == id {
			s.dbgTrace(id)
		}
		s.stepNeeds(id)
		if !c.Alive {
			continue
		}
		s.stepBehaviour(id)
	}
}

// comeOfAge gives a new adult a household slot, keeping them at home if there is room.
func (s *State) comeOfAge(id CharID) {
	c := &s.Chars[id]
	s.diarise(id, "came of age (%.2f gold in hand)", c.Gold)
	if c.Home != NoStruct && s.Structs[c.Home].Residents < s.Structs[c.Home].Capacity() {
		// Stay with the family. They keep the larder they grew up on until they can
		// afford to leave.
		c.housed = true
		s.Structs[c.Home].Residents++
		return
	}
	// The family home is full, so they must find their own.
	c.Home = NoStruct
	s.assignHome(id)
}

// stepNeeds applies hunger, shelter, healing, and death.
func (s *State) stepNeeds(id CharID) {
	c := &s.Chars[id]

	if s.diaries != nil {
		if c.Hunger >= 85 {
			s.noteHunger(id)
		} else if c.Hunger < HungerEatThreshold {
			s.noteFed(id)
		}
	}

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
		cause := CauseHunger
		if c.Home == NoStruct {
			cause = CauseExposure
		}
		s.DeathsStarved++
		if c.Stage() == Child {
			s.DeathsChild++
		}
		if c.Home == NoStruct {
			s.DeathsHomeless++
		}
		s.kill(id, cause)
		return
	}

	// Illness. Independent of the economy, which is what makes it a floor under mortality
	// rather than another symptom of hunger.
	if s.rng.Chance(s.diseaseHazard(id) / TicksPerYear) {
		s.DeathsDisease++
		if c.Stage() == Child {
			s.DeathsChild++
		}
		s.kill(id, CauseDisease)
		return
	}

	// Mortality climbs with age past the elder threshold (§3.2).
	if c.Age >= ElderAge {
		yearly := ElderMortalityBase * math.Pow(1.16, float64(c.Age)-ElderAge)
		if s.rng.Chance(yearly / TicksPerYear) {
			s.DeathsAge++
			s.kill(id, CauseAge)
		}
	}
}

// diseaseHazard is a character's yearly chance of dying of illness.
//
// Deliberately a hazard rate rather than a sickness state. A proper model — infection,
// contagion, recovery — belongs with epidemics as a crisis mechanic (§8.3); this is the
// steady background loss that every population carries, and without it the demography has
// no floor.
func (s *State) diseaseHazard(id CharID) float64 {
	c := &s.Chars[id]

	// Excess risks add; they do not multiply.
	//
	// They used to multiply, and the compound was a number nobody had chosen. A hungry
	// infant in an over-full house drew InfantRisk 4.5 × malnutrition 3.0 × crowding 1.9 —
	// a twenty-six per cent annual death hazard, about one in five surviving to age five,
	// for the exact group the village most needs to survive. Each constant had been set on
	// its own and checked on its own; their product was never checked against the
	// cumulative survival it implied, which is the same fault recorded at InfantRisk = 9.
	//
	// Multiplying is also the wrong shape. Each factor is a mostly separate set of ways to
	// die — being small, being underfed, sharing air — and mostly separate causes stack
	// closer to addition than multiplication. Added, the worst case is about 7× base, a
	// survival to five of roughly two thirds even in a poor crowded house, and every factor
	// still visibly matters.
	excess := 0.0

	switch {
	case c.Age < InfantAge:
		excess += InfantRisk - 1
	case c.Stage() == Child:
		excess += ChildRisk - 1
	case c.Age >= ElderAge:
		excess += float64(c.Age-ElderAge) * 0.06
	}

	// The hungry and the sickly die of things the well-fed survive.
	if c.Hunger > 60 || c.Health < 70 {
		want := math.Max(float64(c.Hunger-60)/40, float64(70-c.Health)/70)
		excess += (MalnutritionRisk - 1) * math.Min(1, want)
	}

	// Crowded households. Measured against the room available rather than the number of
	// heads, so improving a house genuinely makes it healthier — which is most of why
	// anyone would pay for a bigger one.
	//
	// Children count as half a head. The instrumented stack run found disease killing
	// five-to-fifteens at ~3.4% a year — historically the SAFEST decade of life, under
	// one per cent — and crowding was the only age-blind multiplier that could do it: a
	// capacity-six home with two parents and six children read as over-full by five,
	// tripling every child's hazard. Children shared beds; that is historically how
	// large families fit small houses, and it is why a house of eight with six of them
	// small was not a plague pit.
	if c.Home != NoStruct {
		h := &s.Structs[c.Home]
		if over := float64(h.CrowdHeads) - float64(h.Capacity())*0.5; over > 0 {
			excess += over * CrowdingRisk
		}
	}
	return DiseaseBase * (1 + excess)
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
		// Old enough to be useful at home. Children cannot earn, but they can grow food,
		// which is what stops a large family from being purely a drain on one wage.
		if c.Age >= GardenAge && c.Home != NoStruct {
			s.tendGarden(id)
		}
		// And to gather for themselves when the larder fails them.
		if c.Hunger > HungerForage && c.Age >= ForageAge {
			s.forage(id)
		}
		return
	}

	// Hunger interrupts everything else — but eating from what you carry does not
	// interrupt anything, which is the point of carrying it. The last fraction in the pack
	// is a meal too, for the same reason a short measure at the counter is a sale: every
	// all-or-nothing test on food has starved somebody over a rounding.
	if c.Hunger > HungerEatThreshold && c.Rations > 0 {
		take := float32(math.Min(FoodPerMeal, float64(c.Rations)))
		c.Rations -= take
		c.Hunger -= c.Hunger * (take / FoodPerMeal)
		if c.Hunger < 0 {
			c.Hunger = 0
		}
	}

	// Hungry, with food in the house: eat at home before walking anywhere.
	//
	// This has to come before the trip to the granary, and putting it after was killing the
	// young. A sixteen-year-old with a job, living at home beside a larder holding
	// seventy-two meals and rising, was measured starving at hunger 85 — because his
	// rations were under one portion, which sent him to a granary he could barely afford,
	// so he spent his life walking between a shop that sold him one meal and a house full
	// of food he never went into. Of everyone reaching fifteen, forty per cent died within
	// three years and two thirds of those deaths were hunger. Coming of age was the most
	// dangerous thing that happened in this village.
	//
	// Nobody walks past their own larder to queue at a shop. The children's share is still
	// protected; what is spare above it belongs to the household.
	if c.Hunger > HungerEatThreshold && c.Home != NoStruct {
		home := &s.Structs[c.Home]
		if spare := home.Stock[Food] - s.childrensShare(c.Home); spare > 0 {
			c.Activity = GoingToEat
			c.dest = c.Home
			if s.moveToward(id, c.Home) {
				take := float32(FoodPerMeal)
				if take > spare {
					take = spare
				}
				home.Stock[Food] -= take
				c.Hunger -= c.Hunger * (take / FoodPerMeal)
				if c.Hunger < 0 {
					c.Hunger = 0
				}
			}
			return
		}
	}

	// Restock before running out, so the trip happens on the way rather than in a crisis.
	if c.Rations < FoodPerMeal || (c.Hunger > HungerEatThreshold && c.Rations < PackSize/2) {
		g := s.nearestFoodSource(id)
		// A trip worth making, or no trip.
		//
		// Buying a single meal is not worth an evening's walk, and treating it as though it
		// were cost the poor everything. Someone who could afford one portion set out for
		// the granary, came back with it, and had nothing left over, so their rations never
		// rose above the restock threshold and they set out again the next day — and the
		// next. Off-shift hours are half the day and GoingToEat was measured at 50.6% of
		// all adult time: the entire evening, every evening, spent walking for one meal.
		//
		// The garden sits below this in the cascade and was therefore never reached by a
		// single adult in the village. The one thing the design relies on to feed
		// dependants — food entering the world without passing through money (§4.2) — was
		// not operating at all, and only the poor were shut out of it, because the well-off
		// bought a pack of six and stayed home for days afterwards.
		//
		// So: go when you are genuinely out, or when you can buy enough to be worth the
		// journey. Otherwise stay and work the ground, which pays better than the walk.
		worthTheTrip := c.Gold >= s.FoodPriceAt(c.Pos)*MinShoppingTrip
		canBuy := g != NoStruct && c.Gold >= s.FoodPriceAt(c.Pos)*FoodPerMeal
		if canBuy && (c.Rations < FoodPerMeal || (worthTheTrip && !s.Tick.IsWorkTime())) {
			c.Activity = GoingToEat
			c.dest = g
			if s.moveToward(id, g) {
				s.buyAndEat(id, g)
			}
			return
		}
		// No money, but there may be food at home. Go and eat it — unless the children
		// need it more.
		//
		// A penniless parent falling back on the larder was eating the food their infant
		// depends on. Both draw on one store, the parent reaches it first, and a child
		// under six can neither forage nor work the garden, so it always loses. Every
		// infant death in this village was a household with two parents at home, a full
		// granary in the village, and not one coin between them.
		//
		// An adult can forage; a baby cannot. So the larder is the children's store, and a
		// parent takes from it only what is genuinely spare.
		//
		// This is the branch whose absence killed people. Eating from the larder used to
		// require already standing next to it, and the rule sending them to work fired
		// first, so a penniless quarryman starved twenty cells from a pantry with six
		// meals in it. Nobody does that. Hunger sends you home.
		if c.Hunger > HungerEatThreshold && c.Home != NoStruct &&
			s.Structs[c.Home].Stock[Food] > s.childrensShare(c.Home) {
			c.Activity = GoingToEat
			c.dest = c.Home
			if s.moveToward(id, c.Home) {
				// Eat what is spare above the children's share, in whatever portion is
				// there. Demanding a whole meal beyond the reserve locked household
				// members out of a larder their own labour filled: a fifteen-year-old with
				// a quarry job, living at home, was measured starving at hunger 85 beside
				// a larder holding 25.82 meals and rising, kept out by eighteen hundredths
				// of a portion. Coming of age was the most dangerous event in the village
				// — of those reaching fifteen, forty per cent died within three years, and
				// twenty-one of thirty-two of those deaths were hunger.
				home := &s.Structs[c.Home]
				take := float32(FoodPerMeal)
				if spare := home.Stock[Food] - s.childrensShare(c.Home); take > spare {
					take = spare
				}
				home.Stock[Food] -= take
				c.Hunger -= c.Hunger * (take / FoodPerMeal)
				if c.Hunger < 0 {
					c.Hunger = 0
				}
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
	// Do not set out for a shop that has nothing to sell you.
	//
	// nearestWith counted any stock above zero as stock, while buyTools needs a whole tool.
	// A store holding 0.02 of a tool therefore advertised itself to every worker in the
	// village, who walked there, could not buy, walked home, and set out again the next
	// evening — for years. Measured: all twenty-two adults with tools worn to 0.000, the
	// stores holding 0.02 between them, and this branch consuming 50.6% of all adult time,
	// which is the entire off-shift half of the day.
	//
	// The garden sits below this in the cascade, so not one adult in the village ever
	// worked it. The thing the design leans on to feed dependants was switched off by a
	// rounding disagreement between two functions — the same fault as the lumber camp
	// sited on timber it could not reach. Whenever two pieces of code measure the same
	// quantity, they have to measure it the same way.
	if c.Job != NoStruct && c.Tools < ToolBuyBelow && !s.Tick.IsWorkTime() {
		if shop := s.nearestWith(c.Pos, Store, Tools); shop != NoStruct &&
			s.Structs[shop].Stock[Tools] >= 1 {
			if c.Gold >= s.Prices[Tools]+LarderReserve && s.T.Dist(c.Pos, s.Structs[shop].Pos) < 30 {
				if s.moveToward(id, shop) {
					s.buyTools(id, shop)
				}
				// Not GoingToEat: this is a trip to the ironmonger, and labelling it as
				// food sent the diagnosis chasing the granary for hours.
				c.Activity = GoingToWork
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

	// Off shift: home to work the garden, and to the diggings only if there is no coin in
	// the house at all.
	//
	// The order matters and was learned the hard way. Sending parents who were merely short
	// of money to the diggings of an evening was tried and was worse — adulthood fell from
	// eighteen per cent to ten and nobody born here married at all — because an evening in
	// the garden grows food worth more than an evening's panning ever paid. The garden
	// already is the extra work, and it is the better paid. So the garden comes first and
	// keeps first refusal on the evening.
	//
	// What has changed is who may pan at all. The faucet was gated on unemployment (§4.2),
	// and this village has about forty posts for seventeen adults, so nobody was ever
	// unemployed, so in twelve measured years not one coin entered the world while the
	// money supply bled away through inheritance. Destitution is the condition the faucet
	// was actually meant to answer — a man with no money is exactly who it exists for,
	// whether or not somebody has him on a payroll.
	//
	// It is safe to open only because panning now pays less than a day's food. While it
	// yielded more than double subsistence this rule would have emptied the fields.
	if c.Home != NoStruct {
		if s.moveToward(id, c.Home) {
			c.Activity = Resting
			s.tendGarden(id)
			// Garden stocked and not a coin to their name: go and find some.
			//
			// The gate is true destitution, deliberately, and widening it to ordinary
			// shortness of money (under LarderReserve) was measured and reverted: R0
			// 0.621 -> 0.509. The flaw is that pan() is an expedition, not an evening —
			// a panner whose gold is distant goes prospecting and camps at the claim,
			// which was right for the jobless and is ruinous for a parent, who walks
			// away from garden and children for 0.3 a day. And it minted almost nothing:
			// one gold in eight years. The money-supply bleed the ledger found is real,
			// but its fix is on the sink side, not by sending parents to the river.
			if c.Gold < s.FoodPriceAt(c.Pos)*MealsPerDay && c.Activity != Gardening {
				s.pan(id)
			}
		} else if c.Job != NoStruct {
			c.Activity = GoingHome
		}
		return
	}
	// No home to go to and nothing else to do with the evening.
	if c.Gold < s.FoodPriceAt(c.Pos)*MealsPerDay {
		s.pan(id)
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
		s.Led.GoldMinted += take
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

	// A larger household keeps a larger pantry; a flat figure meant a family of six had
	// less than a day's food in store however hard they worked.
	// How full is full enough is a matter of temperament. A diligent gardener keeps
	// filling a store the slack one would have walked away from, which is what makes one
	// household ride out a bad season and the next one not.
	if home.Stock[Food] >= s.larderTarget(c.Home)*c.Traits.Diligence {
		return
	}
	share := 1.0
	if c.Stage() == Child {
		share = ChildGardenShare
	}
	// Good ground grows more, so where a house is put matters.
	soil := 0.5 + 0.5*float64(s.World.Soil[s.T.Index(home.Cell)])
	grown := float32(GardenYield * share * soil * float64(c.Traits.Diligence))
	home.Stock[Food] += grown
	s.Led.FoodGardened += grown
	c.Activity = Gardening
}

// larderTarget scales the household store with the number of mouths in it.
func (s *State) larderTarget(home StructID) float32 {
	n := s.Structs[home].Occupants
	if n < 1 {
		n = 1
	}
	return float32(n) * LarderPerPerson
}

// countHouseholds refreshes how many people live in each home. Once a day is ample: the
// figure changes only on a birth, a death, or a move.
func (s *State) countHouseholds() {
	for i := range s.Structs {
		if s.Structs[i].Type == Home {
			s.Structs[i].Occupants = 0
			s.Structs[i].CrowdHeads = 0
		}
	}
	for i := range s.Chars {
		if c := &s.Chars[i]; c.Alive && c.Home != NoStruct {
			s.Structs[c.Home].Occupants++
			if c.Stage() == Child {
				s.Structs[c.Home].CrowdHeads += 0.5
			} else {
				s.Structs[c.Home].CrowdHeads++
			}
		}
	}
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
	// Escheat (§8.1a): the share that once vanished passes to the town hall, as heirless
	// property passed to the lord. The ledger measured the old destruction at over one
	// per cent of the money supply a year with the faucet shut — a slow strangulation
	// converted here into civic revenue. Destruction remains only when no hall stands.
	residue := c.Gold - estate
	escheated := false
	for i := range s.Structs {
		if s.Structs[i].Alive && s.Structs[i].Type == TownHall {
			s.Structs[i].Gold += residue
			escheated = true
			break
		}
	}
	if !escheated {
		s.Led.GoldDestroyed += residue
	}
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
		// Only worth doing while something is in the ground. Out of season a farm employs
		// nobody usefully, which is part of the rhythm: the winter labour surplus is
		// exactly when a village builds things.
		if s.Tick.InGrowingSeason() {
			st.produce += float32(FarmYieldPerWorker * effort)
		}
	case Fishery:
		// Year round, and that is the whole point of it. Yield scales with the water in
		// reach, so a sea coast out-fishes a creek. Fish are not depleted here: the
		// grounds are large against a village's appetite, and a stock model belongs with
		// the depletion machinery if over-fishing is ever wanted as a mechanic.
		ground := waterWithin(s.World, st.Cell, FisheryReach) / 20
		if ground > 1 {
			ground = 1
		}
		st.Stock[Food] += float32(FishPerWorkerDay / WorkTicksPerDay * effort * ground)
		s.Led.FoodFished += float32(FishPerWorkerDay / WorkTicksPerDay * effort * ground)
	case LumberCamp, Quarry, Mine:
		s.extract(c.Job, extractPerWorkerDay(st.Type)/WorkTicksPerDay*effort)
	case Workshop:
		s.manufacture(c.Job, CraftPerWorkerDay/WorkTicksPerDay*effort)
	case BuildSite:
		s.build(c.Job, effort)
	case Home:
		// Domestic work is productive work: the kitchen garden, the poultry, the
		// preserving. It feeds the household that pays for it — and stops when the
		// household is fed.
		//
		// It did not stop, and the uncapped surplus quietly became the dominant fact of
		// the whole economy. Servants poured food into their employer's larder every
		// working tick forever, and post-mortems on starved toddlers found the result:
		// households holding eighty-two THOUSAND meals in a village of thirty-eight,
		// ninety per cent of all food in the world sitting in private hoards that never
		// sold and fed nobody else, while children died beside larders reading 0.0 and
		// parents holding hundreds of gold found the granary empty — because FoodDays
		// counted the hoards, so the village believed itself provisioned for a decade and
		// stopped sowing.
		//
		// The cap mirrors the garden's: a servant stocks the larder to its target and
		// then finds other work about the house that produces no food. The wage still
		// flows — the household paying servants to keep an already-kept house is exactly
		// the wealth-to-wages sink domestic service exists to be.
		if st.Stock[Food] < s.larderTarget(c.Job)*c.Traits.Diligence {
			soil := 0.5 + 0.5*float64(s.World.Soil[s.T.Index(st.Cell)])
			served := float32(ServantYield * effort * soil)
			st.Stock[Food] += served
			s.Led.FoodServed += served
		}
	}

	// The work itself can kill you.
	if d := Danger[st.Type]; d > 0 {
		// Tools and experience keep a worker alive: a novice with worn kit is in far more
		// danger than an old hand who knows the ground.
		safety := 1 / (1 + 0.5*float64(c.Tools) + 0.4*math.Log(1+float64(c.Skill[st.Type])))
		hazard := d * safety / WorkTicksPerYear
		if s.rng.Chance(hazard) {
			s.DeathsAccident++
			s.kill(id, CauseAccident)
			return
		}
		if s.rng.Chance(hazard * InjuriesPerDeath) {
			// Injured: hurt badly enough to lose the rest of the season's strength.
			c.Health -= 35
			s.Injuries++
			if c.Health <= 0 {
				s.DeathsAccident++
				s.kill(id, CauseAccident)
				return
			}
		}
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
	// Foraged food is eaten as it is found; produced and consumed in the same act. One
	// meal answers HungerEatThreshold points of hunger — the same identity MealsPerDay is
	// derived from — so that is the conversion.
	s.Led.FoodForaged += float32(yield / HungerEatThreshold * FoodPerMeal)
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
	return s.NearestFoodSource(s.Chars[id].Pos)
}

// NearestFoodSource finds the closest place a meal can be bought from a given position.
//
// Exported because it answers the question that explains most deaths in this simulation —
// how far is this person from anywhere selling food — and the inspector needs to show it.
func (s *State) NearestFoodSource(pos torus.Vec2) StructID {
	c := struct{ Pos torus.Vec2 }{pos}
	best, bestD := NoStruct, math.MaxFloat64
	sellers := append(append([]StructID{}, s.byType(Granary)...), s.byType(DiningHall)...)
	for _, sid := range sellers {
		st := &s.Structs[sid]
		if !st.Alive || st.Stock[Food] < FoodPerMeal {
			continue
		}
		// Granaries and the forward kitchens supplied from them. Farms stay excluded:
		// letting people buy at the farm gate cuts the middleman out of the trade it
		// exists to conduct, and every coin ends up in whichever barn is nearest the
		// houses.
		if d := s.T.Dist2(c.Pos, st.Pos); d < bestD {
			best, bestD = sid, d
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
	// Buy what the purse and the shelf allow — a short measure included, with no floor and
	// no exact-equality guard.
	//
	// This function used to be an absorbing state, and it was the largest single killer of
	// young adults in the village. Two faults compounded:
	//
	//   - `want` was clamped to Gold/price and the sale then refused if want*price > Gold.
	//     In float32 that product rounds above Gold often enough that the refusal fired,
	//     and since the refusal changed nothing — same gold, same price — it fired again
	//     every tick, forever. Twelve of seventeen post-mortemed young hunger deaths were
	//     holding exactly 0.32 gold, standing 0.9 cells from a stocked granary, activity
	//     "fetching food": customers at the counter, refused over an epsilon, until dead.
	//   - Anything under a whole meal was rounded UP to one and then refused as
	//     unaffordable, so the poorest bought nothing at all — the same all-or-nothing
	//     fault feedChild had, at the till instead of the larder.
	//
	// The loop was absorbing regardless of income: hungry sent them to buy, the buy
	// failed, hunger kept them coming back, and they never worked another tick. A farmhand
	// earning ten gold a day died at the counter with 0.32 in hand. Coming of age was
	// lethal mostly because the newly adult start with pocket change, and their first
	// marginal purchase walked straight into this.
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
	price := s.FoodPriceAt(st.Pos)
	if affordable := c.Gold / price; want > affordable {
		want = affordable
	}
	if want <= 0 {
		return
	}
	cost := want * price
	if cost > c.Gold {
		cost = c.Gold // rounding, not price policy: never refuse a sale over an epsilon
	}
	c.Gold -= cost
	st.Gold += cost
	st.revenue += cost
	st.Stock[Food] -= want
	c.Rations += want
	st.lastTrade = s.Tick
	s.consume(Food, want)
	// Pay the farmer whose grain this was. The seller keeps the margin as commission,
	// which is the whole of its income now.
	s.settleSale(src, Food, want)

	if c.Hunger > HungerEatThreshold && c.Rations > 0 {
		take := float32(math.Min(FoodPerMeal, float64(c.Rations)))
		c.Rations -= take
		c.Hunger -= c.Hunger * (take / FoodPerMeal)
		if c.Hunger < 0 {
			c.Hunger = 0
		}
	}
	c.Activity = Resting

	// While here, stock the household larder for any children at home. Children cannot
	// work and so cannot buy; somebody has to shop for them.
	s.provision(id, src)
}

// childrensShare is how much of a larder is spoken for by the children of the house.
//
// Kept generous — several days for each child — because a parent who can forage giving up
// the pantry costs them discomfort, while a child who cannot forage losing it costs them
// their life.
func (s *State) childrensShare(home StructID) float32 {
	kids := 0
	for i := range s.Chars {
		c := &s.Chars[i]
		if c.Alive && c.Home == home && c.Age < ForageAge {
			kids++
		}
	}
	return float32(kids) * ChildLarderReserve
}

// ChildLarderReserve is how many meals are held back per small child.
// ChildLarderReserve is how many meals are held back per small child.
//
// Cut to 2 once, and put back. The reasoning was that five meals a head walled off a larder
// a large household rarely exceeds, which was true — but the wall was never the size of the
// reserve, it was that the test demanded a whole portion *above* it and gave nothing below.
// Partial meals fix that; shrinking the reserve as well simply removed the protection.
// Measured over four villages, cutting it to 2 while giving adults easier access to the
// larder took infant hunger deaths from 0 to 10 and survival to age five from 65.6% to
// 56.6%. An infant cannot forage, cannot work a garden and cannot shop, so what is set
// aside for them has to be genuinely set aside.
const ChildLarderReserve = 5

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
	st.lastTrade = s.Tick
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
	// Shop for the household you have, not for a household of one.
	//
	// This filled the larder to a flat eight meals however many mouths were at home, while
	// larderTarget — six meals a head — sat right beside it and was consulted only by the
	// garden. Two answers to the same question, and the wrong one was what parents shopped
	// against: a house with four small children burned eight meals in a day and a half.
	target := s.larderTarget(c.Home)
	if home.Stock[Food] >= target {
		return
	}

	// A parent with a hungry child spends down to nothing, and spends the child's own
	// money on the child's food.
	//
	// Neither was true, and between them they starved the poor. LarderReserve stops an
	// adult spending their last coins on the pantry, which is sound when the alternative is
	// that the earner cannot eat — but applied unconditionally it pins a struggling
	// household at the reserve forever. The measured case: two granary hands, both earning
	// well above subsistence, four small children, and both parents sitting at 8.31 and
	// 8.95 gold against a reserve of 8. They could never buy one meal for the larder, so
	// the children lived on the garden alone and the youngest was starving at eighteen
	// months. The children were holding 6.55 gold each at the time — money inherited from
	// somebody, stranded in the pockets of toddlers with no way to spend it.
	hungryChild := false
	var childPurse float32
	for j := range s.Chars {
		k := &s.Chars[j]
		if !k.Alive || k.Home != c.Home || k.Stage() != Child {
			continue
		}
		childPurse += k.Gold
		if k.Hunger > HungerEatThreshold {
			hungryChild = true
		}
	}
	reserve := float32(LarderReserve)
	if hungryChild {
		reserve = 0
	}

	// takeChildGold draws on the children's own purses, eldest slot first, for their food.
	takeChildGold := func(cost float32) {
		for j := range s.Chars {
			k := &s.Chars[j]
			if !k.Alive || k.Home != c.Home || k.Stage() != Child || cost <= 0 {
				continue
			}
			take := k.Gold
			if take > cost {
				take = cost
			}
			k.Gold -= take
			childPurse -= take
			cost -= take
		}
	}

	st := &s.Structs[src]
	for home.Stock[Food] < target {
		cost := s.FoodPriceAt(st.Pos) * FoodPerMeal
		if st.Stock[Food] < FoodPerMeal {
			return
		}
		switch {
		case childPurse >= cost:
			takeChildGold(cost)
		case c.Gold >= cost+reserve:
			c.Gold -= cost
		default:
			return
		}
		st.Gold += cost
		st.revenue += cost
		st.Stock[Food] -= FoodPerMeal
		home.Stock[Food] += FoodPerMeal
		st.lastTrade = s.Tick
		s.consume(Food, FoodPerMeal)
		s.settleSale(src, Food, FoodPerMeal)
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
	if home.Stock[Food] <= 0 {
		// An empty larder means the household is failing to provision, which is how a
		// household in trouble starves its young first. Letting the household buy directly
		// from a granary at this point was tried and was strictly worse: it drained the
		// treasury faster than wages could refill it and collapsed the whole village.
		return
	}

	// A short meal is still a meal.
	//
	// This used to demand a whole portion and give nothing at all below it, which killed
	// children over rounding. The case that showed it: a toddler of eighteen months at
	// hunger 85 and climbing, in a house holding 0.93 of a meal — its garden yielding food
	// at almost exactly the rate six people ate it — refused every tick for being seven
	// hundredths short. A household able to put 0.93 meals a day on the table fed nobody
	// while one managing 1.0 fed everyone, so the line between life and death fell inside
	// the rounding of a single portion.
	//
	// Nothing about eating is all-or-nothing. Take what is there and be fed in proportion,
	// so a household in difficulty raises hungry children rather than dead ones.
	take := float32(FoodPerMeal)
	if take > home.Stock[Food] {
		take = home.Stock[Food]
	}
	home.Stock[Food] -= take
	c.Hunger -= c.Hunger * (take / FoodPerMeal)
	if c.Hunger < 0 {
		c.Hunger = 0
	}
}

// Household formation (§3.3).
//
// A couple with nowhere to put a family builds one. This is how villages actually grew,
// and it does more here than relieve a housing shortage: it makes marriage economic. A
// pair must be able to afford timber and stone before they can set up on their own, which
// is close to the historical position — a household waited on the means to keep it.
//
// It is also the material economy's first real customer. Every new family is demand for
// wood and stone, which puts money into the lumber camp and the quarry, which until now
// produced things nobody had any use for.
const (
	// HouseFundMargin is how much beyond the bare cost of materials a couple wants in
	// hand before they will commit, covering the builders' wages.
	HouseFundMargin = 1.6

	// MaterialMargin is the slack a commission allows for prices moving between ordering
	// and building.
	MaterialMargin = 1.25

	// BuildWagePremium is what construction must pay against subsistence to draw somebody
	// out of a job they already hold. Below about this the site stands empty: JobSwitchGain
	// means an existing post has to be beaten, not matched.
	BuildWagePremium = 2.0

	// LuxuryMargin is how much better off than the cost of the work a household must be
	// before it improves a house it does not yet need.
	//
	// This is the discretionary motive, and it is what makes house-building a sink for
	// profit rather than only a response to crowding. Set well above the bare cost so that
	// it is the comfortable who build, not the merely solvent.
	LuxuryMargin = 6.0
	// A couple leaves when the house they are in is genuinely full — measured against its
	// capacity, which grows when it is improved. That is what connects the two: a family
	// that extends its house keeps its children under the same roof, and one that cannot
	// afford to watches them leave and build their own.
)

// stepHouseholds lets couples who cannot be accommodated build their own house.
func (s *State) stepHouseholds() {
	// Once a day is ample; nobody decides to build a house twice in an afternoon.
	if s.Tick%TicksPerDay != 0 {
		return
	}

	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Partner == NoChar || c.Sex != 0 || c.Stage() != Adult {
			continue
		}
		p := &s.Chars[c.Partner]
		if !p.Alive {
			continue
		}
		// Already waiting on one?
		if c.newHome != NoStruct {
			if int(c.newHome) < len(s.Structs) && s.Structs[c.newHome].Type == BuildSite {
				continue // still going up
			}
			c.newHome = NoStruct
		}
		// Is the house too full for this couple to bear?
		//
		// "Too full" used to mean full to capacity, and capacity counts only housed
		// adults — so households routinely reached ten to thirteen occupants before any
		// couple in them thought of leaving, and crowding had done its killing long before
		// the trigger fired. The decomposition put the toll plainly: the one-to-five band
		// is the largest graveyard in the village, and its two causes — crowding disease
		// and hunger — are both functions of too many people around one hearth and one
		// garden. Every house that gets built is fewer occupants AND a new garden, food
		// that never passes through money (§4.2), so leaving earlier attacks both.
		//
		// How full is too full is temperament, not arithmetic — this is Rootedness doing
		// the job it was defined for (§3.7), on a decision that matters more than job
		// distance. A restless couple sets out when the house is at three-quarters;
		// clannish ones endure half as much again. And because crowded households bury
		// more of their children, the trait is now under real selection: whichever
		// tolerance actually raises more survivors is the one that spreads. That is the
		// mechanism proposed for it, and the reason it had to wait until houses could
		// actually be built — selection cannot choose an option nobody can take.
		if c.Home != NoStruct {
			home := &s.Structs[c.Home]
			tolerance := float64(home.Capacity()) * 0.75 *
				float64(c.Traits.Rootedness+p.Traits.Rootedness) / 2
			if float64(home.Occupants) < tolerance {
				continue
			}
		}
		if s.findHomeWithRoom(c.Pos) != NoStruct && c.Home == NoStruct {
			continue // somewhere existing will take them
		}

		// Can they afford it?
		cost := s.houseCost()
		if c.Gold+p.Gold < cost {
			continue // saving still
		}
		site, ok := s.findHomeSite(c.Pos)
		if !ok {
			continue // nowhere to put it
		}

		id := s.Build(Home, site)
		// The couple pays for it. Their savings become the site's working capital, which
		// it spends on materials and wages.
		share := cost * (c.Gold / (c.Gold + p.Gold))
		c.Gold -= share
		p.Gold -= cost - share
		s.Structs[id].Gold += cost
		c.newHome, p.newHome = id, id
		s.diarise(CharID(i), "commissioned a house at %v with %d, for %.1f gold", site, c.Partner, cost)
		s.HousesCommissioned++
	}
}

// houseCost is what a couple must have in hand to commission a house: the materials, plus
// a wage bill for the people who will put it up.
//
// The labour half used to be a flat multiplier on materials and was not reserved, so the
// site spent everything it had on timber and stone and had about a gold and a half left to
// pay with. Building offered 0.34 gold a day against a subsistence wage of 0.467, so it
// paid less than eating cost and nobody in full employment would take it. Eleven sites
// stood with their materials delivered, unstaffed, making no progress for ten years.
//
// A wage that has to draw people out of other work must beat what they already earn, not
// merely cover their food, so the bill is struck above subsistence.
func (s *State) houseCost() float32 {
	return s.materialCost(Home) + s.labourCost(Home)
}

// materialCost is what a structure's materials fetch at current prices.
func (s *State) materialCost(t StructType) float32 {
	var materials float32
	for r := Resource(0); r < NumResources; r++ {
		materials += Defs[t].BuildCost[r] * s.Prices[r]
	}
	return materials * MaterialMargin
}

// labourCost is the wage bill for putting a structure up: a full crew for the build, at a
// wage worth leaving another job for.
func (s *State) labourCost(t StructType) float32 {
	crew := float32(Defs[BuildSite].Jobs)
	days := float32(Defs[t].BuildDays)
	return crew * days * WorkTicksPerDay * s.SubsistenceWage() * BuildWagePremium // world rate: a commission is priced before its site is chosen
}

// findHomeSite looks for somewhere worth raising a family.
//
// Good soil matters more than it looks: the kitchen garden is most of what a household
// eats, so where a house stands decides whether it can feed its own children.
func (s *State) findHomeSite(near torus.Vec2) (torus.Cell, bool) {
	from := s.T.CellAt(near)
	best, bestScore := torus.Cell{}, -1.0

	for r := 2; r <= 14; r += 2 {
		steps := maxInt(10, r*4)
		for k := 0; k < steps; k++ {
			ang := 2 * math.Pi * float64(k) / float64(steps)
			cell := s.T.WrapCell(torus.Cell{
				X: from.X + int(math.Round(float64(r)*math.Cos(ang))),
				Y: from.Y + int(math.Round(float64(r)*math.Sin(ang))),
			})
			if !CanPlace(s.World, Home, cell) || s.Occupied(cell) {
				continue
			}
			idx := s.T.Index(cell)
			score := float64(s.World.Soil[idx])
			// Near enough to the village to buy bread and find work.
			if food := s.NearestFoodSource(s.T.Center(cell)); food != NoStruct {
				d := s.T.Dist(s.T.Center(cell), s.Structs[food].Pos)
				score *= 1 / (1 + d/10)
			} else {
				score *= 0.2
			}
			if score > bestScore {
				best, bestScore = cell, score
			}
		}
	}
	return best, bestScore > 0
}

// stepUpgrades lets a crowded household that can afford it improve its house.
//
// The alternative to a young couple leaving to build: a prosperous family extends what it
// has. Both happen in a village, and which one a household chooses comes down to whether
// it has room to grow and money to spend.
func (s *State) stepUpgrades() {
	if s.Tick%TicksPerDay != 0 {
		return
	}
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Type != Home {
			continue
		}
		if st.Improving > 0 {
			st.Improving--
			if st.Improving == 0 {
				st.Level++
				s.Upgrades++
			}
			continue
		}
		need := UpgradeCost(Home, st.Level)
		var materials float32
		for r := Resource(0); r < NumResources; r++ {
			materials += need[r] * s.Prices[r]
		}

		// What the household can put up between them.
		var purse float32
		for j := range s.Chars {
			if c := &s.Chars[j]; c.Alive && c.Home == StructID(i) && c.Stage() != Child {
				purse += c.Gold
			}
		}

		// Two reasons to build: needing the room, or being able to afford it.
		//
		// The second is the point of adding it. Owner profits had nowhere to go — a
		// business made money, its owner accumulated it, and the coin left circulation for
		// good, which held every wage in the village at subsistence. A house is what the
		// wealthy spent on, and it is the right sink here for reasons beyond authenticity:
		// unlike founding another business it adds no posts to a village that already has
		// forty for seventeen adults, it pays the lumber camp and the quarry, which have
		// never had a customer and have sat at their price floor all along, and the room it
		// adds is room for children.
		//
		// Uncapped and steepening, so wealth always has somewhere further to go: each level
		// costs UpgradeCostFactor times the last.
		short := false
		crowded := st.Occupants >= st.Capacity()-1
		wealthy := purse >= materials*LuxuryMargin
		if !crowded && !wealthy {
			continue
		}
		if purse < materials*HouseFundMargin {
			continue // saving still; the margin keeps them from spending their last coin
		}

		// The materials must be there before a penny moves.
		//
		// Getting this order wrong was ruinous. The household paid first and the code
		// bailed out with the money left sitting in the house whenever the timber was
		// short — every day, over and over — so households were stripped to nothing within
		// a year and the village fell to four people. Check first, then pay.
		for r := Resource(0); r < NumResources; r++ {
			if need[r] <= 0 {
				continue
			}
			src := s.nearestWith(st.Pos, Storehouse, r)
			if src == NoStruct || s.Structs[src].Stock[r] < need[r] {
				short = true
			}
		}
		if short {
			continue // the materials are not to be had yet, and nobody has paid for them
		}

		// The household puts the money up, and the house buys the materials with it. The
		// home used to buy from its own gold, of which a house has none, so every purchase
		// moved nothing, the materials never arrived and no upgrade in the game had ever
		// completed.
		for j := range s.Chars {
			c := &s.Chars[j]
			if !c.Alive || c.Home != StructID(i) || c.Stage() == Child || purse <= 0 {
				continue
			}
			share := materials * (c.Gold / purse)
			if share > c.Gold {
				share = c.Gold
			}
			c.Gold -= share
			st.Gold += share
		}

		// Every coin the household spends arrives at a storehouse and travels back down
		// the chain to the people who felled the timber and cut the stone. Nothing is
		// destroyed on the way, which is what makes this circulation rather than one more
		// hole in the money supply.
		for r := Resource(0); r < NumResources; r++ {
			if need[r] <= 0 {
				continue
			}
			if src := s.nearestWith(st.Pos, Storehouse, r); src != NoStruct {
				s.transact(src, StructID(i), r, need[r], s.Prices[r])
			}
		}
		for r := Resource(0); r < NumResources; r++ {
			if st.Stock[r] < need[r] {
				short = true
			}
		}
		if short {
			continue
		}
		for r := Resource(0); r < NumResources; r++ {
			st.Stock[r] -= need[r]
			s.consume(r, need[r])
		}
		st.Improving = UpgradeDays
	}
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
			// near (§3.3) — or, failing that, with someone from a settlement within
			// exogamy range (§2.7a). Pooled marriage markets are the answer to the Allee
			// threshold: the coincidence process that fails at one small village works
			// across two sharing their eligible singles, which is historically exactly
			// why isolated hamlets exchanged spouses.
			d := s.T.Dist(c.Pos, o.Pos)
			reach := 25.0
			if s.Colonies > 0 {
				reach = ExogamyRange
			}
			if d > reach {
				continue
			}
			if d > 25 {
				// Exogamy proper: a match across settlements. The gate on Colonies is
				// load-bearing — before any daughter existed, the long range matched
				// pairs across one valley and relocated people (jobs quit, homes and
				// children left) for what should have been an ordinary local marriage.
				// Measured: every seed's population well below its hall-era trajectory,
				// adult hunger tripled, no colony involved. Settlements exchange
				// partners; a settlement does not exchange partners with itself.
				// A distant match: the less rooted partner relocates (the trait doing
				// its §3.7 job), ties broken toward the higher ID so the outcome never
				// depends on iteration order. Arrival is abstracted as the colony
				// founding's is; the walk is owed to the roads milestone.
				mover, stay := CharID(i), CharID(j)
				if s.Chars[j].Traits.Rootedness < s.Chars[i].Traits.Rootedness ||
					(s.Chars[j].Traits.Rootedness == s.Chars[i].Traits.Rootedness && j > i) {
					mover, stay = CharID(j), CharID(i)
				}
				m := &s.Chars[mover]
				s.quitJob(mover)
				if m.Home != NoStruct && m.housed {
					s.Structs[m.Home].Residents--
				}
				m.housed = false
				m.Home = NoStruct
				m.Pos = s.Chars[stay].Pos
				s.assignHome(mover)
				s.diarise(mover, "married into the settlement at %v", s.T.CellAt(m.Pos))
			}
			c.Partner, o.Partner = CharID(j), CharID(i)
			s.diarise(CharID(i), "married %s (aged %.0f and %.0f)", s.FullName(CharID(j)), c.Age, o.Age)
			s.diarise(CharID(j), "married %s", s.FullName(CharID(i)))
			if c.marriedAt == 0 {
				c.marriedAt = c.Age
			}
			if o.marriedAt == 0 {
				o.marriedAt = o.Age
			}
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
	m := &s.Chars[mother]
	pos := s.Structs[home].Pos

	// A child takes after both parents. Where there is no father on record — he died
	// before the birth — the mother's temperament stands for the pair.
	father := m.Traits
	familyLine := m.FamilySeed
	if m.Partner != NoChar && s.AliveChar(m.Partner) {
		father = s.Chars[m.Partner].Traits
		familyLine = s.Chars[m.Partner].FamilySeed
	}

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
		Traits:   inheritTraits(s.rng, m.Traits, father),
		NameSeed: uint32(s.rng.Uint64()),
		// The family name descends from the father where there is one, the mother
		// otherwise. Lines can therefore be followed down the generations — and, since
		// a line ends when nobody carries it, watched to die out. Surname extinction is
		// a real process (a branching one), and a valley coming to be dominated by three
		// families is a thing that will happen here without anything arranging it.
		FamilySeed: familyLine,
	})
	// No Residents++ here on purpose. A child lives with its parents and takes no
	// housing slot of its own until it grows up (see the housed flag). Counting them
	// here leaked: births incremented the tally, but kill only decrements it for the
	// housed, so every child born or died pushed the count permanently upward until
	// every home looked full and adults could not be housed at all.
	s.Births++
	s.diarise(id, "born, to a mother of %.0f, in home %d", m.Age, home)
	s.diarise(mother, "gave birth to %s (child %d of hers)", GivenName(s.Chars[id].NameSeed), m.Children+1)
	// Both parents are credited, so completed fertility can be counted from either side.
	m.Children++
	if m.Partner != NoChar && s.AliveChar(m.Partner) {
		s.Chars[m.Partner].Children++
	}
	_ = id
}
