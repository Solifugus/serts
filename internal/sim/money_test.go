package sim

import (
	"math"
	"testing"
)

// coin is every piece of gold anywhere, including places TotalCoin deliberately ignores:
// the purses of the dead and the tills of dead structures. A transfer that lands in one of
// those is not destroyed, merely hidden, and the two failures want telling apart.
func coin(s *State) (visible, hidden float64) {
	for i := range s.Chars {
		if s.Chars[i].Alive {
			visible += s.Chars[i].Gold
		} else {
			hidden += s.Chars[i].Gold
		}
	}
	for i := range s.Structs {
		st := &s.Structs[i]
		if st.Alive {
			visible += st.Gold + st.Works
		} else {
			hidden += st.Gold + st.Works
		}
	}
	return visible + s.World.TotalGold(), hidden
}

// Each function that moves money must conserve it. The leak was narrowed to stepBehaviour
// by audit; these pin the individual transfers so the next one cannot hide.
func TestMoneyMoversConserve(t *testing.T) {
	cases := []struct {
		name string
		run  func(s *State) bool // returns false to skip
	}{
		{"buyAndEat", func(s *State) bool {
			for i := range s.Chars {
				c := &s.Chars[i]
				if !c.Alive || c.Gold <= 0 {
					continue
				}
				if src := s.NearestFoodSource(c.Pos); src != NoStruct {
					c.Rations = 0
					c.Hunger = 90
					s.buyAndEat(CharID(i), src)
					return true
				}
			}
			return false
		}},
		{"pan", func(s *State) bool {
			for i := range s.Chars {
				c := &s.Chars[i]
				if !c.Alive || c.Stage() == Child {
					continue
				}
				if s.pan(CharID(i)) {
					return true
				}
			}
			return false
		}},
		{"work", func(s *State) bool {
			for i := range s.Chars {
				c := &s.Chars[i]
				if c.Alive && c.Job != NoStruct && s.Structs[c.Job].Gold > s.Structs[c.Job].Wage {
					s.work(CharID(i))
					return true
				}
			}
			return false
		}},
	}

	for _, tc := range cases {
		s := newTestSim(5)
		s.RunTicks(3 * TicksPerYear)
		if tc.name == "pan" {
			// Put a rich seam under everyone BEFORE weighing, so the test measures the
			// transfer rather than its own injection. A prospector standing on a large
			// balance of ore is exactly the case where a float32 field and a float64
			// purse would disagree.
			for i := range s.Chars {
				if s.Chars[i].Alive {
					s.World.GoldOre[s.T.Index(s.T.CellAt(s.Chars[i].Pos))] = 500
				}
			}
		}
		beforeV, beforeH := coin(s)
		beforeMint := -s.Led.GoldDestroyed
		if !tc.run(s) {
			t.Logf("%s: no opportunity to exercise it", tc.name)
			continue
		}
		afterV, afterH := coin(s)
		afterMint := -s.Led.GoldDestroyed

		// Everything that exists, minus what the ledger says was minted or destroyed.
		total := (afterV + afterH) - (beforeV + beforeH) - (afterMint - beforeMint)
		if math.Abs(total) > 1e-9 {
			t.Errorf("%s: %+.9f gold appeared or vanished outright", tc.name, total)
		}
		// And separately: did any of it merely become invisible to TotalCoin?
		if d := afterH - beforeH; math.Abs(d) > 1e-9 {
			t.Errorf("%s: %+.9f gold moved somewhere TotalCoin cannot see it", tc.name, d)
		}
	}
}
