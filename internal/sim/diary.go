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
		s.diaryTallies = make(map[CharID]diaryTally)
	}
}

// Repetition, and what to do about it.
//
// The first diaries were unreadable, and not for want of good events. Measured over one
// twenty-five-year run: 491,719 entries, of which 93% were taking and leaving work and a
// further 5.5% were drawing relief. Everything that shapes a life — born, came of age,
// married, bore a child, died — came to about 250 lines, one twentieth of one per cent.
// A man's marriage was the second line of his forty-thousand-line diary and everything
// after it was "took work at the town hall / left the town hall after 0.0 years".
//
// The churn is not a bug to be silenced: job-hopping has been measured as load-bearing,
// and a villager who tries three hundred posts in a season is genuinely failing to find
// work. That is worth knowing. It is just not worth three hundred lines, because at that
// length it stops being information and becomes the thing you scroll past to find the
// information.
//
// So repetitive events accumulate into a tally and surface as one sentence. The count is
// preserved — "changed work 340 times without settling" says exactly what the 340 lines
// said, and unlike them it can be read.

// diaryTally holds the repetitive events not yet summarised for one character.
type diaryTally struct {
	// jobsTried counts spells too short to be worth naming individually.
	jobsTried int
	// reliefTook and reliefGold are visits to the town hall and what they yielded.
	reliefTook int
	reliefGold float32
}

// JobSpellYears is how long a post must be held to be named in its own right. Below it,
// a job is not a chapter of a life; it is an afternoon.
const JobSpellYears = 0.25

// record appends an entry without flushing tallies. Everything that writes to a diary
// goes through here eventually; only diarise flushes, or flushing would recurse.
func (s *State) record(id CharID, format string, args ...any) {
	s.diaries[id] = append(s.diaries[id], DiaryEntry{
		Tick: s.Tick,
		What: fmt.Sprintf(format, args...),
	})
}

// diarise records a moment in a character's life. No-op unless diaries are enabled.
//
// Any repetition still pending is summarised first, so that the tally appears in the
// place it happened rather than being flung to the end of the life.
func (s *State) diarise(id CharID, format string, args ...any) {
	if s.diaries == nil {
		return
	}
	s.flushTally(id)
	s.record(id, format, args...)
}

// tallyJobChurn notes a job held too briefly to name.
func (s *State) tallyJobChurn(id CharID) {
	if s.diaries == nil {
		return
	}
	t := s.diaryTallies[id]
	t.jobsTried++
	s.diaryTallies[id] = t
}

// tallyRelief notes a visit to the town hall.
func (s *State) tallyRelief(id CharID, gold float32) {
	if s.diaries == nil {
		return
	}
	t := s.diaryTallies[id]
	t.reliefTook++
	t.reliefGold += gold
	s.diaryTallies[id] = t
}

// ordinal spells a small number the way a person would say it. Diaries are read by
// people, and "her 3rd child" is not how anyone tells it.
func ordinal(n int) string {
	words := []string{"", "first", "second", "third", "fourth", "fifth", "sixth",
		"seventh", "eighth", "ninth", "tenth", "eleventh", "twelfth"}
	if n > 0 && n < len(words) {
		return words[n]
	}
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// tallyLines renders a tally as the sentences it stands for.
func tallyLines(t diaryTally) []string {
	var out []string
	switch {
	case t.jobsTried == 1:
		out = append(out, "took a post and left it again within the season")
	case t.jobsTried > 1:
		out = append(out, fmt.Sprintf("changed work %d times without settling anywhere", t.jobsTried))
	}
	switch {
	case t.reliefTook == 1:
		out = append(out, fmt.Sprintf("went to the town hall for relief, and was given %.1f gold", t.reliefGold))
	case t.reliefTook > 1:
		out = append(out, fmt.Sprintf("went to the town hall for relief %d times, %.1f gold in all",
			t.reliefTook, t.reliefGold))
	}
	return out
}

// flushTally writes out any pending repetition and clears it.
func (s *State) flushTally(id CharID) {
	t, ok := s.diaryTallies[id]
	if !ok {
		return
	}
	delete(s.diaryTallies, id)
	for _, line := range tallyLines(t) {
		s.record(id, "%s", line)
	}
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
	// Repetition not yet closed off by a later event still belongs in the life. Rendered
	// rather than flushed: reading a diary must not alter it, or two identical reads of a
	// running world would disagree.
	d := s.Tick.Date()
	for _, line := range tallyLines(s.diaryTallies[id]) {
		fmt.Fprintf(&b, "y%03d d%03d  %s\n", d.Year, d.Day, line)
	}
	return b.String()
}

// ClosedDiary is a finished life, detached from the slot that held it.
type ClosedDiary struct {
	Name    string
	Entries []DiaryEntry
}

// closeDiary files a completed life away before its slot is reused.
//
// A character ID is a slot, not a person: newChar hands the slots of the dead to the
// newborn. The diaries are keyed by ID, so without this a slot's second occupant appended
// their life to their predecessor's, and the file showed one name being born, dying,
// being born again to different parents, and dying at a different age — three people read
// as one confused ghost. The failure was invisible while entries were bare numbers and
// obvious the moment names and parents were printed beside them, which is the same lesson
// as note 1 arriving from a new direction.
//
// The name is resolved and stored here because the seed goes with the slot: once somebody
// else is living in it, there is no way back to what this person was called.
func (s *State) closeDiary(id CharID) {
	if s.diaries == nil {
		return
	}
	s.flushTally(id)
	entries := s.diaries[id]
	if len(entries) == 0 {
		delete(s.diaries, id)
		return
	}
	s.closed = append(s.closed, ClosedDiary{Name: s.FullName(id), Entries: entries})
	delete(s.diaries, id)
}

// ClosedDiaries returns every life that has ended, oldest first. Append-ordered, so it is
// deterministic without sorting.
func (s *State) ClosedDiaries() []ClosedDiary {
	return s.closed
}

// Render turns a finished life into text.
func (d ClosedDiary) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", d.Name)
	for _, e := range d.Entries {
		t := e.Tick.Date()
		fmt.Fprintf(&b, "y%03d d%03d  %s\n", t.Year, t.Day, e.What)
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
		wage = float32(s.Structs[c.Job].Wage * WorkTicksPerDay)
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
