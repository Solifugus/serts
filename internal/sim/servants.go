package sim

// Domestic service.
//
// The problem this answers is that owner profits had nowhere to go. A business earned,
// its owner accumulated, and the coin left circulation permanently — which holds every
// wage in the village at subsistence, because in a closed loop total wages are total food
// spending less every margin taken along the way. Households were measured eating twelve
// hundred gold of savings in three hundred days and then dying around perfectly healthy
// children.
//
// House-building was the first candidate and it cannot fire: the village holds no timber
// at all and the price sits pinned at its ceiling, so the materials to build with do not
// exist. Service needs no materials, which is exactly why it works here — it is a channel
// from wealth straight to wages with no supply chain in between to bottleneck.
//
// It also fits the spine better than any alternative. §1 says the only relationship in the
// game is employment, and this expresses the sink as a payroll rather than as a special
// case: a rich household simply becomes an employer. Historically it is the ordinary
// arrangement too — domestic service was among the largest categories of paid work for
// most of the period this village lives in.

const (
	// ServantWagePremium is what domestic work pays against bare subsistence. Above one on
	// purpose: the whole point is to put money into a worker's hands that the food trade
	// cannot, so it has to beat what a marginal farmhand earns.
	ServantWagePremium = 1.35

	// ServantWealthYears is how many years of a servant's wages a household must hold
	// before it will take one on. Each further post needs the same again, so a fortune
	// employs proportionally more people and no amount of wealth has nowhere to go.
	ServantWealthYears = 3.0

	// MaxServants bounds one household's establishment. Not an economic limit but a
	// physical one: a house has only so much for anyone to do.
	MaxServants = 4
)

// hireServants sets each household's establishment from what it can afford, and funds the
// day's wages out of the household purse.
//
// Called once a day. The wage itself is paid tick by tick through the ordinary work path,
// so a house that runs out of money turns its servants out exactly as a failing business
// does — no separate rule for it.
func (s *State) hireServants() {
	living := s.SubsistenceWage()
	if living <= 0 {
		return
	}
	yearly := living * ServantWagePremium * WorkTicksPerDay * DaysPerYear

	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Type != Home {
			continue
		}

		var purse float32
		for j := range s.Chars {
			c := &s.Chars[j]
			if c.Alive && c.Home == StructID(i) && c.Stage() != Child {
				purse += c.Gold
			}
		}

		posts := int(purse / (yearly * ServantWealthYears))
		if posts > MaxServants {
			posts = MaxServants
		}
		// A house never turns out more staff than it has to in one day; shrinking the
		// establishment below what is currently employed happens by attrition, when the
		// money runs out mid-tick and the ordinary payroll rule applies.
		if posts < st.Filled {
			posts = st.Filled
		}
		st.Jobs = posts
		st.Wage = living * ServantWagePremium
		if posts == 0 {
			continue
		}

		// Fund the day's wages. The household pays proportionally, so the burden falls on
		// whoever actually has the money rather than being split evenly among people who
		// have none.
		bill := st.Wage * float32(st.Filled) * WorkTicksPerDay
		if st.Gold >= bill || purse <= 0 {
			continue
		}
		want := bill - st.Gold
		for j := range s.Chars {
			c := &s.Chars[j]
			if !c.Alive || c.Home != StructID(i) || c.Stage() == Child {
				continue
			}
			share := want * (c.Gold / purse)
			if share > c.Gold {
				share = c.Gold
			}
			c.Gold -= share
			st.Gold += share
		}
	}
}

// ServantYieldPerDay is what a servant produces for the household in a day.
//
// Domestic work was productive work: the kitchen garden, the poultry, the preserving and
// the brewing. Paid at the same rate as an evening in a household's own garden, so a
// servant is worth roughly what they cost and the arrangement is not simply a transfer.
const ServantYieldPerDay = 2.4

// ServantYield is that rate per working tick.
const ServantYield = ServantYieldPerDay / WorkTicksPerDay
