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
	Camps     int // lumber camps
	Quarries  int
	Mines     int
	Settlers  int
	// Industry controls whether the material economy is founded at all. Being able to
	// switch it off is what makes it possible to tell a food problem from a trade
	// problem, rather than changing several things at once and guessing.
	Industry  bool
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
		Homes:     8,
		Farms:     3,
		Granaries: 1,
		Camps:     1,
		Quarries:  1,
		Mines:     1,
		Industry:  true,
		Settlers:  34,
		Treasury:  6000,
		FoodPrice: 0.9,
		// Provisioned from the size of the settlement rather than a flat figure. Four
		// hundred units is eight days for thirty-four people: the founding village was
		// starting in famine, the price hit its ceiling within a fortnight, and a third
		// of the settlers died before the first crop was in. Nothing they or the market
		// did could have prevented it.
		StartingFood: 0, // computed in New from the settlement's size
	}
}

// New founds a village and returns the simulation.
func New(cfg Config) *State {
	s := &State{
		T:          cfg.World.T,
		World:      cfg.World,
		Seed:       cfg.Seed,
		Treasury:   cfg.Treasury,
		Prices:     DefaultPrices(),
		basePrices: DefaultPrices(),
		rng:        NewRand(cfg.Seed, 1),
		paths:      newPathCache(),
	}

	site := s.findSite()
	s.buildVillage(site, cfg)
	s.settle(site, cfg.Settlers)
	s.countHouseholds()
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
	if cfg.Industry {
		place(Storehouse, 1, 1, 0)
		place(Workshop, 1, 2, 0)
		place(Store, 1, 2, 0)
	}
	s.BuiltHomes = place(Home, cfg.Homes, 3, 0)
	// Farms want decent ground, but not so decent that none can be found.
	s.BuiltFarms = place(Farm, cfg.Farms, 4, 0.40)
	if s.BuiltFarms < cfg.Farms {
		s.BuiltFarms += place(Farm, cfg.Farms-s.BuiltFarms, 4, 0)
	}

	// Extraction has to go where the material is, not where the village is. Each site is
	// placed on the best ground its industry can find within reach, which is why a mine
	// so often ends up further out than anything else — iron is in the hills and the
	// village is on the flat (§2.5).
	if cfg.Industry {
		s.placeExtractor(site, LumberCamp, cfg.Camps)
		s.placeExtractor(site, Quarry, cfg.Quarries)
		s.placeExtractor(site, Mine, cfg.Mines)
		s.placeDiningHalls(site)
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
		food := cfg.StartingFood
		if food <= 0 {
			// Enough to feed everyone well past the coverage the market expects, so the
			// settlement begins solvent rather than in a famine of its own making.
			// Sized from what the settlement actually eats, not from the coverage target.
			// The target counts days of *market* demand, and since households grow most
			// of their own food the market handles only a fraction of what is consumed —
			// so provisioning by that figure over-supplied the village tenfold. Enough
			// here to carry everyone past a first harvest and well into the year after.
			food = float32(cfg.Settlers) * MealsPerDay * 1.4 * DaysPerYear
		}
		s.Structs[granary].Stock[Food] = food
	}

	// Split the endowment across everyone who has to make payroll or buy stock, weighted
	// so that middlemen get more: they buy before they sell, and one with no coin cannot
	// take what a producer makes, which stalls the chain at its first link.
	//
	// The shares are normalised rather than handed out at fixed sizes. An earlier version
	// gave everyone a slice and then topped up the traders, which quietly handed out more
	// than the faction actually had — minting some two and a half thousand coin at the
	// moment of founding.
	weight := func(t StructType) float32 {
		switch t {
		case Granary, Storehouse, Store:
			return 3
		case Workshop:
			return 2
		default:
			if Defs[t].Jobs > 0 {
				return 1
			}
			return 0
		}
	}
	var total float32
	for i := range s.Structs {
		total += weight(s.Structs[i].Type)
	}
	if total <= 0 {
		total = 1
	}
	for i := range s.Structs {
		if w := weight(s.Structs[i].Type); w > 0 {
			s.Structs[i].Gold = cfg.Treasury * w / total
			s.Structs[i].Wage = BaseWage
		}
	}
	s.Treasury = 0
}

