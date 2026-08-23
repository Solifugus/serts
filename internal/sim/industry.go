package sim

import (
	"github.com/solifugus/serts/internal/torus"
)

// The material economy (design §4.1, §5).
//
// Extraction, manufacture, and the wholesale trade that connects them. The shape mirrors
// food deliberately: something is pulled out of the ground or made, a middleman buys it,
// and gold flows back down the chain to the people who did the work. What differs is that
// materials are drawn from the map itself and run out, which is what will eventually make
// a settlement move (§2.7).

// Extraction constants, expressed per worked day and divided down to ticks.
const (
	// FellPerWorkerDay is timber cut by one worker in a day, from woodland at full
	// density.
	//
	// Cut from 1.6, which was scaled to nothing in particular. Five men at the old rate
	// demanded 2,880 units a year from a woodlot holding about 106 and regrowing perhaps
	// ten — some three hundred times what the ground could bear. The camp stripped every
	// reachable cell inside two hundred days and the village never saw timber again.
	// An extraction rate has to be set against what the resource actually renews.
	FellPerWorkerDay = 0.35
	// QuarryPerWorkerDay is stone cut by one worker in a day.
	QuarryPerWorkerDay = 1.1
	// MinePerWorkerDay is ore raised by one worker in a day. Slower than stone: it has to
	// be found and followed, not merely broken off.
	MinePerWorkerDay = 0.55

	// WorkRadius is how far from its site a camp, quarry, or mine reaches. Beyond this
	// the walk costs more than the material is worth.
	WorkRadius = 9

	// LumberRadius is how far woodcutters range, which is much further than a quarryman
	// walks. Stone and ore sit in one place and are dug; timber is scattered and is
	// fetched, and a camp works a whole district rather than a pit.
	//
	// It is also what makes forestry sustainable at all. Yield is the standing stock times
	// the regrowth rate, so the only way a camp can supply a village year after year is to
	// draw on an area large enough that the annual increment is worth having.
	LumberRadius = 18

	// A cell is considered worked out below this.
	Exhausted = 0.001

	// WoodlandFloor is the standing timber a woodcutter leaves behind rather than clearing
	// a cell to the ground.
	//
	// This is coppicing, and it is the difference between a renewable woodlot and a
	// wasteland. Regrowth spreads from neighbouring woodland (§2.5), so a camp that strips
	// every cell it touches destroys the seed source its own recovery depends on. Measured
	// with clear-felling: ninety units of standing timber within reach yielding one to
	// three units a year, against a village needing dozens — the forest was permanently
	// wrecked rather than harvested, and the whole material economy stopped behind it.
	//
	// Leaving a quarter standing costs a little of each cell's yield and returns the wood
	// indefinitely.
	WoodlandFloor = 0.25
)

// resourceOf maps an extraction structure to what it pulls out of the ground.
func resourceOf(t StructType) Resource {
	switch t {
	case LumberCamp:
		return Wood
	case Quarry:
		return Stone
	case Mine:
		return Iron
	}
	return Food
}

// groundAt returns a pointer to the extractable amount of a resource in a cell, or nil if
// that resource is not something dug out of the map.
func (s *State) groundAt(r Resource, i int) *float32 {
	switch r {
	case Wood:
		return &s.World.Woodland[i]
	case Stone:
		return &s.World.Stone[i]
	case Iron:
		return &s.World.IronOre[i]
	}
	return nil
}

// extractPerWorkerDay is the daily output of one worker at full deposit richness.
func extractPerWorkerDay(t StructType) float64 {
	switch t {
	case LumberCamp:
		return FellPerWorkerDay
	case Quarry:
		return QuarryPerWorkerDay
	case Mine:
		return MinePerWorkerDay
	}
	return 0
}

// floorFor is how much of a resource is left in the ground rather than taken. Only timber
// has one: stone and ore do not grow back, so there is nothing to be gained by leaving
// them, while woodland regrows only from what is still standing.
func floorFor(r Resource) float32 {
	if r == Wood {
		return WoodlandFloor
	}
	return Exhausted
}

// workRadiusOf is how far a site of a given type reaches for its material.
func workRadiusOf(t StructType) int {
	if t == LumberCamp {
		return LumberRadius
	}
	return WorkRadius
}

