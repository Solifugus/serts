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
// MILESTONE 3 STATUS — the trades exist; the village does not yet grow.
//
// Materials, extraction, manufacture, tools, the wholesale chain, and construction are
// all implemented and tested. What they have not yet produced is a village that gets
// bigger: it settles at around twenty people whether it is founded with twenty-four or
// thirty-four, because food production, not employment, is still the ceiling.
//
// Adding trades was at first actively fatal, and the reason is worth keeping. Every
// employer offers the same wage, so labour spread evenly over whatever jobs existed and
// the fields emptied — a village that had fed itself for a century starved while its
// people mined iron nobody had a use for. PolicyWeight (§8.1) now stands in for the
// government that would obviously say "get back in the fields", which stops the collapse
// but does not yet produce growth.
//
// The honest reading is that the material economy has no demand behind it. Tools are the
// only thing anyone can buy, and a village this poor buys few, so the chain from mine to
// workshop to store barely turns over. It will need either more to spend money on or a
// reason for the faction itself to buy — which is what construction, and a player with a
// budget, are for.
//
// The village now survives indefinitely. A founding settlement of 24 was still standing
// at 150 in-world years — three generations after everyone who founded it was dead — with
// full employment, healthy people, and children growing up.
//
// Getting there took four structural corrections, each of which had been masquerading as
// a tuning problem:
//
//   - Gold could not enter the world. A conserved money supply always deadlocks, because
//     the coin ends up somewhere it cannot leave. Panning by the unemployed (§4.2) is the
//     faucet that fixes it.
//   - Retail ran through the wrong buildings. Characters could buy at the farm gate,
//     which cut the granary out of the trade it exists to conduct, so it never earned,
//     so it could not buy the harvest. Every coin ended up in the one farm nearest the
//     houses, which had no way to spend it.
//   - Nobody could support a dependant. In a closed circulation the total wage bill
//     exactly equals total spending on food, so there is no aggregate surplus anywhere —
//     a household with more mouths than earners cannot be fed from wages at all. Kitchen
//     gardens, which put food into the world without passing through money, are what
//     feed children.
//   - People shopped one meal at a time. A farmhand twenty cells from the granary spent
//     the working day walking there and back for a single dinner. They carry rations now.
//
// Two things remain honestly unfinished. Child mortality is still high — most children
// born do not reach adulthood, though enough now do to sustain the population. And the
// panning faucet, though in place and tested, is almost never exercised: with farms and a
// granary as the only employers there are about as many jobs as adults, so unemployment
// never appears to open it. It is insurance whose value will only show once there are more
// people than work — which is the situation more trades will create.
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
	Prospecting
	Panning
	Gardening
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
	case Prospecting:
		return "walking to the diggings"
	case Panning:
		return "panning for gold"
	case Gardening:
		return "tending the garden"
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
	// Rations are meals carried. People shop in batches and eat from what they carry,
	// which is what makes working somewhere other than the granary possible at all.
	Rations float32

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
	// Tools is the condition of the character's kit, 0 to 1. Good tools make a worker
	// more productive and wear out with use, which is what gives the material economy a
	// reason to exist and gold somewhere to go besides food (§4.2).
	Tools float32

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
	Stock  Stock   // what the structure holds, by resource
	Wage   float32 // gold per worker per tick
	Jobs   int     // positions offered
	Filled int     // positions currently taken

	// Condition is 0-100. Decay accelerates with disuse (§5.1); this milestone tracks
	// the value without yet letting structures fall to ruin.
	Condition float32

	// Residents counts assigned inhabitants, for homes.
	Residents int

	Alive bool

	// workCell is the cell an extraction structure is currently working, cached until it
	// is exhausted so the search for a new one runs rarely.
	workCell int32

	// Building is what a site will become when finished, along with how much of the work
	// is done. Meaningless on anything but a BuildSite.
	Building StructType
	Progress float32

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
	// Prices are gold per unit of each commodity.
	Prices Prices

	// freeChars recycles the slots of the dead, so the character slice does not grow
	// without bound over a long-running world.
	freeChars []CharID

	rng   *Rand
	paths *pathCache

	// Deaths and births accumulate for reporting.
	Births, DeathsAge, DeathsStarved int
	Built                            int
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
	s.inherit(id)
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