// placeExtractor puts an extraction site on the richest ground it can reach.
//
// Unlike a farm or a house, these cannot be sited by preference: a lumber camp with no
// timber inside its working radius employs people to do nothing. The search therefore
// scores candidate cells by what is actually in the ground around them.
func (s *State) placeExtractor(site torus.Cell, t StructType, count int) {
	r := resourceOf(t)
	const searchRadius = 42

	for n := 0; n < count; n++ {
		best, bestScore := torus.Cell{}, 0.0
		for dy := -searchRadius; dy <= searchRadius; dy += 2 {
			for dx := -searchRadius; dx <= searchRadius; dx += 2 {
				c := s.T.WrapCell(torus.Cell{X: site.X + dx, Y: site.Y + dy})
				if !CanPlace(s.World, t, c) || s.occupied(c) {
					continue
				}
				// Sum what lies within the site's working radius.
				var have float64
				for wy := -WorkRadius; wy <= WorkRadius; wy += 2 {
					for wx := -WorkRadius; wx <= WorkRadius; wx += 2 {
						i := s.T.Index(s.T.WrapCell(torus.Cell{X: c.X + wx, Y: c.Y + wy}))
						if g := s.groundAt(r, i); g != nil {
							have += float64(*g)
						}
					}
				}
				if have <= 0 {
					continue
				}
				// Prefer richer ground, but not at any distance: hauling costs.
				dist := s.T.Dist(s.T.Center(site), s.T.Center(c))
				score := have / (1 + dist/18)
				if score > bestScore {
					best, bestScore = c, score
				}
			}
		}
		if bestScore <= 0 {
			return // this world has none of it within reach
		}
		s.addStructure(t, best)
	}
}

// DiningHallRange is how far a work site may be from the nearest place to eat before it
// wants a kitchen of its own.
const DiningHallRange = 9

// placeDiningHalls puts a kitchen beside any work site too far from the village to walk
// back for a meal.
//
// Extraction cannot be sited by preference — the timber, stone, and ore are where they
// are — so the food has to follow the work rather than the other way round. Without it the
// only defence against people starving at remote sites is forbidding them the job, which
// caps how far a settlement can reach and works against the migration the settlement cycle
// depends on (§2.7).
func (s *State) placeDiningHalls(site torus.Cell) {
	centre := s.T.Center(site)
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive {
			continue
		}
		switch st.Type {
		case LumberCamp, Quarry, Mine:
		default:
			continue
		}
		if s.T.Dist(st.Pos, centre) <= DiningHallRange {
			continue // close enough to eat at home
		}
		if hall := s.nearestOfType(st.Pos, DiningHall); hall != NoStruct &&
			s.T.Dist(st.Pos, s.Structs[hall].Pos) <= DiningHallRange {
			continue // already served by a neighbouring site's kitchen
		}
		if c, ok := s.freeCellNear(st.Cell, DiningHall, 4); ok {
			s.addStructure(DiningHall, c)
		}
	}
}

// freeCellNear finds somewhere buildable within a few cells of a target.
func (s *State) freeCellNear(around torus.Cell, t StructType, radius int) (torus.Cell, bool) {
	for r := 1; r <= radius; r++ {
		steps := maxInt(8, r*6)
		for k := 0; k < steps; k++ {
			ang := 2 * math.Pi * float64(k) / float64(steps)
			c := s.T.WrapCell(torus.Cell{
				X: around.X + int(math.Round(float64(r)*math.Cos(ang))),
				Y: around.Y + int(math.Round(float64(r)*math.Sin(ang))),
			})
			if CanPlace(s.World, t, c) && !s.occupied(c) {
				return c, true
			}
		}
	}
	return torus.Cell{}, false
}

