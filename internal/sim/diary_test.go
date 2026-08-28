package sim

import (
	"strings"
	"testing"
)

// The whole point of the tally is that summarising is not the same as discarding. A diary
// that quietly dropped the churn would read beautifully and lie: a villager who tried
// three hundred posts in a season IS failing to find work, and that is a finding.
func TestJobChurnIsSummarisedNotLost(t *testing.T) {
	s := newTestSim(5)
	s.EnableDiaries()

	const tries = 340
	for i := 0; i < tries; i++ {
		s.tallyJobChurn(0)
	}
	// Nothing is written until something closes the run off, so the summary lands where it
	// happened rather than at the end of the life.
	if got := len(s.diaries[0]); got != 0 {
		t.Errorf("churn wrote %d entries before being flushed, want 0", got)
	}
	// A pending tally is still visible to a reader.
	if txt := s.Diary(0); !strings.Contains(txt, "340") {
		t.Errorf("a pending tally is invisible to Diary:\n%s", txt)
	}
	// Reading must not consume it — two identical reads of a running world must agree.
	if a, b := s.Diary(0), s.Diary(0); a != b {
		t.Error("reading a diary changed it")
	}

	s.diarise(0, "something worth recording")
	txt := s.Diary(0)
	if !strings.Contains(txt, "340") {
		t.Errorf("the count of tries was lost on flush:\n%s", txt)
	}
	// The summary must precede the event that flushed it: the tries happened first.
	if i, j := strings.Index(txt, "340"), strings.Index(txt, "worth recording"); i < 0 || j < 0 || i > j {
		t.Errorf("the tally landed after the event that closed it:\n%s", txt)
	}
	// And it must not repeat once written.
	s.diarise(0, "a later event")
	if n := strings.Count(s.Diary(0), "340"); n != 1 {
		t.Errorf("the tally appears %d times, want 1", n)
	}
}

func TestReliefIsSummarisedWithItsTotal(t *testing.T) {
	s := newTestSim(5)
	s.EnableDiaries()

	for i := 0; i < 12; i++ {
		s.tallyRelief(0, 0.5)
	}
	s.diarise(0, "died")
	txt := s.Diary(0)
	if !strings.Contains(txt, "12 times") {
		t.Errorf("relief count missing:\n%s", txt)
	}
	if !strings.Contains(txt, "6.0 gold") {
		t.Errorf("relief total missing or wrong (want 6.0):\n%s", txt)
	}
}

// A single occurrence should read as a sentence about a person, not as a count of one.
func TestSingleOccurrencesReadNaturally(t *testing.T) {
	for _, c := range []struct {
		tally diaryTally
		want  string
		avoid string
	}{
		{diaryTally{jobsTried: 1}, "within the season", "1 times"},
		{diaryTally{reliefTook: 1, reliefGold: 0.5}, "went to the town hall for relief, and", "1 times"},
	} {
		got := strings.Join(tallyLines(c.tally), " | ")
		if !strings.Contains(got, c.want) {
			t.Errorf("%+v rendered %q, want it to contain %q", c.tally, got, c.want)
		}
		if strings.Contains(got, c.avoid) {
			t.Errorf("%+v rendered %q, which reads as a tally rather than an event", c.tally, got)
		}
	}
}

// Diaries are off by default and must cost nothing when they are — no map, no entries, no
// allocation from the tally sites either.
func TestTallyingIsFreeWhenDiariesAreOff(t *testing.T) {
	s := newTestSim(5)
	s.tallyJobChurn(0)
	s.tallyRelief(0, 1)
	s.diarise(0, "should not be recorded")
	if s.diaries != nil || s.diaryTallies != nil {
		t.Error("recording allocated with diaries disabled")
	}
}

func TestOrdinalReadsLikeSpeech(t *testing.T) {
	cases := map[int]string{1: "first", 3: "third", 12: "twelfth", 13: "13th", 21: "21st", 22: "22nd"}
	for n, want := range cases {
		if got := ordinal(n); got != want {
			t.Errorf("ordinal(%d) = %q, want %q", n, got, want)
		}
	}
}
