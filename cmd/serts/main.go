// Command serts is the world viewer and, now, the game.
//
// The camera scrolls forever in every direction and never clamps, because the world has
// no edges (§8.5); panning across a boundary is still the fastest way to catch a wrapping
// bug, since a seam is invisible in a screenshot and unmistakable when you walk over it.
//
// The village on top of it runs the simulation from internal/sim: people look for work,
// walk to it, earn, buy food, age, and die.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/solifugus/serts/internal/render"
	"github.com/solifugus/serts/internal/sim"
	"github.com/solifugus/serts/internal/torus"
	"github.com/solifugus/serts/internal/worldgen"
)

const (
	windowW, windowH = 1280, 800
	minZoom, maxZoom = 0.5, 16.0
	panSpeed         = 420.0 // world units per second at zoom 1
	// terrainDetail is how many pixels the terrain tile carries per cell. Elevation is a
	// continuous field underneath the grid, so reconstructing it between cell centres
	// keeps hills smooth when zoomed in without pretending the simulation is finer.
	terrainDetail = 4
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

	sim *sim.State
	// speed is how many simulation ticks are consumed per frame. It changes only how
	// fast time is spent, never what a tick does, so a century at 5000x is identical to
	// the same century watched live (§2.10).
	speed  int
	paused bool
	dot    *ebiten.Image // reused 1x1 sprite for drawing people

	sel       selection
	showMkt   bool
	follow    bool
	buildType sim.StructType
	building  bool
	notice    string
	noticeAt  time.Time
}

// buildable lists what the player may order put up, in the order the keys select them.
var buildable = []sim.StructType{
	sim.Home, sim.Farm, sim.Granary, sim.DiningHall,
	sim.LumberCamp, sim.Quarry, sim.Mine,
	sim.Storehouse, sim.Workshop, sim.Store,
}

func newViewer(p worldgen.Params) *viewer {
	v := &viewer{params: p, zoom: 6, showHelp: true, lastFrame: time.Now(), speed: 8}
	v.dot = ebiten.NewImage(1, 1)
	v.dot.Fill(color.White)
	v.regenerate(p.Seed)
	return v
}

func (v *viewer) regenerate(seed int64) {
	v.params.Seed = seed
	start := time.Now()
	v.world = worldgen.Generate(v.params)
	v.genTime = time.Since(start)
	for l := render.Layer(0); int(l) < render.NumLayers; l++ {
		v.tiles[l] = ebiten.NewImageFromImage(render.Image(v.world, l, terrainDetail))
	}
	v.sim = sim.New(sim.DefaultConfig(v.world, seed))
	// Open on the village rather than on an arbitrary corner of the map.
	if len(v.sim.Structs) > 0 {
		v.cam = v.sim.Structs[0].Pos
	} else {
		v.cam = torus.Vec2{X: v.world.T.W / 2, Y: v.world.T.H / 2}
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
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		v.showMkt = !v.showMkt
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		v.follow = !v.follow
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyB) {
		v.building = !v.building
	}
	if v.building {
		for i := range buildable {
			if i < 9 && inpututil.IsKeyJustPressed(ebiten.KeyF1+ebiten.Key(i)) {
				v.buildType = buildable[i]
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyLeftBracket) {
			v.cycleBuild(-1)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyRightBracket) {
			v.cycleBuild(1)
		}
	}
	v.handleClicks()

	// Simulation controls.
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		v.paused = !v.paused
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPeriod) {
		v.speed = minInt(v.speed*2, 4096)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyComma) {
		v.speed = maxInt(v.speed/2, 1)
	}
	if !v.paused {
		v.sim.RunTicks(v.speed)
	}

	// Follow the selected person, so a life can be watched rather than hunted for.
	if v.follow && v.sel.kind == selChar && v.sim.AliveChar(v.sel.char) {
		v.cam = v.sim.Chars[v.sel.char].Pos
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

// cycleBuild steps through the buildable types.
func (v *viewer) cycleBuild(d int) {
	for i, t := range buildable {
		if t == v.buildType {
			v.buildType = buildable[(i+d+len(buildable))%len(buildable)]
			return
		}
	}
	v.buildType = buildable[0]
}

// handleClicks selects what was clicked, or orders a building if in build mode.
func (v *viewer) handleClicks() {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	mx, my := ebiten.CursorPosition()
	p := v.screenToWorld(mx, my)

	if v.building {
		v.order(p)
		return
	}
	v.sel = v.pick(p)
	if v.sel.kind == selNone {
		v.follow = false
	}
}

// order places a construction site, if the ground will take it.
//
// The refusal messages matter as much as the placement. A build order that silently does
// nothing is indistinguishable from a bug, and the reasons a site is rejected — water,
// a river, no fresh water within reach of a farm — are exactly the terrain rules the
// player needs to learn.
func (v *viewer) order(p torus.Vec2) {
	c := v.world.T.CellAt(p)
	t := v.buildType

	switch {
	case !v.world.Walkable(v.world.T.Index(c)):
		v.say("cannot build on water")
	case !sim.CanPlace(v.world, t, c):
		if sim.Defs[t].NeedsFreshWater {
			v.say(fmt.Sprintf("a %v needs fresh water within %d cells",
				t, sim.Defs[t].MaxFreshDist))
		} else {
			v.say(fmt.Sprintf("cannot put a %v there", t))
		}
	case v.sim.Occupied(c):
		v.say("something is already there")
	default:
		id := v.sim.Build(t, c)
		v.sel = selection{kind: selStruct, str: id}
		cost := sim.Defs[t].BuildCost
		v.say(fmt.Sprintf("%v ordered — needs %.0f wood, %.0f stone",
			t, cost[sim.Wood], cost[sim.Stone]))
	}
}

func (v *viewer) say(msg string) {
	v.notice = msg
	v.noticeAt = time.Now()
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
			op.GeoM.Scale(v.zoom/terrainDetail, v.zoom/terrainDetail)
			op.GeoM.Translate(x, y)
			// Nearest-neighbour keeps cell boundaries crisp when zoomed in, which
			// matters for reading terrain the simulation treats as discrete.
			op.Filter = ebiten.FilterNearest
			screen.DrawImage(tile, op)
		}
	}

	v.drawVillage(screen, originX, originY)
	v.drawHUD(screen)
}

