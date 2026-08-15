package sim

import (
	"github.com/solifugus/serts/internal/torus"
)

// The material economy (design §4.1, §5).
//
// Extraction, manufacture, and the wholesale trade that connects them. The shape mirrors
// food deliberately: something is pulled out of the ground or made, a middleman buys it,
// and gold flows back down the chain to the people who did the work. What differs is that
// materials are drawn from the map itself and run out, which is what will eventually make
// a settlement move (§2.7).

// Extraction constants, expressed per worked day and divided down to ticks.
const (
	// FellPerWorkerDay is timber cut by one worker in a day, from woodland at full
	// density.
	FellPerWorkerDay = 1.6
	// QuarryPerWorkerDay is stone cut by one worker in a day.
	QuarryPerWorkerDay = 1.1
	// MinePerWorkerDay is ore raised by one worker in a day. Slower than stone: it has to
	// be found and followed, not merely broken off.
	MinePerWorkerDay = 0.55

	// WorkRadius is how far from its site a camp, quarry, or mine reaches. Beyond this
	// the walk costs more than the material is worth.
	WorkRadius = 9

	// A cell is considered worked out below this.
	Exhausted = 0.001
)

// resourceOf maps an extraction structure to what it pulls out of the ground.
func resourceOf(t StructType) Resource {
	switch t {
	case LumberCamp:
		return Wood
	case Quarry:
		return Stone
	case Mine:
		return Iron
	}
	return Food
}

// groundAt returns a pointer to the extractable amount of a resource in a cell, or nil if
// that resource is not something dug out of the map.
func (s *State) groundAt(r Resource, i int) *float32 {
	switch r {
	case Wood:
		return &s.World.Woodland[i]
	case Stone:
		return &s.World.Stone[i]
	case Iron:
		return &s.World.IronOre[i]
	}
	return nil
}

// extractPerWorkerDay is the daily output of one worker at full deposit richness.
func extractPerWorkerDay(t StructType) float64 {
	switch t {
	case LumberCamp:
		return FellPerWorkerDay
	case Quarry:
		return QuarryPerWorkerDay
	case Mine:
		return MinePerWorkerDay
	}
	return 0
}

// findWorkCell picks the richest cell within reach that still holds something.
//
// The result is cached on the structure until it is worked out, so the search runs once
// per exhausted cell rather than once per tick. Scanning a nine-cell radius every tick for
// every camp would cost more than the whole rest of the simulation.
func (s *State) findWorkCell(sid StructID) {
	st := &s.Structs[sid]
	r := resourceOf(st.Type)

	best, bestAmount := int32(-1), float32(Exhausted)
	for dy := -WorkRadius; dy <= WorkRadius; dy++ {
		for dx := -WorkRadius; dx <= WorkRadius; dx++ {
			if dx*dx+dy*dy > WorkRadius*WorkRadius {
				continue
			}
			i := s.T.Index(s.T.WrapCell(torus.Cell{X: st.Cell.X + dx, Y: st.Cell.Y + dy}))
			if !s.World.Walkable(i) {
				continue
			}
			g := s.groundAt(r, i)
			if g == nil {
				continue
			}
			// Ties break toward the lower index so the choice never depends on
			// iteration order varying between runs (§9.2).
			if *g > bestAmount {
				best, bestAmount = int32(i), *g
			}
		}
	}
	st.workCell = best
}

// extract converts a worker's labour into material pulled from the ground.
//
// Called from work(), so it only happens when somebody actually turned up. Depletion is
// real: what comes out of a cell does not go back, which is what will eventually empty a
// mining town (§2.7).
func (s *State) extract(sid StructID, effort float64) {
	st := &s.Structs[sid]
	r := resourceOf(st.Type)

	if st.workCell < 0 {
		s.findWorkCell(sid)
		if st.workCell < 0 {
			return // nothing left within reach; the site is played out
		}
	}
	g := s.groundAt(r, int(st.workCell))
	if g == nil || *g <= Exhausted {
		s.findWorkCell(sid)
		return
	}

	// Richer ground yields faster. Woodland is a density in [0,1]; stone and iron are
	// stocks, so they work at full rate until nearly gone.
	rate := effort
	if r == Wood {
		rate *= float64(*g)
	}
	take := float32(rate)
	if take > *g {
		take = *g
	}
	*g -= take
	st.Stock[r] += take
	if *g <= Exhausted {
		*g = 0
		st.workCell = -1
	}
}

