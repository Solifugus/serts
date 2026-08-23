# Performance backlog (§9.4)

Success made the simulation expensive: measurement runs went from twenty minutes to
hours because populations tripled and several sweeps scale with headcount. Live play
needs 400+ people at 10 ticks/second; batch measurement needs centuries per hour.

Rules for this work, in force before any of it starts:

- **Profile first; fix measured hot spots only.** The suspect list below is
  pre-registered hypothesis, not a work order — the day's diagnostic record says
  confident guesses run one in five.
- **Behaviour-identical or bust.** Every optimisation must reproduce the same tick
  stream bit for bit (TestSimulationIsDeterministic compares whole structs). Cached
  values must be maintained incrementally at their mutation sites, never recomputed on
  a different schedule — a daily cache of a per-tick quantity is a behaviour change
  wearing a speedup's clothes.
- One change, then the determinism pair, then a timed 20-year run as the benchmark.

Pre-registered suspects, by expected weight:

1. **dependants() / childrensShare()** — O(chars) scans inside per-character decision
   code: worst case O(n²) per tick. Fix: per-home small-child counters maintained at
   birth, death, move, and the tick a child crosses ForageAge (age is checked per tick
   in stepNeeds already, so the crossing is observable incrementally).
2. **Pairing loop (stepBirths)** — O(unmarried²) daily with a distance check. Likely
   acceptable (the unmarried pool is small); measure before touching.
3. **hireServants purse scans** — O(homes × chars) daily. Fix: per-home adult-purse
   sums in the same incremental family of counters, updated where Gold moves... which
   is many sites; alternatively one O(chars) pass building all purses before the homes
   loop (same schedule, same values, strictly fewer reads).
4. **Occupied()/nearestWith/nearestOfType/NearestFoodSource** — linear struct scans
   per call. Structs are few (<100); likely cheap. Measure.
5. **Movement & flow fields** — already shared per destination; goldDirty rebuilds are
   event-driven. Probably fine; the profiler will say.
6. **countHouseholds daily census** — O(chars), fine; candidate HOME for any new
   per-home aggregates rather than new sweeps.

Non-goals: struct-of-arrays (§9.2's note stands — revisit only on profiler evidence),
parallelism inside a tick (determinism risk dwarfs the win at current scale).
