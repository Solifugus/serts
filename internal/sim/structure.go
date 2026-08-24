package sim

import (
	"github.com/solifugus/serts/internal/torus"
	"github.com/solifugus/serts/internal/worldgen"
	"math"
)

// StructType enumerates the buildings this milestone implements.
//
// The full set in §5 is larger; these three are the minimum that closes a food and wage
// loop, which is what "can a village feed itself" requires. Skill is stored per type, so
// adding types later costs an array slot and nothing else.
type StructType uint8

const (
	Home StructType = iota
	Farm
	Granary
	LumberCamp
	Quarry
	Mine
	Storehouse
	Workshop
	Store
	DiningHall
	Fishery
	TownHall
	BuildSite
	NumStructTypes
)

func (t StructType) String() string {
	switch t {
	case Home:
		return "home"
	case Farm:
		return "farm"
	case Granary:
		return "granary"
	case LumberCamp:
		return "lumber camp"
	case Quarry:
		return "quarry"
	case Mine:
		return "mine"
	case Storehouse:
		return "storehouse"
	case Workshop:
		return "workshop"
	case Store:
		return "store"
	case DiningHall:
		return "dining hall"
	case Fishery:
		return "fishery"
	case TownHall:
		return "town hall"
	case BuildSite:
		return "building site"
	}
	return "?"
}

// StructDef is the fixed configuration of a structure type.
type StructDef struct {
	Name string
	// Jobs is how many people the structure employs. Homes employ nobody: they are
	// where people live, not work.
	Jobs int
	// Capacity is how many residents a home takes.
	Capacity int
	// Wage is gold per worker per tick.
	Wage float32
	// NeedsFreshWater constrains placement: farms must be able to irrigate (§2.8).
	NeedsFreshWater bool
	// NeedsWater constrains placement to the water's edge: a fishery must reach it.
	NeedsWater bool
	// MaxFreshDist is how far fresh water may be, in cells.
	MaxFreshDist int16
	// Upkeep is gold per tick to maintain the building (§4.2).
	Upkeep float32
	// BuildCost is the materials needed to put the structure up (§5).
	BuildCost Stock
	// BuildDays is roughly how long a full crew takes to finish it.
	BuildDays float64
}

