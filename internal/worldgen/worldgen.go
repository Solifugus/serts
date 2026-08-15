// Package worldgen builds SERTS worlds from a seed (design §2.9).
//
// Generation runs once at world creation and never again, so it may be expensive; its
// cost budget is entirely separate from the tick budget the load governor manages
// (§2.4). What it may not be is non-deterministic: a world is identified by its seed
// forever, so the same seed must always produce the same terrain.
//
// The pipeline currently covers stages 1-6 of the design, plus temperature:
//
//  1. elevation      warped fractal noise, seamlessly tileable
//  2. sea level      chosen to hit a target land fraction exactly
//  3. lakes          priority-flood depression filling
//  4. flow direction D8 descent, falling back to the flood tree on flats
//  5. accumulation   upstream drainage area per cell
//  6. rivers         cells above an accumulation threshold, width by flow
//  8. temperature    wrapping latitude band plus elevation lapse
//
// Soil, woodland, ore, moisture, and biomes belong to the next milestone.
package worldgen

import (
	"container/heap"
	"fmt"
	"math"
	"slices"

	"github.com/solifugus/serts/internal/noise"
	"github.com/solifugus/serts/internal/torus"
)

// Water classifies a cell's surface water.
type Water uint8

const (
	Dry Water = iota
	Ocean
	Lake
	River
)

// RiverSize buckets a river by flow, which the design turns into crossing difficulty
// (§2.8): streams are fordable anywhere, major rivers need a bridge or boat.
type RiverSize uint8

const (
	NotRiver RiverSize = iota
	Stream
	SmallRiver
	MajorRiver
)

// Params configures generation. The zero value is not useful; see DefaultParams.
type Params struct {
	Seed         int64
	CX, CY       int     // grid dimensions
	LandFraction float64 // proportion of the world above sea level
	Freq         float64 // base terrain frequency, in features across the world
	Octaves      int
	WarpStrength float64 // domain warp in world units; 0 disables
	// RiverDensity is the fraction of land cells that become river. Higher values put
	// more, smaller watercourses on the map.
	RiverDensity float64
}

// DefaultParams returns generation settings tuned for a 256-cell development world.
func DefaultParams(seed int64) Params {
	return Params{
		Seed:         seed,
		CX:           256,
		CY:           256,
		LandFraction: 0.60,
		Freq:         3,
		Octaves:      6,
		WarpStrength: 18,
		RiverDensity: 0.030,
	}
}

// World is a generated world. All per-cell slices are indexed by torus.T.Index.
type World struct {
	T      torus.T
	Params Params

	// Elevation is the raw terrain surface, normalised to [0, 1].
	Elevation []float32
	// Surface is Elevation with depressions filled: the height of the ground, or of a
	// lake's surface where one has formed. Flow routing runs on this, not Elevation.
	Surface []float32
	// Water classifies each cell's surface water.
	Water []Water
	// FlowTo is the downstream cell index, or -1 for a terminal sink (ocean).
	FlowTo []int32
	// FlowAcc is upstream drainage area in cells, including the cell itself.
	FlowAcc []float32
	// RiverSize buckets river cells by flow.
	RiverSize []RiverSize
	// Temperature is normalised to [0, 1]; 0 is coldest.
	Temperature []float32
	// Soil is farmland quality in [0, 1]. Farm yield is local soil multiplied by the
	// global fertility governor (§2.4).
	Soil []float32
	// FreshDist is the cell distance to the nearest river or lake, saturating at
	// freshDistMax. Farms need fresh water within reach to irrigate (§2.8), and it is
	// cheaper to answer that from a precomputed field than to search per placement.
	FreshDist []int16
	// GoldOre is recoverable gold in a cell, in coin. It is the economy's faucet (§4.2)
	// and it depletes: what is panned out does not come back.
	GoldOre []float32

	SeaLevel float64
	// order is the priority-flood pop order, which is a valid downstream-first
	// topological ordering of every cell. Retained because it is exactly what
	// accumulation needs, and cheap to keep.
	order []int32
	// parent is each cell's discoverer in the flood, giving every cell a guaranteed
	// path to the sea. Flow routing falls back to it on flats.
	parent []int32
}

