package sim

import (
	"testing"

	"github.com/solifugus/serts/internal/torus"
)

// A clinic mitigates; it must not abolish. §8.1b is explicit that the hazard is reduced
// within a radius rather than switched off, and DiseaseBase is the floor that stays
// whatever medicine does.
func TestClinicReducesExcessButNotTheBase(t *testing.T) {
	s := newTestSim(5)

	var id CharID = NoChar
	for i := range s.Chars {
		if c := &s.Chars[i]; c.Alive && c.Stage() != Child {
			id = CharID(i)
			break
		}
	}
	if id == NoChar {
		t.Fatal("nobody in the village")
	}
	c := &s.Chars[id]
	// A healthy, uncrowded adult carries no excess, so care can do nothing for them.
	c.Age, c.Hunger, c.Health, c.Home = 30, 0, 100, NoStruct
	if got, want := s.diseaseHazard(id), DiseaseBase; got != want {
		t.Fatalf("a well adult's hazard is %v, want the base %v", got, want)
	}

	// Now make them vulnerable, and compare with and without care.
	c.Hunger = 100
	sick := s.diseaseHazard(id)
	if sick <= DiseaseBase {
		t.Fatalf("malnutrition did not raise the hazard: %v", sick)
	}

	clinics := s.byType(Clinic)
	if len(clinics) == 0 {
		t.Fatal("no clinic was founded")
	}
	cl := &s.Structs[clinics[0]]
	c.Pos = cl.Pos
	cl.Filled = cl.Jobs // fully staffed

	cared := s.diseaseHazard(id)
	if cared >= sick {
		t.Errorf("a staffed clinic did not reduce the hazard: %v vs %v", cared, sick)
	}
	if cared < DiseaseBase {
		t.Errorf("care took the hazard below the base rate: %v < %v — medicine is "+
			"abolishing mortality rather than mitigating it", cared, DiseaseBase)
	}
	// Exactly the specified share of the excess, no more.
	wantExcess := (sick/DiseaseBase - 1) * (1 - ClinicRelief)
	if got := cared/DiseaseBase - 1; got < wantExcess*0.99 || got > wantExcess*1.01 {
		t.Errorf("excess reduced to %v, want %v", got, wantExcess)
	}
}

// An empty clinic is a building. Staffing is what buys the care, which is what makes the
// wage bill — and the hall's ability to pay it — matter.
func TestUnstaffedClinicHelpsNobody(t *testing.T) {
	s := newTestSim(5)
	clinics := s.byType(Clinic)
	if len(clinics) == 0 {
		t.Fatal("no clinic")
	}
	cl := &s.Structs[clinics[0]]
	cl.Filled = 0
	if care := s.clinicCare(cl.Pos); care != 0 {
		t.Errorf("an unstaffed clinic gave care of %v", care)
	}
	cl.Filled = cl.Jobs
	if care := s.clinicCare(cl.Pos); care != ClinicRelief {
		t.Errorf("a fully staffed clinic gave care of %v, want %v", care, ClinicRelief)
	}
	// And it does not reach across the world.
	far := s.T.Add(cl.Pos, torus.Vec2{X: ClinicRange + 5, Y: 0})
	if care := s.clinicCare(far); care != 0 {
		t.Errorf("care reached %v cells away", ClinicRange+5)
	}
}
