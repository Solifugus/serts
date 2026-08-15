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

// Image renders a whole world as one pixel per cell.
//
// One pixel per cell keeps this trivially tileable, which is what lets the viewer draw a
// wrapped world by simply repeating the image (§9.3).
func Image(w *worldgen.World, l Layer) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w.T.CX, w.T.CY))
	// Rivers are scaled against the largest flow so the drainage layer reads on any map.
	maxLogFlow := math.Log1p(float64(w.Stats().MaxFlow))
	if maxLogFlow == 0 {
		maxLogFlow = 1
	}

	for y := 0; y < w.T.CY; y++ {
		for x := 0; x < w.T.CX; x++ {
			c := torus.Cell{X: x, Y: y}
			i := w.T.Index(c)
			col := shade(w, i, l, maxLogFlow)
			if l == Terrain {
				col = hillshade(w, c, i, col)
			}
			img.SetRGBA(x, y, col)
		}
	}
	return img
}

// hillshade lights the terrain from the north-west so relief is readable.
//
// Flat colour ramps make a height map hard to read: without shading, a gentle basin and
// a steep escarpment at the same altitude look identical. The gradient is measured with
// wrapped neighbours, so the lighting is continuous across the seam like everything else.
func hillshade(w *worldgen.World, c torus.Cell, i int, base color.RGBA) color.RGBA {
	if w.Water[i] == worldgen.Ocean {
		return base
	}
	e := func(dx, dy int) float64 {
		return float64(w.Elevation[w.T.Index(torus.Cell{X: c.X + dx, Y: c.Y + dy})])
	}
	// Slope toward the light, which sits to the north-west.
	d := (e(-1, 0) - e(1, 0)) + (e(0, -1) - e(0, 1))
	shade := math.Max(-1, math.Min(1, d*14))

	if shade >= 0 {
		return mix(base, color.RGBA{255, 255, 255, 255}, shade*0.35)
	}
	return mix(base, color.RGBA{0, 0, 0, 255}, -shade*0.40)
}

func shade(w *worldgen.World, i int, l Layer, maxLogFlow float64) color.RGBA {
	elev := float64(w.Elevation[i])
	wat := w.Water[i]

	switch l {
	case Elevation:
		v := uint8(elev * 255)
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
