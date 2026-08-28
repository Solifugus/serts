package sim

import (
	"fmt"
	"sort"
	"strings"
)

// Diaries: the life of every villager, written down as it happens.
//
// Every correct diagnosis in this project has come from following one person — the man
// whose gold drained over years, the toddler refused seven hundredths of a meal, the
// customers dying at the counter over a rounding — and every wrong one came from
// reasoning about aggregates (docs/method.md, notes 1 and 8). But following one person
// has meant writing a bespoke probe per question and paying twenty to forty-five minutes
// of simulation per run, which made the probe cycle, not the thinking, the bottleneck.
//
// A diary inverts the cost: simulate once with recording on, then interrogate the lives
// offline — any question, no re-run. It is the same shift the vital statistics made for
// demography, applied to biography.
//
// Diaries are event-based, not tick-based: a life is a few hundred moments, not forty
// thousand days. Entries live in a map beside the simulation rather than on Character,
// which stays pointer-free (§9.4), and recording off means a nil map and a single branch
// per event site — the game pays nothing.

// DiaryEntry is one moment in a life.
type DiaryEntry struct {
	Tick Tick
	What string
}

// EnableDiaries switches recording on. Do it at founding: a diary that starts mid-life
// misleads exactly like a statistic that starts mid-transient.
func (s *State) EnableDiaries() {
	if s.diaries == nil {
		s.diaries = make(map[CharID][]DiaryEntry)
	}
}

// diarise records a moment in a character's life. No-op unless diaries are enabled.
func (s *State) diarise(id CharID, format string, args ...any) {
	if s.diaries == nil {
		return
	}
	s.diaries[id] = append(s.diaries[id], DiaryEntry{
		Tick: s.Tick,
		What: fmt.Sprintf(format, args...),
	})
}

// Diary returns one character's life as text, one dated line per entry.
func (s *State) Diary(id CharID) string {
	entries := s.diaries[id]
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", s.FullName(id))
	for _, e := range entries {
		d := e.Tick.Date()
		fmt.Fprintf(&b, "y%03d d%03d  %s\n", d.Year, d.Day, e.What)
	}
	return b.String()
}

// DiaryIDs lists every character with a diary, in ID order for determinism.
func (s *State) DiaryIDs() []CharID {
	ids := make([]CharID, 0, len(s.diaries))
	for id := range s.diaries {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	return ids
}

// hungerEpisode marks the crossing into serious hunger, once per episode rather than per
// tick — a diary that says "hungry" two thousand times a day says nothing.
func (s *State) noteHunger(id CharID) {
	if s.diaries == nil {
		return
	}
	c := &s.Chars[id]
	if c.inHungerEpisode {
		return
	}
	c.inHungerEpisode = true
	job, wage := "no job", float32(0)
	if c.Job != NoStruct {
		job = Defs[s.Structs[c.Job].Type].Name
		wage = s.Structs[c.Job].Wage * WorkTicksPerDay
	}
	larder := float32(-1)
	if c.Home != NoStruct {
		larder = s.Structs[c.Home].Stock[Food]
	}
	s.diarise(id, "went seriously hungry (age %.1f, %.2f gold, %.1f rations; %s at %.3f/day; larder %.1f; food at %.2f)",
		c.Age, c.Gold, c.Rations, job, wage, larder, s.Prices[Food])
}

// noteFed closes a hunger episode.
func (s *State) noteFed(id CharID) {
	if s.diaries == nil {
		return
	}
	c := &s.Chars[id]
	if !c.inHungerEpisode {
		return
	}
	c.inHungerEpisode = false
	s.diarise(id, "found enough to eat again")
}
