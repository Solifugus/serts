// Package render turns a generated world into images.
//
// It is deliberately independent of any graphics library: it produces image.RGBA, which
// both the still-image tool and the interactive viewer consume. The design keeps the
// simulation and the renderer either side of an interface (§9.1), and this package sits
// firmly on the presentation side.
package render

import (
	"image"
	"image/color"
	"math"

	"github.com/solifugus/serts/internal/torus"
	"github.com/solifugus/serts/internal/worldgen"
)

// Layer selects what to draw.
type Layer int

const (
	Terrain Layer = iota
	Elevation
	WaterOnly
	Flow
	Temperature
	numLayers
)

// Name returns a short label for a layer, for the viewer's HUD.
func (l Layer) Name() string {
	switch l {
	case Terrain:
		return "terrain"
	case Elevation:
		return "elevation"
	case WaterOnly:
		return "water"
	case Flow:
		return "drainage"
	case Temperature:
		return "temperature"
	}
	return "?"
}

// NumLayers is how many layers the viewer can cycle through.
const NumLayers = int(numLayers)

func lerp8(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	t = math.Min(1, math.Max(0, t))
	return color.RGBA{lerp8(a.R, b.R, t), lerp8(a.G, b.G, t), lerp8(a.B, b.B, t), 255}
}

var (
	deepOcean    = color.RGBA{12, 34, 74, 255}
	shallowOcean = color.RGBA{38, 94, 150, 255}
	lakeColor    = color.RGBA{62, 132, 190, 255}
	riverColor   = color.RGBA{74, 150, 205, 255}
	bigRiver     = color.RGBA{44, 112, 178, 255}
	beach        = color.RGBA{206, 196, 148, 255}
	lowland      = color.RGBA{88, 128, 70, 255}
	upland       = color.RGBA{122, 122, 72, 255}
	rock         = color.RGBA{124, 112, 102, 255}
	snow         = color.RGBA{238, 240, 244, 255}
)

// field wraps a world's per-cell data with bilinear sampling.
//
// The terrain grid is the simulation's resolution, but elevation underneath it is a
// continuous noise field, so it can legitimately be reconstructed between cell centres.
// Doing so is what stops hills from looking like staircases when zoomed in, while
// discrete facts — where a river is, where the coast falls — stay crisp because they are
// sampled nearest rather than blended.
type field struct {
	w *worldgen.World
}

// elevAt bilinearly samples elevation at a continuous world position.
// wetAt is how much of the ground at a point is water, interpolated across cell centres.
//
// Rivers and lakes are per-cell facts the simulation acts on, and the terrain image used
// to draw them exactly as classified — which is honest and looks like a staircase, since
// a river a single cell wide has square corners at every bend. This blends the binary
// field and thresholds it, which is marching-squares in effect: the same water, the same
// width, with the boundary following a smooth curve instead of the cell grid.
//
// The render smooths the picture; it does not tell the simulation anything. A character
// still crosses the river exactly where the cells say it is.
func (f field) wetAt(x, y float64) float64 {
	t := f.w.T
	gx, gy := x-0.5, y-0.5
	x0, y0 := math.Floor(gx), math.Floor(gy)
	tx, ty := gx-x0, gy-y0

	at := func(ix, iy int) float64 {
		switch f.w.Water[t.Index(torus.Cell{X: ix, Y: iy})] {
		case worldgen.Lake, worldgen.River:
			return 1
		}
		return 0
	}
	ix, iy := int(x0), int(y0)
	a := at(ix, iy)*(1-tx) + at(ix+1, iy)*tx
	b := at(ix, iy+1)*(1-tx) + at(ix+1, iy+1)*tx
	return a*(1-ty) + b*ty
}

// wetKindAt reports which of lake or river dominates near a point, so a smoothed edge
// keeps the colour of the water it belongs to.
func (f field) wetKindAt(x, y float64) worldgen.Water {
	t := f.w.T
	best, bestD := worldgen.River, math.MaxFloat64
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			cx, cy := int(math.Floor(x))+dx, int(math.Floor(y))+dy
			k := f.w.Water[t.Index(torus.Cell{X: cx, Y: cy})]
			if k != worldgen.Lake && k != worldgen.River {
				continue
			}
			ddx, ddy := float64(cx)+0.5-x, float64(cy)+0.5-y
			if d := ddx*ddx + ddy*ddy; d < bestD {
				best, bestD = k, d
			}
		}
	}
	return best
}

func (f field) elevAt(x, y float64) float64 {
	t := f.w.T
	// Cell centres sit at +0.5, so shift before flooring to find the enclosing quad.
	gx, gy := x-0.5, y-0.5
	x0, y0 := math.Floor(gx), math.Floor(gy)
	tx, ty := gx-x0, gy-y0

	at := func(ix, iy int) float64 {
		return float64(f.w.Elevation[t.Index(torus.Cell{X: ix, Y: iy})])
	}
	ix, iy := int(x0), int(y0)
	a := at(ix, iy)*(1-tx) + at(ix+1, iy)*tx
	b := at(ix, iy+1)*(1-tx) + at(ix+1, iy+1)*tx
	return a*(1-ty) + b*ty
}

