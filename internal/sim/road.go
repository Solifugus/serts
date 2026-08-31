package sim

import "sort"

// Roads.
//
// A quarter of every adult's time is spent walking: commuting 10.2%, out to the diggings
// 7.2%, home again 6.6%, against 34.4% actually working. The working day is bounded by
// WorkStartHour and WorkEndHour, so an hour saved on the road is an hour spent at the
// bench — which makes roads a productivity lever of the one kind that is not zero-sum at
// full employment. Nobody is taken off anything to build them, and everybody who uses one
// gets more done.
//
// The network is not planned. Traffic accumulates on the cells people actually cross, and
// once a year the hall paves the busiest of them out of the public works fund. So the
// roads appear where the village already walks — the granary to the houses, the houses to
// the fields — and a settlement that moves its business somewhere else stops maintaining
// the old route by simply not repaving it. Desire lines, then paving, which is how most
// real paths were made.
//
// Two costs are deliberately taken on the chin:
//
//   - Roads compete for stone with houses and every other building. The quarry is the
//     village's only source, and paving a mile is a house not improved.
//   - The works fund is the same purse that sends out colonies (§8.1a). A hall that paves
//     is a hall not expanding, which is the sharpest form of the trade-off: better roads
//     here, or a settlement in the next valley.

const (
	// RoadSpeed is how much faster a road is than open ground.
	//
	// Sixty per cent, which is about what a made surface bought over rough country before
	// engines. Deliberately short of double: a road should be worth building and worth
	// arguing about, not a teleporter that makes distance stop mattering — distance is
	// load-bearing in this design, being what makes a second settlement necessary at all.
	RoadSpeed = 1.6

	// RoadStoneCost is the stone in one paved cell.
	RoadStoneCost = 2.0

	// RoadWorksCost is what the hall pays its quarry and its labourers for that stone,
	// out of the public works fund.
	RoadWorksCost = 6.0

	// RoadTrafficFloor is how many crossings a cell must see in a year before it is worth
	// paving. A track nobody uses is a pile of stone.
	RoadTrafficFloor = 4000

	// RoadsPerYear caps how much a hall lays or repairs in one season, so a network grows
	// over decades rather than appearing at once.
	RoadsPerYear = 24

	// RoadNew is the condition of freshly laid stone, and RoadWorn the point below which
	// a surface has stopped being a road and gone back to being a track.
	RoadNew  = 200
	RoadWorn = 40

	// RoadDecayPerYear is what weather alone takes off a surface, and RoadWearPerCrossing
	// what boots and cart wheels add on top.
	//
	// Wear is charged annually against the traffic count rather than per crossing,
	// deliberately: movement is the hottest loop in the simulation and already carries one
	// array write for traffic, and a second write to degrade the surface would double that
	// cost to express a quantity nobody reads until the year turns.
	RoadDecayPerYear    = 8
	RoadWearPerCrossing = 1.0 / 900.0

	// RoadRepairCost is what mending a worn surface costs against laying a new one. Repair
	// is cheaper than paving, which is why a network that has outgrown its hall decays
	// from the edges inward: the busy middle is worth mending and the far ends are not.
	RoadRepairCost = 0.4
)

// noteTraffic records that somebody crossed a cell. One increment on the movement path,
// which is the hottest loop in the simulation, so it stays a bare array write — no bounds
// juggling, no map, no allocation.
func (s *State) noteTraffic(ci int) {
	if s.traffic == nil {
		return
	}
	if s.traffic[ci] < 65000 {
		s.traffic[ci]++
	}
}

// roadFactor is how much faster somebody moves on the cell they are standing in.
func (s *State) roadFactor(ci int) float64 {
	if s.Road == nil {
		return 1
	}
	c := s.Road[ci]
	if c <= RoadWorn {
		return 1 // worn back to a track; no faster than the ground beside it
	}
	// A surface gives up its advantage as it goes: full speed on new stone, tailing to
	// nothing at the point where it stops counting as a road.
	f := float64(c-RoadWorn) / float64(RoadNew-RoadWorn)
	if f > 1 {
		f = 1
	}
	return 1 + (RoadSpeed-1)*f
}

