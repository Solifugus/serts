// Command simrun runs the simulation headlessly and reports what happened.
//
// This is the tuning instrument. Demography is the most failure-prone part of the design
// (§3.2) and it cannot be tuned at wall-clock rate — a single generation takes days. Here
// a century runs in seconds, and because the tick is deterministic (§9.2) the same seed
// replays exactly, so changing one constant and re-running is a controlled experiment
// rather than a guess.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/solifugus/serts/internal/sim"
	"github.com/solifugus/serts/internal/worldgen"
)

func main() {
	var (
		seed     = flag.Int64("seed", 1, "world seed")
		size     = flag.Int("size", 256, "world size in cells")
		years    = flag.Float64("years", 20, "in-world years to simulate")
		every    = flag.Float64("report", 1, "report interval in in-world years")
		days     = flag.Float64("reportdays", 0, "report interval in days; overrides -report")
		settlers = flag.Int("settlers", 34, "founding population")
		farms    = flag.Int("farms", 3, "farms to build")
		homes    = flag.Int("homes", 8, "homes to build")
		dump     = flag.Bool("dump", false, "dump structure detail at each report")
		industry = flag.Bool("industry", true, "found the material economy as well as farms")
	)
	flag.Parse()

	w := worldgen.Generate(func() worldgen.Params {
		p := worldgen.DefaultParams(*seed)
		p.CX, p.CY = *size, *size
		return p
	}())

	cfg := sim.DefaultConfig(w, *seed)
	cfg.Settlers, cfg.Farms, cfg.Homes = *settlers, *farms, *homes
	cfg.Industry = *industry
	s := sim.New(cfg)

	fmt.Printf("world seed %d (%dx%d)\n", *seed, *size, *size)
	fmt.Printf("founded: %d settlers | built %d homes, %d farms, %d granaries\n\n",
		*settlers, s.BuiltHomes, s.BuiltFarms, s.BuiltGranaries)

	totalTicks := int(*years * sim.TicksPerYear)
	reportEvery := int(*every * sim.TicksPerYear)
	if *days > 0 {
		reportEvery = int(*days * sim.TicksPerDay)
	}
	if reportEvery < 1 {
		reportEvery = 1
	}

	start := time.Now()
	for t := 0; t < totalTicks; t++ {
		s.Step()
		if (t+1)%reportEvery == 0 {
			fmt.Println(s.Stats())
			fmt.Println(s.DumpMarket())
			if *dump {
				fmt.Print(s.DumpStructures())
			}
		}
		if s.Population() == 0 {
			fmt.Println(s.Stats())
			fmt.Printf("\nEXTINCT after %.2f in-world years\n", s.Tick.Years())
			os.Exit(1)
		}
	}
	elapsed := time.Since(start)

	st := s.Stats()
	fmt.Printf("\n--- after %.0f in-world years ---\n", *years)
	fmt.Println(st)
	fmt.Printf("\nsimulated %d ticks in %v (%.0f ticks/sec, %.0fx real time)\n",
		totalTicks, elapsed.Round(time.Millisecond),
		float64(totalTicks)/elapsed.Seconds(),
		float64(totalTicks)/elapsed.Seconds()/sim.TickRate)
	fmt.Printf("flow fields: %d computed, %d cache hits\n", st.PathMisses, st.PathHits)
}
