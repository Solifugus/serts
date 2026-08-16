package sim

import "github.com/solifugus/serts/internal/torus"

// Forest regrowth (§2.5).
//
// Depletion was implemented and regrowth was not, which made every extraction site a
// one-way trip. The measured consequence: a lumber camp with five men on its payroll
// cleared every walkable cell of woodland within its nine-cell reach in under two hundred
// days, and from then on produced nothing for the rest of the game. Wood sat pinned at its
// price ceiling with a stock of zero, no house was ever built or upgraded, and the timber
// half of the material economy simply stopped existing.
//
// §2.5 asks for the opposite of a one-way trip: resources deplete in one place and grow in
// another, so that settlements move rather than merely die. The field comment on Woodland
// already specified the mechanism — timber regrows from adjacent woodland — and this is
// that, built.
//
// Seeding from neighbours matters rather than being decoration. Ground stripped bare in
// the middle of a forest comes back quickly; ground stripped bare at the edge of the range
// comes back slowly; and a clearing with no woodland left anywhere near it does not come
// back at all. That is what makes over-felling a decision with consequences instead of a
// delay, and it is what will eventually empty a timber town (§2.7).

// WoodlandRegrowth is the daily rate at which a cell closes the gap to what its ground can
// support, given full woodland all around it. Cleared ground in the middle of a forest is
// back within roughly four years, which is coppice rather than old growth — and coppice is
// what a village cuts.
const WoodlandRegrowth = 0.0016

// regrowWoodland advances the forest by a day.
//
// Swept once a day rather than every tick: at 2,400 ticks to the day the difference is
// invisible and the cost is 2,400 times lower. Iteration is over a plain index range, so
// it stays deterministic (§9.2).
func (s *State) regrowWoodland() {
	w := s.World
	t := s.T
	for y := 0; y < t.CY; y++ {
		for x := 0; x < t.CX; x++ {
			i := t.Index(s.T.WrapCell(torus.Cell{X: x, Y: y}))
			max := w.WoodlandMax[i]
			if max <= 0 {
				continue
			}
			cur := w.Woodland[i]
			if cur >= max {
				continue
			}
			// Seed pressure from the four neighbours. No seed source, no recovery.
			var seed float32
			seed += w.Woodland[t.Index(s.T.WrapCell(torus.Cell{X: x + 1, Y: y}))]
			seed += w.Woodland[t.Index(s.T.WrapCell(torus.Cell{X: x - 1, Y: y}))]
			seed += w.Woodland[t.Index(s.T.WrapCell(torus.Cell{X: x, Y: y + 1}))]
			seed += w.Woodland[t.Index(s.T.WrapCell(torus.Cell{X: x, Y: y - 1}))]
			seed /= 4
			if seed <= 0 {
				continue
			}
			cur += WoodlandRegrowth * seed * (max - cur)
			if cur > max {
				cur = max
			}
			w.Woodland[i] = cur
		}
	}
}
