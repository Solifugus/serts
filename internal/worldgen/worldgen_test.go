package worldgen

import (
	"math"
	"testing"

	"github.com/solifugus/serts/internal/torus"
)

func testParams(seed int64) Params {
	p := DefaultParams(seed)
	p.CX, p.CY = 128, 128 // smaller than the dev world, so tests stay fast
	return p
}

// A world is identified by its seed forever (§9.2), so generation must be reproducible.
func TestGenerationIsDeterministic(t *testing.T) {
	a := Generate(testParams(99))
	b := Generate(testParams(99))

	if a.SeaLevel != b.SeaLevel {
		t.Fatalf("sea level differs: %v vs %v", a.SeaLevel, b.SeaLevel)
	}
	for i := range a.Elevation {
		if a.Elevation[i] != b.Elevation[i] {
			t.Fatalf("elevation differs at cell %d: %v vs %v", i, a.Elevation[i], b.Elevation[i])
		}
		if a.Water[i] != b.Water[i] {
			t.Fatalf("water differs at cell %d: %v vs %v", i, a.Water[i], b.Water[i])
		}
		if a.FlowTo[i] != b.FlowTo[i] {
			t.Fatalf("flow differs at cell %d: %v vs %v", i, a.FlowTo[i], b.FlowTo[i])
		}
		if a.FlowAcc[i] != b.FlowAcc[i] {
			t.Fatalf("accumulation differs at cell %d: %v vs %v", i, a.FlowAcc[i], b.FlowAcc[i])
		}
	}
}

func TestDifferentSeedsGiveDifferentWorlds(t *testing.T) {
	a := Generate(testParams(1))
	b := Generate(testParams(2))
	same := 0
	for i := range a.Elevation {
		if a.Elevation[i] == b.Elevation[i] {
			same++
		}
	}
	if same > len(a.Elevation)/100 {
		t.Errorf("seeds 1 and 2 share %d/%d elevations", same, len(a.Elevation))
	}
}

func TestLandFractionIsHonoured(t *testing.T) {
	for _, want := range []float64{0.3, 0.5, 0.6, 0.8} {
		p := testParams(5)
		p.LandFraction = want
		w := Generate(p)
		var ocean int
		for i := range w.Water {
			if w.Water[i] == Ocean {
				ocean++
			}
		}
		got := 1 - float64(ocean)/float64(w.T.Cells())
		if math.Abs(got-want) > 0.01 {
			t.Errorf("land fraction %v requested, got %v", want, got)
		}
	}
}

func TestElevationIsNormalised(t *testing.T) {
	w := Generate(testParams(3))
	lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
	for _, v := range w.Elevation {
		lo, hi = min(lo, v), max(hi, v)
	}
	if lo < 0 || hi > 1 {
		t.Errorf("elevation range [%v, %v] escapes [0, 1]", lo, hi)
	}
	if math.Abs(float64(lo)) > 1e-6 || math.Abs(float64(hi)-1) > 1e-6 {
		t.Errorf("elevation range [%v, %v] is not normalised to the full span", lo, hi)
	}
}

// Every cell must be reachable by the flood, or later stages silently skip terrain.
func TestFloodVisitsEveryCell(t *testing.T) {
	w := Generate(testParams(11))
	if len(w.order) != w.T.Cells() {
		t.Fatalf("flood visited %d cells, want %d", len(w.order), w.T.Cells())
	}
	seen := make([]bool, w.T.Cells())
	for _, c := range w.order {
		if seen[c] {
			t.Fatalf("cell %d visited twice", c)
		}
		seen[c] = true
	}
}

// The filled surface must never be below the ground it covers.
func TestSurfaceCoversElevation(t *testing.T) {
	w := Generate(testParams(13))
	for i := range w.Surface {
		if w.Surface[i] < w.Elevation[i] {
			t.Fatalf("cell %d: surface %v below ground %v", i, w.Surface[i], w.Elevation[i])
		}
	}
}

// A lake is by definition water standing above the ground; anything else is mislabelled.
func TestLakesStandAboveGround(t *testing.T) {
	w := Generate(testParams(17))
	lakes := 0
	for i := range w.Water {
		if w.Water[i] == Lake {
			lakes++
			if w.Surface[i] <= w.Elevation[i] {
				t.Fatalf("cell %d is a lake but its surface %v is not above ground %v",
					i, w.Surface[i], w.Elevation[i])
			}
		}
	}
	if lakes == 0 {
		t.Log("warning: no lakes formed at this seed")
	}
}

