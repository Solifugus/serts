//go:build probe

// A probe, not a test.
//
// This file measures rather than asserts: it runs the simulation for a long horizon and
// prints what it found, so that a question about the world can be answered with evidence.
// It is excluded from the default build because probes and tests want opposite things
// from a test suite. A test must be cheap enough to run on every change and must fail
// when something breaks; a probe is deliberately expensive and cannot fail, because it
// has no opinion about what the answer should be.
//
// Run one with:
//
//	go test -tags probe ./internal/sim -run TestName -timeout 3h -v
//
// See docs/method.md, note 15.

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