// occupied reports whether a cell already holds a structure.
func (s *State) occupied(c torus.Cell) bool { return s.Occupied(c) }

// Occupied reports whether a cell already holds a structure, for callers placing things.
func (s *State) Occupied(c torus.Cell) bool {
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
			Traits:   rollTraits(s.rng),
		})
		// Settlers were not born at the world's first tick, so their birthday is backdated
		// to match the age they arrive with. Everything else derives age from this.
		s.Chars[id].bornAt = Tick(-float64(age) * TicksPerYear)
		s.Chars[id].settler = true
		s.assignHome(id)
	}
	s.distributeOwnership()
}

// distributeOwnership hands the founding businesses to settlers, one each, spreading them
// so that no one person begins with the whole village.
//
// Somebody has to hold them. An unowned business has no one to take its profits, no one to
// wind it up when it fails, and nobody to sell it to — which is the state the economy was
// in, with gold piling up inside buildings that belonged to nobody.
func (s *State) distributeOwnership() {
	var adults []CharID
	for i := range s.Chars {
		if s.Chars[i].Alive && s.Chars[i].Stage() != Child {
			adults = append(adults, CharID(i))
		}
	}
	if len(adults) == 0 {
		return
	}
	next := 0
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || Defs[st.Type].Jobs == 0 || st.Type == BuildSite || st.Owner != NoChar {
			continue
		}
		st.Owner = adults[next%len(adults)]
		next++
	}
}

// Step advances the simulation by one tick.
//
// The order matters and is deliberate: characters act first and accumulate their labour,
// then structures convert it, so output depends on who actually turned up rather than on
// who is merely on the payroll.
func (s *State) Step() {
	s.Tick++
	if s.Tick%TicksPerDay == 0 {
		s.countHouseholds()
		// Owners take what their trade has earned, and give up what has stopped earning.
		s.drawProfits()
		s.reviewBusinesses()
		s.layOffOutOfSeason()
		s.hireServants()
	}
	s.stepJobs()
	s.stepCharacters()
	s.stepStructures()
	s.stepBirths()
	// Improvement is considered before departure: a family with the means extends the
	// house it has, and only one that cannot afford to sees its children leave.
	s.stepUpgrades()
	s.stepHouseholds()
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
	HousesBuilt, HousesOrdered int
	Upgrades                   int
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

// Watch follows a single character, printing their full state at intervals until they die.
func (s *State) Watch(id CharID, every Tick) {
	s.dbgWatch, s.dbgEvery = id, every
}

// WatchNewborn follows the youngest child in the village, to see whether it is being
// looked after.
func (s *State) WatchNewborn() CharID {
	best, bestAge := NoChar, float32(1e9)
	for i := range s.Chars {
		c := &s.Chars[i]
		if c.Alive && !c.settler && c.Age < bestAge {
			best, bestAge = CharID(i), c.Age
		}
	}
	s.dbgWatch = best
	return best
}

// WatchYoungest follows whoever has most recently come of age.
func (s *State) WatchYoungest() CharID {
	best, bestAge := NoChar, float32(1e9)
	for i := range s.Chars {
		c := &s.Chars[i]
		if c.Alive && c.Age >= AdultAge && c.Age < bestAge {
			best, bestAge = CharID(i), c.Age
		}
	}
	s.dbgWatch = best
	return best
}

// dbgTrace prints everything about the watched character.
func (s *State) dbgTrace(id CharID) {
	if s.dbgEvery <= 0 || s.Tick-s.dbgLastAt < s.dbgEvery {
		return
	}
	s.dbgLastAt = s.Tick
	c := &s.Chars[id]
	if !c.Alive {
		fmt.Printf("  [#%d DIED at age %.1f]\n", id, c.Age)
		s.dbgWatch = NoChar
		return
	}
	job, wage := "none", float32(0)
	if c.Job != NoStruct {
		job, wage = s.Structs[c.Job].Type.String(), s.Structs[c.Job].Wage
	}
	larder := float32(-1)
	if c.Home != NoStruct {
		larder = s.Structs[c.Home].Stock[Food]
	}
	kids := 0
	if c.Home != NoStruct {
		for i := range s.Chars {
			o := &s.Chars[i]
			if o.Alive && o.Home == c.Home && o.Stage() == Child {
				kids++
			}
		}
	}
	// Who else lives in this house, and are any of them able to feed it?
	adults, providers := 0, 0
	var homeFood, granaryFood float32
	if c.Home != NoStruct {
		homeFood = s.Structs[c.Home].Stock[Food]
		for i := range s.Chars {
			o := &s.Chars[i]
			if o.Alive && o.Home == c.Home && o.Stage() != Child {
				adults++
				if o.Gold >= s.Prices[Food] {
					providers++
				}
			}
		}
	}
	if g := s.NearestFoodSource(c.Pos); g != NoStruct {
		granaryFood = s.Structs[g].Stock[Food]
	}
	fmt.Printf("  y%-3d age %5.2f hun %3.0f hp %3.0f larder %6.1f adults %d (%d with money) shop has %7.1f  %s\n",
		int(s.Tick.Years()), c.Age, c.Hunger, c.Health, homeFood, adults, providers,
		granaryFood, c.Activity)
	_ = kids
	_ = job
	_ = wage
	_ = larder
}

// DumpAges reports the age structure and who is dying of what.
//
// A population that never ages is a population dying of something else, and the summary
// line hides it: "0 elders" reads as a detail rather than as the whole story.
func (s *State) DumpAges() string {
	var buckets [9]int // decades
	oldest := 0.0
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive {
			continue
		}
		b := int(c.Age / 10)
		if b > 8 {
			b = 8
		}
		buckets[b]++
		if float64(c.Age) > oldest {
			oldest = float64(c.Age)
		}
	}
	out := "  ages:"
	for i, n := range buckets {
		if n > 0 {
			out += fmt.Sprintf("  %d0s:%d", i, n)
		}
	}
	return out + fmt.Sprintf("   oldest %.0f   deaths: %d age, %d hunger, %d disease, %d accident (%d injured), %d children",
		oldest, s.DeathsAge, s.DeathsStarved, s.DeathsDisease, s.DeathsAccident, s.Injuries, s.DeathsChild)
}