// findWorkCell picks the richest cell within reach that still holds something.
//
// The result is cached on the structure until it is worked out, so the search runs once
// per exhausted cell rather than once per tick. Scanning a nine-cell radius every tick for
// every camp would cost more than the whole rest of the simulation.
func (s *State) findWorkCell(sid StructID) {
	st := &s.Structs[sid]
	r := resourceOf(st.Type)

	radius := workRadiusOf(st.Type)
	best, bestAmount := int32(-1), floorFor(r)
	// The disc is walked row-wise with incremental wrapping — the row coordinate wrapped
	// once per row, the column stepped with a compare-and-reset — because the modulo
	// arithmetic inside WrapCell was, per the profiler, 23% of the entire simulation on
	// its own once this scan became the hot path. Same cells in the same order with the
	// same tie-breaking as the plain form; only the divisions are gone.
	cx, cy := s.T.CX, s.T.CY
	for dy := -radius; dy <= radius; dy++ {
		wy := (st.Cell.Y + dy) % cy
		if wy < 0 {
			wy += cy
		}
		row := wy * cx
		wx := (st.Cell.X - radius) % cx
		if wx < 0 {
			wx += cx
		}
		for dx := -radius; dx <= radius; dx++ {
			i := row + wx
			wx++
			if wx == cx {
				wx = 0
			}
			if dx*dx+dy*dy > radius*radius {
				continue
			}
			if !s.World.Walkable(i) {
				continue
			}
			g := s.groundAt(r, i)
			if g == nil {
				continue
			}
			// Ties break toward the lower index so the choice never depends on
			// iteration order varying between runs (§9.2).
			if *g > bestAmount {
				best, bestAmount = int32(i), *g
			}
		}
	}
	st.workCell = best
	if best < 0 {
		// Nothing in reach: remember the failure for a day. Regrowth moves at daily
		// scales, so a rediscovery delayed by at most a day costs a trickle of timber
		// and buys back nearly three-quarters of the simulation's entire CPU.
		st.workRetryAt = s.Tick + TicksPerDay
	}
}

// extract converts a worker's labour into material pulled from the ground.
//
// Called from work(), so it only happens when somebody actually turned up. Depletion is
// real: what comes out of a cell does not go back, which is what will eventually empty a
// mining town (§2.7).
func (s *State) extract(sid StructID, effort float64) {
	st := &s.Structs[sid]
	r := resourceOf(st.Type)

	if st.workCell < 0 {
		if s.Tick < st.workRetryAt {
			return // played out, and looked recently; do not scan again yet
		}
		s.findWorkCell(sid)
		if st.workCell < 0 {
			return // nothing left within reach; the site is played out
		}
	}
	g := s.groundAt(r, int(st.workCell))
	if g == nil || *g <= floorFor(r) {
		s.findWorkCell(sid)
		return
	}

	// Richer ground yields faster. Woodland is a density in [0,1]; stone and iron are
	// stocks, so they work at full rate until nearly gone.
	rate := effort
	if r == Wood {
		rate *= float64(*g)
	}
	take := float32(rate)
	// Never cut into the remnant that regrows the cell.
	if avail := *g - floorFor(r); take > avail {
		take = avail
	}
	if take <= 0 {
		st.workCell = -1
		return
	}
	*g -= take
	st.Stock[r] += take
	if *g <= floorFor(r) {
		if floorFor(r) <= Exhausted {
			*g = 0
		}
		st.workCell = -1
	}
}

// manufacture turns materials into goods at a workshop.
//
// This is where the material economy earns its place: tools are the first thing a
// character can buy that is not food, which gives gold somewhere to go besides a pocket.
const (
	// ToolWoodCost and ToolIronCost are the materials in one tool.
	ToolWoodCost = 1.4
	ToolIronCost = 0.5
	// CraftPerWorkerDay is tools finished by one worker in a day.
	CraftPerWorkerDay = 0.32
)

func (s *State) manufacture(sid StructID, effort float64) {
	st := &s.Structs[sid]
	made := float32(effort)
	if need := made * ToolWoodCost; need > st.Stock[Wood] {
		made = st.Stock[Wood] / ToolWoodCost
	}
	if need := made * ToolIronCost; need > st.Stock[Iron] {
		made = st.Stock[Iron] / ToolIronCost
	}
	if made <= 0 {
		return // idle for want of materials
	}
	st.Stock[Wood] -= made * ToolWoodCost
	st.Stock[Iron] -= made * ToolIronCost
	st.Stock[Tools] += made
	s.consume(Wood, made*ToolWoodCost)
	s.consume(Iron, made*ToolIronCost)
}