// drawVillage draws structures and people over the terrain.
//
// Everything is placed through the torus, and drawn once per visible copy of the world,
// so a village near the seam appears on both sides exactly as the terrain does.
func (v *viewer) drawVillage(screen *ebiten.Image, originX, originY float64) {
	t := v.world.T
	sw, sh := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())
	tileW, tileH := t.W*v.zoom, t.H*v.zoom
	startX := originX - tileW*math.Ceil(originX/tileW)
	startY := originY - tileH*math.Ceil(originY/tileH)

	for oy := startY; oy < sh; oy += tileH {
		for ox := startX; ox < sw; ox += tileW {
			for i := range v.sim.Structs {
				st := &v.sim.Structs[i]
				if st.Alive {
					v.drawBox(screen, ox+st.Pos.X*v.zoom, oy+st.Pos.Y*v.zoom,
						structSize(st.Type)*v.zoom, structColor(st.Type))
				}
			}
			for i := range v.sim.Chars {
				c := &v.sim.Chars[i]
				if c.Alive {
					v.drawBox(screen, ox+c.Pos.X*v.zoom, oy+c.Pos.Y*v.zoom,
						math.Max(2, 0.45*v.zoom), charColor(c))
				}
			}
			// Ring whatever is selected, so it can be found again in a crowd.
			if p, ok := v.selPos(); ok {
				v.drawRing(screen, ox+p.X*v.zoom, oy+p.Y*v.zoom, math.Max(6, 1.6*v.zoom))
			}
		}
	}
}

// selPos returns where the current selection is, if anything is selected.
func (v *viewer) selPos() (torus.Vec2, bool) {
	switch v.sel.kind {
	case selChar:
		if v.sim.AliveChar(v.sel.char) {
			return v.sim.Chars[v.sel.char].Pos, true
		}
	case selStruct:
		if int(v.sel.str) < len(v.sim.Structs) && v.sim.Structs[v.sel.str].Alive {
			return v.sim.Structs[v.sel.str].Pos, true
		}
	}
	return torus.Vec2{}, false
}

// drawRing marks a selection with four corner ticks rather than a filled shape, so it
// frames what is selected instead of hiding it.
func (v *viewer) drawRing(screen *ebiten.Image, x, y, size float64) {
	col := color.RGBA{255, 240, 120, 255}
	h := size / 2
	const t = 2.0
	for _, c := range [][2]float64{{-h, -h}, {h, -h}, {-h, h}, {h, h}} {
		v.drawBox(screen, x+c[0], y+c[1], t*2, col)
	}
}

