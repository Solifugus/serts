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

	for _, id := range s.DiaryIDs() {
		c := &s.Chars[id]
		header := fmt.Sprintf("=== character %d (alive=%v, age %.1f)\n", id, c.Alive, c.Age)
		f.WriteString(header + s.Diary(id) + "\n")
	}
	t.Logf("wrote %d diaries", len(s.DiaryIDs()))
}
