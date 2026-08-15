package sim

import (
	"fmt"
	"math"

	"github.com/solifugus/serts/internal/torus"
	"github.com/solifugus/serts/internal/worldgen"
)

// Config sets up a new simulation.
type Config struct {
	World *worldgen.World
	Seed  int64

	// Founding village.
	Homes     int
	Farms     int
	Granaries int
	Settlers  int
	Treasury  float32
	FoodPrice float32
	// StartingFood seeds the granary so the village does not starve before its first
	// harvest.
	StartingFood float32
}

// DefaultConfig returns a viable starting village.
func DefaultConfig(w *worldgen.World, seed int64) Config {
	return Config{
		World: w,
		Seed:  seed,
		// Sized so the village can employ nearly everyone it settles. With farms and a
		// granary as the only employers, food demand caps how many farmhands are worth
		// hiring; settling far beyond that guarantees a permanent underclass with no way
		// to earn. More trades (§5) are what will let a village grow past this.
		Homes:        6,
		Farms:        3,
		Granaries:    1,
		Settlers:     24,
		Treasury:     4000,
		FoodPrice:    0.9,
		StartingFood: 400,
	}
}

// New founds a village and returns the simulation.
func New(cfg Config) *State {
	s := &State{
		T:         cfg.World.T,
		World:     cfg.World,
		Seed:      cfg.Seed,
		Treasury:  cfg.Treasury,
		FoodPrice: cfg.FoodPrice,
		rng:       NewRand(cfg.Seed, 1),
		paths:     newPathCache(),
	}

	site := s.findSite()
	s.buildVillage(site, cfg)
	s.settle(site, cfg.Settlers)
	return s
}

// findSite picks somewhere worth founding a village.
//
// The criteria are the ones the design says matter: farmable soil, fresh water within
// reach to irrigate (§2.8), and room to build. Scanning on a stride rather than every
// cell keeps this cheap without materially changing the answer.
func (s *State) findSite() torus.Cell {
	w := s.World
	best, bestScore := torus.Cell{}, -1.0

	const stride = 3
	for y := 0; y < s.T.CY; y += stride {
		for x := 0; x < s.T.CX; x += stride {
			c := torus.Cell{X: x, Y: y}
			i := s.T.Index(c)
			if !w.Walkable(i) || w.Water[i] == worldgen.River {
				continue
			}
			if w.FreshDist[i] > Defs[Farm].MaxFreshDist {
				continue
			}

			// Score the neighbourhood, not the single cell: a village needs a patch.
			var soil float64
			var usable int
			for dy := -4; dy <= 4; dy++ {
				for dx := -4; dx <= 4; dx++ {
					n := s.T.Index(s.T.WrapCell(torus.Cell{X: x + dx, Y: y + dy}))
					if w.Walkable(n) {
						usable++
						soil += float64(w.Soil[n])
					}
				}
			}
			if usable < 60 {
				continue
			}
			score := soil / float64(usable)
			// Temperate ground is more forgiving to a new settlement.
			score *= 1 - math.Abs(float64(w.Temperature[i])-0.6)
			// Reachable gold is worth siting for (§4.2). It is a bonus rather than a
			// requirement: food and water decide whether a village can live at all,
			// while gold only decides whether it can mint its own money — and a village
			// without it is not doomed, merely dependent on trade for coin.
			score *= 1 + goldSiteBonus(w.GoldDist[i])
			if score > bestScore {
				best, bestScore = c, score
			}
		}
	}
	return best
}

// PanningRange is how far a prospector can usefully walk to gold and still get a day's
// work out of it.
//
// At four cells an hour and a twelve-hour day, someone who goes home at dusk can reach
// about ten cells out and back with time left to pan. Beyond that they spend the whole
// day walking, which is worse than useless — so a village sited past this distance from
// any deposit has no faucet at all.
const PanningRange = 10

// goldSiteBonus rewards a settlement site for having gold within reach.
func goldSiteBonus(dist int16) float64 {
	switch {
	case dist <= PanningRange:
		return 0.30 // a day's panning is genuinely available
	case dist <= PanningRange*3:
		return 0.10 // reachable for a determined prospector, or a later outpost
	default:
		return 0
	}
}