// Generate builds a world. It is deterministic in Params alone.
func Generate(p Params) *World {
	if p.CX <= 0 || p.CY <= 0 {
		panic("worldgen: world must have positive dimensions")
	}
	t := torus.New(p.CX, p.CY)
	w := &World{
		T:           t,
		Params:      p,
		Elevation:   make([]float32, t.Cells()),
		Surface:     make([]float32, t.Cells()),
		Water:       make([]Water, t.Cells()),
		FlowTo:      make([]int32, t.Cells()),
		FlowAcc:     make([]float32, t.Cells()),
		RiverSize:   make([]RiverSize, t.Cells()),
		Temperature: make([]float32, t.Cells()),
		Soil:        make([]float32, t.Cells()),
		FreshDist:   make([]int16, t.Cells()),
		GoldOre:     make([]float32, t.Cells()),
	}

	w.genElevation()
	w.chooseSeaLevel()
	w.fillDepressions()
	w.routeFlow()
	w.accumulate()
	w.classifyRivers()
	w.genTemperature()
	w.measureFreshWater()
	w.genSoil()
	w.genGold()
	return w
}

// Gold placement constants. Gold must be scarce enough that panning cannot conjure money
// freely (§4.2), so scarcity is made a property of the map rather than a tuned rate:
// most of the world holds none at all.
const (
	// GoldRarity is the noise threshold a cell must clear to bear gold. Higher is rarer.
	GoldRarity = 0.42
	// GoldLodeYield is the coin in a rich mountain cell.
	GoldLodeYield = 900
	// GoldPlacerShare is how much of a lode's gold washes downstream per cell of river,
	// forming placer deposits a person can pan without a mine.
	GoldPlacerShare = 0.22
)

// genGold places lodes in high ground and washes placer deposits down the rivers.
//
// The two kinds matter differently. Lodes are mining: concentrated, in hard country, and
// the basis of an industrial mint. Placers are panning: thin, spread along riverbeds near
// where people already live, and therefore what an unemployed character can actually
// reach on foot. Deriving placers from lodes by following the drainage network means the
// gold is where geology says it should be rather than where noise happened to put it.
func (w *World) genGold() {
	src := noise.New(w.Params.Seed + 7919)
	cfg := noise.FBmParams{Octaves: 3, Freq: 9, Lacunarity: 2, Gain: 0.5}

	lode := make([]float32, w.T.Cells())
	for y := 0; y < w.T.CY; y++ {
		for x := 0; x < w.T.CX; x++ {
			i := w.T.Index(torus.Cell{X: x, Y: y})
			if w.Water[i] == Ocean || w.Water[i] == Lake {
				continue
			}
			n := src.FBm(float64(x), float64(y), w.T.W, w.T.H, cfg)
			if n <= GoldRarity {
				continue
			}
			rich := (n - GoldRarity) / (1 - GoldRarity)

			// Lodes want high, hard ground.
			above := (float64(w.Elevation[i]) - w.SeaLevel) / math.Max(1-w.SeaLevel, 1e-6)
			height := math.Max(0, (above-0.45)/0.55)
			lode[i] = float32(rich * height * GoldLodeYield)
		}
	}

	// Wash it downstream. The priority-flood order runs low to high, so walking it in
	// reverse visits every cell before the one it drains into — the same trick flow
	// accumulation uses (see accumulate).
	carried := make([]float32, w.T.Cells())
	for k := len(w.order) - 1; k >= 0; k-- {
		c := w.order[k]
		total := lode[c] + carried[c]
		if to := w.FlowTo[c]; to >= 0 && total > 0 {
			carried[to] += total * GoldPlacerShare
		}
	}

	for i := 0; i < w.T.Cells(); i++ {
		w.GoldOre[i] = lode[i]
		// Only watercourses hold placer gold; it is a river deposit.
		if w.Water[i] == River {
			w.GoldOre[i] += carried[i] * (1 - GoldPlacerShare)
		}
	}
}

// TotalGold sums the gold left in the ground, for tuning and for reporting scarcity.
func (w *World) TotalGold() float64 {
	var t float64
	for _, g := range w.GoldOre {
		t += float64(g)
	}
	return t
}

// GoldCells counts how many cells bear any gold at all.
func (w *World) GoldCells() int {
	n := 0
	for _, g := range w.GoldOre {
		if g > 0 {
			n++
		}
	}
	return n
}

// FreshDistMax is the distance at which FreshDist saturates. Nothing in the simulation
// cares how far away water is once it is far enough to be useless.
const FreshDistMax = 64