// Image renders a whole world.
//
// detail is the number of output pixels per terrain cell. 1 renders the raw grid; higher
// values reconstruct elevation between cells so that zooming in shows smooth ground
// rather than squares. The result is still exactly tileable, which is what lets the
// viewer draw a wrapped world by repeating it (§9.3).
func Image(w *worldgen.World, l Layer, detail int) *image.RGBA {
	if detail < 1 {
		detail = 1
	}
	// Only terrain and elevation are continuous fields; the rest are per-cell facts that
	// blending would misrepresent.
	if l != Terrain && l != Elevation {
		detail = 1
	}

	f := field{w}
	t := w.T
	pw, ph := t.CX*detail, t.CY*detail
	img := image.NewRGBA(image.Rect(0, 0, pw, ph))

	maxLogFlow := math.Log1p(float64(w.Stats().MaxFlow))
	if maxLogFlow == 0 {
		maxLogFlow = 1
	}
	step := 1 / float64(detail)

	for py := 0; py < ph; py++ {
		wy := (float64(py) + 0.5) * step
		for px := 0; px < pw; px++ {
			wx := (float64(px) + 0.5) * step
			cell := t.CellAt(torus.Vec2{X: wx, Y: wy})
			i := t.Index(cell)

			elev := float64(w.Elevation[i])
			wat := w.Water[i]
			if detail > 1 {
				elev = f.elevAt(wx, wy)
				// The coastline is where the interpolated ground meets the sea, which is
				// a smooth curve rather than a staircase of cell edges. Lakes and rivers
				// keep their cell classification: those are genuine per-cell facts the
				// simulation acts on, and blending them would invent water where none is.
				if wat == worldgen.Ocean && elev > w.SeaLevel {
					wat = worldgen.Dry
				} else if wat == worldgen.Dry && elev <= w.SeaLevel {
					wat = worldgen.Ocean
				}
				// Inland water gets the same treatment the coast already had: the
				// boundary follows the interpolated field rather than the cell grid, so
				// a river bend is a curve instead of a staircase. Thresholded slightly
				// below half so a one-cell river keeps its width rather than thinning.
				if wat != worldgen.Ocean {
					if wet := f.wetAt(wx, wy); wet > 0.42 {
						wat = f.wetKindAt(wx, wy)
					} else if wat == worldgen.Lake || wat == worldgen.River {
						wat = worldgen.Dry
					}
				}
			}

			col := shade(w, i, l, maxLogFlow, elev, wat)
			if l == Terrain {
				col = hillshade(f, wx, wy, wat == worldgen.Ocean, col)
			}
			img.SetRGBA(px, py, col)
		}
	}
	return img
}

// hillshade lights the terrain from the north-west so relief is readable.
//
// Flat colour ramps make a height map hard to read: without shading, a gentle basin and
// a steep escarpment at the same altitude look identical. The gradient is measured on the
// interpolated field with wrapped sampling, so lighting stays smooth when zoomed in and
// continuous across the seam like everything else.
func hillshade(f field, x, y float64, isOcean bool, base color.RGBA) color.RGBA {
	if isOcean {
		return base
	}
	// Sample at whole-cell spacing regardless of detail, so the apparent relief does not
	// change as the render resolution changes.
	const h = 1.0
	d := (f.elevAt(x-h, y) - f.elevAt(x+h, y)) + (f.elevAt(x, y-h) - f.elevAt(x, y+h))
	shade := math.Max(-1, math.Min(1, d*14))

	if shade >= 0 {
		return mix(base, color.RGBA{255, 255, 255, 255}, shade*0.35)
	}
	return mix(base, color.RGBA{0, 0, 0, 255}, -shade*0.40)
}

func shade(w *worldgen.World, i int, l Layer, maxLogFlow, elev float64, wat worldgen.Water) color.RGBA {

	switch l {
	case Elevation:
		v := uint8(math.Max(0, math.Min(1, elev)) * 255)
		return color.RGBA{v, v, v, 255}

	case WaterOnly:
		switch wat {
		case worldgen.Ocean:
			return deepOcean
		case worldgen.Lake:
			return lakeColor
		case worldgen.River:
			return riverColor
		}
		return color.RGBA{28, 28, 30, 255}

	case Flow:
		// Log scale: drainage area spans several orders of magnitude, so a linear ramp
		// would show only the trunk rivers.
		t := math.Log1p(float64(w.FlowAcc[i])) / maxLogFlow
		if wat == worldgen.Ocean {
			return color.RGBA{10, 12, 20, 255}
		}
		return mix(color.RGBA{18, 22, 30, 255}, color.RGBA{120, 200, 255, 255}, t)

	case Temperature:
		t := float64(w.Temperature[i])
		c := mix(color.RGBA{40, 70, 170, 255}, color.RGBA{210, 70, 40, 255}, t)
		if wat == worldgen.Ocean {
			return mix(c, deepOcean, 0.55)
		}
		return c
	}

	// Terrain: a composite that should read as a map at a glance.
	switch wat {
	case worldgen.Ocean:
		// Shade by depth below sea level so continental shelves are visible.
		d := (w.SeaLevel - elev) / math.Max(w.SeaLevel, 1e-6)
		return mix(shallowOcean, deepOcean, d)
	case worldgen.Lake:
		return lakeColor
	case worldgen.River:
		switch w.RiverSize[i] {
		case worldgen.MajorRiver:
			return bigRiver
		case worldgen.SmallRiver:
			return riverColor
		default:
			return mix(riverColor, lowland, 0.25)
		}
	}

	// Land, ramped by height above sea level and cooled toward snow where cold.
	h := (elev - w.SeaLevel) / math.Max(1-w.SeaLevel, 1e-6)
	var c color.RGBA
	switch {
	case h < 0.04:
		c = mix(beach, lowland, h/0.04)
	case h < 0.45:
		c = mix(lowland, upland, (h-0.04)/0.41)
	case h < 0.75:
		c = mix(upland, rock, (h-0.45)/0.30)
	default:
		c = mix(rock, snow, (h-0.75)/0.25)
	}
	// Cold ground tends toward snow, but only near the cold belt: blending too eagerly
	// whites out whole continents and hides the terrain underneath.
	if t := float64(w.Temperature[i]); t < 0.16 {
		c = mix(c, snow, (0.16-t)/0.16*0.75)
	}
	return c
}
