package noise

import (
	"math"
	"testing"
)

const (
	worldW, worldH = 256.0, 256.0
	// The circle mapping goes through sin/cos, so the two sides of the seam agree to
	// floating-point rounding rather than bit-exactly. Anything at this magnitude is
	// many orders below a visible difference in terrain.
	seamTol = 1e-9
)

// The reason this package exists: the two edges of the world must be the same place.
func TestTileableIsSeamlessInX(t *testing.T) {
	p := New(1)
	for y := 0.0; y < worldH; y += 3.5 {
		a := p.Tileable(0, y, worldW, worldH, 4)
		b := p.Tileable(worldW, y, worldW, worldH, 4)
		if math.Abs(a-b) > seamTol {
			t.Errorf("seam at y=%v: left=%v right=%v (diff %v)", y, a, b, math.Abs(a-b))
		}
	}
}

func TestTileableIsSeamlessInY(t *testing.T) {
	p := New(1)
	for x := 0.0; x < worldW; x += 3.5 {
		a := p.Tileable(x, 0, worldW, worldH, 4)
		b := p.Tileable(x, worldH, worldW, worldH, 4)
		if math.Abs(a-b) > seamTol {
			t.Errorf("seam at x=%v: top=%v bottom=%v (diff %v)", x, a, b, math.Abs(a-b))
		}
	}
}

// Continuity across the seam matters as much as equality: neighbouring cells on opposite
// sides must differ no more than neighbouring cells anywhere else, or the seam shows up
// as an edge even though the values match exactly at the boundary.
func TestTileableIsContinuousAcrossSeam(t *testing.T) {
	p := New(7)
	var interior, atSeam float64
	n := 0
	for y := 0.0; y < worldH; y += 1 {
		// Step across the seam: last cell to first cell.
		d := math.Abs(p.Tileable(worldW-1, y, worldW, worldH, 4) - p.Tileable(0, y, worldW, worldH, 4))
		if d > atSeam {
			atSeam = d
		}
		// Compare against typical interior steps of the same size.
		for x := 10.0; x < 20; x++ {
			interior += math.Abs(p.Tileable(x, y, worldW, worldH, 4) - p.Tileable(x+1, y, worldW, worldH, 4))
			n++
		}
	}
	avgInterior := interior / float64(n)
	// A discontinuity would show as a seam step far larger than a typical neighbour step.
	if atSeam > avgInterior*10 {
		t.Errorf("seam step %v is disproportionate to interior average %v", atSeam, avgInterior)
	}
}

func TestFBmIsSeamless(t *testing.T) {
	p := New(42)
	cfg := DefaultFBm()
	for y := 0.0; y < worldH; y += 7 {
		a := p.FBm(0, y, worldW, worldH, cfg)
		b := p.FBm(worldW, y, worldW, worldH, cfg)
		if math.Abs(a-b) > seamTol {
			t.Errorf("fBm seam at y=%v: %v vs %v", y, a, b)
		}
	}
	for x := 0.0; x < worldW; x += 7 {
		a := p.FBm(x, 0, worldW, worldH, cfg)
		b := p.FBm(x, worldH, worldW, worldH, cfg)
		if math.Abs(a-b) > seamTol {
			t.Errorf("fBm seam at x=%v: %v vs %v", x, a, b)
		}
	}
}

// Domain warping displaces the sample point, which is exactly the operation most likely
// to break periodicity by accident.
func TestWarpedIsSeamless(t *testing.T) {
	p := New(9)
	cfg := DefaultFBm()
	for y := 0.0; y < worldH; y += 7 {
		a := p.Warped(0, y, worldW, worldH, cfg, 20)
		b := p.Warped(worldW, y, worldW, worldH, cfg, 20)
		if math.Abs(a-b) > seamTol {
			t.Errorf("warped seam at y=%v: %v vs %v", y, a, b)
		}
	}
	for x := 0.0; x < worldW; x += 7 {
		a := p.Warped(x, 0, worldW, worldH, cfg, 20)
		b := p.Warped(x, worldH, worldW, worldH, cfg, 20)
		if math.Abs(a-b) > seamTol {
			t.Errorf("warped seam at x=%v: %v vs %v", x, a, b)
		}
	}
}

// A world is defined by its seed forever, so the same seed must give the same terrain.
func TestSameSeedSameNoise(t *testing.T) {
	a, b := New(12345), New(12345)
	for i := 0; i < 500; i++ {
		x, y := float64(i)*0.7, float64(i)*1.3
		va := a.FBm(x, y, worldW, worldH, DefaultFBm())
		vb := b.FBm(x, y, worldW, worldH, DefaultFBm())
		if va != vb {
			t.Fatalf("same seed diverged at (%v,%v): %v vs %v", x, y, va, vb)
		}
	}
}

func TestDifferentSeedsDiffer(t *testing.T) {
	a, b := New(1), New(2)
	same := 0
	const n = 200
	for i := 0; i < n; i++ {
		x, y := float64(i)*1.1, float64(i)*0.9
		if a.FBm(x, y, worldW, worldH, DefaultFBm()) == b.FBm(x, y, worldW, worldH, DefaultFBm()) {
			same++
		}
	}
	if same > n/10 {
		t.Errorf("seeds 1 and 2 agree at %d/%d samples; permutation may not be seeded", same, n)
	}
}

func TestOutputRangeIsSane(t *testing.T) {
	p := New(3)
	lo, hi := math.Inf(1), math.Inf(-1)
	for y := 0.0; y < worldH; y += 2 {
		for x := 0.0; x < worldW; x += 2 {
			v := p.FBm(x, y, worldW, worldH, DefaultFBm())
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	if lo < -1.001 || hi > 1.001 {
		t.Errorf("fBm range [%v, %v] escapes [-1, 1]", lo, hi)
	}
	// Degenerate output (a constant field) would also satisfy the bound above.
	if hi-lo < 0.2 {
		t.Errorf("fBm range [%v, %v] is suspiciously flat", lo, hi)
	}
}

func TestFreqIsRoundedToKeepTilingExact(t *testing.T) {
	p := New(5)
	// A fractional frequency cannot tile, so it is rounded; 4.4 and 4 must agree.
	for y := 0.0; y < 50; y += 5 {
		a := p.Tileable(10, y, worldW, worldH, 4.4)
		b := p.Tileable(10, y, worldW, worldH, 4)
		if a != b {
			t.Errorf("freq 4.4 not rounded to 4 at y=%v: %v vs %v", y, a, b)
		}
	}
}

func BenchmarkFBm(b *testing.B) {
	p := New(1)
	cfg := DefaultFBm()
	var acc float64
	for i := 0; i < b.N; i++ {
		acc += p.FBm(float64(i%256), float64(i%251), worldW, worldH, cfg)
	}
	_ = acc
}
