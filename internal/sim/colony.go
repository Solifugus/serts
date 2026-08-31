package sim

import "github.com/solifugus/serts/internal/torus"

// Daughter settlements (§2.7a): how growth crosses the map.
//
// Everything here is built under Appendix A's constraints. The party must clear critical
// mass, because a colony founded small is founded dying (A.1). It leaves equipped —
// provisions bought with real gold from real granaries, a treasury raised from the hall
// and the wealthy — because the founding transient is survivable only with capital, and
// because conservation is not negotiable: nothing here conjures a coin or a meal.
//
// Two abstractions are inherited from the original founding and flagged rather than
// hidden. Colony structures are granted at arrival exactly as the mother village's were
// at world-start — the construction economy exists and should eventually raise them
// plank by plank, but a re-founding uses the founding's own abstraction until haulage is
// employment (§5). And the party arrives rather than treks: group travel with feeding en
// route is real design work that belongs with roads. Both debts are recorded here so
// they are owed, not forgotten.

// Why a colony did not leave. Seed 5 reached five hundred people and founded nothing in
// a hundred and twenty years; a refusal repeated fifty times is a gate, not luck, and
// counting them is cheaper than guessing which.
const (
	BlockSmallMother = iota
	BlockNoPressure
	BlockNoSite
	BlockNoParty
	BlockNoPurse
	numColonyBlocks
)

var ColonyBlockNames = [numColonyBlocks]string{
	"mother too small", "no pressure", "no site", "no party", "no purse",
}

const (
	// ColonyParty is the founding party size, set from the measured survival curve rather
	// than guessed: thirty foundings at five sizes put growth at 3/6 for 28 settlers, 5/6
	// for 44, 6/6 for 60 — and crucially NOTHING died at any size. An undersized
	// settlement stagnates near its founding number; it does not perish.
	//
	// Forty-four from a mother of a hundred was tried on that reasoning and was ruinous:
	// 2,468 people fell to 633 and seed 5 to EIGHT. The error was transferring the
	// isolated-founding curve to a quantity it does not describe. A fresh village of 56
	// grows five-in-six; a MOTHER reduced to 56 is a different animal — she keeps
	// full-sized infrastructure to staff, has just had her fortunes halved to fund the
	// wagons, and requalifies to send another party as soon as she recovers to a hundred.
	//
	// A survival curve for foundings says nothing about how much a going concern can
	// lose.
	ColonyParty = 60

	// ColonyMinPop is the smallest mother that can send a party and remain above critical
	// mass herself: a hundred sends forty-four and keeps fifty-six, which the same curve
	// puts at five-in-six.
	ColonyMinPop = 130

	// FissionBase is the population around which a village splinters even with food in
	// hand — the valley is simply full of people. Ethnographically villages fission
	// somewhere between 150 and 200, and the spread is as real as the number: some
	// communities tolerate crowding others will not.
	//
	// So this is a base, not a threshold. Each settlement's own figure is this scaled by
	// the mean Rootedness of its adults (see fissionPoint): restless populations splinter
	// early, clannish ones endure, and the variation is causal rather than noise.
	FissionBase = 175.0

	// ColonyCooldown is the least time between colonies from one world: a generation to
	// recover the people and the purse.
	ColonyCooldown = 15 * TicksPerYear

	// ColonyMinPurse is the treasury a colony must raise before it may leave: enough to
	// endow a granary and pay the first years' wages. A party that cannot raise it
	// waits — leaving broke is the measured founding disaster.
	ColonyMinPurse = 2500.0

	// ColonyMinDist keeps the daughter out of the mother's working radius — new valley,
	// not a suburb.
	ColonyMinDist = 40.0

	// ExogamyRange is how far the marriage market reaches between settlements. Two
	// villages within it pool their eligible singles — the direct answer to the Allee
	// threshold (A.1), and historically exactly why isolated hamlets exchanged spouses.
	ExogamyRange = 120.0
)

// considerColony weighs founding a daughter settlement, settlement by settlement, on the
// post-harvest cadence: the one moment of the year the stores speak honestly.
//
// Every gate here once read WORLD totals — the world's population against ColonyMinPop,
// the world's food against its need — which was right while a world was one village and
// wrong the moment it was two. Two settlements of a hundred read as two hundred and pass
// a gate neither could pass alone; a valley in famine reads as comfortable because its
// neighbour has full barns. It is the same fault that made farms get founded in the
// wrong valley, one level up, and it is why a world would expand exactly once: after the
// first colony, no single settlement was ever judged on its own account again.
func (s *State) considerColony() {
	if d := s.Tick.Date().Day; d < HarvestDay {
		return
	}
	for _, v := range s.Settlements() {
		s.considerColonyFrom(v)
	}
}