// manufacture turns materials into goods at a workshop.
//
// This is where the material economy earns its place: tools are the first thing a
// character can buy that is not food, which gives gold somewhere to go besides a pocket.
const (
	// ToolWoodCost and ToolIronCost are the materials in one tool.
	ToolWoodCost = 1.4
	ToolIronCost = 0.5
	// CraftPerWorkerDay is tools finished by one worker in a day.
	CraftPerWorkerDay = 0.32
)

func (s *State) manufacture(sid StructID, effort float64) {
	st := &s.Structs[sid]
	made := float32(effort)
	if need := made * ToolWoodCost; need > st.Stock[Wood] {
		made = st.Stock[Wood] / ToolWoodCost
	}
	if need := made * ToolIronCost; need > st.Stock[Iron] {
		made = st.Stock[Iron] / ToolIronCost
	}
	if made <= 0 {
		return // idle for want of materials
	}
	st.Stock[Wood] -= made * ToolWoodCost
	st.Stock[Iron] -= made * ToolIronCost
	st.Stock[Tools] += made
}

// StoreCapacity bounds what a middleman will hold of any one commodity, so that a glut
// backs up at the producer instead of being absorbed forever.
const StoreCapacity = 400

// tradeMaterials moves goods along the chain and pays for them.
//
// Producers sell to a storehouse, the storehouse sells to workshops and building sites,
// workshops sell to stores, stores sell to people. Each hop is a purchase, which is how
// the coin a villager spends on a spade finds its way back to the man who felled the tree.
func (s *State) tradeMaterials() {
	// Extractors -> storehouses.
	for _, r := range []Resource{Wood, Stone, Iron} {
		s.sellTo(Storehouse, r, func(t StructType) bool {
			return resourceOf(t) == r && t != Workshop
		})
	}
	// Workshops -> stores.
	s.sellTo(Store, Tools, func(t StructType) bool { return t == Workshop })

	// Storehouses -> workshops, which need materials to work with. The workshop is the
	// buyer here, so it pays; a workshop that cannot afford materials falls idle, which
	// is the correct and visible consequence rather than a silent stall.
	for i := range s.Structs {
		wsp := &s.Structs[i]
		if !wsp.Alive || wsp.Type != Workshop {
			continue
		}
		for _, r := range []Resource{Wood, Iron} {
			if wsp.Stock[r] >= WorkshopStock {
				continue
			}
			src := s.nearestWith(wsp.Pos, Storehouse, r)
			if src == NoStruct {
				continue
			}
			s.transact(src, StructID(i), r, WorkshopStock-wsp.Stock[r], s.Prices[r])
		}
	}
}

// WorkshopStock is how much of each input a workshop keeps on hand.
const WorkshopStock = 60

// sellTo moves a resource from every producer matching pred to the nearest middleman of
// the given type, at the wholesale price.
func (s *State) sellTo(buyer StructType, r Resource, pred func(StructType) bool) {
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Stock[r] <= 0 || !pred(st.Type) {
			continue
		}
		dst := s.nearestOfType(st.Pos, buyer)
		if dst == NoStruct {
			continue
		}
		room := StoreCapacity - s.Structs[dst].Stock[r]
		want := st.Stock[r]
		if want > room {
			want = room
		}
		s.transact(StructID(i), dst, r, want, s.Prices[r]*WholesaleShare)
	}
}

