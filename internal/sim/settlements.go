package sim

import (
	"fmt"
	"strings"

	"github.com/solifugus/serts/internal/torus"
)

// Per-settlement reporting.
//
// Every instrument built during the demographic campaign reports the world: population,
// food, the ledger, the vital statistics. That was right while the world was one village.
// With daughter settlements (§2.7a) it is actively misleading — a world holding 138
// people might be two healthy villages or one thriving and one dying, and the aggregate
// cannot tell them apart. This is the same fault as every measurement error recorded in
// docs/method.md: the aggregate hides the individual, one level up.
//
// A settlement is defined by its town hall, since every village founds one and levy and
// relief already partition by nearest hall. People and structures belong to the hall
// nearest them.

// Settlement is one village's state, extracted for reporting.
type Settlement struct {
	Hall     StructID
	Pop      int
	Adults   int
	Children int
	Homes    int
	Farms    int
	// MarketFood is granary, hall and farm stock reachable from here: what the people
	// of this settlement can actually buy.
	MarketFood float32
	Larders    float32
	Treasury   float32
	PeopleGold float32
	// FarmPosts and FarmFilled measure the constraint that governed the whole campaign.
	FarmPosts, FarmFilled int
	// FoodDays is this settlement's own months of eating, not the world's.
	FoodDays float64
}

// Settlements groups the world by town hall, in hall-ID order for determinism (§9.2).
func (s *State) Settlements() []Settlement {
	var halls []StructID
	for i := range s.Structs {
		if s.Structs[i].Alive && s.Structs[i].Type == TownHall {
			halls = append(halls, StructID(i))
		}
	}
	if len(halls) == 0 {
		return nil
	}
	out := make([]Settlement, len(halls))
	for n, hid := range halls {
		out[n] = Settlement{Hall: hid, Treasury: s.Structs[hid].Gold}
	}

	// Wrapped distance, not raw: on a torus a settlement near the seam is close to
	// things whose plain coordinates are a world away, and plain subtraction would
	// assign its people to a hall on the far side of the map.
	nearestHall := func(pos torus.Vec2) int {
		best, bestD := 0, -1.0
		for n, hid := range halls {
			d := s.T.Dist2(pos, s.Structs[hid].Pos)
			if bestD < 0 || d < bestD {
				best, bestD = n, d
			}
		}
		return best
	}

	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive {
			continue
		}
		n := nearestHall(st.Pos)
		switch st.Type {
		case Home:
			out[n].Homes++
			out[n].Larders += st.Stock[Food]
		case Farm:
			out[n].Farms++
			out[n].MarketFood += st.Stock[Food]
			out[n].FarmPosts += st.Jobs
			out[n].FarmFilled += st.Filled
		case Granary, DiningHall:
			out[n].MarketFood += st.Stock[Food]
		}
	}
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive {
			continue
		}
		n := nearestHall(c.Pos)
		out[n].Pop++
		out[n].PeopleGold += c.Gold
		if c.Stage() == Child {
			out[n].Children++
		} else {
			out[n].Adults++
		}
	}
	for n := range out {
		if need := float64(out[n].Pop) * MealsPerDay; need > 0 {
			out[n].FoodDays = float64(out[n].MarketFood) / need
		}
	}
	return out
}

// SettlementReport renders every settlement on one line each.
func (s *State) SettlementReport() string {
	var b strings.Builder
	for _, v := range s.Settlements() {
		staffing := 0.0
		if v.FarmPosts > 0 {
			staffing = 100 * float64(v.FarmFilled) / float64(v.FarmPosts)
		}
		fmt.Fprintf(&b, "  hall %2d at %v: pop %3d (a %3d c %3d) homes %2d farms %d (%3.0f%% staffed) | market %6.0f (%4.0f days) larders %5.0f | treasury %6.0f people %7.0f\n",
			v.Hall, s.Structs[v.Hall].Cell, v.Pop, v.Adults, v.Children, v.Homes, v.Farms,
			staffing, v.MarketFood, v.FoodDays, v.Larders, v.Treasury, v.PeopleGold)
	}
	return b.String()
}