// Defs holds the configuration of every structure type, indexed by StructType.
var Defs = [NumStructTypes]StructDef{
	// Homes have no upkeep cost yet: they earn nothing, so charging them simply drives
	// their balance negative forever. Household maintenance becomes real when residents
	// can pay for it.
	Home: {Name: "home", Jobs: 0, Capacity: 6, Upkeep: 0,
		BuildCost: Stock{Wood: 14, Stone: 6}, BuildDays: 6},
	Farm: {Name: "farm", Jobs: 6, NeedsFreshWater: true, MaxFreshDist: 6, Upkeep: 0.48 / TicksPerDay,
		BuildCost: Stock{Wood: 10}, BuildDays: 5},
	Granary: {Name: "granary", Jobs: 3, Upkeep: 0.384 / TicksPerDay,
		BuildCost: Stock{Wood: 20, Stone: 14}, BuildDays: 9},
	LumberCamp: {Name: "lumber camp", Jobs: 2, Upkeep: 0.36 / TicksPerDay,
		BuildCost: Stock{Wood: 8}, BuildDays: 4},
	Quarry: {Name: "quarry", Jobs: 5, Upkeep: 0.36 / TicksPerDay,
		BuildCost: Stock{Wood: 12}, BuildDays: 5},
	Mine: {Name: "mine", Jobs: 6, Upkeep: 0.48 / TicksPerDay,
		BuildCost: Stock{Wood: 22, Stone: 8}, BuildDays: 8},
	Storehouse: {Name: "storehouse", Jobs: 3, Upkeep: 0.36 / TicksPerDay,
		BuildCost: Stock{Wood: 18, Stone: 10}, BuildDays: 7},
	Workshop: {Name: "workshop", Jobs: 5, Upkeep: 0.48 / TicksPerDay,
		BuildCost: Stock{Wood: 20, Stone: 12}, BuildDays: 8},
	Store: {Name: "store", Jobs: 3, Upkeep: 0.36 / TicksPerDay,
		BuildCost: Stock{Wood: 16, Stone: 8}, BuildDays: 6},
	// A forward kitchen, put where the work is (§5). The design offers a mobile kitchen
	// for armies, which move; a mine sits in the same hills for decades and wants a
	// permanent one. It is what lets a settlement work ground beyond walking distance of
	// its granary — otherwise the only way to stop people starving at remote sites is to
	// forbid them the job, which caps how far a village can ever reach.
	DiningHall: {Name: "dining hall", Jobs: 2, Upkeep: 0.30 / TicksPerDay,
		BuildCost: Stock{Wood: 12, Stone: 4}, BuildDays: 4},
	// A fishery: food from water instead of soil, and — the point — food in WINTER.
	//
	// Every famine this project measured struck in the pre-harvest gap, because farming
	// is seasonal (sow day 60, harvest day 300) and a village eats every day. A fishery
	// produces year round, so a settlement beside water has food exactly when a farming
	// settlement has none. That is comparative advantage in time rather than in kind,
	// and it is the honest reason grain-for-fish is worth trading in a model where both
	// are Food: the timing differs even though the good does not.
	//
	// It also opens the map. colonySite demands decent soil, so lake shores and coasts
	// with poor ground are currently unsettleable; a valley that can fish does not need
	// to farm.
	Fishery: {Name: "fishery", Jobs: 3, NeedsWater: true, Upkeep: 0.36 / TicksPerDay,
		BuildCost: Stock{Wood: 14}, BuildDays: 5},
	// The seat of the faction (§8.1a): two clerks, a treasury, and the place policy
	// lives. Its purse is its own Gold, exactly as every business holds its own — the
	// civic difference is where the money comes from (levy and escheat) and where it
	// goes (relief, and later public works and pensions).
	TownHall: {Name: "town hall", Jobs: 2, Upkeep: 0.30 / TicksPerDay,
		BuildCost: Stock{Wood: 24, Stone: 16}, BuildDays: 10},
	// A building site is not built; it is what building looks like from outside.
	//
	// Two hands, not six. At six a village of thirty-seven adults had thirty-six
	// construction posts standing open, all paying above subsistence; everybody left the
	// fields for them, nothing was sown or harvested, and the population fell to seven
	// inside three years. A cottage does not want a crew of six.
	BuildSite: {Name: "building site", Jobs: 2, Upkeep: 0},
}

// FarmYieldPerWorker is food produced per worker per tick at soil 1.0 and skill 1.0.
//
// Calibrated against consumption rather than guessed, and revised downward once the
// first version showed why it matters. At 0.05 a farmhand on good river soil fed nearly
// eight people, so a village of forty needed five farmers and had thirty-five with
// nothing to do — the granary stood permanently full, the price mechanism correctly drove
// farm revenue to nothing, and the whole settlement collapsed into foraging.
//
// At this value one farmhand feeds roughly two, which is deliberately closer to
// pre-industrial reality: most of a village works the land, and the surplus that frees
// anyone for other trades is thin. It also leaves the labour market something to do.
// Expressed per worked day and divided down, so the clock and the harvest stay
// independent of one another.
// Raised from 2.16 now that there is other work to be had. That figure made one farmhand
// feed about one and a half people, which is close to historical truth but leaves a
// village of thirty with only a handful of people free for anything else. It was chosen
// when farms were the only employer and surplus labour had nowhere to go but the ditch;
// with trades to absorb them, a real agricultural surplus is what pays for industry.
// The farming year (§4.1).
//
// Crops are not produced tick by tick. Labour through the growing season goes into a
// standing crop that cannot be eaten, sold, or counted on, and the whole year's food
// arrives in one lump at harvest. Between harvests the village lives on what it stored.
//
// This is the correction to a model where an hour's work became an hour's food, which is
// not farming but foraging with extra steps. It matters beyond realism: with an instant
// harvest, a shortage looks like something a price signal can fix. It cannot. Nothing
// decided in autumn puts grain in the barn before spring, so the only defence against a
// bad year is having stored enough — which is what a granary is for.
const (
	// SowDay and HarvestDay bound the growing season, in days of the 360-day year.
	SowDay     = 60
	HarvestDay = 300
	// GrowingDays is the season's length, and so the lead time on every decision a
	// village makes about how much to plant.
	GrowingDays = HarvestDay - SowDay
)