// StoreCapacity bounds what a middleman will hold of any one commodity, so that a glut
// backs up at the producer instead of being absorbed forever.
const StoreCapacity = 400

// tradeMaterials moves goods along the chain and pays for them.
//
// Producers sell to a storehouse, the storehouse sells to workshops and building sites,
// workshops sell to stores, stores sell to people. Each hop is a purchase, which is how
// the coin a villager spends on a spade finds its way back to the man who felled the tree.
func (s *State) tradeMaterials() {
	// Extractors -> storehouses.
	for _, r := range []Resource{Wood, Stone, Iron} {
		s.sellTo(Storehouse, r, func(t StructType) bool {
			return resourceOf(t) == r && t != Workshop
		})
	}
	// Workshops -> stores.
	s.sellTo(Store, Tools, func(t StructType) bool { return t == Workshop })

	// Storehouses -> workshops, which need materials to work with. The workshop is the
	// buyer here, so it pays; a workshop that cannot afford materials falls idle, which
	// is the correct and visible consequence rather than a silent stall.
	for i := range s.Structs {
		wsp := &s.Structs[i]
		if !wsp.Alive || wsp.Type != Workshop {
			continue
		}
		for _, r := range []Resource{Wood, Iron} {
			if wsp.Stock[r] >= WorkshopStock {
				continue
			}
			src := s.nearestWith(wsp.Pos, Storehouse, r)
			if src == NoStruct {
				continue
			}
			s.transact(src, StructID(i), r, WorkshopStock-wsp.Stock[r], s.Prices[r])
		}
	}
}

// WorkshopStock is how much of each input a workshop keeps on hand.
const WorkshopStock = 60

// sellTo moves a resource from every producer matching pred to the nearest middleman of
// the given type, at the wholesale price.
func (s *State) sellTo(buyer StructType, r Resource, pred func(StructType) bool) {
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Stock[r] <= 0 || !pred(st.Type) {
			continue
		}
		dst := s.nearestOfType(st.Pos, buyer)
		if dst == NoStruct {
			continue
		}
		room := StoreCapacity - s.Structs[dst].Stock[r]
		want := st.Stock[r]
		if want > room {
			want = room
		}
		s.transact(StructID(i), dst, r, want, s.Prices[r]*WholesaleShare)
	}
}

// transact moves up to want units of r from seller to buyer at a unit price, limited by
// what the buyer can afford. Gold and goods move together or not at all.
func (s *State) transact(seller, buyer StructID, r Resource, want, price float32) {
	if want <= 0 || price <= 0 {
		return
	}
	sell, buy := &s.Structs[seller], &s.Structs[buyer]
	if want > sell.Stock[r] {
		want = sell.Stock[r]
	}
	if cost := want * price; cost > buy.Gold {
		want = buy.Gold / price
	}
	if want <= 0 {
		return
	}
	buy.Gold -= want * price
	buy.revenue -= want * price
	sell.Gold += want * price
	sell.revenue += want * price
	buy.Stock[r] += want
	sell.Stock[r] -= want
	buy.lastTrade, sell.lastTrade = s.Tick, s.Tick
}

// nearestOfType finds the closest living structure of a type.
func (s *State) nearestOfType(from torus.Vec2, t StructType) StructID {
	best, bestD := NoStruct, float64(1<<62)
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Type != t {
			continue
		}
		if d := s.T.Dist2(from, st.Pos); d < bestD {
			best, bestD = StructID(i), d
		}
	}
	return best
}

// nearestWith finds the closest structure of a type that actually holds the resource.
func (s *State) nearestWith(from torus.Vec2, t StructType, r Resource) StructID {
	best, bestD := NoStruct, float64(1<<62)
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Type != t || st.Stock[r] <= 0 {
			continue
		}
		if d := s.T.Dist2(from, st.Pos); d < bestD {
			best, bestD = StructID(i), d
		}
	}
	return best
}

