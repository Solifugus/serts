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
// MILESTONE 3 STATUS — trades and a market exist; the village still does not grow.
//
// Materials, extraction, manufacture, tools, the wholesale chain, construction, and
// supply-and-demand pricing are all implemented and tested. What they have not yet
// produced is a village that gets bigger: it settles around eighteen to twenty people
// whether founded with twenty-four or thirty-four.
//
// Prices now move with coverage, wages are derived from revenue, and labour follows wages
// through the job utility function (§3.5) — the allocation mechanism the design asks for.
// In isolation each piece behaves: scarcity raises prices, glut lowers them, better-paying
// trades attract people, and work that cannot buy food repels them.
//
// What it has not done is replace PolicyWeight, the hardcoded "put people in the fields
// when the granary is low" that was added when trades first proved fatal. Removing it
// still collapses the village. The reason appears to be lag: price moves at four per cent
// a day, wages chase price, labour chases wages, and production chases labour, so the loop
// takes weeks to close while a food shortage becomes a famine in days. Raising the wage
// response from 0.04 to 0.35 nearly doubled the sustainable population, which suggests the
// remaining problem is loop gain rather than structure — but it is not solved, and
// PolicyWeight is still load-bearing.
//
// The other honest reading is that the material economy has little demand behind it.
// Tools are the only thing anyone can buy, and a poor village buys few, so the chain from
// mine to workshop to store barely turns over. A faction that bought things — buildings,
// stockpiles, wages for public work — would give it one, which is what a player with a
// budget is for.
//
// WHY THE VILLAGE DECLINES — diagnosed by following one man from his savings to his grave.
//
// He was healthy, employed, well fed, and going broke at a steady rate for years:
//
//	gold  165 -> 150 -> 136 -> 121 -> 106 -> 96 -> 81 -> 68 -> 53
//	      -> 39 -> 25 -> 11 -> 7 -> 5 -> 3 -> 1 -> 0.32 -> starving
//
// He earned three to four gold a day and his household of six ate nearly three. Five
// children, one earner. The whole village looks like this: sixteen children to nine
// adults, and in a closed circulation where food is the only thing anyone buys, total
// wages equal total food spending exactly. The average adult therefore earns almost
// precisely what it costs to feed their share of the village, and nothing more. There is
// no aggregate surplus anywhere for a dependant to live on.
//
// Kitchen gardens were meant to be that margin — food entering the world without passing
// through money — but at three quarters of a meal a day they cover about a seventh of what
// five children eat.
//
// So every household spends down its founding savings and then starves, and since the
// settlers all started with savings, they all run out at once. That is the decade of
// apparent health followed by collapse, and it is not a bug in any single place: the
// village breeds faster than its economy can feed, which is historically the ordinary
// condition of a subsistence village and makes for a dismal game.
//
// The levers are productivity, the birth rate, and garden yield. Which to pull is a design
// question, not a tuning one.
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

func (l LifeStage) String() string {
	switch l {
	case Child:
		return "child"
	case Adult:
		return "adult"
	case Elder:
		return "elder"
	}
	return "?"
}

