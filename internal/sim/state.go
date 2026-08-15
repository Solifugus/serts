// Package sim is SERTS's simulation core (design §9.2).
//
// The whole game rests on one relationship: employment. A character is employed at a
// structure, the structure defines the work, and faction membership, growth, and decline
// are all consequences of who is on whose payroll (§1). This package implements the
// smallest version of that idea that can still be watched: people who must find work to
// earn, earn to eat, and eat to live.
//
// Two rules govern everything here:
//
//   - The tick is deterministic. Same seed, same ticks, same world — always. No
//     wall-clock, no map iteration order, no unseeded randomness. This is what makes
//     replay and tuning possible, and it cannot be retrofitted.
//   - Entities refer to each other by ID, never by pointer. Large pointer-free arrays are
//     skipped by Go's garbage collector, which is the single most valuable Go-specific
//     optimisation available at population scale (§9.4).
//
// Deviation from the design worth naming: §9.2 calls for struct-of-arrays storage, and
// this uses arrays of (pointer-free) structs. The GC benefit — the main reason for the
// rule — is already captured, and at the populations this milestone reaches the cache
// difference is not measurable. Revisit when profiling says so, not before.
//
// KNOWN LIMITATION — demography is not yet stable.
//
// The economy works: people find work, walk to it, earn, buy food, and stay fed and
// healthy for decades. Employment settles around eighty per cent, food stocks hold, and
// the village runs indefinitely as an economic system.
//
// Reproduction does not sustain it. The founding generation ages out over roughly sixty
// years and is not replaced: most children die young, and the population declines to
// extinction. Child mortality traces to household provisioning — the larder is stocked
// only when an adult happens to be at a granary with money to spare, so households whose
// adults are unemployed, distant, or unlucky starve their children while the village as a
// whole has food. Several fixes were tried and each traded the problem for another; the
// honest position is that this needs a proper model of household consumption rather than
// another tuned constant.
//
// §3.2 warned that population dynamics would be the most failure-prone system in the
// design. It was right.
package sim

import (
	"github.com/solifugus/serts/internal/torus"
	"github.com/solifugus/serts/internal/worldgen"
)

// CharID and StructID index into State's entity slices. They are deliberately not
// pointers.
type (
	CharID   int32
	StructID int32
)

const (
	NoChar   CharID   = -1
	NoStruct StructID = -1
)

// Activity is what a character is currently doing.
type Activity uint8

const (
	Idle Activity = iota
	SeekingWork
	GoingToWork
	Working
	GoingToEat
	GoingHome
	Resting
	numActivities
)

func (a Activity) String() string {
	switch a {
	case Idle:
		return "idle"
	case SeekingWork:
		return "seeking work"
	case GoingToWork:
		return "commuting"
	case Working:
		return "working"
	case GoingToEat:
		return "fetching food"
	case GoingHome:
		return "going home"
	case Resting:
		return "resting"
	}
	return "?"
}

// LifeStage buckets a character's age (§3.2).
type LifeStage uint8

const (
	Child LifeStage = iota
	Adult
	Elder
)

// Character is one person. Every reference it holds is an ID, so the slice of them
// contains no pointers for the collector to scan.
type Character struct {
	Pos    torus.Vec2
	Age    float32 // in-world years
	Health float32 // 0-100; zero is death
	Hunger float32 // 0-100; rises constantly
	Gold   float32

	Home     StructID
	Job      StructID
	Partner  CharID
	Activity Activity

	// Skill is tenure-derived efficiency per structure type (§3.4). Kept per type so
	// that moving a farmer to other work is a real sacrifice.
	Skill [NumStructTypes]float32

	// Tenure is time served with the current employer, in in-world years. Roots grow
	// from it (§3.7), though only skill uses it this milestone.
	Tenure float32

	Alive bool
	Sex   uint8 // 0 or 1; only reproduction reads it
	// housed reports whether this character occupies a resident slot of their own.
	// Children live with their parents and take no slot until they grow up, at which
	// point they must find housing like anyone else — which is where a village that
	// built too few homes discovers the fact.
	housed bool

	// dest is the structure being walked to, if any.
	dest StructID
	// bornAt lets the viewer report lineage without a separate registry.
	bornAt Tick
}