// InGrowingSeason reports whether crops are in the ground and worth tending.
func (t Tick) InGrowingSeason() bool {
	d := t.Date().Day
	return d >= SowDay && d < HarvestDay
}

// Raised so the annual crop is unchanged now that farm work happens only during the
// growing season: two thirds of the year's days, so half as much again per day.
const FarmYieldPerWorkerDay = 5.4
const FarmYieldPerWorker = FarmYieldPerWorkerDay / WorkTicksPerDay

// FarmStorage is how much harvested food a farm will hold before it stops working the
// land. Grain with nowhere to go is not harvested, so a farm that cannot sell idles
// rather than accumulating an infinite pile.
// Must fit a whole year's crop from a full crew, because the whole year's crop arrives on
// a single day.
const FarmStorage = 9000

// Structures hold their own gold and pay wages from it (§4.3).
//
// This was tried once before without a money faucet and failed three different ways: a
// village with full barns and no money, one with money and empty barns, and one with both
// and nobody employed. The diagnosis at the time was that per-building balance sheets were
// over-engineering. That was wrong. The apparatus was fine; what was missing was any way
// for gold to *enter* the world. A conserved money supply always has absorbing states,
// because the coin ends up somewhere it cannot leave and no adjustment of prices or wages
// can move it.
//
// Panning (§4.2) is the faucet, and it is why this arrangement works now.
const (
	// BaseWage is what a structure offers per worked tick. Set so that the working
	// population's earnings cover the whole population's eating, including the children,
	// elders, and unemployed who do not earn.
	BaseWagePerDay = 1.632
	BaseWage       = BaseWagePerDay / WorkTicksPerDay

	// FarmGateShare is the fraction of the retail price a granary pays a farm for grain.
	// The remainder is the granary's margin, which funds its own payroll.
	FarmGateShare = 0.85
)

// GranaryCapacity is the stock a granary aims to hold — a buffer against famine (§5),
// sized at roughly ten days of consumption for a founding village.
//
// The size matters far more than it looks, and it is the parameter this economy proved
// most sensitive to. Too large and the buffer takes weeks to fill, during which everyone
// is employed; then it sits full, drives the farm-gate price to nothing, triggers mass
// layoffs, and takes weeks more to drain — a slow, violent oscillation that settles in
// the collapsed state. A buffer small enough to be refilled constantly keeps the price
// signal fast, because it is the refilling that pays everybody.
// Sized so the granary can physically hold the coverage the market asks for. At 400 it
// could not: about nine days for thirty people against a twenty-five day target, so
// coverage was permanently short, the price pinned itself at its ceiling forever, and a
// meal cost more than a day's wage. People starved standing inside a full granary with
// money in their hands.
//
// A store capped below the target coverage does not merely hold less food; it tells the
// price mechanism the village is in permanent famine, and the mechanism believes it.
const GranaryCapacity = 3000

// PriceRampFraction is the top slice of granary capacity over which the farm-gate price
// falls away. Below it the granary pays full price; at capacity it pays nothing.
const PriceRampFraction = 0.25

