package sim

import (
	"math"

	"github.com/solifugus/serts/internal/torus"
	"github.com/solifugus/serts/internal/worldgen"
)

// WalkSpeed is world units travelled per in-world hour. One cell is one unit, so this is
// also cells per hour.
const WalkSpeed = 4.0

// walkPerTick is how far a character moves in one tick.
const walkPerTick = WalkSpeed * MinutesPerTick / 60.0

// flowField is a precomputed route to one destination.
//
// Every cell holds the neighbour index to step toward, so following it from anywhere on
// the map walks to the target. This is the shape of pathfinding the design calls for
// (§9.4): a hundred people walking to the same farm cost one field, not a hundred paths.
// Per-agent A* would recompute almost the same route once per person.
type flowField struct {
	dir  []int8    // neighbour index 0-7, or -1 where unreachable
	dist []float32 // cost to the destination, for arrival checks and diagnostics
}

// pathCache holds one field per destination structure.
type pathCache struct {
	fields map[StructID]*flowField
	// hits and misses make the cache's value visible in the HUD rather than assumed.
	hits, misses int
}

func newPathCache() *pathCache {
	return &pathCache{fields: make(map[StructID]*flowField)}
}

// invalidate drops a destination's field. Terrain is static this milestone, so this is
// only needed when a structure appears or moves.
func (p *pathCache) invalidate(id StructID) { delete(p.fields, id) }

// fieldTo returns the flow field leading to a structure, computing it on first use.
func (s *State) fieldTo(id StructID) *flowField {
	if f, ok := s.paths.fields[id]; ok {
		s.paths.hits++
		return f
	}
	s.paths.misses++
	f := s.computeField(s.Structs[id].Cell)
	s.paths.fields[id] = f
	return f
}

// computeField runs Dijkstra outward from a destination over walkable terrain.
//
// Diagonal steps cost sqrt(2) so routes do not prefer staircases, and rivers cost extra
// to reflect the effort of fording — which is the seed of the crossing-difficulty model
// in §2.8, and what bridges will one day make cheap.
func (s *State) computeField(dest torus.Cell) *flowField {
	n := s.T.Cells()
	f := &flowField{
		dir:  make([]int8, n),
		dist: make([]float32, n),
	}
	for i := range f.dir {
		f.dir[i] = -1
		f.dist[i] = math.MaxFloat32
	}

	di := s.T.Index(dest)
	if !s.World.Walkable(di) {
		return f // unreachable destination; everyone stays put
	}
	f.dist[di] = 0

	// A simple binary heap keyed by distance. Ties break by cell index so the field is
	// identical on every run, which determinism requires (§9.2).
	type node struct {
		d float32
		c int32
	}
	heapData := []node{{0, int32(di)}}
	push := func(nd node) {
		heapData = append(heapData, nd)
		i := len(heapData) - 1
		for i > 0 {
			p := (i - 1) / 2
			if heapData[p].d < heapData[i].d ||
				(heapData[p].d == heapData[i].d && heapData[p].c <= heapData[i].c) {
				break
			}
			heapData[p], heapData[i] = heapData[i], heapData[p]
			i = p
		}
	}
	pop := func() node {
		top := heapData[0]
		last := len(heapData) - 1
		heapData[0] = heapData[last]
		heapData = heapData[:last]
		i := 0
		for {
			l, r, small := 2*i+1, 2*i+2, i
			if l < last && (heapData[l].d < heapData[small].d ||
				(heapData[l].d == heapData[small].d && heapData[l].c < heapData[small].c)) {
				small = l
			}
			if r < last && (heapData[r].d < heapData[small].d ||
				(heapData[r].d == heapData[small].d && heapData[r].c < heapData[small].c)) {
				small = r
			}
			if small == i {
				break
			}
			heapData[i], heapData[small] = heapData[small], heapData[i]
			i = small
		}
		return top
	}

	for len(heapData) > 0 {
		cur := pop()
		if cur.d > f.dist[cur.c] {
			continue // stale entry
		}
		c := s.T.CellOf(int(cur.c))
		for k := 0; k < 8; k++ {
			nb := s.T.Neighbor8(c, k)
			ni := s.T.Index(nb)
			if !s.World.Walkable(ni) {
				continue
			}
			step := float32(1)
			if torus.Diagonal(k) {
				step = 1.41421356
			}
			step *= s.terrainCost(ni)

			if nd := cur.d + step; nd < f.dist[ni] {
				f.dist[ni] = nd
				// The neighbour steps back toward the cell it was reached from, which is
				// the opposite of the direction travelled to get here.
				f.dir[ni] = int8(opposite(k))
				push(node{nd, int32(ni)})
			}
		}
	}
	return f
}

// opposite returns the neighbour index pointing the other way.
//
// The offset table is symmetric about its centre, but index 4 is missing (a cell is not
// its own neighbour), so the mapping is not simply 7-k.
var oppositeIdx = [8]int{7, 6, 5, 4, 3, 2, 1, 0}

func opposite(k int) int { return oppositeIdx[k] }

// terrainCost is the movement multiplier for a cell.
func (s *State) terrainCost(i int) float32 {
	if s.World.Water[i] == worldgen.River {
		return 3.0 // fording is slow; bridges will make this cheap (§2.8)
	}
	return 1
}

// moveToward advances a character one tick along the flow field to their destination,
// and reports whether they have arrived.
//
// Movement is continuous even though the field is per-cell: the character steers toward
// the centre of the next cell, so paths look like walking rather than stepping between
// squares (§2.2).
func (s *State) moveToward(id CharID, dest StructID) bool {
	c := &s.Chars[id]
	target := s.Structs[dest].Pos

	// Close enough to be at the building.
	const arriveRadius = 0.9
	if s.T.Dist(c.Pos, target) <= arriveRadius {
		return true
	}

	f := s.fieldTo(dest)
	cell := s.T.CellAt(c.Pos)
	ci := s.T.Index(cell)

	var step torus.Vec2
	if d := f.dir[ci]; d >= 0 {
		next := s.T.Neighbor8(cell, int(d))
		// Steer toward the next cell's centre rather than snapping to it.
		step = s.T.Delta(c.Pos, s.T.Center(next))
	} else {
		// No route — either unreachable or already in the destination cell. Walk
		// straight at it and let the arrival check end the journey.
		step = s.T.Delta(c.Pos, target)
	}

	l := math.Hypot(step.X, step.Y)
	if l < 1e-9 {
		return true
	}
	move := walkPerTick
	if move > l {
		move = l
	}
	next := s.T.Add(c.Pos, torus.Vec2{X: step.X / l * move, Y: step.Y / l * move})

	// Never walk into water. The flow field already routes around it, but the
	// straight-line fallback above does not, and a character who steps into a lake has
	// left the walkable graph entirely — after which the field offers them no way back.
	if s.World.Walkable(s.T.Index(s.T.CellAt(next))) {
		c.Pos = next
	}
	return s.T.Dist(c.Pos, target) <= arriveRadius
}
