package sim

import "testing"

// The bank moves coin; it must never make any. Gold conservation is the invariant the
// whole economy rests on, and a vault is a new place for money to hide — TotalCoin counts
// Vault precisely so that a deposit does not read as money vanishing.
func TestBankingConservesGold(t *testing.T) {
	s := newTestSim(5)
	start := s.TotalCoin()
	high, low := start, start

	for i := 0; i < 120*TicksPerDay; i++ {
		s.Step()
		if i%(TicksPerDay/2) != 0 {
			continue
		}
		now := s.TotalCoin()
		if now > high {
			high = now
		}
		if now < low {
			low = now
		}
	}
	tol := start * 0.001
	if high > start+tol {
		t.Errorf("total gold rose from %.2f to %.2f; banking is creating money", start, high)
	}
	// And the direction the old test could not see. A vault or a works fund left out of
	// TotalCoin would show here as money quietly disappearing.
	if low < start-tol {
		t.Errorf("total gold fell from %.2f to %.2f; coin is being lost, or a pool is "+
			"missing from TotalCoin", start, low)
	}
}

// Deposits are a claim on the vault, not a second pile of coin. If the sum of what people
// think they have exceeds what the bank holds, the difference is lent out — which is
// allowed — but the vault must never hold more than has been deposited into it.
func TestVaultAndDepositsAgree(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(8 * TicksPerYear)

	for _, bid := range s.byType(Bank) {
		b := &s.Structs[bid]
		var claims float32
		for i := range s.Chars {
			c := &s.Chars[i]
			if c.Alive && c.DepositAt == bid {
				claims += c.Deposit
			}
		}
		var lent float32
		for i := range s.Structs {
			if st := &s.Structs[i]; st.Alive && st.OwedTo == bid {
				lent += st.Debt
			}
		}
		if b.Vault < 0 {
			t.Errorf("bank %d holds a negative vault of %.2f", bid, b.Vault)
		}
		// Everything deposited is either still in the vault or out on loan. Interest
		// credited to depositors comes from the bank's own earnings and is moved into the
		// vault at the same time, so the identity holds with a float tolerance.
		if claims > b.Vault+lent+1 {
			t.Errorf("bank %d: depositors claim %.2f but vault holds %.2f with %.2f on loan",
				bid, claims, b.Vault, lent)
		}
	}
}

// A business must not be able to borrow its way into an ever-deepening hole. Absorbing
// states are the dominant bug class in this simulation, and a debt spiral is one.
func TestDebtIsCapped(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(8 * TicksPerYear)

	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Debt <= 0 {
			continue
		}
		cap := s.debtCap(StructID(i))
		if cap <= 0 {
			continue // pays nothing, so the cap has no meaning
		}
		// Interest rolls up between daily collections, so the cap can be grazed — but a
		// loan past it defaults and is written off, and nothing may grow beyond it.
		if st.Debt > cap*1.5 {
			t.Errorf("%v (%d) owes %.1f against a cap of %.1f — the debt spiral is open",
				st.Type, i, st.Debt, cap)
		}
	}
}

// Nobody should starve beside their own savings.
func TestHungryDepositorsDrawOnTheirSavings(t *testing.T) {
	s := newTestSim(5)
	s.RunTicks(3 * TicksPerYear)

	// Put somebody in the position: hungry, no coin, money in the bank.
	banks := s.byType(Bank)
	if len(banks) == 0 {
		t.Skip("no bank was founded")
	}
	b := &s.Structs[banks[0]]
	var who CharID = NoChar
	for i := range s.Chars {
		c := &s.Chars[i]
		if c.Alive && c.Stage() != Child {
			who = CharID(i)
			break
		}
	}
	if who == NoChar {
		t.Skip("nobody to test")
	}
	c := &s.Chars[who]
	c.Pos = b.Pos
	c.Gold = 0
	c.Deposit = 100
	c.DepositAt = banks[0]
	b.Vault += 100
	c.Hunger = 90
	c.Rations = 0

	before := c.Deposit
	for i := 0; i < TicksPerDay && s.Chars[who].Gold == 0; i++ {
		s.Step()
	}
	if s.Chars[who].Deposit >= before && s.Chars[who].Gold == 0 {
		t.Errorf("a hungry depositor with %.0f in the bank drew nothing and holds no coin",
			before)
	}
}
