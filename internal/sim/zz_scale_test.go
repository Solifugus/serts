package sim

import (
	"sync"
	"testing"

	"github.com/solifugus/serts/internal/worldgen"
)

// Is the village below its demographically viable size? Marriage was measured as a pure
// coincidence process — 2,957 samples of unmarried fertile adults, zero with an eligible
// partner in range — so cohort thickness, not any economic lever, may bound R0. Test by
// founding at roughly double scale and measuring the same statistic.
func TestScaleHypothesis(t *testing.T) {
	seeds := []int64{5, 7, 108, 209}
	pooled := make([][]Life, len(seeds))
	var wg sync.WaitGroup
	for i, seed := range seeds {
		wg.Add(1)
		go func(i int, seed int64) {
			defer wg.Done()
			w := worldgen.Generate(worldgen.DefaultParams(seed))
			s := New(DefaultConfig(w, seed))
			s.RunTicks(120 * TicksPerYear)
			pooled[i] = s.Lives
		}(i, seed)
	}
	wg.Wait()
	var lives []Life
	for _, ls := range pooled {
		lives = append(lives, ls...)
	}
	v := VitalsOf(lives)
	var born, kids int
	for _, l := range lives {
		if !l.Settler {
			born++
			kids += l.Children
		}
	}
	r0 := 0.0
	if born > 0 {
		r0 = float64(kids) / float64(born) / 2
	}
	t.Logf("DOUBLE-SCALE: %d lives born, R0 = %.3f, reached adulthood %.0f%%, married %.0f%%, expectancy %.1f",
		born, r0, v.ReachedAdulthood*100, v.EverMarried*100, v.ExpectancyAtBirth)
	t.Log("\n" + v.String())
	// Survival curve and death causes by band, to name where the remaining gap lives.
	bands := []float32{1, 5, 10, 15, 18, 25}
	for _, b := range bands {
		n := 0
		for _, l := range lives {
			if !l.Settler && l.Age >= b {
				n++
			}
		}
		t.Logf("  to age %2.0f: %5.1f%%", b, 100*float64(n)/float64(born))
	}
	type bc struct{ h, d, a int }
	byBand := map[string]*bc{}
	name := func(a float32) string {
		switch {
		case a < 1:
			return "0-1"
		case a < 5:
			return "1-5"
		case a < 15:
			return "5-15"
		case a < 45:
			return "15-45"
		}
		return "45+"
	}
	for _, l := range lives {
		if l.Settler {
			continue
		}
		k := name(l.Age)
		if byBand[k] == nil {
			byBand[k] = &bc{}
		}
		switch l.Cause {
		case CauseHunger:
			byBand[k].h++
		case CauseDisease:
			byBand[k].d++
		case CauseAccident:
			byBand[k].a++
		}
	}
	for _, k := range []string{"0-1", "1-5", "5-15", "15-45", "45+"} {
		if c := byBand[k]; c != nil {
			t.Logf("  %s: hunger %d disease %d accident %d", k, c.h, c.d, c.a)
		}
	}
}
