package sim

import "github.com/solifugus/serts/internal/torus"

// Banking (§4.4).
//
// The village is not short of money. Measured at year three: 13,823 gold sat in people's
// purses, 9,815 of it above what a year's food costs, one purse alone holding 2,315 — and
// every employer's till in the world held 1,264 between them. Businesses pushing three
// hundred gold a year through their books ran on floats of under three, and dismissed
// their staff the moment an outflow preceded an inflow.
//
// So the churn was never a shortage of coin. It was the absence of any means of moving it
// from where it was idle to where it was needed. That gap is what a bank is.
//
// Two things follow from measurement rather than from theory:
//
//   - Lending here is bridging, not rescue. Six of the eight employers that ran dry took
//     in across the year at least what they paid out; they are illiquid, not insolvent,
//     which is the one case where credit genuinely helps rather than dressing a failure in
//     debt. The other two were losing money, and BankDebtCap is what stops the bank
//     funding them indefinitely.
//   - Nothing may be conjured. Gold is conserved and the suite enforces it, so a loan is
//     coin moved out of the vault and a repayment is coin moved back. The bank can lend
//     only what it actually holds, which is why deposits come first and why a bank whose
//     borrowers default cannot honour every withdrawal — a bank run, arrived at honestly.
//
// What this deliberately does NOT do yet: no foreclosure, no bankruptcy, no rate that
// responds to demand, no competition between banks. Those are the interesting parts and
// they are worth nothing if the simple loop does not first pay for itself.

const (
	// BankDepositFloor is how much a person keeps to hand before banking the rest, as a
	// multiple of a year's food.
	//
	// Generous on purpose. Someone who banks down to their last coin starves the first
	// time the vault is short, and the measured hoards are many years of food deep — the
	// idle capital this exists to mobilise is well above any sensible reserve, so there is
	// no need to go near the bone to reach it.
	BankDepositFloor = 1.5

	// BankDepositMin is the smallest sum worth a trip to the bank, in gold. Without it
	// every villager banks a fraction of a coin every day and the ledger fills with noise.
	BankDepositMin = 5.0

	// BankLoanDays is how much payroll one loan covers. Short: the point is to carry an
	// employer across the gap between paying wages and taking receipts, not to finance
	// them for a season.
	BankLoanDays = 10.0

	// BankDebtCap is the most an employer may owe, in days of its own payroll. This is the
	// guard against the failure mode that has bitten this project more than any other: an
	// absorbing state, here a business borrowing to pay interest on what it borrowed. A
	// firm at the cap gets no more credit and fails the way it would have failed anyway.
	BankDebtCap = 45.0

	// BankLoanRate and BankDepositRate are annual, and the spread between them is the
	// bank's income — it pays its clerks out of the difference, so a bank that lends
	// nothing cannot staff itself.
	BankLoanRate    = 0.12
	BankDepositRate = 0.05

	// BankRange is how far a person or a business will go to bank, in cells. A settlement's
	// bank serves that settlement.
	BankRange = 40
)

// bankFor finds the bank serving a position, or NoStruct.
func (s *State) bankFor(pos torus.Vec2) StructID {
	best, bestD := NoStruct, BankRange*BankRange*1.0
	for _, sid := range s.byType(Bank) {
		st := &s.Structs[sid]
		if !st.Alive {
			continue
		}
		if d := s.T.Dist2(pos, st.Pos); d < bestD {
			best, bestD = sid, d
		}
	}
	return best
}

// stepBank runs the daily business of every bank: deposits in, loans out, repayments back.
//
// Daily rather than per tick. Payroll is a per-tick event, but the shortfall it exposes
// lasts hours, and a loan drawn once a day is enough to carry an employer through it —
// while a per-tick credit check would put the bank in the hot path of the whole
// simulation for no gain.
func (s *State) stepBank() {
	if s.Tick%TicksPerDay != 0 {
		return
	}
	if len(s.byType(Bank)) == 0 {
		return
	}
	s.takeDeposits()
	s.lendWorkingCapital()
	s.collectRepayments()
	if s.Tick%TicksPerYear == 0 {
		s.payDepositInterest()
	}
}