// considerColonyFrom weighs whether ONE settlement should send a founding party.
func (s *State) considerColonyFrom(v Settlement) {
	hall := &s.Structs[v.Hall]
	if s.Tick-hall.LastColony < ColonyCooldown && hall.LastColony > 0 {
		return
	}
	if v.Pop < ColonyMinPop {
		s.ColonyBlocked[BlockSmallMother]++
		return
	}
	// Two reasons the restless go: this settlement's own stores cannot cover its own year
	// even at their annual peak, or the sheer press of its own numbers.
	//
	// Removing the famine reason was tried, on the reasoning that harvests track staffing
	// and a hungry village should keep its harvesters rather than march them over the
	// horizon. Measured, it cost more than half the world: 2,468 people to 1,064.
	//
	// The reasoning missed what a famine party actually does. It does not redistribute
	// food, it opens NEW LAND — the hungry leave and put a fresh valley under crop, which
	// adds production rather than moving it. That is why famine drove colonisation
	// historically, and why moving people beats moving grain here: caravans fired for the
	// first time in the experiment (54 loads) and could not make up the difference.
	//
	// Fourth mechanism in this project to look backwards and prove load-bearing.
	need := float64(v.Pop) * MealsPerDay * DaysPerYear
	shortfall := float64(v.MarketFood) < need*0.6
	if !shortfall && float64(v.Pop) < s.fissionPoint(v.Hall) {
		s.ColonyBlocked[BlockNoPressure]++
		return
	}

	// Survey at most once a year per settlement. The search scans the whole map, and
	// running it every eligible day cost two five-hour measurement runs.
	if s.Tick-hall.LastColonySearch < TicksPerYear && hall.LastColonySearch > 0 {
		return
	}
	hall.LastColonySearch = s.Tick

	site, ok := s.colonySite()
	if !ok {
		s.ColonyBlocked[BlockNoSite]++
		return
	}
	party := s.selectParty(v.Hall)
	if len(party) < ColonyParty {
		s.ColonyBlocked[BlockNoParty]++
		return
	}
	// Count the money BEFORE taking any of it.
	//
	// This used to raise the funds and then test the total, and a failed test returned
	// without giving anything back — so an attempt that fell short had already emptied the
	// hall's works fund and taken half of every resident's savings above the levy floor,
	// and all of it simply ceased to exist. Two such attempts in twenty years destroyed
	// 1,765 coin, 3.4% of the money supply, with no colony founded and nothing recorded.
	//
	// It was invisible to every short audit: the per-tick economy conserves exactly over
	// eight years, and every daily phase conserves exactly over five, because the fault
	// fires twice a generation and only when an attempt gets as far as the purse and
	// fails there.
	if s.estimateColonyFunds(v.Hall) < ColonyMinPurse {
		s.ColonyBlocked[BlockNoPurse]++
		return
	}
	purse, provisions := s.raiseColonyFunds(v.Hall, party)
	if purse < ColonyMinPurse {
		// The estimate said it was there and the collection disagrees, which should not
		// happen — but if it ever does, the money has been taken and must go somewhere
		// rather than vanishing. It goes back to the hall that levied it.
		s.Structs[v.Hall].Works += purse
		s.ColonyBlocked[BlockNoPurse]++
		return
	}

	s.foundColony(site, party, purse, provisions)
	s.Structs[v.Hall].LastColony = s.Tick
	s.Colonies++
}

// fissionPoint is the population at which THIS settlement splinters: the ethnographic
// base scaled by how rooted its people are.
//
// The consequence is a chain that no constant could produce. Founding parties are chosen
// by low Rootedness — the restless go first (§3.7) — and Rootedness is heritable, so a
// daughter settlement is founded by wanderers, inherits their restlessness, and splinters
// at a lower population than its mother did. Expansion compounds through selection rather
// than through a threshold anyone lowered, and a frontier settled by the restless keeps
// moving while the old country stays put.
func (s *State) fissionPoint(hall StructID) float64 {
	var sum float32
	n := 0
	for i := range s.Chars {
		c := &s.Chars[i]
		if c.Alive && c.Stage() != Child && s.marketHall(c.Pos) == hall {
			sum += c.Traits.Rootedness
			n++
		}
	}
	if n == 0 {
		return FissionBase
	}
	return FissionBase * float64(sum/float32(n))
}