// DumpMarket reports prices, stocks, and how many days of each the faction holds. It is
// the readout that says whether supply and demand is doing anything.
func (s *State) DumpMarket() string {
	out := "  market:"
	for r := Resource(0); r < NumResources; r++ {
		out += fmt.Sprintf("  %s %.2f (%.0fu, %.0fd)",
			r, s.Prices[r], s.StockOf(r), s.Coverage(r))
	}
	out += fmt.Sprintf("\n  wages:")
	seen := map[StructType]bool{}
	for i := range s.Structs {
		st := &s.Structs[i]
		if st.Alive && Defs[st.Type].Jobs > 0 && !seen[st.Type] {
			seen[st.Type] = true
			out += fmt.Sprintf("  %s %.4f", st.Type, st.Wage)
		}
	}
	out += fmt.Sprintf("   subsistence %.4f", s.SubsistenceWage())
	return out
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
			st.Type, st.Gold, st.Stock[Food], st.Wage, st.Filled, st.Jobs)
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
		HousesBuilt:    s.Built,
		HousesOrdered:  s.HousesCommissioned,
		Upgrades:       s.Upgrades,
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
			larder += s.Structs[i].Stock[Food]
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
		fmt.Sprintf(" [child %d, houses %d built of %d ordered, %d upgrades, larder %.1f | coin %.0f circulating + %.0f in ground = %.0f total, %d panning]",
			st.DeathsChild, st.HousesBuilt, st.HousesOrdered, st.Upgrades, st.AvgLarder, st.GoldHeld, st.GoldInGround, st.TotalCoin, st.Panning)
}