// buildVillage places the founding structures around a site.
func (s *State) buildVillage(site torus.Cell, cfg Config) {
	// place spirals outward from the site looking for somewhere buildable. The radius
	// keeps growing until the quota is met, because a village that silently builds one
	// farm instead of four does not fail loudly — it just starves later, for reasons
	// that look like an economy bug rather than a placement one.
	place := func(t StructType, want int, minR float64, minSoil float32) int {
		built := 0
		for r := minR; r <= 60 && built < want; r += 1.2 {
			steps := int(math.Max(10, r*5))
			for k := 0; k < steps && built < want; k++ {
				ang := 2 * math.Pi * float64(k) / float64(steps)
				c := s.T.WrapCell(torus.Cell{
					X: site.X + int(math.Round(r*math.Cos(ang))),
					Y: site.Y + int(math.Round(r*math.Sin(ang))),
				})
				if !CanPlace(s.World, t, c) || s.occupied(c) {
					continue
				}
				if minSoil > 0 && s.World.Soil[s.T.Index(c)] < minSoil {
					continue
				}
				s.addStructure(t, c)
				built++
			}
		}
		return built
	}

	s.BuiltGranaries = place(Granary, cfg.Granaries, 0, 0)
	s.BuiltHomes = place(Home, cfg.Homes, 2, 0)
	// Farms want decent ground, but not so decent that none can be found.
	s.BuiltFarms = place(Farm, cfg.Farms, 4, 0.40)
	if s.BuiltFarms < cfg.Farms {
		s.BuiltFarms += place(Farm, cfg.Farms-s.BuiltFarms, 4, 0)
	}

	// Seed the granary so the settlement survives to its first harvest, and endow the
	// employers with coin. Structures pay wages from their own reserves (§4.3), so an
	// unfunded granary cannot buy a harvest and an unfunded farm cannot hire.
	//
	// The endowment only has to get the wheel turning. Once it is, panning keeps money
	// entering the world through whoever is out of work (§4.2), so the settlement is not
	// living off its founding purse forever.
	var granary StructID = NoStruct
	for i := range s.Structs {
		if s.Structs[i].Type == Granary {
			granary = StructID(i)
			break
		}
	}
	if granary != NoStruct {
		s.Structs[granary].Food = cfg.StartingFood
		s.Structs[granary].Gold = cfg.Treasury * 0.6
	}
	farmShare := cfg.Treasury * 0.4 / float32(maxInt(s.BuiltFarms, 1))
	for i := range s.Structs {
		if s.Structs[i].Type == Farm {
			s.Structs[i].Gold = farmShare
		}
		if Defs[s.Structs[i].Type].Jobs > 0 {
			s.Structs[i].Wage = BaseWage
		}
	}
	s.Treasury = 0
}

// occupied reports whether a cell already holds a structure.
func (s *State) occupied(c torus.Cell) bool {
	for i := range s.Structs {
		if s.Structs[i].Alive && s.Structs[i].Cell == c {
			return true
		}
	}
	return false
}

// settle creates the founding population.
func (s *State) settle(site torus.Cell, n int) {
	centre := s.T.Center(site)
	for i := 0; i < n; i++ {
		// A spread of adult ages, so the village does not age as one cohort and die
		// together — a founding population all born in the same year produces a
		// demographic cliff one lifetime later.
		age := float32(s.rng.Range(AdultAge+2, ElderAge-6))
		// Settle on dry ground. Scattering blindly around the site drops people into
		// lakes, and a character standing in water is outside the walkable graph the
		// flow fields are built on.
		pos := centre
		for try := 0; try < 24; try++ {
			ang := s.rng.Float64() * 2 * math.Pi
			r := s.rng.Range(0, 8)
			p := s.T.Add(centre, torus.Vec2{X: r * math.Cos(ang), Y: r * math.Sin(ang)})
			if s.World.Walkable(s.T.Index(s.T.CellAt(p))) {
				pos = p
				break
			}
		}

		id := s.newChar(Character{
			Pos:      pos,
			Age:      age,
			Health:   100,
			Hunger:   float32(s.rng.Range(0, 30)),
			Gold:     float32(s.rng.Range(20, 60)),
			Home:     NoStruct,
			Job:      NoStruct,
			Partner:  NoChar,
			Activity: SeekingWork,
			Sex:      uint8(i % 2),
			dest:     NoStruct,
		})
		s.assignHome(id)
	}
}

// Step advances the simulation by one tick.
//
// The order matters and is deliberate: characters act first and accumulate their labour,
// then structures convert it, so output depends on who actually turned up rather than on
// who is merely on the payroll.
func (s *State) Step() {
	s.Tick++
	s.stepJobs()
	s.stepCharacters()
	s.stepStructures()
	s.stepBirths()
}

// RunTicks advances the simulation by n ticks.
func (s *State) RunTicks(n int) {
	for i := 0; i < n; i++ {
		s.Step()
	}
}

