package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

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

// personWord names a character in one word from their sex and age.
//
// The simulation stores sex as a bit because only reproduction reads it, and stage as an
// age bucket; on screen the two belong together. "woman child" is not something anyone
// says, and English already has the words: girl, man, old woman.
func personWord(sex uint8, stage sim.LifeStage) string {
	female := sex == 0
	switch stage {
	case sim.Child:
		if female {
			return "girl"
		}
		return "boy"
	case sim.Elder:
		if female {
			return "old woman"
		}
		return "old man"
	}
	if female {
		return "woman"
	}
	return "man"
}

// traitLine describes a personality in words rather than numbers.
//
// The traits are multipliers around one and "Rootedness 1.13" tells a reader nothing, so
// they are named. But naming every trait that deviates does not work either: founders draw
// uniformly on 0.67–1.33, so at any threshold tight enough to be meaningful almost
// everybody is flagged on three or four traits at once, and a five-word line describes
// nobody. What makes a person legible is their two most pronounced traits — the same way
// one describes an acquaintance — so only those are named, and only when they stand out.
const (
	// traitNotable is how far from the mean a trait must sit to be worth mentioning, and
	// traitStrong how far to be worth emphasising.
	traitNotable = 0.12
	traitStrong  = 0.29
	// traitsNamed caps how many traits a line will mention.
	traitsNamed = 2
)

func traitLine(t sim.Traits) string {
	type named struct {
		v         float32
		low, high string
	}
	all := []named{
		{t.Patience, "restless", "patient"},
		{t.Caution, "reckless", "cautious"},
		{t.Diligence, "idle", "diligent"},
		{t.Rootedness, "rootless", "rooted"},
		{t.Ambition, "content", "ambitious"},
	}
	// Most pronounced first. A stable sort keeps ties in declared order, so two people with
	// identical personalities read identically.
	sort.SliceStable(all, func(a, b int) bool {
		return math.Abs(float64(all[a].v-1)) > math.Abs(float64(all[b].v-1))
	})

	var words []string
	for _, n := range all {
		d := math.Abs(float64(n.v - 1))
		if d < traitNotable || len(words) >= traitsNamed {
			break // sorted, so once one is unremarkable the rest are too
		}
		word := n.high
		if n.v < 1 {
			word = n.low
		}
		if d >= traitStrong {
			word = "very " + word
		}
		words = append(words, word)
	}
	if len(words) == 0 {
		return "even-tempered"
	}
	return strings.Join(words, ", ")
}

// parentLine names whoever a character was born to, marking those who have died.
//
// Empty for the founding settlers, who have no parents on record. A dead parent is still
// named: half of what makes a life legible is knowing who is already gone.
func parentLine(s *sim.State, id sim.CharID) string {
	var parts []string
	for _, p := range []sim.CharID{s.MotherOf(id), s.FatherOf(id)} {
		if p == sim.NoChar {
			continue
		}
		name := s.FullName(p)
		if !s.AliveChar(p) {
			name += " (dead)"
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, " and ")
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

		// The name first: a person is who they are before they are a row of statistics.
		out := fmt.Sprintf("%s — %s, age %.0f\n",
			s.FullName(v.sel.char), personWord(c.Sex, c.Stage()), c.Age)
		if c.Partner != sim.NoChar && s.AliveChar(c.Partner) {
			out += fmt.Sprintf("  married: %s\n", s.FullName(c.Partner))
		}
		// Parents, living or dead — being someone's child does not stop at their funeral.
		// The founding settlers came from somewhere this simulation does not model, so
		// they have none, and the line is omitted rather than padded with "unknown".
		if kin := parentLine(s, v.sel.char); kin != "" {
			out += fmt.Sprintf("  born to: %s\n", kin)
		}
		out += fmt.Sprintf("  nature:  %s\n", traitLine(c.Traits))
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