// --- Construction (§5) ---
//
// A planned building exists immediately as a site that hires labour and consumes
// materials, rather than as an abstract order that resolves later. It competes for
// workers in the same job market as everything else, which means an under-funded or
// under-supplied build simply cannot attract anyone — a far better failure mode than a
// silent stall, and the reason construction is a structure type at all.

// Build places a site that will become the given structure type.
func (s *State) Build(t StructType, c torus.Cell) StructID {
	sid := s.addStructure(BuildSite, c)
	st := &s.Structs[sid]
	st.Building = t
	st.Jobs = Defs[BuildSite].Jobs
	st.Wage = BaseWage
	return sid
}

// build applies one worker-tick of construction.
//
// Materials have to be on site, so a site draws them from a storehouse as it goes. What
// it cannot get, it waits for.
// supplyBuildSites buys materials for every site that can afford them, once a day.
//
// Materials used to be fetched from inside build(), which only runs when somebody is
// already working — so a site could not get timber until it had a crew, and would not
// keep a crew with nothing to build. Ordering is everything here: materials first, then
// hire.
func (s *State) supplyBuildSites() {
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Type != BuildSite {
			continue
		}
		need := Defs[st.Building].BuildCost
		for r := Resource(0); r < NumResources; r++ {
			if need[r] <= 0 || st.Stock[r] >= need[r] {
				continue
			}
			src := s.nearestWith(st.Pos, Storehouse, r)
			if src == NoStruct {
				continue
			}
			// Do not spend the wage bill on materials. A site that buys timber it has no
			// money to raise is a site that stands unfinished forever.
			want := need[r] - st.Stock[r]
			if spare := st.Gold - s.labourCost(st.Building); spare < want*s.Prices[r] {
				want = spare / s.Prices[r]
			}
			if want <= 0 {
				continue
			}
			before := st.Stock[r]
			s.transact(src, StructID(i), r, want, s.Prices[r])
			// Timber taken onto a site is spoken for and gone from the market, so it is
			// demand now rather than on the day the roof goes on.
			s.consume(r, st.Stock[r]-before)
		}
	}
}

// siteReady reports whether a site has everything it needs to employ anybody: its
// materials on the ground and the money to pay for the work.
func (s *State) siteReady(sid StructID) bool {
	st := &s.Structs[sid]
	need := Defs[st.Building].BuildCost
	for r := Resource(0); r < NumResources; r++ {
		if st.Stock[r] < need[r] {
			return false
		}
	}
	return st.Gold >= st.Wage*float32(st.Jobs)*WorkTicksPerDay
}

func (s *State) build(sid StructID, effort float64) {
	st := &s.Structs[sid]
	need := Defs[st.Building].BuildCost

	for r := Resource(0); r < NumResources; r++ {
		if st.Stock[r] < need[r] {
			return // waiting on materials
		}
	}

	days := Defs[st.Building].BuildDays
	if days <= 0 {
		days = 1
	}
	st.Progress += float32(effort / (days * float64(Defs[BuildSite].Jobs) * WorkTicksPerDay))
	if st.Progress >= 1 {
		s.completeBuild(sid)
	}
}

// completeBuild turns a finished site into the building it was becoming.
func (s *State) completeBuild(sid StructID) {
	st := &s.Structs[sid]
	t := st.Building

	// Everyone on the site is out of a job the moment it is finished; they re-enter the
	// market like anyone else and may well be hired straight back by the new building.
	for i := range s.Chars {
		if s.Chars[i].Alive && s.Chars[i].Job == sid {
			s.quitJob(CharID(i))
		}
	}

	d := Defs[t]
	st.Type = t
	st.Building = 0
	st.Progress = 0
	st.Jobs = d.Jobs
	st.Filled = 0
	st.Wage = BaseWage
	st.workCell = -1

	// Whoever paid for a house moves into it, with their children.
	if t == Home {
		for i := range s.Chars {
			c := &s.Chars[i]
			if !c.Alive || c.newHome != sid {
				continue
			}
			old := c.Home
			if old != NoStruct && c.housed {
				s.Structs[old].Residents--
			}
			c.Home, c.housed, c.newHome = sid, true, NoStruct
			s.Structs[sid].Residents++
			// Their young children come with them.
			for j := range s.Chars {
				k := &s.Chars[j]
				if k.Alive && k.Stage() == Child && k.Home == old && !k.housed {
					k.Home = sid
				}
			}
		}
	}

	// Materials went into the walls. They were counted as demand when they arrived on
	// site, so nothing is recorded here.
	st.Stock = Stock{}
	s.paths.invalidate(sid)
	s.Built++
}

