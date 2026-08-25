package sim

// Caravans: trade between settlements (§2.7a).
//
// A settlement in a bad year and a settlement with full barns are, at present, strangers
// with no way to help each other — grain rots in one valley while children starve in the
// next. This is the machinery that connects them, and it is deliberately the transfer of
// GOODS rather than of people: Appendix A.5, the most expensive lesson in this project,
// is that moving money and cargo works where moving behaviour ruins. Nine interventions
// have now tested it and none has overturned it.
//
// It is logistics, not arbitrage. With one world price there is no spread for a merchant
// to profit on, so trade is driven by need instead: a granary that cannot feed its own
// settlement buys from one that can spare, at the market price, and pays for carriage.
// The same shape as deliverToGranaries, which already moves a harvest from farm to
// granary — one valley further.
//
// Carriage is paid in cargo. A share of every load proportional to the distance is eaten
// on the road, which is what feeds the carters and what makes distance genuinely costly:
// a valley three days away is dearer than one next door without any price needing to say
// so. That is an abstraction of employed haulage, exactly as intra-settlement delivery
// is, and it inherits the same debt recorded in §5 — when carrying is a job, this becomes
// a payroll.

const (
	// TradeRange is how far a caravan will travel. Wider than migration, because sending
	// grain is a smaller commitment than sending a family.
	TradeRange = 200.0

	// CaravanLossPerCell is the share of a load eaten on the road, per cell travelled.
	// At a hundred cells a caravan consumes about a fifth of what it carries, which makes
	// distant trade real but marginal — as it was, before roads.
	CaravanLossPerCell = 0.002

	// TradeNeedDays is the food position at which a settlement starts buying from its
	// neighbours: less than this many days in its own market.
	TradeNeedDays = 150.0

	// TradeSpareDays is what a settlement keeps for itself before selling. Well above
	// TradeNeedDays so that grain flows from the genuinely comfortable to the genuinely
	// short, and never sloshes back and forth between two equally placed valleys.
	TradeSpareDays = 400.0
)

// stepCaravans moves food from settlements with a surplus to settlements that are short.
// Once a month: a caravan is a journey, not a daily errand.
func (s *State) stepCaravans() {
	if s.Tick%(30*TicksPerDay) != 0 {
		return
	}
	places := s.Settlements()
	if len(places) < 2 {
		return
	}

	for _, buyer := range places {
		if buyer.Pop == 0 || buyer.FoodDays >= TradeNeedDays {
			continue
		}
		// The buying granary — where the food will land and the money comes from.
		dst := s.granaryOf(buyer.Hall)
		if dst == NoStruct {
			continue
		}
		want := float32(float64(buyer.Pop)*MealsPerDay*TradeNeedDays) - buyer.MarketFood
		if want <= 0 {
			continue
		}

		for _, seller := range places {
			if seller.Hall == buyer.Hall || seller.FoodDays <= TradeSpareDays {
				continue
			}
			dist := s.T.Dist(s.Structs[buyer.Hall].Pos, s.Structs[seller.Hall].Pos)
			if dist > TradeRange {
				continue
			}
			src := s.granaryOf(seller.Hall)
			if src == NoStruct || s.Structs[src].Stock[Food] <= 0 {
				continue
			}
			// The seller keeps its own year first.
			spare := seller.MarketFood - float32(float64(seller.Pop)*MealsPerDay*TradeSpareDays)
			if spare <= 0 {
				continue
			}
			load := want
			if load > spare {
				load = spare
			}
			if load > s.Structs[src].Stock[Food] {
				load = s.Structs[src].Stock[Food]
			}
			// The buyer pays at market price for what LEAVES, and receives what arrives:
			// the road takes its share either way, which is what makes distance cost.
			price := s.Prices[Food]
			if cost := load * price; cost > s.Structs[dst].Gold {
				load = s.Structs[dst].Gold / price
			}
			if load <= 0 {
				continue
			}
			lost := float32(dist * CaravanLossPerCell)
			if lost > 0.6 {
				lost = 0.6 // beyond this the journey is not worth making
			}
			delivered := load * (1 - lost)
			if delivered <= 0 {
				continue
			}

			cost := load * price
			s.Structs[dst].Gold -= cost
			s.Structs[src].Gold += cost
			s.Structs[src].revenue += cost
			s.Structs[src].Stock[Food] -= load
			s.Structs[dst].Stock[Food] += delivered
			s.Structs[src].lastTrade = s.Tick
			s.Structs[dst].lastTrade = s.Tick
			// Eaten on the road: real consumption, not evaporation.
			s.consume(Food, load-delivered)
			s.Led.FoodCarried += delivered
			s.Caravans++

			want -= delivered
			if want <= 0 {
				break
			}
		}
	}
}

// granaryOf finds a settlement's granary — where its market food is bought and sold.
func (s *State) granaryOf(hall StructID) StructID {
	best, bestD := NoStruct, 0.0
	for _, gid := range s.byType(Granary) {
		if !s.Structs[gid].Alive {
			continue
		}
		d := s.T.Dist2(s.Structs[hall].Pos, s.Structs[gid].Pos)
		if best == NoStruct || d < bestD {
			best, bestD = gid, d
		}
	}
	if best != NoStruct && s.marketHall(s.Structs[best].Pos) != hall {
		return NoStruct // that granary belongs to another settlement
	}
	return best
}