// Wage adjustment (§4.3). Structures set wages from their own reserves: one that cannot
// fill its positions raises its offer, and one whose gold is draining cuts back.
//
// This is deliberately emergent rather than a tuned constant. Hand-balancing wages
// against food prices is fragile — the first version of this economy drained its
// treasury and starved a village sitting on 57,000 units of food — whereas a structure
// that simply cannot pay stops hiring, which is both self-correcting and exactly what
// §3.5 says a failing employer looks like.
const (
	// Wages must react fast. The loop from price to wage to labour to production already
	// carries weeks of lag, and a slow wage response adds weeks more — by which time a
	// shortage the price signalled has become a famine. Raising this from 0.04 to 0.35
	// nearly doubled the population the village could hold.
	WageAdjustRate = 0.35
	MinWage        = 0.0005
	MaxWage        = 0.20

	PriceAdjustRate = 0.05
	MinFoodPrice    = 0.25
	MaxFoodPrice    = 4.0

	// ReserveDrawRate is the share of its reserves a structure will put toward wages in
	// a day, on top of what it earned. It is what lets a new farm hire before its first
	// harvest, and an established one keep its crew through a bad season.
	ReserveDrawRate = 0.02
)

// Danger is the yearly chance a full-time worker at a trade is killed outright, with
// roughly ten times as many injuries short of that.
//
// It is a property of the work, which is the point: a mine is not a farm. Beyond realism
// it does something useful to the labour market — dangerous work has to pay more to
// attract anyone, and that falls out of the job utility function rather than being
// scripted. Economists call it a compensating differential; here it emerges.
var Danger = [NumStructTypes]float64{
	Home:       0,
	Farm:       0.004, // scythes, hooves, and falls from carts
	Granary:    0.002,
	LumberCamp: 0.020, // felling is the most dangerous work there is
	Quarry:     0.016,
	Mine:       0.024, // and underground the most dangerous of all
	Storehouse: 0.002,
	Workshop:   0.008,
	Store:      0.001,
	DiningHall: 0.001,
	BuildSite:  0.014, // scaffolds and lifted stone
}

// InjuriesPerDeath is how many injuries accompany each fatality. Most accidents maim
// rather than kill, and a maimed worker is a worker who cannot earn for a while.
const InjuriesPerDeath = 10

// Upgrades (§5).
//
// Any structure may be improved, and improvement is never capped — only ever dearer. The
// benefit of each level is constant while its price roughly doubles, so upgrading stops
// being worth it long before it becomes impossible. That is a better limit than a ceiling:
// nothing is forbidden, it simply ceases to pay.
//
// It also gives wealth somewhere to go. Characters had only food and tools to spend on and
// accumulated hundreds of gold, which withdraws money from circulation as effectively as
// destroying it. A household that can always put its savings into a better house is a
// household whose savings keep moving.
const (
	// UpgradeCostFactor is how much dearer each level is than the last.
	UpgradeCostFactor = 2.0
	// HomeCapacityPerLevel is how many more people a home holds for each improvement.
	HomeCapacityPerLevel = 3
	// UpgradeDays is how long the work takes, whatever the level.
	UpgradeDays = 12
)

// Capacity is how many residents a structure holds at its current level.
func (st *Structure) Capacity() int {
	base := Defs[st.Type].Capacity
	if st.Type != Home {
		return base
	}
	return base + st.Level*HomeCapacityPerLevel
}

// UpgradeCost is the materials needed to take a structure to its next level, doubling with
// each one and never capped.
func UpgradeCost(t StructType, level int) Stock {
	var out Stock
	scale := float32(math.Pow(UpgradeCostFactor, float64(level+1)))
	for r := Resource(0); r < NumResources; r++ {
		out[r] = Defs[t].BuildCost[r] * scale
	}
	return out
}

// CanPlace reports whether a structure type may be built on a cell, and why not.
func CanPlace(w *worldgen.World, t StructType, c torus.Cell) bool {
	i := w.T.Index(c)
	if !w.Walkable(i) {
		return false
	}
	if w.Water[i] == worldgen.River {
		return false // do not build in the watercourse itself
	}
	d := Defs[t]
	if d.NeedsFreshWater && w.FreshDist[i] > d.MaxFreshDist {
		return false
	}
	if d.NeedsWater && waterWithin(w, c, FisheryReach) <= 0 {
		return false
	}
	return true
}

// FisheryReach is how far a fishery works from its landing, in cells.
const FisheryReach = 4