// measureFreshWater computes the distance from every cell to the nearest fresh water by
// multi-source breadth-first search, wrapping like everything else.
func (w *World) measureFreshWater() {
	n := w.T.Cells()
	queue := make([]int32, 0, n)
	for i := 0; i < n; i++ {
		if w.Water[i] == River || w.Water[i] == Lake {
			w.FreshDist[i] = 0
			queue = append(queue, int32(i))
		} else {
			w.FreshDist[i] = FreshDistMax
		}
	}
	for head := 0; head < len(queue); head++ {
		ci := queue[head]
		d := w.FreshDist[ci]
		if d >= FreshDistMax {
			continue
		}
		c := w.T.CellOf(int(ci))
		for k := 0; k < 8; k++ {
			ni := w.T.Index(w.T.Neighbor8(c, k))
			if w.FreshDist[ni] > d+1 {
				w.FreshDist[ni] = d + 1
				queue = append(queue, int32(ni))
			}
		}
	}
}

// genSoil derives farmland quality from climate, slope, and proximity to rivers.
//
// The floodplain bonus is the mechanically important part (§2.8): river valleys become
// the map's prime farmland, which is what draws settlement onto water without any rule
// saying it must, and what makes riverside ruins worth resettling first.
func (w *World) genSoil() {
	src := noise.New(w.Params.Seed + 5501)
	cfg := noise.FBmParams{Octaves: 4, Freq: 5, Lacunarity: 2, Gain: 0.5}

	for y := 0; y < w.T.CY; y++ {
		for x := 0; x < w.T.CX; x++ {
			c := torus.Cell{X: x, Y: y}
			i := w.T.Index(c)
			if w.Water[i] == Ocean || w.Water[i] == Lake {
				w.Soil[i] = 0
				continue
			}

			// Temperate ground is best; extremes of heat and cold are poor.
			temp := float64(w.Temperature[i])
			climate := 1 - math.Abs(temp-0.58)/0.58
			climate = math.Max(0, math.Min(1, climate))

			// Steep ground sheds soil.
			slope := w.slopeAt(c)
			flat := math.Max(0, 1-slope*18)

			// Floodplains: rich beside a watercourse, tailing off within a few cells.
			flood := math.Max(0, 1-float64(w.FreshDist[i])/6)

			v := 0.20 + 0.35*climate + 0.20*flat + 0.30*flood
			v += 0.12 * src.FBm(float64(x), float64(y), w.T.W, w.T.H, cfg)
			w.Soil[i] = float32(math.Max(0, math.Min(1, v)))
		}
	}
}

// slopeAt measures local steepness from wrapped neighbours.
func (w *World) slopeAt(c torus.Cell) float64 {
	e := func(dx, dy int) float64 {
		return float64(w.Elevation[w.T.Index(torus.Cell{X: c.X + dx, Y: c.Y + dy})])
	}
	dx := e(1, 0) - e(-1, 0)
	dy := e(0, 1) - e(0, -1)
	return math.Hypot(dx, dy) / 2
}

// Walkable reports whether a cell can be crossed on foot.
//
// Rivers are fordable everywhere for now. Crossing difficulty by river size, and the
// bridges that go with it (§2.8), arrive with the structures that make them matter.
func (w *World) Walkable(i int) bool {
	return w.Water[i] != Ocean && w.Water[i] != Lake
}

