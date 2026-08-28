package main

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/solifugus/serts/internal/sim"
	"github.com/solifugus/serts/internal/torus"
)

// Little figures, and the machinery to animate them.
//
// People were single coloured squares, which is fine at a hundred paces and useless at
// the scale the camera can now reach. This draws them as small humanoid figures that
// face the way they are walking and stride as they go.
//
// The art here is deliberately placeholder — procedurally drawn stick figures, not
// craft. What is NOT placeholder is the machinery around it: a sprite sheet indexed by
// facing and frame, a walk cycle driven by the simulation clock, and per-activity poses.
// Real art drops into figureSheet without anything else changing, because nothing outside
// this file knows how a person is drawn.
//
// Everything is generated once at startup. Drawing a few hundred people per frame must
// not allocate, so the sheet is built up front and only ever blitted.

const (
	// figurePx is the pixel size of one figure cell in the sheet. Small on purpose: these
	// are people seen from a distance, and a bigger canvas would invite detail the
	// placeholder cannot honestly carry.
	figurePx = 8
	// figureFrames is the length of the walk cycle.
	figureFrames = 4
	// figureFacings is how many directions a figure can face: N, E, S, W. Four rather
	// than eight because a stick figure cannot express a diagonal, and pretending
	// otherwise would just add noise.
	figureFacings = 4
)

// Facings, in sheet order.
const (
	faceS = iota
	faceW
	faceE
	faceN
)

// figureSheet holds every pose: [facing][frame].
type figureSheet struct {
	img *ebiten.Image
	// cell is where each pose sits in the image.
	cell [figureFacings][figureFrames]image.Rectangle
}

// newFigureSheet draws the placeholder figures once.
//
// A figure is a head, a body, two legs and — when walking — arms that swing opposite the
// legs. It is drawn in white so the draw call can tint it to anything: activity, faction,
// settlement. Tinting a white sprite is how one sheet serves every colour scheme without
// a second copy in memory.
func newFigureSheet() *figureSheet {
	s := &figureSheet{}
	w := figurePx * figureFrames
	h := figurePx * figureFacings
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))

	set := func(x, y int, a uint8) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		rgba.SetRGBA(x, y, color.RGBA{255, 255, 255, a})
	}

	for face := 0; face < figureFacings; face++ {
		for frame := 0; frame < figureFrames; frame++ {
			ox, oy := frame*figurePx, face*figurePx
			s.cell[face][frame] = image.Rect(ox, oy, ox+figurePx, oy+figurePx)

			// The stride: legs apart on frames 1 and 3, together on 0 and 2, so the cycle
			// reads as left-step, pass, right-step, pass.
			stride := 0
			switch frame {
			case 1:
				stride = 1
			case 3:
				stride = -1
			}

			cx := figurePx / 2
			// Head.
			set(ox+cx, oy+1, 255)
			set(ox+cx-1, oy+1, 200)
			// Body.
			for y := 2; y <= 4; y++ {
				set(ox+cx, oy+y, 255)
				set(ox+cx-1, oy+y, 220)
			}
			// Legs, parting with the stride.
			set(ox+cx-1-abs(stride), oy+5, 235)
			set(ox+cx+abs(stride), oy+5, 235)
			set(ox+cx-1-abs(stride), oy+6, 200)
			set(ox+cx+abs(stride), oy+6, 200)

			// Arms swing opposite the legs, and only when striding — a figure standing
			// still should look still.
			if stride != 0 {
				set(ox+cx-2, oy+3+clampInt(-stride, 0, 1), 190)
				set(ox+cx+1, oy+3+clampInt(stride, 0, 1), 190)
			}
			// A figure seen from behind loses its face; one seen from the front keeps a
			// suggestion of it. At this size that is the only cue that direction exists.
			if face == faceS {
				set(ox+cx-1, oy+1, 120)
			}
			if face == faceW {
				set(ox+cx-1, oy+2, 255)
			}
			if face == faceE {
				set(ox+cx, oy+2, 255)
			}
		}
	}
	s.img = ebiten.NewImageFromImage(rgba)
	return s
}

// facingOf turns a movement vector into one of the four facings. A person who is not
// moving keeps facing south, toward the viewer, which reads as "idle" rather than
// "frozen mid-stride".
func facingOf(d torus.Vec2) int {
	if math.Abs(d.X) < 1e-9 && math.Abs(d.Y) < 1e-9 {
		return faceS
	}
	if math.Abs(d.X) > math.Abs(d.Y) {
		if d.X > 0 {
			return faceE
		}
		return faceW
	}
	if d.Y > 0 {
		return faceS
	}
	return faceN
}

// frameOf picks the walk frame from the simulation clock and the character's identity.
//
// Keyed to the tick rather than to real time so that pausing freezes the stride, and
// offset by the character's ID so that a crowd does not march in lockstep — which is the
// single thing that most makes a crowd look mechanical.
func frameOf(tick sim.Tick, id int, moving bool) int {
	if !moving {
		return 0
	}
	const ticksPerFrame = 12
	return int((int64(tick)/ticksPerFrame + int64(id))) % figureFrames
}

// draw blits one figure, tinted, centred on a screen position and scaled to the zoom.
func (s *figureSheet) draw(dst *ebiten.Image, x, y, size float64, facing, frame int, col color.RGBA) {
	if size < 2 {
		// Below a couple of pixels a figure is a dot whatever is drawn, and blitting a
		// sprite for it is wasted work.
		return
	}
	if x < -size || y < -size || x > float64(dst.Bounds().Dx())+size || y > float64(dst.Bounds().Dy())+size {
		return
	}
	scale := size / figurePx
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	// Feet at the position, not the centre: a person stands ON the ground they occupy.
	op.GeoM.Translate(x-size/2, y-size*0.85)
	op.ColorScale.ScaleWithColor(col)
	dst.DrawImage(s.img.SubImage(s.cell[facing][frame]).(*ebiten.Image), op)
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
