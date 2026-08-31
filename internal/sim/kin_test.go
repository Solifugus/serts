package sim

import (
	"strings"
	"testing"
	"unsafe"
)

// birth() takes a pointer into s.Chars, then appends to s.Chars, then writes through the
// pointer. If the append grows the slice the write lands in the abandoned backing array
// and the mother's child count is silently lost — corrupting Life.Children, which is what
// completed fertility and every vital statistic are computed from.
func TestBirthCountsTheChildWhenTheSliceGrows(t *testing.T) {
	s := newTestSim(5)

	// Find a mother with a home and no free slots to recycle, so the birth must append.
	s.freeChars = s.freeChars[:0]
	var mother CharID = NoChar
	for i := range s.Chars {
		c := &s.Chars[i]
		if c.Alive && c.Sex == 0 && c.Home != NoStruct {
			mother = CharID(i)
			break
		}
	}
	if mother == NoChar {
		t.Skip("no housed woman in the founding village")
	}

	// Force the next append to reallocate: make length equal capacity exactly.
	s.Chars = s.Chars[:len(s.Chars):len(s.Chars)]
	before := s.Chars[mother].Children
	oldCap := cap(s.Chars)

	s.birth(mother, s.Chars[mother].Home)

	if cap(s.Chars) == oldCap {
		t.Fatal("the slice did not grow, so this does not test what it means to")
	}
	if got := s.Chars[mother].Children; got != before+1 {
		t.Errorf("mother's child count is %d, want %d — the increment was written to the "+
			"abandoned backing array", got, before+1)
	}
}

func TestCharacterStaysCompact(t *testing.T) {
	// Character is held in one large pointer-free slice so the collector can skip it
	// entirely (§9.2). Width is the cost that matters: every pass over the population
	// touches it, and the growth is worth tracing because each step looked free where it
	// was written and is charged to every person alive, forever.
	//
	//	200  the baseline
	//	208  kin links: Mother and Father
	//	216  the clinic — not for any field of its own, but because Skill is
	//	     [NumStructTypes]float32, so every new KIND OF BUILDING costs four bytes a head
	//	224  money as float64 (see Structure.Gold)
	//
	// The last of those is expensive and the number is worth having exactly. Measured on
	// BenchmarkStep with population held equal at 77 either side, and with the road and
	// clinic hot paths stubbed out in BOTH arms so that only the money types differ:
	//
	//	7,998 ns   float32 money
	//	12,886 ns  float64 money        +61%
	//	13,556 ns  and the road and clinic work switched back on, so those two cost
	//	           about 5% between them
	//
	// A first attempt at this split was wrong and is worth recording as a caution: the
	// isolation stubs were written as one-liners, gofmt reformatted them across three
	// lines, and the scripted removal then matched nothing and silently left them in. Two
	// benchmarks and a full gate ran against instrumented code. Grep for the marker before
	// trusting any number that depended on removing it.
	//
	// The price is paid deliberately. With float32, gold leaked about 3.2% of the money
	// supply every twenty years, and a worker holding ten thousand gold could not receive
	// a wage at all: the payment was smaller than the smallest change their balance could
	// represent, so it rounded to nothing while the employer's till still emptied.
	if got := int(unsafe.Sizeof(Character{})); got > 224 {
		t.Errorf("Character is %d bytes; adding to it slows every pass over the population", got)
	}
}

// CharID(0) is a real person, so a forgotten field would silently make the whole founding
// generation children of the first settler.
func TestFoundersHaveNoParents(t *testing.T) {
	s := newTestSim(5)
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive {
			continue
		}
		if c.Mother != NoChar || c.Father != NoChar {
			t.Fatalf("settler %d was born to %d and %d", i, c.Mother, c.Father)
		}
	}
}

func TestChildrenKnowTheirParents(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(6 * TicksPerYear)

	checked := 0
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.bornAt == 0 {
			continue // a founder, who has none
		}
		m := s.MotherOf(CharID(i))
		if m == NoChar {
			continue // mother's slot has been reissued, which parentOf is right to reject
		}
		checked++
		if s.Chars[m].Sex != 0 {
			t.Errorf("character %d's mother %d is male", i, m)
		}
		if s.Chars[m].bornAt >= c.bornAt {
			t.Errorf("character %d's mother %d was born no earlier than they were", i, m)
		}
		if f := s.FatherOf(CharID(i)); f != NoChar && s.Chars[f].Sex != 1 {
			t.Errorf("character %d's father %d is female", i, f)
		}
	}
	if checked == 0 {
		t.Error("no child in six years had a traceable mother")
	}
	t.Logf("%d children with traceable parents", checked)
}