// stepRoads lays the year's paving.
//
// Annual, because a road is not a daily decision and because sorting the traffic of every
// cell in the world is not something to do often. Each hall paves near itself, from its
// own works fund, so a colony builds its own roads rather than inheriting the mother's.
func (s *State) stepRoads() {
	if s.Tick%TicksPerYear != 0 || s.Tick == 0 || s.traffic == nil {
		return
	}
	// Weather and wheels, charged once for the year against the traffic that caused them.
	for i, c := range s.Road {
		if c == 0 {
			continue
		}
		wear := RoadDecayPerYear + int(float64(s.traffic[i])*RoadWearPerCrossing)
		if wear >= int(c) {
			s.Road[i] = 0 // gone back to open ground
			s.RoadsLaid--
			continue
		}
		s.Road[i] = c - uint8(wear)
	}

	defer func() {
		// Traffic decays rather than resetting: a route that mattered last year still
		// matters a little this year, and a route abandoned fades instead of vanishing
		// between one tick and the next.
		for i := range s.traffic {
			s.traffic[i] = s.traffic[i] / 2
		}
	}()

	halls := s.byType(TownHall)
	if len(halls) == 0 {
		return
	}

	// The ground worth spending on, in one pass: surfaces wearing out that are still
	// carrying traffic, and unpaved ground busy enough to deserve stone.
	//
	// Repair comes first and is cheaper, which is what makes a network shrink sensibly
	// rather than uniformly. A hall that has laid more road than its levy can maintain
	// keeps mending the busy middle and lets the far ends go back to grass — which is
	// how real networks contract, and it happens here without anything deciding it.
	type cand struct {
		idx     int
		traffic uint16
		repair  bool
	}
	var cands []cand
	for i, t := range s.traffic {
		if !s.World.Walkable(i) {
			continue
		}
		switch {
		case s.Road[i] > 0 && s.Road[i] < RoadNew/2:
			cands = append(cands, cand{i, t, true})
		case s.Road[i] == 0 && t >= RoadTrafficFloor:
			cands = append(cands, cand{i, t, false})
		}
	}
	if len(cands) == 0 {
		return
	}
	// Busiest first, ties broken by index so that the same world always paves the same
	// stones in the same order (§9.2).
	sort.Slice(cands, func(a, b int) bool {
		// Mending what exists before laying what does not.
		if cands[a].repair != cands[b].repair {
			return cands[a].repair
		}
		if cands[a].traffic != cands[b].traffic {
			return cands[a].traffic > cands[b].traffic
		}
		return cands[a].idx < cands[b].idx
	})

	laid := 0
	for _, c := range cands {
		if laid >= RoadsPerYear {
			break
		}
		pos := s.T.Center(s.T.CellOf(c.idx))
		hall := s.marketHall(pos)
		if hall == NoStruct {
			continue
		}
		h := &s.Structs[hall]
		works := float64(RoadWorksCost)
		stone := float32(RoadStoneCost)
		if c.repair {
			works *= RoadRepairCost
			stone *= RoadRepairCost
		}
		if h.Works < works {
			continue // this hall is saving for a colony, or has nothing
		}
		// The stone has to exist. Roads take it from the same store that houses do, which
		// is the competition this is meant to have.
		src := s.nearestWith(pos, Storehouse, Stone)
		if src == NoStruct || s.Structs[src].Stock[Stone] < stone {
			src = s.nearestWith(pos, Quarry, Stone)
		}
		if src == NoStruct || s.Structs[src].Stock[Stone] < stone {
			continue
		}
		st := &s.Structs[src]
		st.Stock[Stone] -= stone
		h.Works -= works
		st.Gold += works // the quarry is paid for its stone
		if !c.repair {
			s.RoadsLaid++
		}
		s.Road[c.idx] = RoadNew
		laid++
	}
}
