package sim

import (
	"math"

	"github.com/solifugus/serts/internal/torus"
)

// Founding new businesses.
//
// The village's economy was founding-sized forever: three farms and a handful of trades,
// fixed on day one, however many people were later born. That ceiling did nothing while
// child mortality kept the population below it. The moment child survival improved —
// households splitting before crowding kills, gardens multiplying — the ceiling became the
// killer: a third more adults reached working age against the same fifty-odd posts and the
// same fields, and the whole survival gain converted into adult starvation. Malthus, in
// the specific form of an economy that cannot scale.
//
// So the wealthy found businesses. This was deliberately deferred until now — founding
// more employers into a village with two and a half posts per adult would have spread thin
// labour thinner — and the trigger is the one set down then: labour surplus. Nobody sinks
// capital into a trade that cannot hire.
//
// What to found comes from prices, which is the same signal that steers everything else
// here (§4.3): a good priced far above its base is a good the village is short of, and
// the trade that produces it is the trade worth entering. Capital chasing scarcity is the
// mechanism by which the economy grows to fit its population — and it is where the
// owners' accumulated gold finally goes back to work, which the profit draw has needed
// since it was added.

const (
	// FoundingSurplus is how many more adults than posts there must be before anyone
	// risks founding. Counted structurally — posts, not vacancies — so seasonal winter
	// unemployment does not read as a labour surplus.
	FoundingSurplus = 4

	// ScarcitySignal is how far above its base a price must sit to mark a trade worth
	// entering. Below this, existing producers can plainly cover demand.
	ScarcitySignal = 1.4

	// FounderReserve is what a founder keeps back for their own household. Nobody sinks
	// their last coin into a venture; the same principle as LarderReserve, at the scale
	// of capital.
	FounderReserve = LarderReserve * 4
)

// foundBusinesses lets the richest villager open a new trade when labour is spare and a
// price says which one. Once a day, one at a time: a village does not raise two ventures
// in an afternoon.
func (s *State) foundBusinesses() {
	// A labour surplus, or nothing. Founding into a tight labour market spreads thin
	// labour thinner and starves the trades that already exist.
	adults, posts := 0, 0
	for i := range s.Chars {
		if s.Chars[i].Alive && s.Chars[i].Stage() != Child {
			adults++
		}
	}
	for i := range s.Structs {
		st := &s.Structs[i]
		if st.Alive && st.Type != BuildSite && st.Jobs > 0 {
			posts += st.Jobs
		}
	}
	if adults-posts < FoundingSurplus {
		return
	}

	// The scarcest good names the trade. Food is judged by the granary as well as the
	// price, because the affordability cap holds food prices down in exactly the famines
	// where a new farm matters most.
	candidates := []struct {
		r Resource
		t StructType
	}{
		{Food, Farm}, {Wood, LumberCamp}, {Stone, Quarry}, {Iron, Mine}, {Tools, Workshop},
	}
	bestType, bestRatio := NumStructTypes, float32(ScarcitySignal)
	for _, c := range candidates {
		if s.basePrices[c.r] <= 0 {
			continue
		}
		ratio := s.Prices[c.r] / s.basePrices[c.r]
		if c.r == Food && s.FoodDays() < 60 {
			ratio = 2 * ScarcitySignal // short granaries outrank every other signal
		}
		if ratio > bestRatio {
			bestType, bestRatio = c.t, ratio
		}
	}
	if bestType == NumStructTypes {
		return
	}

	// The founder: whoever can best afford it. Ties break toward the lower ID so the
	// choice never depends on iteration order (§9.2).
	cost := s.materialCost(bestType) + s.labourCost(bestType)
	founder, founderGold := NoChar, cost+FounderReserve
	for i := range s.Chars {
		c := &s.Chars[i]
		if c.Alive && c.Stage() != Child && c.Gold > founderGold {
			founder, founderGold = CharID(i), c.Gold
		}
	}
	if founder == NoChar {
		return
	}

	site, ok := s.businessSite(bestType)
	if !ok {
		return
	}

	sid := s.Build(bestType, site)
	s.Chars[founder].Gold -= cost
	s.Structs[sid].Gold += cost
	// The founder owns what they paid for, from the first day of the works. completeBuild
	// leaves Owner untouched, so ownership survives into the finished business.
	s.Structs[sid].Owner = founder
	s.BusinessesFounded++
}

// businessSite finds ground for a new venture of the given type.
func (s *State) businessSite(t StructType) (torus.Cell, bool) {
	centre := s.villageCentre()
	switch t {
	case LumberCamp, Quarry, Mine:
		return s.bestResourceSite(centre, t)
	}
	// Trades that live among people: spiral outward from the village looking for legal
	// ground, insisting on decent soil for a farm.
	for r := 3.0; r <= 40; r += 1.2 {
		steps := maxInt(10, int(r*5))
		for k := 0; k < steps; k++ {
			ang := 2 * math.Pi * float64(k) / float64(steps)
			c := s.T.WrapCell(torus.Cell{
				X: centre.X + int(math.Round(r*math.Cos(ang))),
				Y: centre.Y + int(math.Round(r*math.Sin(ang))),
			})
			if !CanPlace(s.World, t, c) || s.Occupied(c) {
				continue
			}
			if t == Farm && s.World.Soil[s.T.Index(c)] < 0.40 {
				continue
			}
			return c, true
		}
	}
	return torus.Cell{}, false
}

// villageCentre is where the village is, for siting purposes: its granary, or failing
// that its first home.
func (s *State) villageCentre() torus.Cell {
	for i := range s.Structs {
		if s.Structs[i].Alive && s.Structs[i].Type == Granary {
			return s.Structs[i].Cell
		}
	}
	for i := range s.Structs {
		if s.Structs[i].Alive && s.Structs[i].Type == Home {
			return s.Structs[i].Cell
		}
	}
	return torus.Cell{}
}
