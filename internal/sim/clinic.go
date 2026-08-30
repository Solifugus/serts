package sim

import "github.com/solifugus/serts/internal/torus"

// Clinics (§8.1b).
//
// Disease is what caps this village. Measured on the cohort born in the first twenty
// years: 39 completed lives, 54% reaching adulthood, and 26 of the 39 deaths from illness
// against 8 from hunger — with R0 at 0.897, which is a village that does not replace
// itself. Meanwhile there are 147 posts for 36 adults and 544 days of food in store. The
// economy is not the constraint and has not been for some time.
//
// The design says to sequence this carefully, and the reason is written into the record:
// twice before, reducing disease while food was binding simply relabelled the death
// certificates as hunger. That is competing risks — cure one cause at fixed carrying
// capacity and the deaths reappear under another heading — and it is the specific way
// this change has failed in the past. The revert note said to try again the day the food
// constraint broke. Today's readings say it has: nobody dies for want of food on the
// shelf, only for want of the coin to buy it.
//
// So the measurement that matters is not "did disease deaths fall". Of course they will.
// It is whether the population rises AND hunger deaths do not swallow the children saved.
// TestFoodSourcingBaseline reports both, which is why it is the instrument here.
//
// What this is NOT: it is not a cure, not a sickness model, and not free. A clinic
// reduces the hazard within reach of it, is staffed by people who must be paid, and is
// funded by the hall out of the same levy that pays for relief and for founding colonies
// — so medicine genuinely competes with bread and with expansion. §8.1b makes that a
// player's choice between fee-for-service and tax-funded; there is no player yet, so it
// stands at tax-funded, which is the option that keeps the poor alive and strains the
// treasury.

const (
	// ClinicRange is how far a clinic's care reaches, in cells. A settlement's clinic
	// serves that settlement, on the same footing as its hall and its market.
	ClinicRange = 40

	// ClinicRelief is how much of the excess disease hazard a fully staffed clinic
	// removes from those within reach.
	//
	// Excess, not base. The floor of DiseaseBase stays whatever medicine does — people
	// die of illness in every society — and what a clinic works on is the part added by
	// being small, underfed or crowded, which is the part that actually varies and the
	// part that is killing the children here. Setting it against the total instead would
	// make a clinic a switch for mortality rather than a mitigation of it, which §8.1b
	// explicitly rules out.
	//
	// A half is a guess wearing a constant's clothes and is calibrated only by the
	// twelve-seed measurement.
	ClinicRelief = 0.5

	// ClinicWagePremium sets healer pay against subsistence. Like the hall's clerks, a
	// clinic has a purse rather than a trade, so its wage is declared: revenue-derived
	// pay on a subsidy would price healers off the treasury's whole balance.
	ClinicWagePremium = 1.3

	// ClinicSubsidyDays is how much payroll the hall keeps in a clinic's till. The hall
	// tops it up daily, so a clinic in a poor settlement is staffed only as far as the
	// levy reaches — medicine competing with relief, which is the point.
	ClinicSubsidyDays = 20.0
)

// clinicCare returns how much of a character's excess disease hazard is answered by a
// clinic within reach, from 0 to ClinicRelief.
//
// Scaled by staffing rather than by mere existence: an empty clinic is a building. This is
// also what makes the wage bill matter — a settlement too poor to staff its clinic gets
// the mortality of one that never built it.
func (s *State) clinicCare(pos torus.Vec2) float64 {
	best := 0.0
	for _, cid := range s.byType(Clinic) {
		st := &s.Structs[cid]
		if !st.Alive || st.Jobs == 0 || st.Filled == 0 {
			continue
		}
		if s.T.Dist2(pos, st.Pos) > ClinicRange*ClinicRange {
			continue
		}
		if care := ClinicRelief * float64(st.Filled) / float64(st.Jobs); care > best {
			best = care
		}
	}
	return best
}

// stepClinics tops up each clinic's till from the hall that funds it.
//
// Daily, and only as far as the hall's own purse allows. Relief, public works and
// medicine are drawn from the same levy, so a hall that is feeding its poor cannot also
// staff a clinic — which is the trade-off §8.1b exists to make real, and it is left to
// bite rather than being smoothed over with a subsidy from nowhere.
func (s *State) stepClinics() {
	if s.Tick%TicksPerDay != 0 {
		return
	}
	clinics := s.byType(Clinic)
	if len(clinics) == 0 {
		return
	}
	for _, cid := range clinics {
		st := &s.Structs[cid]
		if !st.Alive || st.Jobs == 0 {
			continue
		}
		want := st.Wage * WorkTicksPerDay * float32(st.Jobs) * ClinicSubsidyDays
		if st.Gold >= want {
			continue
		}
		hall := s.marketHall(st.Pos)
		if hall == NoStruct {
			continue
		}
		h := &s.Structs[hall]
		// Relief has first call on the hall's purse; medicine takes what is left above
		// the floor that keeps the hungry fed.
		spare := h.Gold - ReliefFloor
		if spare <= 0 {
			continue
		}
		give := want - st.Gold
		if give > spare {
			give = spare
		}
		h.Gold -= give
		st.Gold += give
		s.Led.Medicine += give
	}
}
