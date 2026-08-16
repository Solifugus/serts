package sim

// Ownership, consignment, and what happens to a business that fails.
//
// Until now a structure's gold belonged to nobody. Revenue accumulated, wages came out of
// it, and whatever was left simply sat there as a number with no claimant — which is not a
// simplification of an economy but a hole in one. Giving each business an owner closes it:
// the owner is the residual claimant on the payroll, which is what the other side of §1
// has always implied.
//
// It also fixes something that was killing the village. The granary used to buy the
// harvest outright, so it needed working capital to hold stock at all; when its gold
// reached zero it could not buy grain, and with no grain it could not sell, and with no
// sales it could never see gold again. Zero and zero was a state with no exit, and the
// village starved beside farms holding twenty-two thousand units of food.
//
// A granary that buys outright is a merchant, and a merchant needs capital. The pre-modern
// arrangement is consignment: the farmer keeps title to the grain, the granary holds and
// sells it, and the farmer is paid out of the proceeds while the granary keeps a
// commission. A granary with no money can still fill its shelves and still trade, so the
// deadlock cannot form. Buying the harvest outright is the capitalised version of the same
// business and properly belongs with banks, when there is capital to pool.

// Consignment is stock held by one structure on behalf of another, to be paid for when it
// sells.
//
// The ledger lives on State rather than on Structure so that Structure stays comparable
// with ==, which the determinism test depends on.
type Consignment struct {
	Holder StructID // who has the goods
	From   StructID // who is owed for them
	Res    Resource
	Units  float32
	Price  float32 // agreed per-unit settlement price
}

// The commission is not a separate constant: the granary settles with the farmer at
// FarmGateShare of the retail price and keeps the rest, so the margin it lives on is
// already expressed by the prices it trades at. A named CommissionShare beside them would
// be a second, silently conflicting answer to the same question.

// consign hands goods to a holder without payment, recording what is owed.
func (s *State) consign(from, holder StructID, r Resource, units, price float32) {
	if units <= 0 || price <= 0 {
		return
	}
	s.Structs[from].Stock[r] -= units
	s.Structs[holder].Stock[r] += units
	// Merge with the most recent matching entry rather than growing the ledger every
	// harvest. The order of settlement is unaffected and the list stays short.
	if n := len(s.Consignments); n > 0 {
		last := &s.Consignments[n-1]
		if last.Holder == holder && last.From == from && last.Res == r && last.Price == price {
			last.Units += units
			return
		}
	}
	s.Consignments = append(s.Consignments, Consignment{
		Holder: holder, From: from, Res: r, Units: units, Price: price,
	})
}

// settleSale pays the producers behind goods that have just been sold out of a holder's
// stock, oldest consignment first.
//
// The holder keeps whatever the sale fetched above the settlement price. If it cannot pay
// in full — the sale went for less than the agreed price — the producer takes what there
// is. Nothing is created: every coin paid out comes from the holder's own till.
func (s *State) settleSale(holder StructID, r Resource, units float32) {
	if units <= 0 {
		return
	}
	h := &s.Structs[holder]
	for i := range s.Consignments {
		cn := &s.Consignments[i]
		if cn.Holder != holder || cn.Res != r || cn.Units <= 0 {
			continue
		}
		take := units
		if take > cn.Units {
			take = cn.Units
		}
		owed := take * cn.Price
		if owed > h.Gold {
			owed = h.Gold
		}
		if owed > 0 {
			h.Gold -= owed
			h.revenue -= owed
			s.Structs[cn.From].Gold += owed
			s.Structs[cn.From].revenue += owed
			s.Structs[cn.From].lastTrade = s.Tick
		}
		cn.Units -= take
		units -= take
		if units <= 0 {
			break
		}
	}
	s.compactConsignments()
}

// compactConsignments drops settled entries. Called after settlement so the ledger does
// not grow without bound over a long game.
func (s *State) compactConsignments() {
	out := s.Consignments[:0]
	for _, cn := range s.Consignments {
		if cn.Units > 1e-4 {
			out = append(out, cn)
		}
	}
	s.Consignments = out
}

// releaseConsignments returns unsold goods to their producers, for when a holder is wound
// up. Grain in a dead granary belongs to the farmer who grew it, not to nobody.
func (s *State) releaseConsignments(holder StructID) {
	for i := range s.Consignments {
		cn := &s.Consignments[i]
		if cn.Holder != holder || cn.Units <= 0 {
			continue
		}
		back := cn.Units
		if have := s.Structs[holder].Stock[cn.Res]; back > have {
			back = have
		}
		s.Structs[holder].Stock[cn.Res] -= back
		if s.Structs[cn.From].Alive {
			s.Structs[cn.From].Stock[cn.Res] += back
		}
		cn.Units = 0
	}
	s.compactConsignments()
}

// --- Ownership ---

// OwnerReserve is the working balance an owner leaves in the business before drawing
// anything out. Sweeping the till bare each night would leave nothing to pay wages with
// before the next day's takings came in.
const OwnerReserve = 25.0

// OwnerDrawShare is how much of the surplus above the reserve the owner takes each day.
// The rest stays in the business, which is what lets a good trade build a buffer.
const OwnerDrawShare = 0.5

// drawProfits pays each owner what their business has earned beyond its working balance.
//
// This is the only route by which trade makes anybody personally rich, and so the only
// source of the concentrated wealth that robbery and, later, the pooling of capital both
// depend on.
func (s *State) drawProfits() {
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Owner == NoChar || Defs[st.Type].Jobs == 0 {
			continue
		}
		if !s.AliveChar(st.Owner) {
			s.succeed(StructID(i))
			continue
		}
		spare := st.Gold - OwnerReserve
		if spare <= 0 {
			continue
		}
		draw := spare * OwnerDrawShare
		st.Gold -= draw
		s.Chars[st.Owner].Gold += draw
	}
}