// takeDeposits moves idle coin out of purses and into the vault.
//
// Only what is genuinely surplus: a floor of a year and a half's food stays in hand, which
// is far above what anyone needs day to day and far below the hoards this is reaching for.
func (s *State) takeDeposits() {
	// The reserve is struck against each bank's own local prices, once per bank rather
	// than once per villager. Computing it per character meant a hall lookup for every
	// adult every day, which is the same hot spot that cost 24% of the tick when
	// marketHall was first written.
	banks := s.byType(Bank)
	keeps := make([]float32, len(banks))
	room := make([]float32, len(banks))
	for i, bid := range banks {
		keeps[i] = float32(BankDepositFloor) * s.yearOfFood(s.Structs[bid].Pos)
		// A bank takes deposits it can lend, and no more.
		//
		// Without this it is a hole in the money supply rather than a conduit through it.
		// Measured: the first version drew 13,084 gold out of circulation in a single year
		// and had 289 of it out on loan, because the borrowing need is genuinely small —
		// a few gold, for a few hours, a few dozen times a year. Idle coin in a vault is
		// no more use to anybody than idle coin in a mattress, and taking it there cost a
		// quarter of the village.
		room[i] = s.lendingRoom(bid) - s.Structs[bid].Vault
	}

	for i := range s.Chars {
		c := &s.Chars[i]
		if !c.Alive || c.Stage() == Child {
			continue
		}
		// Cheapest test first: almost nobody has a spare five gold on any given day, and
		// finding their bank is more work than reading their purse.
		if c.Gold < BankDepositMin {
			continue
		}
		bid, which := NoStruct, -1
		bestD := BankRange * BankRange * 1.0
		for k, cand := range banks {
			st := &s.Structs[cand]
			if !st.Alive {
				continue
			}
			if d := s.T.Dist2(c.Pos, st.Pos); d < bestD {
				bid, which, bestD = cand, k, d
			}
		}
		if bid == NoStruct {
			continue
		}
		spare := c.Gold - keeps[which]
		if spare > room[which] {
			spare = room[which]
		}
		if spare < BankDepositMin {
			continue
		}
		room[which] -= spare
		b := &s.Structs[bid]
		c.Gold -= spare
		b.Vault += spare
		c.Deposit += spare
		c.DepositAt = bid
		s.diarise(CharID(i), "put %.0f gold in the bank", spare)
	}
}

// Withdraw returns a depositor's money when they need it, up to what the vault can pay.
//
// Called when somebody cannot afford to eat. A bank that has lent to defaulters cannot
// honour every claim, which is a bank run and is left to happen rather than papered over:
// the coin genuinely is in somebody else's purse.
func (s *State) withdraw(id CharID, want float32) float32 {
	c := &s.Chars[id]
	if c.Deposit <= 0 || c.DepositAt == NoStruct {
		return 0
	}
	b := &s.Structs[c.DepositAt]
	if !b.Alive || b.Type != Bank {
		return 0
	}
	got := want
	if got > c.Deposit {
		got = c.Deposit
	}
	if got > b.Vault {
		got = b.Vault
	}
	if got <= 0 {
		return 0
	}
	b.Vault -= got
	c.Deposit -= got
	c.Gold += got
	if c.Deposit <= 0 {
		c.DepositAt = NoStruct
	}
	return got
}

// lendWorkingCapital advances an employer enough to meet payroll while it waits to be paid.
func (s *State) lendWorkingCapital() {
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Jobs == 0 || st.Wage <= 0 || st.Filled == 0 || st.Type == Bank {
			continue
		}
		// A day's payroll is the unit of need. Borrow only when the till cannot cover it.
		daily := st.Wage * WorkTicksPerDay * float32(st.Filled)
		if daily <= 0 || st.Gold >= daily {
			continue
		}
		if s.Tick-st.DefaultedAt < BankDefaultBar && st.DefaultedAt > 0 {
			continue // recently wrote a loan off against this business
		}
		cap := s.debtCap(StructID(i))
		if st.Debt >= cap {
			continue // at the limit: no more credit, and it fails as it would have
		}
		bid := s.bankFor(st.Pos)
		if bid == NoStruct {
			continue
		}
		b := &s.Structs[bid]
		want := daily*BankLoanDays - st.Gold
		if room := cap - st.Debt; want > room {
			want = room
		}
		if want > b.Vault {
			want = b.Vault // lend only what is actually held
		}
		if want <= 0 {
			continue
		}
		b.Vault -= want
		st.Gold += want
		st.Debt += want
		st.OwedTo = StructID(bid)
		s.Led.Lent += want
	}
}

// collectRepayments takes back principal and interest from businesses that have money.
//
// A business repays out of what is left after it can meet a day's wages, so servicing a
// loan never causes the dismissal the loan existed to prevent.
func (s *State) collectRepayments() {
	daysPerYear := float32(DaysPerYear)
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Debt <= 0 || st.OwedTo == NoStruct {
			continue
		}
		b := &s.Structs[st.OwedTo]
		if !b.Alive || b.Type != Bank {
			st.Debt, st.OwedTo = 0, NoStruct // the lender is gone; so is the debt
			continue
		}
		// Interest first, and it accrues whether or not there is anything to pay it with.
		interest := st.Debt * BankLoanRate / daysPerYear
		// Leave a working buffer, not a day's wages. Sweeping everything above one day's
		// payroll into the bank left every borrower permanently at the edge of the
		// insolvency that made it borrow — the cure administering the disease.
		daily := st.Wage * WorkTicksPerDay * float32(st.Filled)
		spare := st.Gold - daily*BankLoanDays
		if spare <= 0 {
			// Interest rolls up on a borrower that cannot service it — but only to the
			// cap. Past that the loan has failed, and pretending otherwise is the debt
			// spiral this was supposed to prevent: the first version capped new LENDING
			// and let interest compound freely, and a granary with a daily payroll of 1.46
			// came to owe 4,858 — three thousand days of wages, growing forever, on a
			// business that was never going to repay.
			st.Debt += interest
			if st.Debt > s.debtCap(StructID(i)) {
				s.defaultLoan(StructID(i))
			}
			continue
		}
		pay := spare
		if pay > interest {
			b.Gold += interest // the bank's own earnings, which pay its clerks
			pay -= interest
			st.Gold -= interest
			s.Led.Interest += interest
			if pay > st.Debt {
				pay = st.Debt
			}
			st.Gold -= pay
			st.Debt -= pay
			b.Vault += pay // principal returns to the depositors' pool
			s.Led.Repaid += pay
			if st.Debt <= 0 {
				st.Debt, st.OwedTo = 0, NoStruct
			}
		} else {
			b.Gold += pay
			st.Gold -= pay
			st.Debt += interest - pay
			s.Led.Interest += pay
		}
	}
}