// Stage returns the character's life stage.
func (c *Character) Stage() LifeStage {
	switch {
	case c.Age < AdultAge:
		return Child
	case c.Age < ElderAge:
		return Adult
	default:
		return Elder
	}
}

// Structure is one building. Structures are the only employers (§5).
type Structure struct {
	Type   StructType
	Cell   torus.Cell
	Pos    torus.Vec2 // centre, cached for distance work
	Gold   float32
	Food   float32 // stock held, for granaries and farms
	Wage   float32 // gold per worker per tick
	Jobs   int     // positions offered
	Filled int     // positions currently taken

	// Condition is 0-100. Decay accelerates with disuse (§5.1); this milestone tracks
	// the value without yet letting structures fall to ruin.
	Condition float32

	// Residents counts assigned inhabitants, for homes.
	Residents int

	Alive bool

	// produce accumulates this tick's labour contribution from workers present, before
	// the structure converts it into output. Workers add to it during the character
	// step; the structure step consumes it. Keeping it separate is what lets yield
	// depend on who actually turned up rather than merely on who is on the payroll.
	produce float32
}

// Openings returns how many more workers a structure will take.
func (s *Structure) Openings() int { return s.Jobs - s.Filled }

// State is the whole simulated world at one instant.
type State struct {
	T     torus.T
	World *worldgen.World // terrain; read-only during a tick

	Tick Tick
	Seed int64

	Chars   []Character
	Structs []Structure

	// Treasury is the faction's own gold, distinct from what its structures hold. It is
	// the founding endowment, distributed to structures at settlement; wages and food
	// revenue move between structures and characters, not through here (§4.3).
	Treasury float32
	// FoodPrice is gold per unit of food at a granary.
	FoodPrice float32

	// freeChars recycles the slots of the dead, so the character slice does not grow
	// without bound over a long-running world.
	freeChars []CharID

	rng   *Rand
	paths *pathCache

	// Deaths and births accumulate for reporting.
	Births, DeathsAge, DeathsStarved int
	DeathsChild, DeathsHomeless      int

	// What the founding actually managed to build, which may be less than was asked for
	// if the site could not take it.
	BuiltHomes, BuiltFarms, BuiltGranaries int
}

// Cell returns the terrain cell a character stands in.
func (s *State) Cell(id CharID) torus.Cell { return s.T.CellAt(s.Chars[id].Pos) }

// Alive reports whether a character ID still refers to a living person.
func (s *State) AliveChar(id CharID) bool {
	return id >= 0 && int(id) < len(s.Chars) && s.Chars[id].Alive
}

// newChar allocates a character slot, reusing a dead one when possible.
func (s *State) newChar(c Character) CharID {
	c.Alive = true
	c.bornAt = s.Tick
	if n := len(s.freeChars); n > 0 {
		id := s.freeChars[n-1]
		s.freeChars = s.freeChars[:n-1]
		s.Chars[id] = c
		return id
	}
	s.Chars = append(s.Chars, c)
	return CharID(len(s.Chars) - 1)
}

// kill removes a character from the world, releasing their job, home, and partner.
func (s *State) kill(id CharID) {
	c := &s.Chars[id]
	if c.Job != NoStruct {
		s.Structs[c.Job].Filled--
		c.Job = NoStruct
	}
	if c.Home != NoStruct {
		if c.housed {
			s.Structs[c.Home].Residents--
		}
		c.Home = NoStruct
		c.housed = false
	}
	if c.Partner != NoChar && s.AliveChar(c.Partner) {
		s.Chars[c.Partner].Partner = NoChar
	}
	c.Alive = false
	s.freeChars = append(s.freeChars, id)
}

// Population counts the living.
func (s *State) Population() int {
	n := 0
	for i := range s.Chars {
		if s.Chars[i].Alive {
			n++
		}
	}
	return n
}
