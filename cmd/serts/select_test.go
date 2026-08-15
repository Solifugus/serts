package main

import (
	"math"
	"testing"

	"github.com/solifugus/serts/internal/sim"
	"github.com/solifugus/serts/internal/torus"
	"github.com/solifugus/serts/internal/worldgen"
)

func testViewer(t *testing.T) *viewer {
	t.Helper()
	p := worldgen.DefaultParams(7)
	p.CX, p.CY = 128, 128
	w := worldgen.Generate(p)
	v := &viewer{params: p, zoom: 6, world: w}
	v.sim = sim.New(sim.DefaultConfig(w, 7))
	v.cam = v.sim.Structs[0].Pos
	return v
}

// Screen-to-world is the inverse of the draw transform on a map that wraps, which is the
// one piece of this that can be quietly wrong: the same world point has a screen position
// in every visible copy of the world, and they must all resolve to the same place.
func TestScreenToWorldInvertsTheDrawTransform(t *testing.T) {
	v := testViewer(t)

	// The centre of the screen is, by construction, wherever the camera is looking.
	got := v.screenToWorld(windowW/2, windowH/2)
	if d := v.world.T.Dist(got, v.cam); d > 1e-6 {
		t.Errorf("screen centre resolved %v from the camera position", d)
	}

	// Stepping n screen pixels must move exactly n/zoom world units.
	a := v.screenToWorld(windowW/2, windowH/2)
	b := v.screenToWorld(windowW/2+120, windowH/2)
	if want, got := 120/v.zoom, v.world.T.Dist(a, b); math.Abs(got-want) > 1e-6 {
		t.Errorf("120 pixels moved %v world units, want %v", got, want)
	}
}

// Every result must land inside the world however far off-screen the click was, because
// the camera can sit anywhere and the world repeats forever in both directions.
func TestScreenToWorldAlwaysLandsInTheWorld(t *testing.T) {
	v := testViewer(t)
	for _, cam := range []torus.Vec2{{X: 0, Y: 0}, {X: 127.9, Y: 127.9}, {X: 64, Y: 3}} {
		v.cam = cam
		for _, p := range [][2]int{{0, 0}, {windowW, windowH}, {-4000, -4000}, {9000, 9000}} {
			got := v.screenToWorld(p[0], p[1])
			if got.X < 0 || got.X >= v.world.T.W || got.Y < 0 || got.Y >= v.world.T.H {
				t.Fatalf("camera %v, click %v resolved to %v, outside the world", cam, p, got)
			}
		}
	}
}

func TestPickFindsThePersonClickedOn(t *testing.T) {
	v := testViewer(t)
	var who sim.CharID = sim.NoChar
	for i := range v.sim.Chars {
		if v.sim.Chars[i].Alive {
			who = sim.CharID(i)
			break
		}
	}
	if who == sim.NoChar {
		t.Fatal("nobody in the world")
	}

	sel := v.pick(v.sim.Chars[who].Pos)
	if sel.kind != selChar {
		t.Fatalf("clicking a person selected %v", sel.kind)
	}
	// It need not be that exact person — villagers stand close together — but it must be
	// somebody standing about where the click was.
	if d := v.world.T.Dist(v.sim.Chars[sel.char].Pos, v.sim.Chars[who].Pos); d > 2 {
		t.Errorf("selected somebody %v cells from the click", d)
	}
}

func TestPickFindsBuildingsAndEmptyGround(t *testing.T) {
	v := testViewer(t)

	// A building well away from people should be selectable.
	var far sim.StructID = sim.NoStruct
	for i := range v.sim.Structs {
		if v.sim.Structs[i].Type == sim.Farm {
			far = sim.StructID(i)
		}
	}
	if far != sim.NoStruct {
		if sel := v.pick(v.sim.Structs[far].Pos); sel.kind == selNone {
			t.Error("clicking a farm selected nothing")
		}
	}

	// Open water far from the village holds nothing.
	empty := v.world.T.Add(v.sim.Structs[0].Pos, torus.Vec2{X: 60, Y: 60})
	if sel := v.pick(empty); sel.kind != selNone {
		t.Errorf("clicking empty ground selected a %v", sel.kind)
	}
}

// The inspector must never panic and must always say something, including for the dead —
// a selected villager can die at any moment while the panel is open.
func TestInspectHandlesEverySelection(t *testing.T) {
	v := testViewer(t)
	if v.inspect() != "" {
		t.Error("nothing is selected but the inspector spoke")
	}

	v.sel = selection{kind: selChar, char: 0}
	if v.inspect() == "" {
		t.Error("a living person inspected to nothing")
	}
	v.sim.Chars[0].Alive = false
	if txt := v.inspect(); txt == "" {
		t.Error("a dead person inspected to nothing")
	}

	for i := range v.sim.Structs {
		v.sel = selection{kind: selStruct, str: sim.StructID(i)}
		if v.inspect() == "" {
			t.Errorf("structure %d (%v) inspected to nothing", i, v.sim.Structs[i].Type)
		}
	}
}

func TestMarketPanelCoversEveryCommodity(t *testing.T) {
	v := testViewer(t)
	txt := v.market()
	for r := sim.Resource(0); r < sim.NumResources; r++ {
		if !contains(txt, r.String()) {
			t.Errorf("the market panel never mentions %v", r)
		}
	}
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
