package main

import (
	"fmt"
	"math"

	"github.com/solifugus/serts/internal/sim"
	"github.com/solifugus/serts/internal/torus"
)

// Selection and inspection.
//
// This exists as much for debugging as for play, and the distinction turned out not to
// matter. Every real fault found in this simulation so far was invisible in the aggregate
// statistics and obvious the moment one character was examined: average hunger read
// nineteen while quarrymen starved, employment read a hundred per cent while farms stood
// empty, and one prospector held the same position to the centimetre for two thousand
// ticks while the summary reported him walking. Numbers about everybody hide what is
// happening to somebody.

// selKind distinguishes what the player has picked out.
type selKind uint8

const (
	selNone selKind = iota
	selChar
	selStruct
)

type selection struct {
	kind selKind
	char sim.CharID
	str  sim.StructID
}

// screenToWorld converts a screen position to a world position.
//
// The world wraps, so this is a genuine inverse of the draw transform followed by a wrap:
// the same world point has infinitely many screen positions, one per visible copy, and
// they must all resolve to the same place.
func (v *viewer) screenToWorld(sx, sy int) torus.Vec2 {
	sw, sh := float64(windowW), float64(windowH)
	originX := sw/2 - v.cam.X*v.zoom
	originY := sh/2 - v.cam.Y*v.zoom
	return v.world.T.Wrap(torus.Vec2{
		X: (float64(sx) - originX) / v.zoom,
		Y: (float64(sy) - originY) / v.zoom,
	})
}

// pick finds whatever the player clicked on, preferring people to buildings.
//
// People are preferred because they are smaller, harder to hit, and far more interesting
// to inspect — a building can be found again, a particular villager may be dead in a
// minute.
func (v *viewer) pick(p torus.Vec2) selection {
	t := v.world.T

	// Radius in world units, widened when zoomed out so a click is not a test of aim.
	r := math.Max(1.2, 6/v.zoom)

	best, bestD := sim.NoChar, r*r
	for i := range v.sim.Chars {
		c := &v.sim.Chars[i]
		if !c.Alive {
			continue
		}
		if d := t.Dist2(p, c.Pos); d < bestD {
			best, bestD = sim.CharID(i), d
		}
	}
	if best != sim.NoChar {
		return selection{kind: selChar, char: best}
	}

	bs, bsD := sim.NoStruct, math.Max(2.5, 8/v.zoom)
	bsD *= bsD
	for i := range v.sim.Structs {
		st := &v.sim.Structs[i]
		if !st.Alive {
			continue
		}
		if d := t.Dist2(p, st.Pos); d < bsD {
			bs, bsD = sim.StructID(i), d
		}
	}
	if bs != sim.NoStruct {
		return selection{kind: selStruct, str: bs}
	}
	return selection{}
}

// inspect describes the selection in full.
//
// Deliberately exhaustive rather than tidy. The point is to answer "why is this person
// doing that?", and the answer has repeatedly turned out to be a field nobody thought to
// print — an empty purse, a pack with no rations, a job twenty cells from the only place
// selling food.
func (v *viewer) inspect() string {
	s := v.sim
	switch v.sel.kind {
	case selChar:
		if !s.AliveChar(v.sel.char) {
			return "(they have died)"
		}
		c := &s.Chars[v.sel.char]

		job := "out of work"
		if c.Job != sim.NoStruct {
			st := &s.Structs[c.Job]
			job = fmt.Sprintf("%v, paid %.4f/tick (subsistence %.4f)",
				st.Type, st.Wage, s.SubsistenceWage())
		}
		home := "no home"
		if c.Home != sim.NoStruct {
			home = fmt.Sprintf("home holds %.0f meals", s.Structs[c.Home].Stock[sim.Food])
		}

		out := fmt.Sprintf("PERSON #%d — %s, age %.0f\n", v.sel.char, c.Stage().String(), c.Age)
		out += fmt.Sprintf("  doing:   %v\n", c.Activity)
		out += fmt.Sprintf("  hunger:  %.0f/100   health: %.0f/100\n", c.Hunger, c.Health)
		out += fmt.Sprintf("  purse:   %.1f gold   carrying %.1f meals\n", c.Gold, c.Rations)
		out += fmt.Sprintf("  work:    %s\n", job)
		out += fmt.Sprintf("  living:  %s\n", home)
		out += fmt.Sprintf("  tools:   %.0f%%\n", c.Tools*100)

		// Distance to the nearest food is the number that explains most deaths.
		if src := s.NearestFoodSource(c.Pos); src != sim.NoStruct {
			out += fmt.Sprintf("  food:    %v %.0f cells away, %.0f in stock\n",
				s.Structs[src].Type, v.world.T.Dist(c.Pos, s.Structs[src].Pos),
				s.Structs[src].Stock[sim.Food])
		} else {
			out += "  food:    nowhere is selling\n"
		}
		return out

	case selStruct:
		st := &s.Structs[v.sel.str]
		out := fmt.Sprintf("%v #%d\n", st.Type, v.sel.str)
		if st.Type == sim.BuildSite {
			out += fmt.Sprintf("  building: %v, %.0f%% done\n", st.Building, st.Progress*100)
		}
		out += fmt.Sprintf("  staff:   %d of %d   wage %.4f\n", st.Filled, st.Jobs, st.Wage)
		out += fmt.Sprintf("  purse:   %.1f gold\n", st.Gold)
		for r := sim.Resource(0); r < sim.NumResources; r++ {
			if st.Stock[r] > 0.01 {
				out += fmt.Sprintf("  %-8s %.0f\n", r.String()+":", st.Stock[r])
			}
		}
		if st.Type == sim.Home {
			out += fmt.Sprintf("  residents: %d\n", st.Residents)
		}
		return out
	}
	return ""
}

// market renders the economy at a glance: what things cost, who pays what, what is short.
func (v *viewer) market() string {
	s := v.sim
	out := "MARKET\n"
	for r := sim.Resource(0); r < sim.NumResources; r++ {
		out += fmt.Sprintf("  %-6s %6.2f gold   %7.0f held   %5.0f days\n",
			r, s.Prices[r], s.StockOf(r), s.Coverage(r))
	}
	out += "\nWAGES  (subsistence " + fmt.Sprintf("%.4f)\n", s.SubsistenceWage())
	seen := map[sim.StructType]bool{}
	for i := range s.Structs {
		st := &s.Structs[i]
		if st.Alive && !seen[st.Type] && st.Jobs > 0 {
			seen[st.Type] = true
			out += fmt.Sprintf("  %-13s %.4f\n", st.Type, st.Wage)
		}
	}
	return out
}