// colonySite finds a new valley: the same scoring the world's first village used,
// excluding everything near an existing settlement.
func (s *State) colonySite() (torus.Cell, bool) {
	best, bestScore := torus.Cell{}, -1.0
	const stride = 4
	for y := 0; y < s.T.CY; y += stride {
		for x := 0; x < s.T.CX; x += stride {
			c := torus.Cell{X: x, Y: y}
			i := s.T.Index(c)
			w := s.World
			if !w.Walkable(i) || w.FreshDist[i] > Defs[Farm].MaxFreshDist {
				continue
			}
			// Not in anyone's valley already. Halls only, through the type index: a
			// settlement is where its seat is, and scanning every structure here was
			// part of what made this search ruinous.
			tooClose := false
			for _, hid := range s.halls() {
				if s.T.Dist(s.T.Center(c), s.Structs[hid].Pos) < ColonyMinDist {
					tooClose = true
					break
				}
			}
			if tooClose {
				continue
			}
			var soil float64
			usable := 0
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
			// Water counts as land does. A shore with poor ground can still feed a
			// settlement — year round, which farmland cannot — so scoring on soil
			// alone was refusing every coast and lakeside on the map.
			if fish := waterWithin(w, c, FisheryReach); fish > 0 {
				bonus := fish / 40
				if bonus > 0.35 {
					bonus = 0.35
				}
				score += bonus
			}
			if score > bestScore {
				best, bestScore = c, score
			}
		}
	}
	return best, bestScore > 0.35 // decent farmland or nothing: a colony on scrub starves
}

// selectParty picks who goes: the restless first — Rootedness doing the job it was
// defined for (§3.7) — whole households at a time, so no child is left behind and no
// couple is split. Deterministic order throughout.
func (s *State) selectParty(hall StructID) []CharID {
	// Rank adult households by the minimum Rootedness of their adults: the household
	// leaves when its most restless member wins the argument.
	type cand struct {
		id   CharID
		root float32
	}
	var cands []cand
	taken := map[CharID]bool{}
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Stage() == Child || taken[CharID(i)] {
			continue
		}
		if s.marketHall(c.Pos) != hall {
			continue // a settlement sends its own people, not its neighbour's
		}
		r := c.Traits.Rootedness
		if c.Partner != NoChar && s.AliveChar(c.Partner) {
			if pr := s.Chars[c.Partner].Traits.Rootedness; pr < r {
				r = pr
			}
			taken[c.Partner] = true
		}
		taken[CharID(i)] = true
		cands = append(cands, cand{CharID(i), r})
	}
	// Stable order: rootedness ascending, ID ascending on ties (§9.2).
	for i := 1; i < len(cands); i++ {
		for j := i; j > 0 && (cands[j].root < cands[j-1].root ||
			(cands[j].root == cands[j-1].root && cands[j].id < cands[j-1].id)); j-- {
			cands[j], cands[j-1] = cands[j-1], cands[j]
		}
	}

	var party []CharID
	add := func(id CharID) {
		if id != NoChar && s.AliveChar(id) {
			party = append(party, id)
		}
	}
	for _, cd := range cands {
		if len(party) >= ColonyParty {
			break
		}
		c := &s.Chars[cd.id]
		add(cd.id)
		add(c.Partner)
		// Children of the household come with their parents.
		if c.Home != NoStruct {
			for j := range s.Chars {
				k := &s.Chars[j]
				if k.Alive && k.Home == c.Home && k.Stage() == Child {
					add(CharID(j))
				}
			}
		}
	}
	return party
}

