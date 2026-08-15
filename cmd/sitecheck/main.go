// Command sitecheck reports where villages get founded and what they can reach.
package main

import (
	"fmt"

	"github.com/solifugus/serts/internal/sim"
	"github.com/solifugus/serts/internal/worldgen"
)

func main() {
	fmt.Printf("%-6s %-9s %-9s %-8s %-9s %s\n", "seed", "gold", "goldcells", "site", "golddist", "verdict")
	for seed := int64(1); seed <= 10; seed++ {
		w := worldgen.Generate(worldgen.DefaultParams(seed))
		s := sim.New(sim.DefaultConfig(w, seed))
		site := s.Structs[0].Cell
		d := w.GoldDist[w.T.Index(site)]
		verdict := "no faucet"
		if d <= sim.PanningRange {
			verdict = "can pan"
		} else if d <= sim.PanningRange*3 {
			verdict = "marginal"
		}
		fmt.Printf("%-6d %-9.0f %-9d %-8s %-9d %s\n",
			seed, w.TotalGold(), w.GoldCells(),
			fmt.Sprintf("%d,%d", site.X, site.Y), d, verdict)
	}
}