// transact moves up to want units of r from seller to buyer at a unit price, limited by
// what the buyer can afford. Gold and goods move together or not at all.
func (s *State) transact(seller, buyer StructID, r Resource, want, price float32) {
	if want <= 0 || price <= 0 {
		return
	}
	sell, buy := &s.Structs[seller], &s.Structs[buyer]
	if want > sell.Stock[r] {
		want = sell.Stock[r]
	}
	if cost := want * price; cost > buy.Gold {
		want = buy.Gold / price
	}
	if want <= 0 {
		return
	}
	buy.Gold -= want * price
	sell.Gold += want * price
	buy.Stock[r] += want
	sell.Stock[r] -= want
}

// nearestOfType finds the closest living structure of a type.
func (s *State) nearestOfType(from torus.Vec2, t StructType) StructID {
	best, bestD := NoStruct, float64(1<<62)
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Type != t {
			continue
		}
		if d := s.T.Dist2(from, st.Pos); d < bestD {
			best, bestD = StructID(i), d
		}
	}
	return best
}

// nearestWith finds the closest structure of a type that actually holds the resource.
func (s *State) nearestWith(from torus.Vec2, t StructType, r Resource) StructID {
	best, bestD := NoStruct, float64(1<<62)
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Type != t || st.Stock[r] <= 0 {
			continue
		}
		if d := s.T.Dist2(from, st.Pos); d < bestD {
			best, bestD = StructID(i), d
		}
	}
	return best
}

// --- Construction (§5) ---
//
// A planned building exists immediately as a site that hires labour and consumes
// materials, rather than as an abstract order that resolves later. It competes for
// workers in the same job market as everything else, which means an under-funded or
// under-supplied build simply cannot attract anyone — a far better failure mode than a
// silent stall, and the reason construction is a structure type at all.

// Build places a site that will become the given structure type.
func (s *State) Build(t StructType, c torus.Cell) StructID {
	sid := s.addStructure(BuildSite, c)
	st := &s.Structs[sid]
	st.Building = t
	st.Jobs = Defs[BuildSite].Jobs
	st.Wage = BaseWage
	return sid
}

// build applies one worker-tick of construction.
//
// Materials have to be on site, so a site draws them from a storehouse as it goes. What
// it cannot get, it waits for.
func (s *State) build(sid StructID, effort float64) {
	st := &s.Structs[sid]
	need := Defs[st.Building].BuildCost

	// Fetch what is missing. The site pays, so a faction that cannot afford materials
	// cannot build — which is the point of having an economy at all.
	for r := Resource(0); r < NumResources; r++ {
		if need[r] <= 0 || st.Stock[r] >= need[r] {
			continue
		}
		if src := s.nearestWith(st.Pos, Storehouse, r); src != NoStruct {
			s.transact(src, sid, r, need[r]-st.Stock[r], s.Prices[r])
		}
	}
	for r := Resource(0); r < NumResources; r++ {
		if st.Stock[r] < need[r] {
			return // waiting on materials
		}
	}

	days := Defs[st.Building].BuildDays
	if days <= 0 {
		days = 1
	}
	st.Progress += float32(effort / (days * float64(Defs[BuildSite].Jobs) * WorkTicksPerDay))
	if st.Progress >= 1 {
		s.completeBuild(sid)
	}
}

// completeBuild turns a finished site into the building it was becoming.
func (s *State) completeBuild(sid StructID) {
	st := &s.Structs[sid]
	t := st.Building

	// Everyone on the site is out of a job the moment it is finished; they re-enter the
	// market like anyone else and may well be hired straight back by the new building.
	for i := range s.Chars {
		if s.Chars[i].Alive && s.Chars[i].Job == sid {
			s.quitJob(CharID(i))
		}
	}

	d := Defs[t]
	st.Type = t
	st.Building = 0
	st.Progress = 0
	st.Jobs = d.Jobs
	st.Filled = 0
	st.Wage = BaseWage
	st.workCell = -1
	// Materials went into the walls.
	st.Stock = Stock{}
	s.paths.invalidate(sid)
	s.Built++
}