// raiseColonyFunds gathers the treasury and buys the provisions, all with real money.
//
// The hall gives what it holds beyond a working float; the wealthy sponsor the rest,
// proportionally above their own reserves. Provisions are bought from the granaries at
// the market price — food and coin both genuinely move.
// purse is money and float64; provisions is a quantity of food and stays float32.
func (s *State) raiseColonyFunds(hall StructID, party []CharID) (purse float64, provisions float32) {
	// The sending settlement's own works fund — money levied there and deliberately
	// saved for this — not the treasury that pays its relief and clerks, and not its
	// neighbours' savings.
	if h := &s.Structs[hall]; h.Works > 0 {
		purse += h.Works
		h.Works = 0
	}
	// Sponsors: everyone wealthy, whether they go or stay — the stayers buy their kin's
	// future; the leavers carry their own.
	//
	// Narrowing this to the departing party alone was tried, on the reasoning that a man
	// staying home should not be emptied to buy another man's wagons. Measured, it was
	// ruinous: 2,432 people across four worlds fell to 1,622, and seed 5 — which reached
	// 926 by founding one colony — collapsed to 196 because sixty poor settlers could
	// never raise a purse between them, so it never expanded at all. Expansion is worth
	// four times what a village achieves alone, and a village-wide levy is what pays for
	// it. Third instance of method note 13: the ugly mechanism was load-bearing.
	//
	// It is defensible as well as effective. Colonial ventures were financed by the
	// community that sent them — crown, subscription, joint stock — precisely because a
	// founding party never has capital of its own.
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Gold <= LevyFloor || s.marketHall(c.Pos) != hall {
			continue
		}
		give := (c.Gold - LevyFloor) * 0.5
		c.Gold -= give
		purse += give
	}

	// Provisions to first harvest, bought where the grain is.
	want := float32(len(party)) * MealsPerDay * 90 // a season's food in the wagons
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || provisions >= want {
			continue
		}
		if st.Type != Granary && st.Type != Farm {
			continue
		}
		take := want - provisions
		if take > st.Stock[Food] {
			take = st.Stock[Food]
		}
		price := s.FoodPriceAt(st.Pos)
		cost := float64(take) * price
		if cost > float64(purse) {
			take = float32(float64(purse) / price)
			cost = float64(take) * price
		}
		if take <= 0 {
			continue
		}
		st.Stock[Food] -= take
		st.Gold += cost
		st.revenue += cost
		purse -= cost
		provisions += take
	}
	return purse, provisions
}

// foundColony raises the daughter settlement and moves the party in.
// purse is money and float64; provisions is food and stays float32.
func (s *State) foundColony(site torus.Cell, party []CharID, purse float64, provisions float32) {
	// The structures are granted as the mother's were at world-start: the founding's own
	// abstraction, reused for a re-founding, debt recorded at the top of this file.
	cfg := Config{
		World: s.World, Seed: s.Seed,
		Homes: 14, Farms: 5, Granaries: 2, Camps: 3, Quarries: 1, Mines: 1,
		Industry: true,
		Treasury: purse,
		// The provisions actually bought — never a granted number. If the wagons are
		// light, the colony's first winter is hard, exactly as it should be.
		StartingFood: provisions,
		FoodPrice:    float32(s.Prices[Food]),
	}
	s.buildVillage(site, cfg)

	// The party arrives. Old ties are released properly: jobs quit, home slots freed.
	centre := s.T.Center(site)
	for _, id := range party {
		c := &s.Chars[id]
		s.quitJob(id, QuitMoved)
		if c.Home != NoStruct && c.housed {
			s.Structs[c.Home].Residents--
		}
		c.housed = false
		c.Home = NoStruct
		c.newHome = NoStruct
		c.dest = NoStruct
		c.Pos = centre
		s.diarise(id, "left with the founding party for %v", site)
		if c.Stage() != Child {
			s.assignHome(id)
		}
	}
	// Children move in with a parent once the adults are housed.
	for _, id := range party {
		c := &s.Chars[id]
		if c.Stage() != Child {
			continue
		}
		// The mother if she went; else the father; else any adult of the party.
		for _, pid := range party {
			p := &s.Chars[pid]
			if p.Stage() != Child && p.Home != NoStruct {
				c.Home = p.Home
				break
			}
		}
	}
	s.countHouseholds()
}

// estimateColonyFunds is what a settlement could raise, computed without taking anything.
//
// It must mirror raiseColonyFunds exactly in what it counts, minus the provisions, which
// are bought after the decision. Kept adjacent to it for that reason: if one changes and
// the other does not, a settlement will either refuse a colony it could afford or take
// money for one it cannot.
func (s *State) estimateColonyFunds(hall StructID) float64 {
	var purse float64
	if h := &s.Structs[hall]; h.Works > 0 {
		purse += h.Works
	}
	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Gold <= LevyFloor || s.marketHall(c.Pos) != hall {
			continue
		}
		purse += (c.Gold - LevyFloor) * 0.5
	}
	return purse
}