// The invariant that catches cycles: following the flow from anywhere must reach the sea.
func TestFlowAlwaysReachesTheSea(t *testing.T) {
	w := Generate(testParams(23))
	limit := w.T.Cells() + 1
	for start := 0; start < w.T.Cells(); start++ {
		c := int32(start)
		steps := 0
		for w.FlowTo[c] >= 0 {
			c = w.FlowTo[c]
			steps++
			if steps > limit {
				t.Fatalf("flow from cell %d does not terminate; cycle in the drainage network", start)
			}
		}
		if w.Water[c] != Ocean {
			t.Fatalf("flow from cell %d ends at %d, which is not ocean", start, c)
		}
	}
}

// Water must not run uphill: every step of the network descends or stays level.
func TestFlowNeverRunsUphill(t *testing.T) {
	w := Generate(testParams(29))
	for i := 0; i < w.T.Cells(); i++ {
		to := w.FlowTo[i]
		if to < 0 {
			continue
		}
		if w.Surface[to] > w.Surface[i]+1e-6 {
			t.Fatalf("cell %d (surface %v) flows uphill to %d (surface %v)",
				i, w.Surface[i], to, w.Surface[to])
		}
	}
}

// Flow targets must be actual neighbours, or the network teleports across the map.
func TestFlowGoesToAdjacentCells(t *testing.T) {
	w := Generate(testParams(31))
	for i := 0; i < w.T.Cells(); i++ {
		to := w.FlowTo[i]
		if to < 0 {
			continue
		}
		d := w.T.CellDelta(w.T.CellOf(i), w.T.CellOf(int(to)))
		if d.X < -1 || d.X > 1 || d.Y < -1 || d.Y > 1 {
			t.Fatalf("cell %d flows to non-adjacent cell %d (offset %v)", i, to, d)
		}
	}
}

// Conservation: every cell contributes exactly one unit of area, and all of it must end
// up in the sea. A leak means a cycle swallowed it; an excess means double counting.
func TestAccumulationIsConserved(t *testing.T) {
	w := Generate(testParams(37))
	var atSinks float64
	for i := 0; i < w.T.Cells(); i++ {
		if w.FlowTo[i] < 0 {
			atSinks += float64(w.FlowAcc[i])
		}
	}
	want := float64(w.T.Cells())
	// float32 accumulation over ~16k cells loses a little precision; 0.1% is ample.
	if math.Abs(atSinks-want) > want*0.001 {
		t.Errorf("accumulation at sinks = %v, want %v (drainage network leaks or double counts)", atSinks, want)
	}
}

func TestAccumulationIsAtLeastOne(t *testing.T) {
	w := Generate(testParams(41))
	for i, a := range w.FlowAcc {
		if a < 1 {
			t.Fatalf("cell %d has accumulation %v, below its own contribution", i, a)
		}
	}
}

// Rivers are the cells carrying the most water, so they must out-drain ordinary land.
func TestRiversCarryMoreFlowThanLand(t *testing.T) {
	w := Generate(testParams(43))
	var minRiver float32 = math.MaxFloat32
	var maxDry float32
	rivers := 0
	for i := range w.Water {
		switch w.Water[i] {
		case River:
			rivers++
			minRiver = min(minRiver, w.FlowAcc[i])
		case Dry:
			maxDry = max(maxDry, w.FlowAcc[i])
		}
	}
	if rivers == 0 {
		t.Fatal("no rivers generated")
	}
	if minRiver < maxDry {
		t.Errorf("weakest river (%v) carries less than the wettest dry cell (%v)", minRiver, maxDry)
	}
}

func TestRiverSizesAreAssigned(t *testing.T) {
	w := Generate(testParams(47))
	counts := map[RiverSize]int{}
	for i := range w.Water {
		if w.Water[i] == River {
			counts[w.RiverSize[i]]++
			if w.RiverSize[i] == NotRiver {
				t.Fatalf("cell %d is a river with no size", i)
			}
		}
	}
	for _, sz := range []RiverSize{Stream, SmallRiver, MajorRiver} {
		if counts[sz] == 0 {
			t.Errorf("no rivers of size %v generated", sz)
		}
	}
	if counts[MajorRiver] > counts[Stream] {
		t.Errorf("more major rivers (%d) than streams (%d); size buckets look inverted",
			counts[MajorRiver], counts[Stream])
	}
}

