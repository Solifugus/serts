package torus

import (
	"math"
	"testing"
)

const eps = 1e-9

func TestWrapFoldsIntoRange(t *testing.T) {
	w := New(256, 256)
	cases := []struct{ in, want Vec2 }{
		{Vec2{0, 0}, Vec2{0, 0}},
		{Vec2{255.5, 255.5}, Vec2{255.5, 255.5}},
		{Vec2{256, 256}, Vec2{0, 0}},
		{Vec2{257.25, 260}, Vec2{1.25, 4}},
		{Vec2{-1, -0.5}, Vec2{255, 255.5}},
		{Vec2{-256, -512}, Vec2{0, 0}},
		{Vec2{-700.5, 1000.25}, Vec2{67.5, 232.25}},
	}
	for _, c := range cases {
		got := w.Wrap(c.in)
		if math.Abs(got.X-c.want.X) > eps || math.Abs(got.Y-c.want.Y) > eps {
			t.Errorf("Wrap(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestWrapIsAlwaysInRange(t *testing.T) {
	w := New(256, 256)
	for i := -1000; i <= 1000; i++ {
		v := float64(i) * 0.37
		p := w.Wrap(Vec2{v, -v})
		if p.X < 0 || p.X >= w.W || p.Y < 0 || p.Y >= w.H {
			t.Fatalf("Wrap produced out-of-range %v for %v", p, v)
		}
	}
}

// Delta must take the short way around, which is the whole point of the package.
func TestDeltaTakesShortWay(t *testing.T) {
	w := New(256, 256)
	cases := []struct {
		a, b Vec2
		want Vec2
	}{
		{Vec2{10, 10}, Vec2{20, 20}, Vec2{10, 10}},   // no seam involved
		{Vec2{250, 10}, Vec2{5, 10}, Vec2{11, 0}},    // forward across the seam
		{Vec2{5, 10}, Vec2{250, 10}, Vec2{-11, 0}},   // backward across the seam
		{Vec2{10, 250}, Vec2{10, 5}, Vec2{0, 11}},    // vertical seam
		{Vec2{0, 0}, Vec2{200, 200}, Vec2{-56, -56}}, // long way is shorter backwards
	}
	for _, c := range cases {
		got := w.Delta(c.a, c.b)
		if math.Abs(got.X-c.want.X) > eps || math.Abs(got.Y-c.want.Y) > eps {
			t.Errorf("Delta(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestDeltaNeverExceedsHalfWorld(t *testing.T) {
	w := New(256, 256)
	for i := 0; i < 256; i++ {
		for j := 0; j < 256; j += 7 {
			d := w.Delta(Vec2{float64(i), 0}, Vec2{float64(j), 0})
			if math.Abs(d.X) > w.W/2+eps {
				t.Fatalf("Delta(%d, %d).X = %v exceeds half world", i, j, d.X)
			}
		}
	}
}

func TestDistIsSymmetricAndWraps(t *testing.T) {
	w := New(256, 256)
	a, b := Vec2{2, 2}, Vec2{254, 254}
	// Naive Euclidean distance would be ~356; across the seam it is 4*sqrt(2).
	got := w.Dist(a, b)
	want := math.Hypot(4, 4)
	if math.Abs(got-want) > eps {
		t.Errorf("Dist(%v, %v) = %v, want %v", a, b, got, want)
	}
	if r := w.Dist(b, a); math.Abs(r-got) > eps {
		t.Errorf("Dist is not symmetric: %v vs %v", got, r)
	}
}

func TestMaxDistIsHalfDiagonal(t *testing.T) {
	w := New(256, 256)
	// The most distant point from any origin is the antipode at (W/2, H/2).
	got := w.Dist(Vec2{0, 0}, Vec2{128, 128})
	want := math.Hypot(128, 128)
	if math.Abs(got-want) > eps {
		t.Errorf("antipodal distance = %v, want %v", got, want)
	}
	// Nothing may exceed it.
	for x := 0; x < 256; x += 3 {
		for y := 0; y < 256; y += 3 {
			if d := w.Dist(Vec2{0, 0}, Vec2{float64(x), float64(y)}); d > want+eps {
				t.Fatalf("Dist to (%d,%d) = %v exceeds half-diagonal %v", x, y, d, want)
			}
		}
	}
}

func TestAddWraps(t *testing.T) {
	w := New(256, 256)
	got := w.Add(Vec2{255, 255}, Vec2{2, 3})
	want := Vec2{1, 2}
	if math.Abs(got.X-want.X) > eps || math.Abs(got.Y-want.Y) > eps {
		t.Errorf("Add = %v, want %v", got, want)
	}
}

// Go's % is truncating, so negative indices are the case that breaks naive code.
func TestWrapCellHandlesNegatives(t *testing.T) {
	w := New(256, 128)
	cases := []struct{ in, want Cell }{
		{Cell{0, 0}, Cell{0, 0}},
		{Cell{255, 127}, Cell{255, 127}},
		{Cell{256, 128}, Cell{0, 0}},
		{Cell{-1, -1}, Cell{255, 127}},
		{Cell{-256, -128}, Cell{0, 0}},
		{Cell{-257, 300}, Cell{255, 44}},
	}
	for _, c := range cases {
		if got := w.WrapCell(c.in); got != c.want {
			t.Errorf("WrapCell(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCellDeltaTakesShortWay(t *testing.T) {
	w := New(256, 256)
	cases := []struct {
		a, b Cell
		want Cell
	}{
		{Cell{10, 10}, Cell{20, 20}, Cell{10, 10}},
		{Cell{250, 10}, Cell{5, 10}, Cell{11, 0}},
		{Cell{5, 10}, Cell{250, 10}, Cell{-11, 0}},
	}
	for _, c := range cases {
		if got := w.CellDelta(c.a, c.b); got != c.want {
			t.Errorf("CellDelta(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestCellAtBridgesLayers(t *testing.T) {
	w := New(256, 256)
	cases := []struct {
		in   Vec2
		want Cell
	}{
		{Vec2{0, 0}, Cell{0, 0}},
		{Vec2{0.999, 0.999}, Cell{0, 0}},
		{Vec2{1.0, 1.0}, Cell{1, 1}},
		{Vec2{255.999, 255.999}, Cell{255, 255}},
		{Vec2{-0.001, -0.001}, Cell{255, 255}}, // just before the seam
	}
	for _, c := range cases {
		if got := w.CellAt(c.in); got != c.want {
			t.Errorf("CellAt(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCenterRoundTrips(t *testing.T) {
	w := New(256, 256)
	for _, c := range []Cell{{0, 0}, {5, 9}, {255, 255}, {128, 0}} {
		if got := w.CellAt(w.Center(c)); got != c {
			t.Errorf("CellAt(Center(%v)) = %v", c, got)
		}
	}
}

func TestIndexRoundTrips(t *testing.T) {
	w := New(64, 32)
	seen := make(map[int]Cell, w.Cells())
	for y := 0; y < w.CY; y++ {
		for x := 0; x < w.CX; x++ {
			c := Cell{x, y}
			i := w.Index(c)
			if i < 0 || i >= w.Cells() {
				t.Fatalf("Index(%v) = %d out of range", c, i)
			}
			if prev, dup := seen[i]; dup {
				t.Fatalf("Index collision: %v and %v both map to %d", prev, c, i)
			}
			seen[i] = c
			if got := w.CellOf(i); got != c {
				t.Errorf("CellOf(Index(%v)) = %v", c, got)
			}
		}
	}
}

func TestIndexWrapsOutOfRangeCells(t *testing.T) {
	w := New(64, 32)
	if a, b := w.Index(Cell{-1, -1}), w.Index(Cell{63, 31}); a != b {
		t.Errorf("Index(-1,-1) = %d, want same as Index(63,31) = %d", a, b)
	}
}

// The seam is where terrain automata silently break, so neighbours are checked there
// specifically rather than only in the interior.
func TestNeighbors8WrapAcrossSeam(t *testing.T) {
	w := New(256, 256)
	got := w.Neighbors8(Cell{0, 0})
	want := map[Cell]bool{
		{255, 255}: true, {0, 255}: true, {1, 255}: true,
		{255, 0}: true, {1, 0}: true,
		{255, 1}: true, {0, 1}: true, {1, 1}: true,
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected neighbour %v of origin", n)
		}
		delete(want, n)
	}
	if len(want) != 0 {
		t.Errorf("missing neighbours: %v", want)
	}
}

func TestNeighbors8AreDistinctAndAdjacent(t *testing.T) {
	w := New(256, 256)
	for _, c := range []Cell{{0, 0}, {255, 255}, {128, 0}, {0, 128}, {77, 43}} {
		ns := w.Neighbors8(c)
		seen := map[Cell]bool{}
		for i, n := range ns {
			if seen[n] {
				t.Errorf("duplicate neighbour %v of %v", n, c)
			}
			seen[n] = true
			if n == c {
				t.Errorf("cell %v is its own neighbour", c)
			}
			// Every neighbour must be one step away on the torus.
			d := w.CellDelta(c, n)
			if d.X < -1 || d.X > 1 || d.Y < -1 || d.Y > 1 {
				t.Errorf("neighbour %d of %v is %v, offset %v is not adjacent", i, c, n, d)
			}
			if w.Neighbor8(c, i) != n {
				t.Errorf("Neighbor8(%v, %d) disagrees with Neighbors8", c, i)
			}
		}
	}
}

func TestNeighborOrderIsStable(t *testing.T) {
	// Determinism (§9.2) requires that iteration order never varies between runs.
	w := New(256, 256)
	first := w.Neighbors8(Cell{7, 9})
	for i := 0; i < 100; i++ {
		if w.Neighbors8(Cell{7, 9}) != first {
			t.Fatal("Neighbors8 order is not stable across calls")
		}
	}
}

func TestDiagonalClassification(t *testing.T) {
	diag := 0
	for i := 0; i < 8; i++ {
		if Diagonal(i) {
			diag++
		}
	}
	if diag != 4 {
		t.Errorf("got %d diagonal neighbours, want 4", diag)
	}
}

// A non-square world is the case where mixing up axes stops being invisible.
func TestNonSquareWorld(t *testing.T) {
	w := New(64, 16)
	if got := w.WrapCell(Cell{70, 20}); got != (Cell{6, 4}) {
		t.Errorf("WrapCell = %v, want {6 4}", got)
	}
	if got := w.CellAt(Vec2{65.5, 17.5}); got != (Cell{1, 1}) {
		t.Errorf("CellAt = %v, want {1 1}", got)
	}
	d := w.Delta(Vec2{1, 1}, Vec2{63, 15})
	if math.Abs(d.X-(-2)) > eps || math.Abs(d.Y-(-2)) > eps {
		t.Errorf("Delta = %v, want {-2 -2}", d)
	}
}

func BenchmarkDist(b *testing.B) {
	w := New(1024, 1024)
	p, q := Vec2{1, 2}, Vec2{1000, 1010}
	var acc float64
	for i := 0; i < b.N; i++ {
		acc += w.Dist(p, q)
	}
	_ = acc
}

func BenchmarkNeighbors8(b *testing.B) {
	w := New(1024, 1024)
	var acc int
	for i := 0; i < b.N; i++ {
		acc += w.Neighbors8(Cell{0, 0})[3].X
	}
	_ = acc
}