// The guard that stops a diary announcing a stranger's death to somebody's child.
func TestReissuedSlotIsNotMistakenForAParent(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(2 * TicksPerYear)

	var child CharID = NoChar
	for i := range s.Chars {
		if s.Chars[i].Alive && s.MotherOf(CharID(i)) != NoChar {
			child = CharID(i)
			break
		}
	}
	if child == NoChar {
		t.Skip("no child with a traceable mother yet")
	}
	mother := s.MotherOf(child)

	// Somebody new takes the mother's slot: same ID, born later. The link must not follow.
	s.Chars[mother].bornAt = s.Chars[child].bornAt + 1
	if got := s.MotherOf(child); got != NoChar {
		t.Errorf("a reissued slot is still reported as the mother (%d)", got)
	}
}

// Deaths must reach the people they happened to.
func TestDeathIsRecordedInTheSurvivorsDiaries(t *testing.T) {
	s := newTestSim(5)
	s.EnableDiaries()
	s.RunTicks(3 * TicksPerYear)

	var child CharID = NoChar
	for i := range s.Chars {
		if s.Chars[i].Alive && s.MotherOf(CharID(i)) != NoChar && s.AliveChar(s.MotherOf(CharID(i))) {
			child = CharID(i)
			break
		}
	}
	if child == NoChar {
		t.Skip("no living child with a living mother yet")
	}
	mother := s.MotherOf(child)

	s.kill(mother, CauseDisease)

	if txt := s.Diary(child); !strings.Contains(txt, "died at") {
		t.Errorf("a child's diary does not mention losing their mother:\n%s", txt)
	}
	// Her own death is in her own record — which is now a closed diary, filed away when
	// she died so that the next occupant of her slot does not inherit her life.
	closed := s.ClosedDiaries()
	last := closed[len(closed)-1].Render()
	if !strings.Contains(last, "died of") {
		t.Errorf("the dead woman's closed diary does not record her death:\n%s", last)
	}
	if s.Diary(mother) != "" && strings.Contains(s.Diary(mother), "died of") {
		t.Error("the dead woman's diary is still occupying her slot")
	}
}

// A character ID is a slot, not a person. When the dead are recycled the newcomer must not
// inherit their predecessor's diary — that produced a file in which one name was born,
// died, was born again to different parents, and died at a different age.
func TestARecycledSlotStartsAFreshDiary(t *testing.T) {
	s := newTestSim(5)
	s.EnableDiaries()
	s.RunTicks(2 * TicksPerYear)

	var victim CharID = NoChar
	for i := range s.Chars {
		if s.Chars[i].Alive && len(s.diaries[CharID(i)]) > 0 {
			victim = CharID(i)
			break
		}
	}
	if victim == NoChar {
		t.Skip("nobody has a diary yet")
	}
	nameBefore := s.FullName(victim)
	closedBefore := len(s.ClosedDiaries())

	s.kill(victim, CauseDisease)

	if len(s.ClosedDiaries()) != closedBefore+1 {
		t.Fatalf("the dead person's diary was not filed away")
	}
	if got := s.ClosedDiaries()[closedBefore].Name; got != nameBefore {
		t.Errorf("the finished life is filed under %q, want %q", got, nameBefore)
	}
	if n := len(s.diaries[victim]); n != 0 {
		t.Errorf("the slot still holds %d entries for the next occupant to inherit", n)
	}

	// Whoever takes the slot starts clean.
	reused := s.newChar(Character{
		Home: NoStruct, Job: NoStruct, Partner: NoChar,
		Mother: NoChar, Father: NoChar, dest: NoStruct,
	})
	if reused != victim {
		t.Skipf("the slot was not reused (got %d), so there is nothing to check", reused)
	}
	if n := len(s.diaries[reused]); n != 0 {
		t.Errorf("the new occupant inherited %d entries from the dead", n)
	}
}