// The seam is where a wrapped world betrays a mistake. Terrain must be no more
// discontinuous across the boundary than it is anywhere else.
func TestTerrainIsContinuousAcrossSeam(t *testing.T) {
	w := Generate(testParams(53))

	step := func(a, b torus.Cell) float64 {
		return math.Abs(float64(w.Elevation[w.T.Index(a)] - w.Elevation[w.T.Index(b)]))
	}

	var seamX, seamY, interior float64
	n := 0
	for i := 0; i < w.T.CY; i++ {
		seamX = math.Max(seamX, step(torus.Cell{X: w.T.CX - 1, Y: i}, torus.Cell{X: 0, Y: i}))
		seamY = math.Max(seamY, step(torus.Cell{X: i, Y: w.T.CY - 1}, torus.Cell{X: i, Y: 0}))
		for x := 20; x < 40; x++ {
			interior += step(torus.Cell{X: x, Y: i}, torus.Cell{X: x + 1, Y: i})
			n++
		}
	}
	avg := interior / float64(n)
	// A torn world shows an elevation cliff running the whole height of the map.
	if seamX > avg*20 {
		t.Errorf("x seam step %v vastly exceeds mean interior step %v: world is torn", seamX, avg)
	}
	if seamY > avg*20 {
		t.Errorf("y seam step %v vastly exceeds mean interior step %v: world is torn", seamY, avg)
	}
}

// The latitude band uses cos(2*pi*y/H) precisely so climate wraps; a linear gradient
// would tear here.
func TestTemperatureWrapsInY(t *testing.T) {
	w := Generate(testParams(59))
	var seam, interior float64
	n := 0
	for x := 0; x < w.T.CX; x++ {
		top := w.Temperature[w.T.Index(torus.Cell{X: x, Y: 0})]
		bottom := w.Temperature[w.T.Index(torus.Cell{X: x, Y: w.T.CY - 1})]
		seam = math.Max(seam, math.Abs(float64(top-bottom)))
		for y := 20; y < 40; y++ {
			a := w.Temperature[w.T.Index(torus.Cell{X: x, Y: y})]
			b := w.Temperature[w.T.Index(torus.Cell{X: x, Y: y + 1})]
			interior += math.Abs(float64(a - b))
			n++
		}
	}
	if avg := interior / float64(n); seam > avg*20 {
		t.Errorf("temperature seam step %v vastly exceeds interior step %v", seam, avg)
	}
}

func TestTemperatureHasBothWarmAndColdBelts(t *testing.T) {
	w := Generate(testParams(61))
	lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
	for _, v := range w.Temperature {
		lo, hi = min(lo, v), max(hi, v)
	}
	if hi-lo < 0.5 {
		t.Errorf("temperature range [%v, %v] is too narrow for distinct climate belts", lo, hi)
	}
}

func TestStatsAddUp(t *testing.T) {
	w := Generate(testParams(67))
	s := w.Stats()
	if s.Ocean+s.Lake+s.River+s.Land != s.Cells {
		t.Errorf("water classes sum to %d, want %d", s.Ocean+s.Lake+s.River+s.Land, s.Cells)
	}
	if s.MaxFlow > float32(s.Cells) {
		t.Errorf("max flow %v exceeds total cells %d", s.MaxFlow, s.Cells)
	}
}

// Degenerate parameters must not hang or panic; they are reachable from the viewer.
func TestExtremeLandFractions(t *testing.T) {
	for _, f := range []float64{0.0, 0.05, 0.99, 1.0} {
		p := testParams(71)
		p.CX, p.CY = 64, 64
		p.LandFraction = f
		w := Generate(p) // must not panic or hang
		if len(w.order) != w.T.Cells() {
			t.Errorf("land fraction %v: flood visited %d of %d cells", f, len(w.order), w.T.Cells())
		}
	}
}

func BenchmarkGenerate256(b *testing.B) {
	p := DefaultParams(1)
	for i := 0; i < b.N; i++ {
		Generate(p)
	}
}

func BenchmarkGenerate1024(b *testing.B) {
	p := DefaultParams(1)
	p.CX, p.CY = 1024, 1024
	for i := 0; i < b.N; i++ {
		Generate(p)
	}
}
