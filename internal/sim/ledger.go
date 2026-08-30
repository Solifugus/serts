package sim

import (
	"fmt"
	"strings"
)

// The ledger: national accounts for a village.
//
// The most expensive class of bug in this project has been the invisible flow fault —
// money or food accumulating where nothing spends it, or leaking where nothing records
// it. Owner hoards, the payroll ratchet, servant larder hoards at eighty thousand meals,
// inheritance destroying a third of every estate, a till that refused sales forever: each
// was found late, by surprise, from a post-mortem, when a yearly statement of where the
// gold and the food actually sit would have shown every one of them the year it began.
//
// Two kinds of figures, matching the two kinds of fault:
//
//   - Flows: what entered and left the world this period, by cause. Gold is minted only
//     by panning and destroyed only by inheritance loss; food is produced by named
//     sources and destroyed by eating. If the change in stocks does not equal flows in
//     minus flows out, something is leaking, and that inequality is checkable in a test.
//   - Holdings: where the stocks sit, by class of holder. A hoard is a holding that
//     grows without a corresponding flow out, and it is visible in one line.
//
// Measurement only. Nothing here changes behaviour, and the counters are a few adds per
// event — cheap enough to leave on permanently.

// Ledger accumulates flow totals. Reset per reporting period by the caller.
type Ledger struct {
	// Medicine is what the halls have paid to staff their clinics (§8.1b).
	Medicine float32

	// Gold flows.
	GoldMinted    float32 // panning: the only way gold enters the world (§4.2)
	GoldDestroyed float32 // the inheritance share that passes to nobody
	GoldLevied    float32 // the town hall's wealth levy (§8.1a)
	GoldRelieved  float32 // the dole paid out of the hall's treasury

	// Food flows, by source.
	FoodFarmed   float32 // harvests brought in
	FoodGardened float32 // household gardens
	FoodFished   float32 // fisheries: the year-round food channel
	FoodCarried  float32 // delivered between settlements by caravan
	FoodServed   float32 // servants' production for their employers
	FoodForaged  float32 // eaten straight off the land
	FoodEaten    float32 // meals consumed from any store or pack
}

// LedgerReport is a point-in-time statement: the period's flows plus where everything
// currently sits.
func (s *State) LedgerReport() string {
	var chars, businesses, homes float32
	var charFood, businessFood, homeFood float32
	for i := range s.Chars {
		if s.Chars[i].Alive {
			chars += s.Chars[i].Gold
			charFood += s.Chars[i].Rations
		}
	}
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive {
			continue
		}
		if st.Type == Home {
			homes += st.Gold
			homeFood += st.Stock[Food]
		} else {
			businesses += st.Gold
			businessFood += st.Stock[Food]
		}
	}

	l := &s.Led
	var b strings.Builder
	fmt.Fprintf(&b, "gold: people %.0f, businesses %.0f, homes %.0f, total %.0f | minted %+.1f destroyed %-.1f levied %.1f relieved %.1f\n",
		chars, businesses, homes, chars+businesses+homes, l.GoldMinted, l.GoldDestroyed, l.GoldLevied, l.GoldRelieved)
	fmt.Fprintf(&b, "food: market %.0f, larders %.0f, packs %.0f | farmed %.0f fished %.0f gardened %.0f served %.0f foraged %.0f carried %.0f eaten %.0f",
		businessFood, homeFood, charFood,
		l.FoodFarmed, l.FoodFished, l.FoodGardened, l.FoodServed, l.FoodForaged, l.FoodCarried, l.FoodEaten)
	return b.String()
}

// ResetLedger zeroes the period's flow counters, typically after a report.
func (s *State) ResetLedger() { s.Led = Ledger{} }
