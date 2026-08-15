// Package noise provides seamlessly tileable gradient noise for SERTS's toroidal world
// (design §2.9).
//
// Ordinary 2D noise sampled on a plane does not tile. On a world that wraps, that
// produces a visible discontinuity running the full height and width of the map — a
// permanent scar on a world that is never regenerated.
//
// The fix is to sample 4D noise along two circles: the x axis traces one circle, the y
// axis another, so both axes return exactly to their starting values. This is exactly
// seamless rather than approximately so, and costs only the difference between 2D and 4D
// noise evaluation.
package noise

import "math"

// Perlin is a seeded 4D Perlin noise source. A given seed always produces identical
// output, which is what makes worlds reproducible from a seed alone (§9.2).
type Perlin struct {
	perm [512]int // p[0:256] shuffled, duplicated to avoid masking on every lookup
}

// New returns a noise source for the given seed.
func New(seed int64) *Perlin {
	p := &Perlin{}
	var base [256]int
	for i := range base {
		base[i] = i
	}
	// Fisher-Yates with an explicit LCG rather than math/rand: the sequence must be
	// stable across Go versions, since a world's terrain is defined by its seed forever.
	state := uint64(seed)*6364136223846793005 + 1442695040888963407
	next := func() uint64 {
		state = state*6364136223846793005 + 1442695040888963407
		return state >> 33
	}
	for i := 255; i > 0; i-- {
		j := int(next() % uint64(i+1))
		base[i], base[j] = base[j], base[i]
	}
	for i := 0; i < 512; i++ {
		p.perm[i] = base[i&255]
	}
	return p
}

// grad4 is the standard set of 32 four-dimensional gradient vectors: every vector with
// one zero component and the rest +/-1.
var grad4 = [32][4]float64{
	{0, 1, 1, 1}, {0, 1, 1, -1}, {0, 1, -1, 1}, {0, 1, -1, -1},
	{0, -1, 1, 1}, {0, -1, 1, -1}, {0, -1, -1, 1}, {0, -1, -1, -1},
	{1, 0, 1, 1}, {1, 0, 1, -1}, {1, 0, -1, 1}, {1, 0, -1, -1},
	{-1, 0, 1, 1}, {-1, 0, 1, -1}, {-1, 0, -1, 1}, {-1, 0, -1, -1},
	{1, 1, 0, 1}, {1, 1, 0, -1}, {1, -1, 0, 1}, {1, -1, 0, -1},
	{-1, 1, 0, 1}, {-1, 1, 0, -1}, {-1, -1, 0, 1}, {-1, -1, 0, -1},
	{1, 1, 1, 0}, {1, 1, -1, 0}, {1, -1, 1, 0}, {1, -1, -1, 0},
	{-1, 1, 1, 0}, {-1, 1, -1, 0}, {-1, -1, 1, 0}, {-1, -1, -1, 0},
}

// fade is Perlin's quintic interpolant, 6t^5 - 15t^4 + 10t^3. Its first and second
// derivatives vanish at 0 and 1, which is what keeps cell boundaries invisible.
func fade(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }

func lerp(t, a, b float64) float64 { return a + t*(b-a) }

func (p *Perlin) grad(hash int, x, y, z, w float64) float64 {
	g := grad4[hash&31]
	return g[0]*x + g[1]*y + g[2]*z + g[3]*w
}