// payDepositInterest shares the year's earnings with the depositors whose money did the
// lending. Paid out of what the bank has actually earned, so a bank that lent badly pays
// its depositors nothing rather than inventing the difference.
func (s *State) payDepositInterest() {
	for _, bid := range s.byType(Bank) {
		b := &s.Structs[bid]
		if !b.Alive || b.Gold <= 0 {
			continue
		}
		var owed float32
		for i := range s.Chars {
			c := &s.Chars[i]
			if c.Alive && c.DepositAt == bid && c.Deposit > 0 {
				owed += c.Deposit * BankDepositRate
			}
		}
		if owed <= 0 {
			continue
		}
		// Never more than the bank made. Wages come first — a bank that cannot pay its
		// clerks closes, and then nobody's deposit is worth anything.
		reserve := b.Wage * WorkTicksPerDay * float32(b.Jobs) * 30
		payable := b.Gold - reserve
		if payable <= 0 {
			continue
		}
		share := float32(1)
		if payable < owed {
			share = payable / owed
		}
		for i := range s.Chars {
			c := &s.Chars[i]
			if !c.Alive || c.DepositAt != bid || c.Deposit <= 0 {
				continue
			}
			amt := c.Deposit * BankDepositRate * share
			if amt <= 0 {
				continue
			}
			b.Gold -= amt
			b.Vault += amt
			c.Deposit += amt
		}
	}
}

// yearOfFood is what a year's eating costs where somebody stands.
func (s *State) yearOfFood(pos torus.Vec2) float32 {
	return s.FoodPriceAt(pos) * MealsPerDay * DaysPerYear
}

// lendingRoom is how much a bank can usefully hold: what its borrowers currently owe, plus
// a reserve against the demand it can expect from the employers it serves.
//
// The measured need is small — a business borrows a few days of payroll, briefly — so a
// vault sized to it leaves the rest of the world's coin where it was. A bank is worth
// having because it moves money to where work is waiting on it, not because it is large.
func (s *State) lendingRoom(bid StructID) float32 {
	b := &s.Structs[bid]
	var need float32
	for i := range s.Structs {
		st := &s.Structs[i]
		if !st.Alive || st.Jobs == 0 || st.Wage <= 0 || st.Filled == 0 || st.Type == Bank {
			continue
		}
		if s.T.Dist2(b.Pos, st.Pos) > BankRange*BankRange {
			continue
		}
		need += st.Wage * WorkTicksPerDay * float32(st.Filled) * BankLoanDays
	}
	return need
}

// BankDefaultBar is how long a business waits for credit after failing on a loan.
const BankDefaultBar = Tick(5 * TicksPerYear)

// debtCap is the most a business may owe, in days of the payroll it would run at full
// staffing. Measured against its posts rather than the people currently in them, so that
// a business which has just shed its workers does not acquire a cap of nothing and default
// on that account alone.
func (s *State) debtCap(sid StructID) float32 {
	st := &s.Structs[sid]
	staff := st.Filled
	if staff < 1 {
		staff = 1
	}
	return st.Wage * WorkTicksPerDay * float32(staff) * BankDebtCap
}

// defaultLoan writes a failed loan off against the bank's own capital.
//
// The coin is long gone — it was paid out as wages months ago and is in somebody's purse.
// What is destroyed here is the bank's claim on it, which is why the loss falls on the
// bank and, through a thinner vault, on its depositors. That is what bad lending costs,
// and it is left to cost it.
func (s *State) defaultLoan(sid StructID) {
	st := &s.Structs[sid]
	if st.Debt <= 0 {
		return
	}
	s.Led.WrittenOff += st.Debt
	st.Debt = 0
	st.OwedTo = NoStruct
	st.DefaultedAt = s.Tick
	if st.Owner != NoChar && s.AliveChar(st.Owner) {
		s.diarise(st.Owner, "defaulted on the loan against the %s", Defs[st.Type].Name)
	}
}
