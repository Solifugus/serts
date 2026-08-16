package sim

import (
	"fmt"
	"sort"
)

// Vital statistics: the village measured as a demographer would measure it.
//
// The only test of demographic health until now was a headcount — "are there more than
// fifteen people at year twenty?" That passed and failed all day while saying almost
// nothing about whether a *life* works, and it passed for months against a world half the
// size of the one the game ships, because a headcount cannot tell you why it is what it
// is.
//
// These figures can. Every one has a plausible historical range, so they turn "readings I
// cannot account for" into pass or fail. A life expectancy at birth of twenty-two against
// thirty-four at age fifteen says infant mortality; the same two figures close together
// says adults are dying young. A headcount says neither.

// DeathCause records what killed someone.
type DeathCause uint8

const (
	CauseHunger DeathCause = iota
	CauseDisease
	CauseAccident
	CauseAge
	CauseExposure
	numCauses
)

func (d DeathCause) String() string {
	switch d {
	case CauseHunger:
		return "hunger"
	case CauseDisease:
		return "disease"
	case CauseAccident:
		return "accident"
	case CauseAge:
		return "old age"
	case CauseExposure:
		return "exposure"
	}
	return "?"
}

// Life is the record of one completed life, kept so the dead can be counted properly.
type Life struct {
	Born      Tick
	Age       float32
	Cause     DeathCause
	Married   bool
	MarriedAt float32 // age at marriage
	Children  int
	// Settler marks someone who arrived at founding rather than being born here. Their
	// lives are excluded from most figures: they began mid-life, so counting them would
	// flatter infant survival and understate longevity.
	Settler bool
}

// Vitals is the demographic summary.
type Vitals struct {
	Lives int // completed lives born in the village

	ExpectancyAtBirth float32 // mean age at death
	ExpectancyAt15    float32 // mean age at death among those who reached adulthood
	MedianAgeAtDeath  float32
	OldestEver        float32

	ReachedAdulthood  float32 // share of those born
	EverMarried       float32 // share of those who reached adulthood
	MeanAgeAtMarriage float32
	ChildrenPerLife   float32 // among those who lived out their fertile years

	// ByCause counts every death including the founding settlers, so it describes what
	// the village dies of. AllDeaths is its total — kept distinct from Lives, which counts
	// only those born here, because mixing the two produced the memorable nonsense
	// "hunger killed 36 of 8".
	ByCause   [numCauses]int
	AllDeaths int
}

// Vitals computes the demographic summary over completed lives.
func (s *State) Vitals() Vitals {
	var v Vitals
	var ages, adultAges []float32
	var marriedAges []float32
	var completedFertility []int

	for _, l := range s.Lives {
		v.ByCause[l.Cause]++
		v.AllDeaths++
		if l.Settler {
			continue // began mid-life; counting them would flatter the figures
		}
		v.Lives++
		ages = append(ages, l.Age)
		if l.Age > v.OldestEver {
			v.OldestEver = l.Age
		}
		if l.Age >= AdultAge {
			adultAges = append(adultAges, l.Age)
			if l.Married {
				marriedAges = append(marriedAges, l.MarriedAt)
			}
		}
		if l.Age >= FertileMax {
			completedFertility = append(completedFertility, l.Children)
		}
	}
	if v.Lives == 0 {
		return v
	}

	v.ExpectancyAtBirth = mean(ages)
	v.ExpectancyAt15 = mean(adultAges)
	v.MedianAgeAtDeath = median(ages)
	v.ReachedAdulthood = float32(len(adultAges)) / float32(v.Lives)
	if len(adultAges) > 0 {
		v.EverMarried = float32(len(marriedAges)) / float32(len(adultAges))
	}
	v.MeanAgeAtMarriage = mean(marriedAges)
	if len(completedFertility) > 0 {
		var total int
		for _, c := range completedFertility {
			total += c
		}
		v.ChildrenPerLife = float32(total) / float32(len(completedFertility))
	}
	return v
}

func mean(xs []float32) float32 {
	if len(xs) == 0 {
		return 0
	}
	var t float32
	for _, x := range xs {
		t += x
	}
	return t / float32(len(xs))
}

func median(xs []float32) float32 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float32(nil), xs...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[len(c)/2]
}

func (v Vitals) String() string {
	if v.Lives == 0 {
		return "  vitals: nobody born here has died yet"
	}
	out := fmt.Sprintf("  vitals over %d completed lives born here:\n", v.Lives)
	out += fmt.Sprintf("    life expectancy   %.1f at birth, %.1f for those reaching 15   median %.1f   oldest ever %.1f\n",
		v.ExpectancyAtBirth, v.ExpectancyAt15, v.MedianAgeAtDeath, v.OldestEver)
	out += fmt.Sprintf("    reached adulthood %.0f%%   ever married %.0f%% (at %.1f)   children per completed life %.2f\n",
		v.ReachedAdulthood*100, v.EverMarried*100, v.MeanAgeAtMarriage, v.ChildrenPerLife)
	out += "    died of:"
	for c := DeathCause(0); c < numCauses; c++ {
		if v.ByCause[c] > 0 {
			out += fmt.Sprintf("  %s %d", c, v.ByCause[c])
		}
	}
	return out
}