// genElevation fills Elevation with warped fractal noise, normalised to [0, 1].
func (w *World) genElevation() {
	p := w.Params
	src := noise.New(p.Seed)
	cfg := noise.FBmParams{Octaves: p.Octaves, Freq: p.Freq, Lacunarity: 2, Gain: 0.5}

	lo, hi := math.Inf(1), math.Inf(-1)
	for y := 0; y < w.T.CY; y++ {
		for x := 0; x < w.T.CX; x++ {
			v := src.Warped(float64(x), float64(y), w.T.W, w.T.H, cfg, p.WarpStrength)
			w.Elevation[w.T.Index(torus.Cell{X: x, Y: y})] = float32(v)
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	if hi <= lo {
		return // degenerate; leave flat
	}
	scale := 1 / (hi - lo)
	for i, v := range w.Elevation {
		w.Elevation[i] = float32((float64(v) - lo) * scale)
	}
}

// chooseSeaLevel picks the threshold that yields exactly the requested land fraction.
//
// Sorting a copy and taking the quantile is exact and simpler than binary searching the
// threshold, and generation is a one-time cost where that trade is free.
func (w *World) chooseSeaLevel() {
	f := w.Params.LandFraction
	if f <= 0 {
		w.SeaLevel = 1.1 // everything is ocean
		return
	}
	if f >= 1 {
		w.SeaLevel = -0.1 // no ocean at all; fillDepressions handles the consequences
		return
	}
	sorted := slices.Clone(w.Elevation)
	slices.Sort(sorted)
	idx := int(float64(len(sorted)) * (1 - f))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	w.SeaLevel = float64(sorted[idx])
}

// --- Priority flood ---

type pqItem struct {
	level float32
	cell  int32
}

type pq []pqItem

func (q pq) Len() int { return len(q) }
func (q pq) Less(i, j int) bool {
	if q[i].level != q[j].level {
		return q[i].level < q[j].level
	}
	// Ties broken by index so the pop order — and therefore every downstream result —
	// is identical on every run. Determinism (§9.2) depends on this.
	return q[i].cell < q[j].cell
}
func (q pq) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *pq) Push(x any)   { *q = append(*q, x.(pqItem)) }
func (q *pq) Pop() any     { old := *q; n := len(old); it := old[n-1]; *q = old[:n-1]; return it }

// fillDepressions runs priority flood (Barnes et al.), which computes for every cell the
// water level at which it would drain to the ocean. Cells whose water level exceeds their
// ground become lakes.
//
// The traversal also yields two things the later stages need: a pop order that is a valid
// downstream-first topological ordering, and a parent link giving each cell a guaranteed
// path to the sea. The parent link is what makes flow routing robust on flats, where
// steepest descent has no answer.
func (w *World) fillDepressions() {
	n := w.T.Cells()
	visited := make([]bool, n)
	parent := make([]int32, n)
	w.order = make([]int32, 0, n)

	q := make(pq, 0, n/4)
	sea := float32(w.SeaLevel)

	// Seed with the ocean, the only terminal sink on a torus. Without a map edge, water
	// that cannot reach the sea has nowhere to go at all.
	for i := 0; i < n; i++ {
		parent[i] = -1
		if w.Elevation[i] <= sea {
			w.Surface[i] = sea
			w.Water[i] = Ocean
			visited[i] = true
			q = append(q, pqItem{level: sea, cell: int32(i)})
		}
	}
	if len(q) == 0 {
		// No ocean: seed the single lowest cell so the flood still has a root and the
		// world drains somewhere rather than deadlocking.
		low := int32(0)
		for i := 1; i < n; i++ {
			if w.Elevation[i] < w.Elevation[low] {
				low = int32(i)
			}
		}
		w.Surface[low] = w.Elevation[low]
		visited[low] = true
		q = append(q, pqItem{level: w.Elevation[low], cell: low})
	}
	heap.Init(&q)

	for q.Len() > 0 {
		it := heap.Pop(&q).(pqItem)
		w.order = append(w.order, it.cell)
		c := w.T.CellOf(int(it.cell))

		for k := 0; k < 8; k++ {
			nb := w.T.Neighbor8(c, k)
			ni := w.T.Index(nb)
			if visited[ni] {
				continue
			}
			visited[ni] = true
			parent[ni] = it.cell
			// The neighbour's surface is its own ground, or the level of the water
			// backed up behind it, whichever is higher.
			lvl := w.Elevation[ni]
			if it.level > lvl {
				lvl = it.level
			}
			w.Surface[ni] = lvl
			if w.Water[ni] != Ocean && lvl > w.Elevation[ni] {
				w.Water[ni] = Lake
			}
			heap.Push(&q, pqItem{level: lvl, cell: int32(ni)})
		}
	}
	w.parent = parent
}

// routeFlow assigns every cell a downstream neighbour.
//
// Steepest descent on the filled surface is used where a strictly lower neighbour exists.
// On flats — lake surfaces especially — it has no answer, so the flood tree's parent link
// is used instead. Both rules move to a cell that was popped earlier, so the combination
// cannot produce a cycle.
func (w *World) routeFlow() {
	n := w.T.Cells()
	for i := 0; i < n; i++ {
		if w.Water[i] == Ocean {
			w.FlowTo[i] = -1 // the sea is where drainage ends
			continue
		}
		c := w.T.CellOf(i)
		best := int32(-1)
		bestLevel := w.Surface[i]
		for k := 0; k < 8; k++ {
			ni := int32(w.T.Index(w.T.Neighbor8(c, k)))
			if w.Surface[ni] < bestLevel {
				bestLevel = w.Surface[ni]
				best = ni
			}
		}
		if best < 0 {
			best = w.parent[i] // flat: fall back to the guaranteed path seawards
		}
		w.FlowTo[i] = best
	}
}

// accumulate sums upstream drainage area into every cell.
//
// The priority-flood pop order is non-decreasing in surface level, and every flow target
// sits at or below its source and was therefore popped earlier. Walking that order in
// reverse visits every cell before its downstream neighbour, which makes this a single
// linear pass with no sorting and no risk of a cycle.
func (w *World) accumulate() {
	for i := range w.FlowAcc {
		w.FlowAcc[i] = 1
	}
	for i := len(w.order) - 1; i >= 0; i-- {
		c := w.order[i]
		to := w.FlowTo[c]
		if to >= 0 {
			w.FlowAcc[to] += w.FlowAcc[c]
		}
	}
}

// classifyRivers marks land cells carrying enough flow to be watercourses, and buckets
// them by size. The threshold is derived from a target density so that maps with more or
// less land still get a comparable river network.
func (w *World) classifyRivers() {
	type cellFlow struct {
		acc  float32
		cell int32
	}
	land := make([]cellFlow, 0, w.T.Cells())
	for i := 0; i < w.T.Cells(); i++ {
		if w.Water[i] == Dry {
			land = append(land, cellFlow{w.FlowAcc[i], int32(i)})
		}
	}
	if len(land) == 0 {
		return
	}
	slices.SortFunc(land, func(a, b cellFlow) int {
		if a.acc != b.acc {
			if a.acc > b.acc {
				return -1
			}
			return 1
		}
		return int(a.cell - b.cell) // deterministic tie-break
	})

	count := int(float64(len(land)) * w.Params.RiverDensity)
	if count > len(land) {
		count = len(land)
	}
	for i := 0; i < count; i++ {
		c := land[i].cell
		w.Water[c] = River
		// Size by position within the river set: the top tenth are the trunk rivers.
		switch {
		case i < count/10:
			w.RiverSize[c] = MajorRiver
		case i < count/3:
			w.RiverSize[c] = SmallRiver
		default:
			w.RiverSize[c] = Stream
		}
	}
}

// genTemperature builds a climate gradient that survives wrapping.
//
// A linear north-south gradient would tear at the seam. cos(2*pi*y/H) gives one warm belt
// and one cold belt that meet themselves smoothly, so the world has real climate variety
// while remaining perfectly toroidal (§2.9). Elevation then cools high ground.
func (w *World) genTemperature() {
	src := noise.New(w.Params.Seed + 977)
	cfg := noise.FBmParams{Octaves: 3, Freq: 2, Lacunarity: 2, Gain: 0.5}
	const lapse = 0.35 // how strongly altitude cools

	for y := 0; y < w.T.CY; y++ {
		band := 0.5 + 0.5*math.Cos(2*math.Pi*float64(y)/w.T.H)
		for x := 0; x < w.T.CX; x++ {
			i := w.T.Index(torus.Cell{X: x, Y: y})
			jitter := 0.12 * src.FBm(float64(x), float64(y), w.T.W, w.T.H, cfg)
			above := math.Max(0, float64(w.Elevation[i])-w.SeaLevel)
			v := band + jitter - above*lapse
			w.Temperature[i] = float32(math.Min(1, math.Max(0, v)))
		}
	}
}

// --- Reporting ---

// Stats summarises a generated world, for validation (§2.9) and for sanity-checking
// generation parameters.
type Stats struct {
	Cells                    int
	Ocean, Lake, River, Land int
	LandFraction             float64
	MaxFlow                  float32
	MeanElevation            float64
}

// Stats computes summary statistics.
func (w *World) Stats() Stats {
	s := Stats{Cells: w.T.Cells()}
	var sum float64
	for i := 0; i < s.Cells; i++ {
		switch w.Water[i] {
		case Ocean:
			s.Ocean++
		case Lake:
			s.Lake++
		case River:
			s.River++
		default:
			s.Land++
		}
		sum += float64(w.Elevation[i])
		if w.FlowAcc[i] > s.MaxFlow {
			s.MaxFlow = w.FlowAcc[i]
		}
	}
	s.LandFraction = float64(s.Cells-s.Ocean) / float64(s.Cells)
	s.MeanElevation = sum / float64(s.Cells)
	return s
}

func (s Stats) String() string {
	return fmt.Sprintf(
		"%d cells: %.1f%% land (%d dry, %d river, %d lake, %d ocean)  maxflow=%.0f  mean elev=%.3f",
		s.Cells, s.LandFraction*100, s.Land, s.River, s.Lake, s.Ocean, s.MaxFlow, s.MeanElevation)
}
