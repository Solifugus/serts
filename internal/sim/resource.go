package sim

// Resource enumerates the commodities the economy moves (design §4.1).
//
// Food is the one the previous milestone had, and it was enough to prove that people can
// feed themselves. It was not enough to build an economy: with nothing else to make and
// nothing else to buy, there were about as many jobs as adults, so nobody was ever out of
// work, the money faucet never opened, and gold piled up in pockets with nowhere to go.
// Materials and goods are what give labour somewhere else to be and coin something else
// to do.
type Resource uint8

const (
	Food Resource = iota
	Wood
	Stone
	Iron
	// Tools are the first manufactured good. They exist to make the material economy
	// matter to the food economy: a worker with good tools produces more, and tools wear
	// out, so buying them is a recurring reason to spend (§4.2).
	Tools
	NumResources
)

func (r Resource) String() string {
	switch r {
	case Food:
		return "food"
	case Wood:
		return "wood"
	case Stone:
		return "stone"
	case Iron:
		return "iron"
	case Tools:
		return "tools"
	}
	return "?"
}

// Stock is a structure's holdings, indexed by Resource.
type Stock [NumResources]float32

// Total sums everything held, for reporting.
func (s Stock) Total() float32 {
	var t float32
	for _, v := range s {
		t += v
	}
	return t
}

// Prices are gold per unit, faction-wide (§4.3).
//
// Prices are fixed policy values for now. Making them respond to stock was tried in the
// previous milestone and destabilised the whole economy: retail price feeds the living
// wage, the living wage feeds the wage bill, and the wage bill feeds layoffs, so scarcity
// amplified itself into collapse. Responsive prices need a damped wage floor to be safe,
// which belongs with the full economy (§4.2).
type Prices [NumResources]float32

// DefaultPrices returns the retail price of each commodity.
//
// The relative values matter more than the absolute ones. Food is cheap and bought
// constantly; tools are dear and bought rarely, which is what makes them a money sink
// rather than another daily expense.
func DefaultPrices() Prices {
	return Prices{
		Food:  0.9,
		Wood:  0.6,
		Stone: 0.5,
		Iron:  2.2,
		Tools: 14.0,
	}
}

// WholesaleShare is what a middleman pays a producer, as a fraction of retail. The gap is
// the middleman's margin, which funds its own payroll.
const WholesaleShare = 0.85
