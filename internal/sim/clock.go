package sim

import "fmt"

// The master constant (design §2.10): one real second is thirty in-world minutes.
//
// Every rate in the simulation is expressed against these, so that changing the pace of
// the world is a single edit rather than a hunt through scattered magic numbers.
const (
	TickRate       = 10 // ticks per real second
	MinutesPerTick = 3  // in-world minutes advanced by one tick

	TicksPerHour = 60 / MinutesPerTick // 20
	TicksPerDay  = 24 * TicksPerHour   // 480 — 48 real seconds
	DaysPerYear  = 365
	TicksPerYear = DaysPerYear * TicksPerDay // 175,200 — ~4.9 real hours
)

// Tick is a count of simulation steps since the world began. It is the only clock the
// simulation has: nothing may consult wall-clock time, or replay stops reproducing
// (§9.2).
type Tick int64

// Date is a Tick decomposed into human-readable in-world time.
type Date struct {
	Year   int
	Day    int // 0-364
	Hour   int // 0-23
	Minute int
}

// Date converts a tick to in-world time.
func (t Tick) Date() Date {
	d := Date{}
	d.Year = int(t / TicksPerYear)
	rem := t % TicksPerYear
	d.Day = int(rem / TicksPerDay)
	rem %= TicksPerDay
	d.Hour = int(rem / TicksPerHour)
	d.Minute = int(rem%TicksPerHour) * MinutesPerTick
	return d
}

// Hour returns the hour of the in-world day, 0-23.
func (t Tick) Hour() int { return int(t % TicksPerDay / TicksPerHour) }

// DayOf returns which whole day a tick falls in, counting from the world's creation.
func (t Tick) DayOf() int64 { return int64(t / TicksPerDay) }

// Years converts a tick count to a fractional number of in-world years, which is the
// unit ages and tenure are kept in.
func (t Tick) Years() float64 { return float64(t) / TicksPerYear }

func (d Date) String() string {
	return fmt.Sprintf("year %d, day %d, %02d:%02d", d.Year+1, d.Day+1, d.Hour, d.Minute)
}

// Working hours. People go to work after dawn and go home before dark, which gives the
// village a visible daily rhythm rather than a uniform smear of activity.
const (
	WorkStartHour = 6
	WorkEndHour   = 18
)

// IsWorkTime reports whether the working day is under way.
func (t Tick) IsWorkTime() bool {
	h := t.Hour()
	return h >= WorkStartHour && h < WorkEndHour
}

// --- Deterministic randomness ---

// Rand is a small deterministic pseudo-random source.
//
// math/rand is avoided deliberately: its algorithm and seeding have changed between Go
// releases, and a world must replay identically for as long as the code exists. This is
// splitmix64, which is short enough to be obviously stable.
type Rand struct{ state uint64 }

// NewRand returns a source seeded from a world seed and a stream identifier. Distinct
// streams let unrelated systems draw numbers without perturbing each other's sequences.
func NewRand(seed int64, stream uint64) *Rand {
	return &Rand{state: uint64(seed)*0x9E3779B97F4A7C15 + stream*0xBF58476D1CE4E5B9}
}

// Uint64 returns the next value in the sequence.
func (r *Rand) Uint64() uint64 {
	r.state += 0x9E3779B97F4A7C15
	z := r.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Float64 returns a value in [0, 1).
func (r *Rand) Float64() float64 {
	return float64(r.Uint64()>>11) / (1 << 53)
}

// Intn returns a value in [0, n).
func (r *Rand) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.Uint64() % uint64(n))
}

// Chance reports whether an event of probability p occurs.
func (r *Rand) Chance(p float64) bool { return r.Float64() < p }

// Range returns a value in [lo, hi).
func (r *Rand) Range(lo, hi float64) float64 { return lo + r.Float64()*(hi-lo) }
