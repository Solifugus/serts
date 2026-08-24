package sim

import "github.com/solifugus/serts/internal/torus"

// Markets: where a price is quoted.
//
// Prices were global while the world was one village, which was right then and is a
// fiction now. Food is grown and eaten in a valley; its scarcity is a fact about that
// valley. Two settlements sharing one price cannot signal a shortage to each other, so
// there is nothing for trade to respond to and nothing for a merchant to profit by —
// the price signal that steers every other decision in the simulation (§4.3) simply
// stops working the moment the world has more than one market.
//
// Each town hall quotes its settlement's food price. Everything positional resolves
// through the nearest hall; anything genuinely world-wide (materials, for now) still
// uses State.Prices, which for food is kept as the world mean for reporting.

// localPricesOff holds per-market pricing back until labour can move between
// settlements.
//
// The machinery is built and correct — each hall quotes its own settlement's food price,
// and everything positional resolves through the nearest hall. Measured on its own it
// cost about nine per cent of the world's population, with a legible mechanism: a
// struggling valley prices food dear, which raises its local subsistence wage, which
// makes its own jobs read as unpayable, which fires the patience clock and empties its
// payroll exactly when it needs workers. A death spiral, because nothing equilibrates
// it. The missing counterweight is people MOVING to where food is cheap; exogamy shifts
// a few spouses and nothing else moves labour between valleys.
//
// Local prices and inter-settlement migration are one feature, and shipping the first
// without the second is what the measurement caught. Flip this when migration exists.
const localPricesOff = true

// byType returns every living structure of a type, cached.
//
// The same fault as the uncached hall scan, in two more places: NearestFoodSource and
// nearestWith each walked all hundred-plus structures to find the handful that could
// possibly answer — 22% of runtime between them at settlement scale. Rebuilt whenever a
// structure is added or changes type; ID order preserved, so nothing depends on
// iteration order (§9.2).
func (s *State) byType(t StructType) []StructID {
	if s.typeCache == nil {
		s.typeCache = make([][]StructID, NumStructTypes)
		for i := range s.Structs {
			st := &s.Structs[i]
			if st.Alive {
				s.typeCache[st.Type] = append(s.typeCache[st.Type], StructID(i))
			}
		}
	}
	return s.typeCache[t]
}

// halls returns the town halls, cached.
//
// The uncached scan was measured at 24% of all simulation time — every price lookup
// walking every structure in the world to find two or three halls, and price is looked
// up constantly. The cache is rebuilt whenever a structure is added, so a hall raised
// mid-tick is visible immediately; ID order is preserved, so nothing depends on
// iteration order (§9.2).
func (s *State) halls() []StructID { return s.byType(TownHall) }

// marketHall returns the hall whose market governs a position: the nearest one, on
// wrapped distance.
func (s *State) marketHall(pos torus.Vec2) StructID {
	hs := s.halls()
	if len(hs) == 1 {
		return hs[0] // the common case by far: one settlement, no arithmetic needed
	}
	best, bestD := NoStruct, 0.0
	for _, hid := range hs {
		d := s.T.Dist2(pos, s.Structs[hid].Pos)
		if best == NoStruct || d < bestD {
			best, bestD = hid, d
		}
	}
	return best
}

// FoodPriceAt is the price of a meal where somebody is standing.
func (s *State) FoodPriceAt(pos torus.Vec2) float32 {
	if h := s.marketHall(pos); h != NoStruct && s.Structs[h].FoodPrice > 0 {
		return s.Structs[h].FoodPrice
	}
	return s.Prices[Food] // no hall yet: the world price stands in
}

// SubsistenceWageAt is what a day's eating costs per working tick, locally. A wage that
// feeds a man in one valley may not in another, which is the whole point of local
// prices — and which makes work worth walking to.
func (s *State) SubsistenceWageAt(pos torus.Vec2) float32 {
	return s.FoodPriceAt(pos) * MealsPerDay / WorkTicksPerDay
}

// adjustFoodPrices moves each settlement's food price toward whatever would clear its
// own market, on the same elasticity and the same clamps as the world market (§4.3).
//
// Coverage uses the SAME basis as the world market — the smoothed demand estimate,
// allocated across settlements by population share — so that the only thing this change
// varies is locality.
//
// The first attempt measured coverage as stock over mouths, which is the more correct
// quantity and was therefore a second, unintended variable: demand[] counts only market
// purchases and misses every meal eaten from a larder or garden, so the world's estimate
// has always run low and its prices low with it. Switching basis raised every food price
// at once and cost the world 982 people to 608, with no colony founded anywhere. One
// change at a time; the undercount is a real bug and gets its own measurement later.
func (s *State) adjustFoodPrices() {
	if localPricesOff {
		return // leaves every hall at FoodPrice 0, so lookups fall through to the world
	}
	worldPop := s.Population()
	worldDemand := float64(s.demand[Food])
	for _, v := range s.Settlements() {
		h := &s.Structs[v.Hall]
		if h.FoodPrice <= 0 {
			h.FoodPrice = s.Prices[Food] // a new settlement opens at the world price
		}
		if v.Pop == 0 {
			continue
		}
		// This settlement's share of the world's measured daily demand.
		share := worldDemand * float64(v.Pop) / float64(maxInt(worldPop, 1))
		if share < 1e-6 {
			continue // nothing measured yet; leave the price where it stands
		}
		ratio := float64(v.MarketFood) / share / TargetCoverage[Food]
		step := PriceElasticity * (1 - ratio)
		if step > PriceMaxStep {
			step = PriceMaxStep
		}
		if step < -PriceMaxStep {
			step = -PriceMaxStep
		}
		h.FoodPrice *= float32(1 + step)

		lo := s.basePrices[Food] * PriceFloor
		hi := s.basePrices[Food] * PriceCeiling
		// The affordability cap, struck against local wages: a market where nobody can
		// afford the one thing they must buy is not a market under strain, it is a dead
		// one. Never below the floor, or the seller earns nothing and cannot restock.
		if cap := s.affordableFoodPriceAt(v); cap > lo && cap < hi {
			hi = cap
		}
		if h.FoodPrice < lo {
			h.FoodPrice = lo
		}
		if h.FoodPrice > hi {
			h.FoodPrice = hi
		}
	}
}

// affordableFoodPriceAt is what the prevailing local wage can pay for a day's meals.
func (s *State) affordableFoodPriceAt(v Settlement) float32 {
	var total float32
	var n int
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Filled <= 0 || st.Wage <= 0 {
			continue
		}
		if s.marketHall(st.Pos) != v.Hall {
			continue
		}
		total += st.Wage * float32(st.Filled)
		n += st.Filled
	}
	if n == 0 {
		return 0
	}
	meanDaily := total / float32(n) * WorkTicksPerDay
	return meanDaily * FoodShareOfIncome / MealsPerDay
}
