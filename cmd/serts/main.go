// Command serts is the world viewer.
//
// Milestone 1 has no simulation in it yet: this shows a generated world and lets you
// move around it. That is deliberate. The camera scrolls forever in every direction and
// never clamps, because the world has no edges (§8.5), and panning across a boundary is
// the fastest way to catch a wrapping bug — a seam is invisible in a static screenshot
// and unmistakable when you walk over it.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/solifugus/serts/internal/render"
	"github.com/solifugus/serts/internal/torus"
	"github.com/solifugus/serts/internal/worldgen"
)

const (
	windowW, windowH = 1280, 800
	minZoom, maxZoom = 0.5, 16.0
	panSpeed         = 420.0 // world units per second at zoom 1
)

type viewer struct {
	params worldgen.Params
	world  *worldgen.World
	layer  render.Layer

	// tiles holds one pre-rendered image per layer, one pixel per cell. Because the
	// world is a torus, drawing it wrapped is just a matter of repeating this image.
	tiles   [render.NumLayers]*ebiten.Image
	genTime time.Duration

	cam       torus.Vec2 // world position at the centre of the screen
	zoom      float64    // screen pixels per world unit
	showHelp  bool
	lastFrame time.Time
}

func newViewer(p worldgen.Params) *viewer {
	v := &viewer{params: p, zoom: 3, showHelp: true, lastFrame: time.Now()}
	v.regenerate(p.Seed)
	v.cam = torus.Vec2{X: v.world.T.W / 2, Y: v.world.T.H / 2}
	return v
}

func (v *viewer) regenerate(seed int64) {
	v.params.Seed = seed
	start := time.Now()
	v.world = worldgen.Generate(v.params)
	v.genTime = time.Since(start)
	for l := render.Layer(0); int(l) < render.NumLayers; l++ {
		v.tiles[l] = ebiten.NewImageFromImage(render.Image(v.world, l))
	}
	fmt.Printf("seed %d: %s (%v)\n", seed, v.world.Stats(), v.genTime.Round(time.Millisecond))
}

func (v *viewer) Update() error {
	now := time.Now()
	dt := now.Sub(v.lastFrame).Seconds()
	v.lastFrame = now
	if dt > 0.1 {
		dt = 0.1 // do not lurch after a stall
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyH) {
		v.showHelp = !v.showHelp
	}

	// Layer selection: number keys pick directly, Tab cycles.
	for i := 0; i < render.NumLayers; i++ {
		if inpututil.IsKeyJustPressed(ebiten.Key1 + ebiten.Key(i)) {
			v.layer = render.Layer(i)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		v.layer = render.Layer((int(v.layer) + 1) % render.NumLayers)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		v.regenerate(v.params.Seed + 1)
	}

	// Panning. The camera is a world position, so wrapping it is all that is needed for
	// scrolling to continue forever in any direction.
	var dx, dy float64
	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		dx -= 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		dx += 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		dy -= 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		dy += 1
	}
	if dx != 0 || dy != 0 {
		// Normalise so diagonal panning is not faster, and scale so the apparent speed
		// on screen is the same at every zoom level.
		l := math.Hypot(dx, dy)
		step := panSpeed * dt / v.zoom
		v.cam = v.world.T.Add(v.cam, torus.Vec2{X: dx / l * step, Y: dy / l * step})
	}

	// Zoom, centred on the screen.
	zoomStep := 0.0
	if _, wy := ebiten.Wheel(); wy != 0 {
		zoomStep = wy * 0.15
	}
	if ebiten.IsKeyPressed(ebiten.KeyEqual) || ebiten.IsKeyPressed(ebiten.KeyKPAdd) {
		zoomStep += 2.0 * dt
	}
	if ebiten.IsKeyPressed(ebiten.KeyMinus) || ebiten.IsKeyPressed(ebiten.KeyKPSubtract) {
		zoomStep -= 2.0 * dt
	}
	if zoomStep != 0 {
		v.zoom = math.Min(maxZoom, math.Max(minZoom, v.zoom*math.Exp(zoomStep)))
	}
	return nil
}