// At4 evaluates 4D Perlin noise, returning roughly [-1, 1].
func (p *Perlin) At4(x, y, z, w float64) float64 {
	xi, yi := int(math.Floor(x)), int(math.Floor(y))
	zi, wi := int(math.Floor(z)), int(math.Floor(w))
	xf, yf := x-float64(xi), y-float64(yi)
	zf, wf := z-float64(zi), w-float64(wi)

	xi, yi, zi, wi = xi&255, yi&255, zi&255, wi&255
	u, v, s, t := fade(xf), fade(yf), fade(zf), fade(wf)

	pm := &p.perm
	// Hash the sixteen corners of the hypercube.
	a := pm[xi] + yi
	aa, ab := pm[a]+zi, pm[a+1]+zi
	b := pm[xi+1] + yi
	ba, bb := pm[b]+zi, pm[b+1]+zi

	aaa, aab := pm[aa]+wi, pm[aa+1]+wi
	aba, abb := pm[ab]+wi, pm[ab+1]+wi
	baa, bab := pm[ba]+wi, pm[ba+1]+wi
	bba, bbb := pm[bb]+wi, pm[bb+1]+wi

	// Interpolate along w, then z, then y, then x.
	x1 := lerp(t, p.grad(pm[aaa], xf, yf, zf, wf), p.grad(pm[aaa+1], xf, yf, zf, wf-1))
	x2 := lerp(t, p.grad(pm[aba], xf, yf-1, zf, wf), p.grad(pm[aba+1], xf, yf-1, zf, wf-1))
	y1 := lerp(v, x1, x2)

	x1 = lerp(t, p.grad(pm[aab], xf, yf, zf-1, wf), p.grad(pm[aab+1], xf, yf, zf-1, wf-1))
	x2 = lerp(t, p.grad(pm[abb], xf, yf-1, zf-1, wf), p.grad(pm[abb+1], xf, yf-1, zf-1, wf-1))
	y2 := lerp(v, x1, x2)
	z1 := lerp(s, y1, y2)

	x1 = lerp(t, p.grad(pm[baa], xf-1, yf, zf, wf), p.grad(pm[baa+1], xf-1, yf, zf, wf-1))
	x2 = lerp(t, p.grad(pm[bba], xf-1, yf-1, zf, wf), p.grad(pm[bba+1], xf-1, yf-1, zf, wf-1))
	y1 = lerp(v, x1, x2)

	x1 = lerp(t, p.grad(pm[bab], xf-1, yf, zf-1, wf), p.grad(pm[bab+1], xf-1, yf, zf-1, wf-1))
	x2 = lerp(t, p.grad(pm[bbb], xf-1, yf-1, zf-1, wf), p.grad(pm[bbb+1], xf-1, yf-1, zf-1, wf-1))
	y2 = lerp(v, x1, x2)
	z2 := lerp(s, y1, y2)

	return lerp(u, z1, z2)
}

// Tileable evaluates noise at (x, y) in a world of size (w, h) such that the result is
// exactly periodic in both axes.
//
// freq is the number of noise features across the world: freq=4 repeats roughly four
// times horizontally and vertically. It must be an integer for the tiling to be exact,
// and is rounded if it is not.
func (p *Perlin) Tileable(x, y, w, h, freq float64) float64 {
	f := math.Round(freq)
	if f < 1 {
		f = 1
	}
	// Map each axis onto a circle. A full traverse of the world is a full revolution,
	// so the sample returns exactly to where it started.
	u := 2 * math.Pi * x / w
	v := 2 * math.Pi * y / h
	r := f / (2 * math.Pi)
	return p.At4(r*math.Cos(u), r*math.Sin(u), r*math.Cos(v), r*math.Sin(v))
}

// FBmParams configures fractal Brownian motion: several octaves of noise summed at
// increasing frequency and decreasing amplitude.
type FBmParams struct {
	Octaves    int     // number of layers; each doubles frequency
	Freq       float64 // base frequency, in features across the world
	Lacunarity float64 // frequency multiplier per octave (2 is conventional)
	Gain       float64 // amplitude multiplier per octave (0.5 is conventional)
}

// DefaultFBm returns reasonable terrain parameters.
func DefaultFBm() FBmParams {
	return FBmParams{Octaves: 6, Freq: 3, Lacunarity: 2, Gain: 0.5}
}

// FBm evaluates tileable fractal noise, normalised to roughly [-1, 1].
//
// Every octave frequency stays integral, so the sum remains exactly periodic.
func (p *Perlin) FBm(x, y, w, h float64, cfg FBmParams) float64 {
	var sum, amp, norm float64 = 0, 1, 0
	freq := cfg.Freq
	for i := 0; i < cfg.Octaves; i++ {
		sum += amp * p.Tileable(x, y, w, h, freq)
		norm += amp
		amp *= cfg.Gain
		freq *= cfg.Lacunarity
	}
	if norm == 0 {
		return 0
	}
	return sum / norm
}

// Warped evaluates fractal noise whose sample point has itself been displaced by noise.
//
// Undisplaced fBm produces recognisably blobby, isotropic shapes. Domain warping bends
// the sampling grid, yielding ragged coastlines and ridges that read as geology rather
// than as noise. The offsets keep the warp fields decorrelated from each other and from
// the base field.
func (p *Perlin) Warped(x, y, w, h float64, cfg FBmParams, strength float64) float64 {
	if strength == 0 {
		return p.FBm(x, y, w, h, cfg)
	}
	warp := FBmParams{Octaves: 4, Freq: math.Max(1, math.Round(cfg.Freq/2)), Lacunarity: 2, Gain: 0.5}
	// The warp is applied in world units, and both displaced coordinates stay in the
	// world's domain, so periodicity survives.
	dx := p.FBm(x+w*0.37, y+h*0.11, w, h, warp)
	dy := p.FBm(x+w*0.71, y+h*0.83, w, h, warp)
	return p.FBm(x+dx*strength, y+dy*strength, w, h, cfg)
}