// --- Supply and demand (§4.3) ---

// consume records a draw on market stock, which is what prices are steered against.
//
// The distinction is the whole of it: demand means goods leaving the market, not calories
// entering a person. Food bought at a granary is a draw; the same meal eaten an hour later
// is not, and a cabbage out of a kitchen garden never touched the market at all.
//
// An earlier version counted every meal eaten, on the reasoning that measuring sales would
// be wrong because "a commodity nobody can afford would read as a commodity nobody wants".
// That is exactly backwards. When nobody can afford food, demand *should* read as falling,
// so coverage rises and the price comes down — which is the self-correction that lets a
// market recover. Counting meals instead pinned the price at its ceiling while the village
// starved, because people kept eating out of gardens and pantries and the market read that
// as demand it was failing to meet.
func (s *State) consume(r Resource, amount float32) {
	if amount > 0 {
		s.consumed[r] += amount
		if r == Food {
			s.Led.FoodEaten += amount
		}
	}
}

// StockOf sums a commodity across every structure in the faction.
func (s *State) StockOf(r Resource) float32 {
	var t float32
	for i := range s.Structs {
		if s.Structs[i].Alive {
			t += s.Structs[i].Stock[r]
		}
	}
	return t
}

// Coverage is how many days of consumption the faction holds of a commodity.
//
// With no demand at all the ratio is undefined, and the answer depends on whether there
// is anything in store. A warehouse full of something nobody wants is a glut and its
// price should fall; an empty shelf for something nobody wants is simply uninteresting.
// Treating both as "satisfied" left the price of every material pinned at its opening
// value forever, which is the same as having no market at all.
func (s *State) Coverage(r Resource) float64 {
	d := float64(s.demand[r])
	if d < 1e-6 {
		if s.StockOf(r) > 0 {
			return TargetCoverage[r] * 10 // unwanted and piling up
		}
		// No stock and no recorded draws reads as neutral. For food this is a real
		// distortion — an empty market records zero demand precisely because there is
		// nothing to draw, so famine freezes the price — but the "honest" fix (coverage
		// zero, price to the ceiling) was measured and it made things worse: the spiking
		// price dragged every price-indexed quantity up with it, parents' provisioning
		// reserves included, so larders went unstocked in exactly the famine. Two
		// individually-principled changes compounding into harm (docs/method.md, note 12).
		// The distortion stays until the things that hang off the price are untangled.
		return TargetCoverage[r]
	}
	return float64(s.StockOf(r)) / d
}

// adjustPrices moves every price toward whatever would clear the market.
//
// This is the mechanism PolicyWeight was standing in for. A commodity in short supply
// gets dearer, which makes its producers richer, which lets them pay more, which pulls
// labour toward them through the wage term of the job utility function (§3.5). Production
// rises and the price falls back. Nothing decides that farms matter more than mines when
// people are hungry; the price says so.
//
// The version in the previous milestone did the opposite and collapsed the economy. It
// let a rising food price raise a *mandatory* wage floor, so scarcity bankrupted employers
// and forced layoffs, which cut production, which deepened the scarcity. The signal has to
// reach producers as revenue before it reaches them as costs.
func (s *State) adjustPrices() {
	for r := Resource(0); r < NumResources; r++ {
		// Roll today's consumption into the running estimate.
		s.demand[r] = s.demand[r]*(1-DemandSmoothing) + s.consumed[r]*DemandSmoothing
		s.consumed[r] = 0

		target := TargetCoverage[r]
		if target <= 0 {
			continue
		}
		ratio := s.Coverage(r) / target

		// Below target the price rises; above it, falls. Capped so no single day moves
		// a price far.
		step := PriceElasticity * (1 - ratio)
		if step > PriceMaxStep {
			step = PriceMaxStep
		}
		if step < -PriceMaxStep {
			step = -PriceMaxStep
		}
		s.Prices[r] *= float32(1 + step)

		lo := s.basePrices[r] * PriceFloor
		hi := s.basePrices[r] * PriceCeiling
		// Food may not be priced beyond what work can pay for.
		//
		// A market where nobody can afford the only thing they must buy is not a market
		// under strain, it is a dead one: purchases stop, so sellers earn nothing, so
		// wages fall, so it becomes less affordable still. That spiral killed a third of
		// every founding village. Real sellers cut prices to clear stock rather than watch
		// their customers starve, and this is the crude version of that.
		if r == Food {
			// Never below the floor. The cap tracks prevailing wages, so when wages
			// collapse it collapses with them — and a price of nothing means the seller
			// earns nothing, cannot restock, and the market never recovers. A cap meant
			// to stop food becoming unaffordable must not be able to make it worthless.
			if cap := s.affordableFoodPrice(); cap > lo && cap < hi {
				hi = cap
			}
		}
		if s.Prices[r] < lo {
			s.Prices[r] = lo
		}
		if s.Prices[r] > hi {
			s.Prices[r] = hi
		}
	}
}