// Character is one person. Every reference it holds is an ID, so the slice of them
// contains no pointers for the collector to scan.
type Character struct {
	Pos    torus.Vec2
	Age    float32 // in-world years
	Health float32 // 0-100; zero is death
	Hunger float32 // 0-100; rises constantly
	Gold   float64 // see Structure.Gold for why this is not float32
	// Rations are meals carried. People shop in batches and eat from what they carry,
	// which is what makes working somewhere other than the granary possible at all.
	Rations float32

	Home     StructID
	Job      StructID
	Partner  CharID
	Activity Activity

	// Mother and Father are who this person was born to, NoChar for the founding settlers
	// and for a parent who had already died when the child was conceived. Set once at
	// birth and never changed: a parent can die, but they do not stop being the parent.
	//
	// Eight bytes on a two-hundred-byte struct, and they are the difference between a log
	// of one person's transactions and a life with other people in it. Without them a
	// death appears only in the dead person's own diary — a man could be widowed twice and
	// orphaned once and his record would show three unexplained gaps. They also make
	// lineage answerable (whose line died out, who descends from the founders) and are
	// what a marriage market needs to stop matching siblings.
	//
	// IDs, not pointers, so Character stays pointer-free and the collector goes on
	// skipping the whole array (§9.2).
	//
	// An ID outlives the person: newChar recycles the slots of the dead, so a stored
	// parent ID can come to name a stranger born long afterwards. Never read these
	// directly — go through parentOf, which rejects a reissued slot by checking that the
	// occupant was born before the child. That test is exact here rather than merely
	// prudent: characters are created only at founding and at birth, founding finishes
	// before anyone has died, so every recycled slot necessarily holds someone born later
	// than any existing child.
	//
	// The zero value is a trap worth naming. CharID(0) is a real person, not an absence,
	// so both creation sites set these to NoChar explicitly and TestFoundersHaveNoParents
	// holds them to it.
	Mother, Father CharID

	// Skill is tenure-derived efficiency per structure type (§3.4). Kept per type so
	// that moving a farmer to other work is a real sacrifice.
	Skill [NumStructTypes]float32

	// Tenure is time served with the current employer, in in-world years. Roots grow
	// from it (§3.7), though only skill uses it this milestone.
	Tenure float32

	// NameSeed and FamilySeed generate a person's name on demand (names.go). Four bytes
	// each rather than two strings, because Character must stay pointer-free for the GC
	// (§9.2) and a name is needed only when something displays it.
	NameSeed, FamilySeed uint32
	// Traits is this character's personality, drawn at birth and inherited (see traits.go).
	Traits Traits
	// leanFor is how long the current job's wage has been below subsistence. It is the
	// memory that lets patience be a duration rather than a threshold.
	leanFor Tick

	Alive bool
	Sex   uint8 // 0 or 1; only reproduction reads it
	// Tools is the condition of the character's kit, 0 to 1. Good tools make a worker
	// more productive and wear out with use, which is what gives the material economy a
	// reason to exist and gold somewhere to go besides food (§4.2).
	Tools float32

	// newHome is a house this character has commissioned and is waiting on, so that a
	// couple saving for a home does not order a second one every day.
	newHome StructID

	// housed reports whether this character occupies a resident slot of their own.
	// Children live with their parents and take no slot until they grow up, at which
	// point they must find housing like anyone else — which is where a village that
	// built too few homes discovers the fact.
	housed bool

	// dest is the structure being walked to, if any.
	dest StructID
	// inHungerEpisode tracks whether the diary has already noted the current spell of
	// serious hunger, so an episode is one entry rather than two thousand.
	inHungerEpisode bool
	// bornAt lets the viewer report lineage without a separate registry, and is what age
	// is derived from.
	bornAt Tick
	// settler marks someone who arrived at founding rather than being born here.
	settler bool
	// Children ever fathered or borne, and the age at which they married. Kept so that a
	// completed life can be counted properly once it ends.
	Children  int
	marriedAt float32
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
	Type StructType
	Cell torus.Cell
	Pos  torus.Vec2 // centre, cached for distance work
	// Gold, Wage and revenue are float64, and the reason is measured rather than stylistic.
	//
	// float32 keeps ~7 significant digits wherever it sits on the number line, so its
	// absolute resolution degrades with magnitude. A tick's wage is about 0.00039 gold;
	// at a balance of 2,500 the smallest representable step is 0.00024, so a wage is 1.6
	// of them, and at 10,000 it is 0.4 — below half a step, which means the addition does
	// not happen at all. A worker holding ten thousand gold was measured receiving NOTHING
	// across eighty simulated days while their employer's till still emptied.
	//
	// That destroyed about 3.2% of the money supply every twenty years, roughly the rate
	// panning minted it, and it distorted the economics as well as the books: the richer a
	// character became, the less of their wage could physically land in their purse.
	//
	// float64 puts a wage at ~7e8 steps at the same balance. Integers in fixed point would
	// make conservation exact by construction and remain the honest long-term answer; this
	// is the change with the better ratio of benefit to risk. See docs/method.md note 20.
	Gold   float64
	Stock  Stock   // what the structure holds, by resource — a quantity, not a balance
	Wage   float64 // gold per worker per tick
	Jobs   int     // positions offered
	Filled int     // positions currently taken

	// Condition is 0-100. Decay accelerates with disuse (§5.1); this milestone tracks
	// the value without yet letting structures fall to ruin.
	Condition float32

	// Owner is the character who holds the business and takes its profits. NoChar means
	// nobody owns it — homes, build sites, and anything that has fallen vacant.
	Owner CharID
	// lastTrade is when the business last took money for anything. Idleness is measured
	// by trade rather than by staffing: a granary with nobody on the payroll that still
	// sells grain every day is a going concern, and winding it up churned ownership
	// nearly a thousand times in twenty years.
	lastTrade Tick

	// Residents counts assigned inhabitants, for homes.
	Residents int
	// Occupants is everyone living here including children, recounted daily. Computing it
	// on demand was an O(n^2) sweep every tick and brought the simulation to a halt.
	Occupants int
	// LastColony and LastColonySearch are per-settlement: each hall decides its own
	// expansion on its own cadence, so a world with several settlements can grow in
	// several places at once.
	LastColony, LastColonySearch Tick
	// Works is the hall's public-works fund: levied, saved, and spent on founding
	// settlements rather than on relief (§8.1a). Kept apart from Gold precisely so that
	// the day's poor cannot consume next decade's expansion.
	Works float64
	// FoodPrice is this settlement's own price of a meal — meaningful only on a town
	// hall, which is what makes a hall a market as well as a government (§4.3).
	// Food is grown and eaten locally, so its scarcity is local: a valley whose harvest
	// failed should not read the same price as the valley next door with full barns,
	// and until it does not, there is nothing for trade between them to respond to.
	FoodPrice float32
	// CrowdHeads is Occupants with children counted as half, for the disease crowding
	// term — children share beds, which is how large families fit small houses.
	// Recounted daily alongside Occupants; computing it per hazard check was O(n²).
	CrowdHeads float32

	Alive bool

	// revenue accumulates income since the last daily review, net of what was paid out to
	// acquire goods. Wages are set from it directly (§4.3).
	revenue float64

	// workCell is the cell an extraction structure is currently working, cached until it
	// is exhausted so the search for a new one runs rarely.
	workCell int32
	// workRetryAt caches the FAILED search: a site with nothing in reach does not look
	// again before this tick. The success-only cache was 73% of all simulation CPU — a
	// played-out camp re-scanned its full working disc (1,369 cells of torus arithmetic
	// for a lumber camp) every work tick of every worker, forever, and coppicing made
	// played-out-and-regrowing the ordinary rhythm of forestry rather than a rare state.
	workRetryAt Tick

	// Growing is the standing crop: labour already spent that will not become food until
	// harvest. It cannot be eaten, sold, or borrowed against.
	Growing float32

	// Level is how far a structure has been improved. Zero is as first built.
	Level int
	// Improving counts down the days of an upgrade in progress.
	Improving int

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
	Treasury float64
	// Prices are gold per unit of each commodity, and move with supply and demand (§4.3).
	Prices Prices
	// basePrices are the opening values, which bound how far prices may wander.
	basePrices Prices
	// consumed accumulates what has actually been used up since the last daily review;
	// demand is the smoothed per-day figure prices are steered against.
	//
	// Consumption is measured rather than inferred. Sales would be the easier proxy and
	// the wrong one: food eaten out of a household larder never passes through a shop,
	// and a commodity nobody can afford would read as a commodity nobody wants.
	consumed [NumResources]float32
	demand   [NumResources]float32

	// freeChars recycles the slots of the dead, so the character slice does not grow
	// without bound over a long-running world.
	freeChars []CharID

	rng *Rand
	// onDeath, when set, is called at the moment of each death with the character's final
	// state still intact. A diagnostic and, later, a game hook (obituaries, inheritance
	// disputes, vengeance).
	onDeath func(CharID, DeathCause)
	// Led accumulates the period's economic flows (ledger.go).
	Led Ledger
	// typeCache indexes living structures by type; nil means "rebuild on next use".
	// Invalidated whenever a structure is added or changes type (market.go).
	typeCache [][]StructID
	// diaries holds each character's life events when recording is enabled (diary.go).
	// Beside the simulation, not on Character, which stays pointer-free (§9.4).
	diaries map[CharID][]DiaryEntry
	// diaryTallies accumulates the repetitive events — job churn, relief — that are
	// summarised into one sentence rather than written a line at a time (diary.go).
	diaryTallies map[CharID]diaryTally
	// Quits counts why people stopped working somewhere, indexed by QuitReason. A
	// diagnostic counter in the manner of ColonyBlocked: the job churn was misdiagnosed
	// twice from plausible reasoning, and counting settles what arguing could not.
	Quits [NumQuitReasons]int
	// Road marks paved cells, one byte each, indexed like every other world field.
	// Non-nil once the world is founded (road.go).
	Road []uint8
	// traffic counts crossings per cell, decayed yearly. Unexported: it is the input to
	// paving, not a fact about the world anyone else should read.
	traffic []uint16
	// RoadsLaid counts paved cells, for the ledger and the viewer.
	RoadsLaid int
	// closed holds the diaries of the dead, filed away before their slots were reused.
	closed []ClosedDiary
	paths  *pathCache

	// Lives records every completed life, which is what the vital statistics are computed
	// over. A population is best judged by its dead.
	Lives []Life

	// Deaths and births accumulate for reporting.
	Births, DeathsAge, DeathsStarved int
	DeathsDisease, DeathsAccident    int
	Injuries                         int
	Built                            int
	HousesCommissioned               int
	Upgrades                         int
	Harvests                         int
	// BusinessSales counts changes of ownership through the market for businesses.
	BusinessSales int
	// BusinessesFounded counts new ventures commissioned by the wealthy.
	BusinessesFounded int
	// lastFarmFounding is when a farm was last commissioned against a harvest shortfall,
	// so one lean autumn founds one farm rather than sixty.
	lastFarmFounding Tick
	// ColonyBlocked counts, by cause, the years a colony was considered and refused.
	ColonyBlocked [numColonyBlocks]int
	// Caravans counts loads carried between settlements (caravan.go).
	Caravans int
	// Migrations counts households that moved between settlements (migration.go).
	Migrations int
	// Colonies counts founding parties sent (§2.7a).
	Colonies int
	// Consignments is the ledger of goods held by one business on behalf of another.
	Consignments []Consignment
	Harvested    float32

	// dbgWatch follows one character through their whole life, printing their state.
	// Lesson 1 of docs/method.md: watch one person, not the average.
	dbgWatch                    CharID
	dbgEvery                    Tick
	dbgLastAt                   Tick
	DeathsChild, DeathsHomeless int

	// What the founding actually managed to build, which may be less than was asked for
	// if the site could not take it.
	BuiltHomes, BuiltFarms, BuiltGranaries int
	// BuiltFisheries is surveyed from the site's water, not configured (see fisheriesWorth).
	BuiltFisheries int
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
	c.newHome = NoStruct
	if n := len(s.freeChars); n > 0 {
		id := s.freeChars[n-1]
		s.freeChars = s.freeChars[:n-1]
		s.Chars[id] = c
		return id
	}
	s.Chars = append(s.Chars, c)
	return CharID(len(s.Chars) - 1)
}