func (v *viewer) drawBox(screen *ebiten.Image, x, y, size float64, col color.RGBA) {
	if size < 1 {
		size = 1
	}
	// Cheap rejection: most of the world is off-screen most of the time.
	if x < -size || y < -size || x > float64(screen.Bounds().Dx())+size || y > float64(screen.Bounds().Dy())+size {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(size, size)
	op.GeoM.Translate(x-size/2, y-size/2)
	op.ColorScale.ScaleWithColor(col)
	screen.DrawImage(v.dot, op)
}

func structSize(t sim.StructType) float64 {
	switch t {
	case sim.Granary, sim.Storehouse:
		return 2.4
	case sim.Farm, sim.Workshop, sim.Mine:
		return 2.0
	default:
		return 1.6
	}
}

func structColor(t sim.StructType) color.RGBA {
	switch t {
	case sim.Home:
		return color.RGBA{210, 170, 110, 255}
	case sim.Farm:
		return color.RGBA{225, 205, 90, 255}
	case sim.Granary:
		return color.RGBA{235, 130, 60, 255}
	case sim.DiningHall:
		return color.RGBA{240, 160, 90, 255}
	case sim.LumberCamp:
		return color.RGBA{110, 160, 80, 255}
	case sim.Quarry:
		return color.RGBA{170, 170, 175, 255}
	case sim.Mine:
		return color.RGBA{130, 110, 150, 255}
	case sim.Storehouse:
		return color.RGBA{160, 130, 95, 255}
	case sim.Workshop:
		return color.RGBA{190, 110, 100, 255}
	case sim.Store:
		return color.RGBA{200, 150, 190, 255}
	case sim.BuildSite:
		return color.RGBA{250, 250, 210, 255}
	}
	return color.RGBA{255, 255, 255, 255}
}

// charColor shows at a glance what people are doing, which is the whole point of
// watching a village rather than reading its statistics.
func charColor(c *sim.Character) color.RGBA {
	switch {
	case c.Stage() == sim.Child:
		return color.RGBA{255, 235, 160, 255}
	case c.Hunger > 70:
		return color.RGBA{255, 80, 80, 255} // hungry
	case c.Job == sim.NoStruct:
		return color.RGBA{170, 170, 180, 255} // out of work
	case c.Activity == sim.Working:
		return color.RGBA{120, 255, 140, 255}
	}
	return color.RGBA{235, 235, 255, 255}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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

	st := v.sim.Stats()
	state := fmt.Sprintf("%dx", v.speed)
	if v.paused {
		state = "PAUSED"
	}
	msg := fmt.Sprintf(
		"SERTS  |  seed %d  %dx%d  |  %.0f fps  |  %s  |  %s\n"+
			"pop %d (%d children, %d adults, %d elders)   %d employed, %.0f%% out of work\n"+
			"food %.0f   avg hunger %.0f   avg health %.0f   avg age %.0f\n"+
			"born %d   died %d of age, %d of hunger\n"+
			"under cursor: cell %d,%d — %s  soil %.2f  elev %.3f\n"+
			"layer: %s",
		v.params.Seed, t.CX, t.CY, ebiten.ActualFPS(), state, st.Date,
		st.Population, st.Children, st.Adults, st.Elders, st.Employed, st.Unemployment*100,
		st.Food, st.AvgHunger, st.AvgHealth, st.AvgAge,
		st.Births, st.DeathsAge, st.DeathsStarve,
		cell.X, cell.Y, kind, v.world.Soil[i], v.world.Elevation[i],
		v.layer.Name(),
	)
	if v.building {
		msg += fmt.Sprintf("\n\nBUILD MODE — click to site a %v   [ ] to change   B to stop", v.buildType)
	}
	if v.notice != "" && time.Since(v.noticeAt) < 6*time.Second {
		msg += "\n" + v.notice
	}
	if v.showHelp {
		msg += "\n\nWASD/arrows pan (no edges — keep going)   +/- or wheel zoom\n" +
			"click a person or building to inspect   F follow   B build   M market\n" +
			"space pause   , / . slower / faster   Tab layer   R new world   H help   Q quit\n" +
			"green working   grey out of work   red hungry   pale yellow children"
	}
	ebitenutil.DebugPrint(screen, msg)

	// Inspector, bottom left: everything about whatever is selected.
	if txt := v.inspect(); txt != "" {
		ebitenutil.DebugPrintAt(screen, txt, 8, windowH-150)
	}
	// Market, right side.
	if v.showMkt {
		ebitenutil.DebugPrintAt(screen, v.market(), windowW-330, 8)
	}
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
	// Give the screenshot something to show: run a little, select somebody, open the
	// market panel.
	if *screenshot != "" {
		v.sim.RunTicks(60000)
		v.showMkt = true
		for i := range v.sim.Chars {
			if v.sim.Chars[i].Alive && v.sim.Chars[i].Job != sim.NoStruct {
				v.sel = selection{kind: selChar, char: sim.CharID(i)}
				v.cam = v.sim.Chars[i].Pos
				break
			}
		}
	}

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
