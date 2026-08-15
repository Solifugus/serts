package sim

import (
	"github.com/solifugus/serts/internal/torus"
	"github.com/solifugus/serts/internal/worldgen"
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
	// MaxFreshDist is how far fresh water may be, in cells.
	MaxFreshDist int16
	// Upkeep is gold per tick to maintain the building (§4.2).
	Upkeep float32
}

// Defs holds the configuration of every structure type, indexed by StructType.
var Defs = [NumStructTypes]StructDef{
	// Homes have no upkeep cost yet: they earn nothing, so charging them simply drives
	// their balance negative forever. Household maintenance becomes real when residents
	// can pay for it.
	Home:    {Name: "home", Jobs: 0, Capacity: 6, Upkeep: 0},
	Farm:    {Name: "farm", Jobs: 6, Wage: 0.014, NeedsFreshWater: true, MaxFreshDist: 6, Upkeep: 0.0010},
	Granary: {Name: "granary", Jobs: 3, Wage: 0.012, Upkeep: 0.0008},
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
const FarmYieldPerWorker = 0.009

// FarmStorage is how much harvested food a farm will hold before it stops working the
// land. Grain with nowhere to go is not harvested, so a farm that cannot sell idles
// rather than accumulating an infinite pile.
const FarmStorage = 300

// Wage is a faction-wide policy value this milestone, and the treasury pays it.
//
// A more elaborate economy was tried first and abandoned, which is worth recording
// because the failure was instructive. Giving every building its own balance sheet — the
// granary buying grain from farms, each funding wages from its own reserves, prices and
// headcount discovered from revenue — produced three separate absorbing states in
// succession: a village with full barns and no money, a village with money and empty
// barns, and a village with both and nobody employed. Each was individually fixable and
// the next one appeared behind it.
//
// The apparatus was buying nothing. §4.3's competing employers only mean something when
// there is more than one bidder for a worker, and this milestone has one faction. So the
// faction pays wages from its treasury, food revenue returns to it, and the balance is
// arithmetic that can be checked by hand: total wages paid must equal total spent on
// food, or the treasury drifts. Firm-level economics belongs with rival factions.
const (
	// BaseWage is set so that the working population's earnings cover the whole
	// population's eating — including the children, elders, and unemployed who do not
	// earn. Set merely to cover a worker's own meals, it leaves nothing for dependants,
	// and every household with a child in it slowly goes broke.
	BaseWage = 0.0068
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
const GranaryCapacity = 400

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
	WageAdjustRate = 0.04
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
	return true
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
	})
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
			// Yield is local soil times the global fertility governor (§2.4), times the
			// accumulated skill of whoever actually turned up to work.
			soil := float64(s.World.Soil[s.T.Index(st.Cell)])
			st.Food += float32(float64(st.produce) * soil * fertility)
			if st.Food > FarmStorage {
				st.Food = FarmStorage
			}
			st.produce = 0
		}
	}
	s.deliverToGranaries()
}

// deliverToGranaries moves harvested food to where people buy it.
//
// Haulage is abstracted this milestone: food moves without anyone carrying it. Carrying
// is work, and work is employment, so the spine (§1) says this placeholder must
// eventually be filled by people with jobs rather than by a transfer.
//
// No money changes hands, because both ends belong to the same faction. Buying and
// selling between a faction's own buildings was tried and abandoned — see the note on
// the economy in adjustHeadcount's absence below.
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
		if !st.Alive || st.Type != Farm || st.Food <= 0 {
			continue
		}
		best, bestD := granaries[0], s.T.Dist2(st.Pos, s.Structs[granaries[0]].Pos)
		for _, g := range granaries[1:] {
			if d := s.T.Dist2(st.Pos, s.Structs[g].Pos); d < bestD {
				best, bestD = g, d
			}
		}
		gr := &s.Structs[best]
		want := st.Food
		if room := GranaryCapacity - gr.Food; want > room {
			want = room
		}
		if want <= 0 {
			continue // stores full; the harvest waits at the farm
		}
		gr.Food += want
		st.Food -= want
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
			f += s.Structs[i].Food
		}
	}
	return f
}

// WorkTicksPerDay is how many ticks of the day are actually worked.
const WorkTicksPerDay = (WorkEndHour - WorkStartHour) * TicksPerHour

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
