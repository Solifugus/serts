package sim

import (
	"fmt"
	"os"
	"testing"

	"github.com/solifugus/serts/internal/worldgen"
)

// Seed 108 founded a colony and then fell from 138 to 86. Watch both settlements
// separately across the founding — the aggregate cannot say which one is dying.
func TestColonyAftermath(t *testing.T) {
	w := worldgen.Generate(worldgen.DefaultParams(108))
	s := New(DefaultConfig(w, 108))
	for y := 1; y <= 120; y++ {
		s.RunTicks(TicksPerYear)
		// Yearly once a colony exists, every fifth year before.
		if s.Colonies == 0 && y%10 != 0 {
			continue
		}
		fmt.Fprintf(os.Stderr, "y%3d world pop %3d colonies %d\n%s",
			y, s.Population(), s.Colonies, s.SettlementReport())
	}
}