// kill removes a character from the world, releasing their job, home, and partner, and
// records the life that has ended.
func (s *State) kill(id CharID, cause DeathCause) {
	// The hook fires while the character's final state is still intact — job, home, purse,
	// position — because that state is the evidence. Post-mortem diagnosis from aggregates
	// has been wrong four times in this project; the death itself is the one moment the
	// truth is all in one place.
	if s.onDeath != nil {
		s.onDeath(id, cause)
	}
	s.diarise(id, "died of %s, aged %.1f (%.2f gold, hunger %.0f, health %.0f)",
		cause, s.Chars[id].Age, s.Chars[id].Gold, s.Chars[id].Hunger, s.Chars[id].Health)
	// A death belongs in more than one life: the widow, the parents who have outlived a
	// child, the children left with one parent or none. Recorded before inherit(), which
	// is what dissolves the marriage — after it there is no widow to write to (kin.go).
	s.noteBereavements(id, cause)
	s.inherit(id)
	c := &s.Chars[id]

	s.Lives = append(s.Lives, Life{
		Born:      c.bornAt,
		Age:       c.Age,
		Cause:     cause,
		Married:   c.marriedAt > 0,
		MarriedAt: c.marriedAt,
		Children:  c.Children,
		Settler:   c.settler,
	})

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
	// Close the diary before the slot can be handed to somebody else (diary.go).
	s.closeDiary(id)
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
