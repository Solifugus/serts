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

	// LivingTraits is the mean personality of everyone currently alive. The founding
	// generation is drawn around one on every trait, so any lasting departure from one is
	// selection: the village reporting which temperaments its conditions actually reward.
	// It is worth more than a guess at the constants, given how many of those guesses have
	// been wrong.
	LivingTraits Traits
	Living       int
}

// Vitals computes the demographic summary over completed lives.
func (s *State) Vitals() Vitals {
	v := VitalsOf(s.Lives)

	for i := range s.Chars {
		if !s.Chars[i].Alive {
			continue
		}
		t := s.Chars[i].Traits
		v.Living++
		v.LivingTraits.Patience += t.Patience
		v.LivingTraits.Caution += t.Caution
		v.LivingTraits.Diligence += t.Diligence
		v.LivingTraits.Rootedness += t.Rootedness
		v.LivingTraits.Ambition += t.Ambition
	}
	if v.Living > 0 {
		n := float32(v.Living)
		v.LivingTraits.Patience /= n
		v.LivingTraits.Caution /= n
		v.LivingTraits.Diligence /= n
		v.LivingTraits.Rootedness /= n
		v.LivingTraits.Ambition /= n
	}
	return v
}

// VitalsOf summarises a set of completed lives.
//
// Separate from State so that lives from several runs can be pooled. One village of this
// size produces too few completed lives for the figures to mean anything on their own:
// across four variants of the same code the share reaching adulthood read 44%, 22%, 21%
// and 30%, and the share ever marrying 29%, 60%, 25% and 0%. That is sampling noise, and
// tuning against it is worse than not measuring at all.
func VitalsOf(lives []Life) Vitals {
	var v Vitals
	var ages, adultAges []float32
	var marriedAges []float32
	var completedFertility []int

	for _, l := range lives {
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
	if v.Living > 0 {
		t := v.LivingTraits
		out += fmt.Sprintf("\n    living temperament (founding mean 1.00)  patience %.2f  caution %.2f  diligence %.2f  rooted %.2f  ambition %.2f",
			t.Patience, t.Caution, t.Diligence, t.Rootedness, t.Ambition)
	}
	return out
}