// waterWithin counts water cells within reach — the fishing ground. Ocean, lake and
// river all fish; the sea is richest.
func waterWithin(w *worldgen.World, c torus.Cell, reach int) float64 {
	var total float64
	for dy := -reach; dy <= reach; dy++ {
		for dx := -reach; dx <= reach; dx++ {
			if dx*dx+dy*dy > reach*reach {
				continue
			}
			i := w.T.Index(w.T.WrapCell(torus.Cell{X: c.X + dx, Y: c.Y + dy}))
			switch w.Water[i] {
			case worldgen.Ocean:
				total += 1.0
			case worldgen.Lake:
				total += 0.8
			case worldgen.River:
				total += 0.5
			}
		}
	}
	return total
}

// addStructure places a structure and returns its ID.
func (s *State) addStructure(t StructType, c torus.Cell) StructID {
	d := Defs[t]
	s.Structs = append(s.Structs, Structure{
		Type:      t,
		Cell:      c,
		Pos:       s.T.Center(c),
		Wage:      d.Wage,
		Jobs:      d.Jobs,
		Condition: 100,
		Alive:     true,
		workCell:  -1,
		Owner:     NoChar,
	})
	s.typeCache = nil // the index is stale, and the slice may have moved
	return StructID(len(s.Structs) - 1)
}

// stepStructures runs production and delivery for one tick.
func (s *State) stepStructures() {
	fertility := s.Fertility()

	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive {
			continue
		}
		if st.Type == Farm && st.produce > 0 {
			// Labour goes into the ground, not into the barn.
			// Yield is local soil times the global fertility governor (§2.4), times the
			// accumulated skill of whoever actually turned up to work.
			soil := float64(s.World.Soil[s.T.Index(st.Cell)])
			st.Growing += float32(float64(st.produce) * soil * fertility)
			st.produce = 0
		}
	}
	// Harvest, once a year, on the day the season ends.
	if s.Tick%TicksPerYear == HarvestDay*TicksPerDay {
		s.harvest()
	}

	s.deliverToGranaries()
	s.supplyDiningHalls()
	s.tradeMaterials()

	// The daily review: prices find their level, wages follow revenue (§4.3). Once a day
	// rather than every tick — the outcome is the same and the cost is 2,400 times lower.
	if s.Tick%TicksPerDay == 0 {
		s.adjustPrices()
		s.setWages()
	}
}

// DiningHallStock is how many meals a forward kitchen keeps on hand.
//
// Sized to the crew it feeds — six workers eating about nine meals a day, so a few days'
// buffer. The first attempt gave each hall two hundred and twenty against a village
// granary holding four hundred, so two kitchens tried to swallow the entire food supply
// and the village starved to a single survivor. A forward store is a few days of meals,
// not a second granary.
const DiningHallStock = 40

// GranaryReserveShare is the fraction of its stock a granary keeps for the village before
// supplying outlying kitchens. The people at home have first call on it.
const GranaryReserveShare = 0.25

// supplyDiningHalls moves food from the granaries out to the kitchens at the work sites.
//
// A dining hall is a buyer, not a gift: it pays wholesale and recovers the margin selling
// meals, exactly as any other link in the chain. A hall that cannot afford stock goes
// empty and the people beside it go hungry — a supply line failing visibly rather than
// silently.
func (s *State) supplyDiningHalls() {
	for i := range s.Structs {
		hall := &s.Structs[i]
		if !hall.Alive || hall.Type != DiningHall || hall.Stock[Food] >= DiningHallStock {
			continue
		}
		src := s.nearestWith(hall.Pos, Granary, Food)
		if src == NoStruct {
			continue
		}
		// Only the surplus above the village's own reserve goes out to the work sites.
		spare := s.Structs[src].Stock[Food] - GranaryCapacity*GranaryReserveShare
		if spare <= 0 {
			continue
		}
		want := DiningHallStock - hall.Stock[Food]
		if want > spare {
			want = spare
		}
		before := s.Structs[src].Stock[Food]
		s.transact(src, StructID(i), Food, want, s.FoodPriceAt(hall.Pos)*WholesaleShare)
		// Grain sold on to a hall is still the farmer's until it is paid for.
		s.settleSale(src, Food, before-s.Structs[src].Stock[Food])
	}
}

