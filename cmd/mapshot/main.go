// Command mapshot generates a world and writes it to a PNG.
//
// It exists so terrain can be inspected without a display, which makes it the tool of
// choice for checking generation changes and for spotting a seam: rendering the world
// two-by-two puts every boundary in the middle of the image, where a tear is obvious.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"time"

	"github.com/solifugus/serts/internal/render"
	"github.com/solifugus/serts/internal/worldgen"
)

func main() {
	var (
		seed  = flag.Int64("seed", 1, "world seed")
		size  = flag.Int("size", 256, "world size in cells (square)")
		land  = flag.Float64("land", 0.60, "fraction of the world above sea level")
		layer = flag.String("layer", "terrain", "terrain|elevation|water|drainage|temperature")
		tile  = flag.Bool("tile", false, "draw the world 2x2 so the seams fall mid-image")
		scale = flag.Int("scale", 2, "pixels per cell")
		out   = flag.String("o", "world.png", "output file")
	)
	flag.Parse()

	l, err := parseLayer(*layer)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	p := worldgen.DefaultParams(*seed)
	p.CX, p.CY = *size, *size
	p.LandFraction = *land

	start := time.Now()
	w := worldgen.Generate(p)
	genTime := time.Since(start)

	img := render.Image(w, l)
	if *tile {
		img = tile2x2(img)
	}
	if *scale > 1 {
		img = upscale(img, *scale)
	}

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create:", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}

	fmt.Printf("seed %d, %dx%d\n", *seed, *size, *size)
	fmt.Printf("%s\n", w.Stats())
	fmt.Printf("generated in %v, wrote %s (%s layer)\n", genTime.Round(time.Millisecond), *out, l.Name())
}

func parseLayer(s string) (render.Layer, error) {
	for l := render.Layer(0); int(l) < render.NumLayers; l++ {
		if l.Name() == s {
			return l, nil
		}
	}
	return 0, fmt.Errorf("unknown layer %q", s)
}

// tile2x2 repeats the world four times. Because the world is a torus, the result must be
// continuous everywhere: any visible grid line in the output is a wrapping bug.
func tile2x2(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w*2, h*2))
	for ty := 0; ty < 2; ty++ {
		for tx := 0; tx < 2; tx++ {
			r := image.Rect(tx*w, ty*h, (tx+1)*w, (ty+1)*h)
			draw.Draw(dst, r, src, b.Min, draw.Src)
		}
	}
	return dst
}

func upscale(src *image.RGBA, n int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*n, b.Dy()*n))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := src.RGBAAt(b.Min.X+x, b.Min.Y+y)
			for dy := 0; dy < n; dy++ {
				for dx := 0; dx < n; dx++ {
					dst.SetRGBA(x*n+dx, y*n+dy, c)
				}
			}
		}
	}
	return dst
}