// succeed passes a dead owner's business on: to their partner if there is one, otherwise
// to a grown child, otherwise it falls vacant and goes to market.
func (s *State) succeed(sid StructID) {
	st := &s.Structs[sid]
	dead := st.Owner
	st.Owner = NoChar
	if dead == NoChar {
		return
	}
	if p := s.Chars[dead].Partner; p != NoChar && s.AliveChar(p) {
		st.Owner = p
		return
	}
	// A grown child, eldest first. Lineage is not recorded directly, so the household
	// stands in for it: whoever shares the dead owner's home and is old enough to run it.
	home := s.Chars[dead].Home
	if home == NoStruct {
		return
	}
	best, bestAge := NoChar, float32(0)
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Home != home || c.Stage() == Child || CharID(i) == dead {
			continue
		}
		if c.Age > bestAge {
			best, bestAge = CharID(i), c.Age
		}
	}
	st.Owner = best
}

// --- Failure ---

// DistressDiscount is what a failing owner will accept for the stock left on their hands.
// Somebody winding up sells to whoever will take it, which is not the going rate.
const DistressDiscount = 0.5

// IdleDaysBeforeSale is how long a business must be both unstaffed and untrading before
// its owner gives it up.
//
// It has to be longer than the harvest cycle. At sixty days a farm — which takes money
// once a year, when the crop comes in — counted as failing for ten months of every twelve,
// so its grain was dumped at half price over and over. Ownership changed hands twelve
// hundred times in twenty years and the village ended with no food at all. A rule for
// winding up businesses has to be slower than the slowest legitimate trade in the game.
const IdleDaysBeforeSale = 1.5 * DaysPerYear

// reviewBusinesses runs once a day: owners clear stock they can no longer trade, and give
// up businesses that have stopped working.
func (s *State) reviewBusinesses() {
	for i := range s.Structs {
		sid := StructID(i)
		st := &s.Structs[i]
		if !st.Alive || Defs[st.Type].Jobs == 0 || st.Type == BuildSite {
			continue
		}
		// Both tests, not either. Staff alone is wrong — a granary that sells all day
		// with nobody on the payroll is trading. Trade alone is wrong — a farm between
		// harvests has taken nothing for months and is perfectly healthy.
		if st.Filled > 0 || s.Tick-st.lastTrade < IdleDaysBeforeSale*TicksPerDay {
			continue
		}
		// Sell off what is left, cheap, to anyone who can use it. This is the owner
		// getting what they can rather than watching it rot.
		s.liquidateStock(sid)
		s.offerForSale(sid)
		st.lastTrade = s.Tick
	}
}

// liquidateStock dumps a failing business's own goods at a discount. Consigned goods are
// not its to sell, and go back to their producers.
func (s *State) liquidateStock(sid StructID) {
	s.releaseConsignments(sid)
	st := &s.Structs[sid]
	for r := Resource(0); r < NumResources; r++ {
		if st.Stock[r] <= 0 {
			continue
		}
		price := s.Prices[r] * DistressDiscount
		// Anyone who trades in it. The nearest buyer with money takes the lot.
		buyer := NoStruct
		bestD := 0.0
		for j := range s.Structs {
			b := &s.Structs[j]
			if !b.Alive || StructID(j) == sid || b.Gold <= 0 || Defs[b.Type].Jobs == 0 {
				continue
			}
			if !buysResource(b.Type, r) {
				continue
			}
			d := s.T.Dist(st.Pos, b.Pos)
			if buyer == NoStruct || d < bestD {
				buyer, bestD = StructID(j), d
			}
		}
		if buyer == NoStruct {
			continue
		}
		s.transact(sid, buyer, r, st.Stock[r], price)
	}
}

// buysResource reports whether a trade would have any use for a resource.
func buysResource(t StructType, r Resource) bool {
	switch r {
	case Food:
		return t == Granary || t == DiningHall
	case Wood, Stone, Iron:
		return t == Storehouse || t == Workshop
	case Tools:
		return t == Store
	}
	return false
}

// BusinessYears is how many years of takings a buyer will pay for a going concern. A
// failed one is worth a good deal less, which is what makes buying a wreck attractive.
const BusinessYears = 2.0

// offerForSale finds someone with the money and the appetite to take a business on.
//
// A business nobody will buy simply stays with its owner. It is not destroyed: a shut
// granary is still a granary, and when the village needs one again somebody will pay for
// it. That is the mechanism behind ghost towns coming back to life (§2.7).
func (s *State) offerForSale(sid StructID) {
	st := &s.Structs[sid]
	// What it is worth: its takings, discounted heavily because it is not currently
	// trading, plus whatever is in the till.
	price := st.Wage*float32(st.Jobs)*WorkTicksPerDay*DaysPerYear*BusinessYears*DistressDiscount + st.Gold
	if price < 1 {
		price = 1
	}

	best, bestGold := NoChar, price
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Stage() == Child || CharID(i) == st.Owner {
			continue
		}
		// Buyers keep enough to live on. Nobody spends their last coin on a shut shop.
		if c.Gold-price < LarderReserve*4 {
			continue
		}
		if c.Gold > bestGold {
			best, bestGold = CharID(i), c.Gold
		}
	}
	if best == NoChar {
		return
	}
	s.Chars[best].Gold -= price
	if s.AliveChar(st.Owner) {
		s.Chars[st.Owner].Gold += price
	}
	st.Owner = best
	s.BusinessSales++
}
