// Command sitecheck reports where villages get founded and what they can reach.
//
// Settlement siting has to weigh several resources at once (§2.9), and the failure it
// guards against is silent: a village founded out of reach of gold cannot mint money, and
// one founded away from timber cannot build. This prints the answer for a spread of seeds
// so those failures are visible before they become mysterious economic behaviour.
package main

import (
	"fmt"

	"github.com/solifugus/serts/internal/sim"
	"github.com/solifugus/serts/internal/worldgen"
)

func main() {
	fmt.Printf("%-5s %-9s %-9s %-9s %-8s %-8s %s\n",
		"seed", "gold", "woodland", "iron", "site", "golddist", "verdict")
	for seed := int64(1); seed <= 8; seed++ {
		w := worldgen.Generate(worldgen.DefaultParams(seed))
		s := sim.New(sim.DefaultConfig(w, seed))
		site := s.Structs[0].Cell
		i := w.T.Index(site)
		d := w.GoldDist[i]

		// How much timber lies within a working radius of the settlement?
		var nearWood float64
		for dy := -12; dy <= 12; dy++ {
			for dx := -12; dx <= 12; dx++ {
				n := w.T.Index(w.T.WrapCell(worldgen.CellAt(site, dx, dy)))
				nearWood += float64(w.Woodland[n])
			}
		}

		verdict := "no faucet"
		if d <= sim.PanningRange {
			verdict = "can pan"
		} else if d <= sim.PanningRange*3 {
			verdict = "marginal"
		}
		if nearWood < 40 {
			verdict += ", little timber"
		}
		fmt.Printf("%-5d %-9.0f %-9.0f %-9.0f %-8s %-8d %s\n",
			seed, w.TotalGold(), nearWood, w.IronOre[i], fmt.Sprintf("%d,%d", site.X, site.Y), d, verdict)
	}
}
