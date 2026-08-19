package sim

import (
	"fmt"
	"sync"
	"testing"
)

// When a small child starves, is the village out of food (production) or is the food
// somewhere the child cannot reach (distribution)? Opposite fixes; measure which.
func TestChildHungerProductionOrDistribution(t *testing.T) {
	seeds := []int64{5, 7}
	reports := make([][]string, len(seeds))
	var wg sync.WaitGroup
	for i, seed := range seeds {
		wg.Add(1)
		go func(i int, seed int64) {
			defer wg.Done()
			s := newTestSim(seed)
			s.onDeath = func(id CharID, cause DeathCause) {
				c := &s.Chars[id]
				if cause != CauseHunger || c.Age >= 5 || c.settler {
					return
				}
				larder, occ := float32(-1), 0
				var parentGold, kidsShare float32
				if c.Home != NoStruct {
					h := &s.Structs[c.Home]
					larder, occ = h.Stock[Food], h.Occupants
					kidsShare = s.childrensShare(c.Home)
					for j := range s.Chars {
						o := &s.Chars[j]
						if o.Alive && o.Home == c.Home && o.Stage() != Child {
							parentGold += o.Gold
						}
					}
				}
				var granary, farms, homes float32
				for j := range s.Structs {
					st := &s.Structs[j]
					if !st.Alive {
						continue
					}
					switch st.Type {
					case Granary, DiningHall:
						granary += st.Stock[Food]
					case Farm:
						farms += st.Stock[Food]
					case Home:
						homes += st.Stock[Food]
					}
				}
				reports[i] = append(reports[i], fmt.Sprintf(
					"y%3d d%3d age %4.2f | larder %6.1f (kidsShare %4.1f) occ %2d parentGold %7.2f | village: granary %7.0f farms %7.0f homes %6.0f | pop %3d foodDays %5.1f price %.2f",
					int(s.Tick/TicksPerYear), s.Tick.Date().Day, c.Age,
					larder, kidsShare, occ, parentGold,
					granary, farms, homes, s.Population(), s.FoodDays(), s.Prices[Food]))
			}
			s.RunTicks(120 * TicksPerYear)
		}(i, seed)
	}
	wg.Wait()
	for i, seed := range seeds {
		t.Logf("--- seed %d: %d under-5 hunger deaths", seed, len(reports[i]))
		for j, r := range reports[i] {
			if j < 20 || j%3 == 0 {
				t.Log(r)
			}
		}
	}
}