// Stats summarises the simulation for the HUD and for tuning runs.
type Stats struct {
	Tick                       Tick
	Date                       Date
	Population                 int
	Children                   int
	Adults                     int
	Elders                     int
	Employed                   int
	Unemployment               float64
	Food                       float32
	Treasury                   float32
	GoldHeld                   float32
	GoldInGround               float64
	TotalCoin                  float64
	Panning                    int
	AvgHunger                  float32
	AvgHealth                  float32
	AvgAge                     float32
	AvgGold                    float32
	Foraging                   int
	Births                     int
	DeathsAge                  int
	DeathsStarve               int
	DeathsChild                int
	DeathsHomeless             int
	AvgLarder                  float32
	FeedCalls, FeedOK, BadHome int
	PathHits                   int
	PathMisses                 int
}

// DumpStructures returns a line per structure, for diagnosing the economy.
func (s *State) DumpStructures() string {
	out := ""
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive {
			continue
		}
		out += fmt.Sprintf("  %-8s gold %8.1f food %6.1f wage %.5f staff %d/%d\n",
			st.Type, st.Gold, st.Food, st.Wage, st.Filled, st.Jobs)
	}
	return out
}

// TotalCoin is every piece of gold in existence: what characters carry, what structures
// hold, and what is still in the ground.
//
// Gold can only enter the world by being panned out of the ground, and can only leave via
// upkeep and the share of an estate lost on death (§4.2). This total must therefore never
// rise. It is the sharpest available check that the economy is not quietly minting money.
func (s *State) TotalCoin() float64 {
	var held float64
	for i := range s.Chars {
		if s.Chars[i].Alive {
			held += float64(s.Chars[i].Gold)
		}
	}
	for i := range s.Structs {
		if s.Structs[i].Alive {
			held += float64(s.Structs[i].Gold)
		}
	}
	return held + s.World.TotalGold()
}

// Stats computes a summary of the current state.
func (s *State) Stats() Stats {
	st := Stats{
		Tick:           s.Tick,
		Date:           s.Tick.Date(),
		Food:           s.TotalFood(),
		Treasury:       s.Treasury,
		Births:         s.Births,
		DeathsAge:      s.DeathsAge,
		DeathsStarve:   s.DeathsStarved,
		DeathsChild:    s.DeathsChild,
		DeathsHomeless: s.DeathsHomeless,
		PathHits:       s.paths.hits,
		PathMisses:     s.paths.misses,
	}
	var hunger, health, age, gold float64
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive {
			continue
		}
		st.Population++
		switch c.Stage() {
		case Child:
			st.Children++
		case Adult:
			st.Adults++
		default:
			st.Elders++
		}
		if c.Job != NoStruct {
			st.Employed++
		}
		hunger += float64(c.Hunger)
		health += float64(c.Health)
		age += float64(c.Age)
		gold += float64(c.Gold)
		st.GoldHeld += c.Gold
		switch c.Activity {
		case Idle:
			st.Foraging++
		case Panning, Prospecting:
			st.Panning++
		}
	}
	if st.Population > 0 {
		n := float64(st.Population)
		st.AvgHunger = float32(hunger / n)
		st.AvgHealth = float32(health / n)
		st.AvgAge = float32(age / n)
		st.AvgGold = float32(gold / n)
	}
	var larder float32
	var homes int
	for i := range s.Structs {
		if s.Structs[i].Alive && s.Structs[i].Type == Home {
			larder += s.Structs[i].Food
			homes++
		}
	}
	if homes > 0 {
		st.AvgLarder = larder / float32(homes)
	}
	for i := range s.Structs {
		if s.Structs[i].Alive {
			st.GoldHeld += s.Structs[i].Gold
		}
	}
	st.GoldInGround = s.World.TotalGold()
	st.TotalCoin = s.TotalCoin()
	st.Unemployment = s.Unemployment()
	return st
}

func (st Stats) String() string {
	return fmt.Sprintf(
		"%s | pop %d (%dc/%da/%de) empl %d (%.0f%% jobless, %d foraging) | food %.0f | gold %.1f | hunger %.0f health %.0f | +%d births -%d age -%d starved",
		st.Date, st.Population, st.Children, st.Adults, st.Elders,
		st.Employed, st.Unemployment*100, st.Foraging,
		st.Food, st.AvgGold, st.AvgHunger, st.AvgHealth,
		st.Births, st.DeathsAge, st.DeathsStarve) +
		fmt.Sprintf(" [child %d, larder %.1f | coin %.0f circulating + %.0f in ground = %.0f total, %d panning]",
			st.DeathsChild, st.AvgLarder, st.GoldHeld, st.GoldInGround, st.TotalCoin, st.Panning)
}
