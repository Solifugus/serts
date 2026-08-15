// Package torus is the single source of truth for coordinate arithmetic on SERTS's
// wrapped world (design §9.3).
//
// The world wraps on both axes, which means the naive difference between two
// coordinates is wrong wherever it crosses the seam. Every distance check, direction
// vector, neighbour lookup, and area query in the simulation must therefore go through
// this package. Inconsistent wrapping is the largest single bug source in wrapped-world
// games, and it fails silently: a seam bug is invisible until something happens to
// straddle the boundary.
//
// Two layers coexist. Vec2 is continuous world space, where characters live and move at
// any angle. Cell is the discrete terrain grid, where soil, woodland, and ore are
// stored. Both wrap, by different arithmetic, and CellAt is the only sanctioned bridge
// between them.
//
// Vec2 and Cell are structs rather than bare coordinate pairs specifically so that Go's
// type system rejects `a - b`. There is no valid Euclidean subtraction on a torus.
package torus

import "math"

// Vec2 is a position in continuous world space.
type Vec2 struct{ X, Y float64 }

// Cell is a position on the discrete terrain grid.
type Cell struct{ X, Y int }

// T describes a toroidal world of a given size.
//
// The design fixes one grid cell per world unit (§2.2), so conversion between layers is
// a floor operation, but nothing here assumes that.
type T struct {
	W, H   float64 // continuous extent
	CX, CY int     // grid dimensions in cells
}

// New returns a toroidal world of cx by cy cells, one world unit per cell.
func New(cx, cy int) T {
	return T{W: float64(cx), H: float64(cy), CX: cx, CY: cy}
}

// Cells returns the total number of grid cells.
func (t T) Cells() int { return t.CX * t.CY }

// ---- Continuous layer ----

// wrap1 folds v into [0, size).
func wrap1(v, size float64) float64 {
	r := math.Mod(v, size)
	if r < 0 {
		r += size
	}
	// math.Mod can return size for tiny negative v after rounding; clamp defensively so
	// callers never see an out-of-range coordinate.
	if r >= size {
		r = 0
	}
	return r
}

// Wrap folds a position into the world.
func (t T) Wrap(p Vec2) Vec2 {
	return Vec2{wrap1(p.X, t.W), wrap1(p.Y, t.H)}
}

// delta1 is the shortest signed distance from a to b, taking the short way around.
func delta1(a, b, size float64) float64 {
	d := math.Mod(b-a, size)
	if d > size/2 {
		d -= size
	}
	if d < -size/2 {
		d += size
	}
	return d
}

// Delta returns the shortest signed vector from a to b. This is the only correct
// substitute for b - a, and the value to steer along when moving from a toward b.
func (t T) Delta(a, b Vec2) Vec2 {
	return Vec2{delta1(a.X, b.X, t.W), delta1(a.Y, b.Y, t.H)}
}

// Dist returns the shortest distance between two positions.
func (t T) Dist(a, b Vec2) float64 {
	d := t.Delta(a, b)
	return math.Hypot(d.X, d.Y)
}

// Dist2 returns the squared shortest distance, for hot paths that only compare
// distances and should not pay for a square root.
func (t T) Dist2(a, b Vec2) float64 {
	d := t.Delta(a, b)
	return d.X*d.X + d.Y*d.Y
}

// Add moves p by v and wraps the result.
func (t T) Add(p, v Vec2) Vec2 {
	return t.Wrap(Vec2{p.X + v.X, p.Y + v.Y})
}

// ---- Discrete layer ----

// wrapCell1 folds a cell index into [0, n).
//
// Go's % is truncating rather than modular, so a negative index needs the extra fold.
// Getting this wrong makes terrain automata fail to propagate across the seam.
func wrapCell1(i, n int) int { return ((i % n) + n) % n }

// WrapCell folds a cell coordinate into the grid.
func (t T) WrapCell(c Cell) Cell {
	return Cell{wrapCell1(c.X, t.CX), wrapCell1(c.Y, t.CY)}
}

// cellDelta1 is the shortest signed cell offset from a to b.
func cellDelta1(a, b, n int) int {
	d := wrapCell1(b-a, n)
	if d > n/2 {
		d -= n
	}
	return d
}

// CellDelta returns the shortest signed cell offset from a to b.
func (t T) CellDelta(a, b Cell) Cell {
	return Cell{cellDelta1(a.X, b.X, t.CX), cellDelta1(a.Y, b.Y, t.CY)}
}

// CellDist2 returns the squared shortest cell distance between two cells.
func (t T) CellDist2(a, b Cell) int {
	d := t.CellDelta(a, b)
	return d.X*d.X + d.Y*d.Y
}

// ---- The bridge between layers ----

// CellAt returns the grid cell containing a continuous position. This is the only
// sanctioned conversion between the two layers.
func (t T) CellAt(p Vec2) Cell {
	sx := t.W / float64(t.CX)
	sy := t.H / float64(t.CY)
	return t.WrapCell(Cell{
		int(math.Floor(p.X / sx)),
		int(math.Floor(p.Y / sy)),
	})
}

// Center returns the continuous position at the centre of a cell.
func (t T) Center(c Cell) Vec2 {
	sx := t.W / float64(t.CX)
	sy := t.H / float64(t.CY)
	c = t.WrapCell(c)
	return Vec2{(float64(c.X) + 0.5) * sx, (float64(c.Y) + 0.5) * sy}
}

// ---- Grid storage ----

// Index maps a cell to an offset in a flat row-major array of length Cells().
// The cell is wrapped first, so out-of-range input is folded rather than panicking.
func (t T) Index(c Cell) int {
	c = t.WrapCell(c)
	return c.Y*t.CX + c.X
}

// CellOf is the inverse of Index.
func (t T) CellOf(i int) Cell {
	return Cell{i % t.CX, i / t.CX}
}

// offsets8 lists the eight neighbours in a fixed order. The order is fixed because
// iteration order must never vary between runs: determinism (§9.2) depends on it.
var offsets8 = [8]Cell{
	{-1, -1}, {0, -1}, {1, -1},
	{-1, 0}, {1, 0},
	{-1, 1}, {0, 1}, {1, 1},
}

// Neighbors8 returns the eight wrapped neighbours of a cell, in a fixed order.
//
// This is what woodland spread and soil recovery iterate over (§2.5), and what D8 flow
// routing uses during map generation.
func (t T) Neighbors8(c Cell) [8]Cell {
	var out [8]Cell
	for i, o := range offsets8 {
		out[i] = t.WrapCell(Cell{c.X + o.X, c.Y + o.Y})
	}
	return out
}

// Neighbor8 returns the i'th wrapped neighbour of a cell, for callers that want to
// avoid materialising the whole array in a hot loop.
func (t T) Neighbor8(c Cell, i int) Cell {
	o := offsets8[i]
	return t.WrapCell(Cell{c.X + o.X, c.Y + o.Y})
}

// Diagonal reports whether neighbour index i is a diagonal step, which costs sqrt(2)
// rather than 1.
func Diagonal(i int) bool {
	o := offsets8[i]
	return o.X != 0 && o.Y != 0
}
