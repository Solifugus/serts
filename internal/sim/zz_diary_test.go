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
)

// One instrumented run; the diaries of everyone who starved young, and their parents,
// written to the scratchpad for offline reading.
func TestWriteDiaries(t *testing.T) {
	s := newTestSim(5)
	s.EnableDiaries()
	s.RunTicks(25 * TicksPerYear)

	out := "/home/solifugus/development/serts/diaries.txt"
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// The dead first, in the order they died, then the living. Finished lives are filed
	// away as they end (closeDiary) because a character ID is a slot rather than a person
	// and gets handed on; keyed by ID alone, three occupants of one slot read as a single
	// person born and dying repeatedly.
	for _, d := range s.ClosedDiaries() {
		f.WriteString("=== died\n" + d.Render() + "\n")
	}
	for _, id := range s.DiaryIDs() {
		c := &s.Chars[id]
		header := fmt.Sprintf("=== living (age %.1f)\n", c.Age)
		f.WriteString(header + s.Diary(id) + "\n")
	}
	t.Logf("wrote %d finished lives and %d living", len(s.ClosedDiaries()), len(s.DiaryIDs()))
}
