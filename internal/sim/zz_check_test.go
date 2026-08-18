package sim

import "testing"

func TestYoungAdults(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(TicksPerYear * 10)
	reported := 0
	for d := 0; d < 360*15 && reported < 10; d++ {
		for i := 0; i < TicksPerDay; i++ {
			s.Step()
			for j := range s.Chars {
				c := &s.Chars[j]
				if !c.Alive || c.Age < AdultAge || c.Age > 18 || c.Hunger < 85 || reported >= 10 {
					continue
				}
				reported++
				job, wage := "none", float32(0)
				if c.Job != NoStruct {
					job = Defs[s.Structs[c.Job].Type].Name
					wage = s.Structs[c.Job].Wage * WorkTicksPerDay
				}
				larder, occ, cap := float32(-1), 0, 0
				if c.Home != NoStruct {
					larder = s.Structs[c.Home].Stock[Food]
					occ = s.Structs[c.Home].Occupants
					cap = s.Structs[c.Home].Capacity()
				}
				t.Logf("age %5.2f hunger %5.1f health %5.1f gold %7.2f rations %4.1f housed=%v home %d (larder %7.2f occ %d/%d) | job %-12s wage/day %.3f | act %s",
					c.Age, c.Hunger, c.Health, c.Gold, c.Rations, c.housed, c.Home, larder, occ, cap, job, wage, c.Activity)
			}
		}
	}
	if reported == 0 {
		t.Log("no starving young adult found")
	}
}