// FoodShareOfIncome is the most of a day's earnings that a day's food may cost. Above
// this, a working household cannot feed itself, let alone a child.
const FoodShareOfIncome = 0.6

// affordableFoodPrice is the most a meal may cost given what people are actually paid.
//
// Measured from the wages being offered rather than from any fixed figure, so it moves
// with the economy: a richer village can bear dearer food, and a poor one cannot.
func (s *State) affordableFoodPrice() float32 {
	var total float32
	var n int
	for i := range s.Structs {
		st := &s.Structs[i]
		if st.Alive && st.Filled > 0 && st.Wage > 0 {
			total += st.Wage * float32(st.Filled)
			n += st.Filled
		}
	}
	if n == 0 {
		return 0 // nobody is employed; no wage to reason from
	}
	dailyEarnings := total / float32(n) * WorkTicksPerDay
	return dailyEarnings * FoodShareOfIncome / MealsPerDay
}

// revenuePerWorker is what a structure earned per worked tick, per member of staff, with
// a draw on reserves so that a funded but idle employer can still make an offer.
//
// Per head, and deliberately not per unit of effective labour. Skill-indexed pay — the
// wage as a rate per efficiency unit, each worker paid wage × their own efficiency — was
// implemented and measured, and it cost R0 0.621 -> 0.498 with a legible mechanism:
// veterans gained, novices lost, and the novices are the fifteen-to-twenty-year-olds
// saving to marry (marriage fell 80% to 67%). Seniority pay taxes exactly the
// family-founding age, which a subsistence-margin village cannot afford. A sound idea
// for a village with surplus; revisit after replacement is achieved.
func (s *State) revenuePerWorker(st *Structure) float32 {
	staff := float32(maxInt(st.Filled, 1))
	budget := st.revenue
	if st.Gold > 0 {
		budget += st.Gold * ReserveDrawRate
	}
	return budget / (staff * WorkTicksPerDay)
}

// setWages lets each structure offer what its trade can actually bear (§4.3).
//
// Wages are derived, not decreed. An employer selling a commodity that has become dear
// can offer more and will draw people in; one whose goods nobody wants offers less and
// loses them. That is the whole allocation mechanism, and it only works if nothing else
// pins wages to a common value.
//
// Note what is deliberately absent: no minimum, and no forced redundancies. A wage too low
// to live on simply fails to attract anybody, which empties the payroll gradually through
// the job market rather than all at once through a rule.
func (s *State) setWages() {
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || Defs[st.Type].Jobs == 0 {
			continue
		}
		// A building site has a fund, not a trade.
		//
		// Deriving its wage from revenue gave it almost nothing to offer — 0.135 gold a day
		// against a granary's 0.346 and a subsistence cost of 0.467 — because a site earns
		// nothing by definition and revenuePerWorker was reading its dwindling float. So no
		// one ever left a job for it, and eleven sites stood with materials delivered and
		// no crew for ten years while the houses their owners had paid for were never
		// built. It pays what it was funded to pay.
		if st.Type == BuildSite {
			st.Wage = s.SubsistenceWage() * BuildWagePremium
			st.revenue = 0
			continue
		}
		target := s.revenuePerWorker(st)
		st.Wage += (target - st.Wage) * WageAdjustRate
		if st.Wage < 0 {
			st.Wage = 0
		}
		if st.Wage > MaxWage {
			st.Wage = MaxWage
		}
		st.revenue = 0
	}
}