// harvest brings in the year's crop.
//
// Everything the workers put into the ground since sowing becomes food at once. It is the
// most consequential event in the village's year: what comes in now must last until this
// time next year, and no amount of labour, money or good intention between now and then
// will add to it.
func (s *State) harvest() {
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Type != Farm || st.Growing <= 0 {
			continue
		}
		got := st.Growing
		if room := FarmStorage - st.Stock[Food]; got > room {
			got = room
		}
		st.Stock[Food] += got
		s.Harvested += got
		s.Led.FoodFarmed += got
		st.Growing = 0
	}
	s.Harvests++
}

// deliverToGranaries moves harvested food to where people buy it, and pays for it.
//
// Haulage is abstracted this milestone: food moves without anyone carrying it. Carrying
// is work, and work is employment, so the spine (§1) says this placeholder must
// eventually be filled by people with jobs rather than by a transfer.
//
// The payment is not abstracted. A granary buys grain with its own coin, which is how the
// money people spend on bread reaches the people who grew it.
func (s *State) deliverToGranaries() {
	var granaries []StructID
	for i := range s.Structs {
		if s.Structs[i].Alive && s.Structs[i].Type == Granary {
			granaries = append(granaries, StructID(i))
		}
	}
	if len(granaries) == 0 {
		return
	}
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || (st.Type != Farm && st.Type != Fishery) || st.Stock[Food] <= 0 {
			continue
		}
		best, bestD := granaries[0], s.T.Dist2(st.Pos, s.Structs[granaries[0]].Pos)
		for _, g := range granaries[1:] {
			if d := s.T.Dist2(st.Pos, s.Structs[g].Pos); d < bestD {
				best, bestD = g, d
			}
		}
		gr := &s.Structs[best]
		want := st.Stock[Food]
		if room := GranaryCapacity - gr.Stock[Food]; want > room {
			want = room
		}
		if want <= 0 {
			continue // stores full; the harvest waits in the barn
		}
		// Taken on consignment, not bought. The granary used to pay for grain out of its
		// own coin, which meant it needed capital to hold stock at all — and a granary
		// whose gold reached zero could not buy grain, could not sell what it did not
		// have, and so could never earn again. Zero stock and zero gold was a state with
		// no way out, and the village starved beside barns holding twenty-two thousand
		// units of food.
		//
		// The farmer keeps title and is paid as the grain sells (see settleSale). A
		// granary with an empty till can still fill its shelves, so the deadlock has
		// nowhere to form.
		s.consign(StructID(i), best, Food, want, s.FoodPriceAt(st.Pos)*FarmGateShare)
	}
}

// Fertility is the global carrying-capacity multiplier (§2.4).
//
// The load governor that drives it from host CPU is not wired up yet; this returns the
// neutral value so that everything downstream is already written against the hook.
func (s *State) Fertility() float64 { return 1.0 }

// TotalFood sums food held across all structures.
func (s *State) TotalFood() float32 {
	var f float32
	for i := range s.Structs {
		if s.Structs[i].Alive {
			f += s.Structs[i].Stock[Food]
		}
	}
	return f
}

// WorkTicksPerDay is how many ticks of the day are actually worked, and WorkTicksPerYear
// the same across a year. Yearly rates must be divided by the latter; dividing an annual
// hazard by the daily figure made workplace accidents three hundred and sixty times too
// lethal and emptied the village inside five years.
const (
	WorkTicksPerDay  = (WorkEndHour - WorkStartHour) * TicksPerHour
	WorkTicksPerYear = WorkTicksPerDay * DaysPerYear
)

// SubsistenceWage is what a day's work must pay to cover a day's food at current prices.
//
// It moves with the food price, which is the point: when bread is dear, work that was
// worth taking last month is not worth taking now, and people go looking for something
// better or fall back on the diggings.
func (s *State) SubsistenceWage() float32 {
	return s.Prices[Food] * MealsPerDay / WorkTicksPerDay
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