func (v *viewer) Draw(screen *ebiten.Image) {
	t := v.world.T
	tile := v.tiles[v.layer]
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Position the world so that v.cam sits at the centre of the screen.
	originX := float64(sw)/2 - v.cam.X*v.zoom
	originY := float64(sh)/2 - v.cam.Y*v.zoom
	tileW, tileH := t.W*v.zoom, t.H*v.zoom

	// Draw the world repeatedly to cover the viewport. This is what a torus looks like:
	// there is no edge to stop at, so copies simply continue in every direction (§9.3).
	// The modulo places the first copy just off-screen top-left.
	startX := originX - tileW*math.Ceil(originX/tileW)
	startY := originY - tileH*math.Ceil(originY/tileH)

	for y := startY; y < float64(sh); y += tileH {
		for x := startX; x < float64(sw); x += tileW {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(v.zoom, v.zoom)
			op.GeoM.Translate(x, y)
			// Nearest-neighbour keeps cell boundaries crisp when zoomed in, which
			// matters for reading terrain the simulation treats as discrete.
			op.Filter = ebiten.FilterNearest
			screen.DrawImage(tile, op)
		}
	}

	v.drawHUD(screen)
}

func (v *viewer) drawHUD(screen *ebiten.Image) {
	t := v.world.T
	cell := t.CellAt(v.cam)
	i := t.Index(cell)

	kind := "land"
	switch v.world.Water[i] {
	case worldgen.Ocean:
		kind = "ocean"
	case worldgen.Lake:
		kind = "lake"
	case worldgen.River:
		switch v.world.RiverSize[i] {
		case worldgen.MajorRiver:
			kind = "major river"
		case worldgen.SmallRiver:
			kind = "river"
		default:
			kind = "stream"
		}
	}

	s := v.world.Stats()
	msg := fmt.Sprintf(
		"SERTS world viewer  |  seed %d  %dx%d  |  %.1f fps\n"+
			"layer: %s  (1-%d or Tab)   zoom %.1fx\n"+
			"centre: cell %d,%d — %s   elev %.3f   flow %.0f   temp %.2f\n"+
			"%s\n"+
			"generated in %v",
		v.params.Seed, t.CX, t.CY, ebiten.ActualFPS(),
		v.layer.Name(), render.NumLayers, v.zoom,
		cell.X, cell.Y, kind, v.world.Elevation[i], v.world.FlowAcc[i], v.world.Temperature[i],
		s, v.genTime.Round(time.Millisecond),
	)
	if v.showHelp {
		msg += "\n\nWASD/arrows pan (no edges — keep going)   +/- or wheel zoom\n" +
			"R new seed   Tab next layer   H hide help   Q quit"
	}
	ebitenutil.DebugPrint(screen, msg)
}

func (v *viewer) Layout(_, _ int) (int, int) { return windowW, windowH }

// LayoutF keeps the viewer crisp on scaled displays.
func (v *viewer) LayoutF(outsideWidth, outsideHeight float64) (float64, float64) {
	return outsideWidth, outsideHeight
}

func main() {
	var (
		seed       = flag.Int64("seed", 1, "world seed")
		size       = flag.Int("size", 256, "world size in cells (square)")
		land       = flag.Float64("land", 0.60, "fraction of the world above sea level")
		screenshot = flag.String("screenshot", "", "render one frame to this PNG and exit")
	)
	flag.Parse()

	p := worldgen.DefaultParams(*seed)
	p.CX, p.CY = *size, *size
	p.LandFraction = *land

	v := newViewer(p)

	if *screenshot != "" {
		if err := shoot(v, *screenshot); err != nil {
			log.Fatal(err)
		}
		return
	}

	ebiten.SetWindowSize(windowW, windowH)
	ebiten.SetWindowTitle("SERTS")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(v); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
}

// shoot renders a single frame and writes it to disk, so the viewer can be verified
// from a script rather than only by eye.
//
// Ebitengine has no offscreen mode: drawing needs a real graphics context, so this still
// opens a window, runs the loop for two frames, and captures the second.
func shoot(v *viewer, path string) error {
	c := &capturer{viewer: v, path: path}
	ebiten.SetWindowSize(windowW, windowH)
	ebiten.SetWindowTitle("SERTS (screenshot)")
	if err := ebiten.RunGame(c); err != nil && err != ebiten.Termination {
		return err
	}
	return c.err
}

// capturer wraps the viewer, saves the first drawn frame, and then exits.
type capturer struct {
	viewer *viewer
	path   string
	frames int
	err    error
}

func (c *capturer) Update() error {
	c.frames++
	if c.frames > 2 {
		return ebiten.Termination
	}
	return nil
}

func (c *capturer) Draw(screen *ebiten.Image) {
	c.viewer.Draw(screen)
	if c.frames == 2 && c.err == nil {
		c.err = savePNG(screen, c.path)
	}
}

func (c *capturer) Layout(_, _ int) (int, int) { return windowW, windowH }

func savePNG(img *ebiten.Image, path string) error {
	b := img.Bounds()
	out := image.NewRGBA(b)
	pix := make([]byte, 4*b.Dx()*b.Dy())
	img.ReadPixels(pix)
	copy(out.Pix, pix)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, out)
}
