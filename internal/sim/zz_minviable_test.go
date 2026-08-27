package sim

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/solifugus/serts/internal/worldgen"
)

// What is the smallest village that can survive on its own?
//
// 34 settlers was measured to fail — 2,957 samples of unmarried fertile adults found an
// eligible partner in range zero times, an Allee effect, and R0 capped near 0.62 whatever
// the economy did. 68 works. ColonyParty is 60 because it sits between those two numbers,
// which is a guess wearing a constant's clothes. The boundary decides the founding
// default, the party size, and whether a daughter settlement can ever send daughters of
// its own.
//
// Infrastructure scales with the party so that only the HEADCOUNT varies: a village of
// thirty with sixteen homes and six farms would be testing endowment, not viability.
func TestMinimumViableFounding(t *testing.T) {
	sizes := []int{28, 36, 44, 52, 60}
	seeds := []int64{5, 7, 108, 209, 311, 407}

	type result struct {
		size            int
		seed            int64
		y50, y100, y150 int
	}
	results := make([]result, len(sizes)*len(seeds))

	var wg sync.WaitGroup
	for si, size := range sizes {
		for di, seed := range seeds {
			wg.Add(1)
			go func(si, di, size int, seed int64) {
				defer wg.Done()
				w := worldgen.Generate(worldgen.DefaultParams(seed))
				cfg := DefaultConfig(w, seed)
				// Scale the settlement to the party, keeping the ratios of the measured
				// default (68 settlers, 16 homes, 6 farms, 2 granaries, 4 camps).
				f := float64(size) / 68.0
				cfg.Settlers = size
				cfg.Homes = maxInt(4, int(16*f+0.5))
				cfg.Farms = maxInt(2, int(6*f+0.5))
				cfg.Granaries = maxInt(1, int(2*f+0.5))
				cfg.Camps = maxInt(1, int(4*f+0.5))
				cfg.Treasury = float32(12000 * f)
				s := New(cfg)
				r := result{size: size, seed: seed}
				s.RunTicks(50 * TicksPerYear)
				r.y50 = s.Population()
				s.RunTicks(50 * TicksPerYear)
				r.y100 = s.Population()
				s.RunTicks(50 * TicksPerYear)
				r.y150 = s.Population()
				results[si*len(seeds)+di] = r
				fmt.Fprintf(os.Stderr, "founded %2d (seed %3d): y50 %3d  y100 %3d  y150 %3d\n",
					size, seed, r.y50, r.y100, r.y150)
			}(si, di, size, seed)
		}
	}
	wg.Wait()

	// Survival is probabilistic, not a threshold: report the RATE per size, which is the
	// quantity the question actually has. A single run tells you whether one village was
	// lucky.
	t.Log("founding size vs survival, 150 years:")
	for _, size := range sizes {
		grew, died, total := 0, 0, 0
		sum := 0
		for _, r := range results {
			if r.size != size {
				continue
			}
			total++
			sum += r.y150
			if r.y150 == 0 {
				died++
			} else if r.y150 >= r.size {
				grew++
			}
		}
		t.Logf("  %2d settlers: grew %d/%d, died %d/%d, mean final %d",
			size, grew, total, died, total, sum/maxInt(total, 1))
	}
}
