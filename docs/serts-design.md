# SERTS — Simulation Eternal RTS

## 1. Premise

SERTS is an RTS shaped like an open-world simulation. Characters are not units — they
are people. They age, take work, earn wages, form partnerships, raise children, grow
skilled, and die. The player is not their hand; the player is their government.

Players operate factions primarily by setting **policy** and **objectives**, and only
occasionally by direct command. The world is **eternal**: it runs whether or not you are
watching, and it is shared with other players.

### Design pillars

1. **People are not fungible.** A 40-year veteran smith is not replaceable by a
   conscript. Losing one should hurt.
2. **Population is the contested resource.** Territory and ore matter because people
   work them. The primary way to grow is to attract, out-breed, or capture people —
   not merely to kill them.
3. **The player governs, and intervenes.** Most of the time you steer. Sometimes you
   grab the wheel.
4. **The world outlives the session.** Nothing pauses because you logged off.

### The spine: a faction is its payroll

There is exactly one relationship in this game — **employment** — and everything else is
a consequence of it. There are no abstract "activity types"; a character is simply
employed at a structure, and the structure defines the work.

From that single bond, the rest follows:

| Concept | Is really |
|---|---|
| Faction membership | Who employs you |
| Recruitment | Hiring |
| Conquest | Acquiring employees |
| Capture | Forced unemployment, then rehiring |
| Defection | A better offer |
| Growth | Payroll expansion |
| Birth and death | Payroll turnover |
| Construction | A site that employs people until it is finished |
| War | A hostile takeover |
| Trade | Business between firms |

**Use this as the design test.** When considering a new mechanic, ask whether it routes
through employment. Mechanics that bypass it are where incoherence enters — see §8.4 for
the standing example, and §3.7 for the counterweight that keeps employment from reducing
people to commodities.

### Relation to Globulation 2

Globulation 2 is the closest existing relative, and a deliberate reference point: an RTS
built around *indirect* control, where units self-assign to work rather than being
individually micromanaged. SERTS shares that core philosophy.

Where SERTS diverges:

- **Generations.** Characters age, partner, reproduce, and die. Glob2's units do not
  have life cycles; in SERTS, demography *is* the game.
- **A monetary economy.** Wages, prices, taxation, and gold faucets and sinks, rather
  than direct assignment of abstract resources.
- **Capture over kill.** Conquest is primarily about acquiring people (§7.2).
- **A living carrying capacity.** World fertility is governed dynamically (§2.4).
- **Persistent and eternal.** The world runs continuously; there is no match to win.
- **Presentation.** Fullscreen world view with overlay HUD, rather than a viewport
  framed by fixed panels (§8.5).

---

## 2. The World

### 2.1 A wrapped world — the torus

The world is a **fixed-size rectangular space whose edges wrap on both axes**. A
character crossing past `max_x` reappears at `x = 0`, and likewise for `y`. This is a
*torus* — a "pseudo-sphere" for gameplay purposes.

A torus is not geometrically a sphere, and that is an advantage: it has zero curvature,
so there is no distortion, no polar singularities, and no variation in density or
distance anywhere on the map. Every point is topologically identical to every other.

**Design consequences:**

- **No corners and no edges.** No player gets a naturally defensible back wall. Every
  faction can be approached from all four directions at all times.
- **No safe rear.** Constant omnidirectional pressure. Defensive terrain must be
  *generated* — mountains, rivers, lakes, ravines — because the topology gives you none
  for free. This is the main burden placed on map generation (§2.8, §2.9).
- **Encirclement is natural.** Since there is no edge to back against, genuinely
  surrounding a group is achievable, which makes the capture mechanic (§7.2) far more
  live than it would be on a bounded map.
- **One wall divides nothing.** A wall spanning the full height of the world wraps into
  a closed loop — but cutting a torus along such a loop yields a *cylinder*, which is
  still connected. Attackers simply go around the other way. **Two** parallel wrapping
  walls are required to split the world into two halves. Enclosing a local pocket of
  territory works normally.
- **World size is the scarcity dial.** With no expansion mechanic, the fixed dimensions
  set total available land and therefore the entire pressure curve of the game.

### 2.2 Continuous space over grid terrain

**The world is continuous in both space and time.**

- **Movement is continuous.** Characters hold float positions and move at any angle.
  Nothing snaps to a cell, and crowds flow rather than step.
- **Terrain is a grid.** Soil fertility, woodland, and ore deposits are stored per cell.
  This is not a compromise but a requirement: the woodland regrowth rule is a
  cellular automaton (§2.5) — wood spreads only from adjacent wood — and soil depletion and
  recovery are naturally per-cell quantities.
- **Time is continuous.** The world runs in real time, always, with no pause, no
  loading, no zoning, and no boundaries of any kind (§2.10).

Starting size: **1024 × 1024 terrain cells**, with world space running 1024.0 × 1024.0
units so that one cell is exactly one unit and conversion is a floor operation. Tuned so
crossing the world on foot is a meaningful journey rather than a formality.

The two layers pair cleanly for movement: **flow fields are computed over the grid, and
characters sample the field and steer continuously**. Grid where the simulation wants
discrete cells, continuous where the player wants smooth motion. See §9.3 for the
wrapping rules both layers must obey.

### 2.3 Regions

The world is internally partitioned into **regions** — bounded chunks of terrain used
purely as an implementation detail for:

- simulation parallelism (one tick job per region)
- spatial queries and culling
- persistence granularity

Regions are invisible to players and carry no authority semantics. Region borders wrap
along with the world.

### 2.4 Carrying capacity — the load governor

The number of characters the world supports is **governed by the real compute resources
of the host machine**, expressed in-world as the fertility of the land.

- The host samples its tick-budget utilization over a rolling window (~60s).
- It targets a set utilization (start at **70%** of tick budget).
- It emits a `fertility` multiplier, clamped to **[0.25, 2.0]**, scaling farm yield and
  wild forage.
- Abundant CPU → fertile land → food surplus → population grows.
- Strained CPU → lean land → scarcity → population contracts.

**Constraint — keep it out of the tick.** The multiplier is decided on a slow cycle and
written into world state as a versioned value. The simulation reads only that stored
value, never a live CPU sample. This keeps the tick deterministic and replayable (§9.2);
if the simulation reads machine conditions directly, identical replays stop producing
identical results and tuning becomes guesswork.

**Constraint — damping.** Population responds to food supply with a long lag (gestation,
then a childhood dependency period). A naive controller will oscillate into boom-famine
cycles that read as bugs. Required mitigations:

- adjust slowly — cap change at ~5% per minute
- a deadband of ±10% around target utilization, inside which nothing changes
- derivative damping — react to the *trend* of load, not the instantaneous value

**Constraint — dress it in-world.** A governor visibly driven by machine load breaks the
fiction: on a busy server everyone's harvest fails for no reason the world can explain,
and players will read it as arbitrary. Wrap it in a **seasonal and climate cycle** the
fiction already justifies — good years and lean years, long winters, droughts. The
governor then rides on top of variation players already accept, which makes it both
invisible and thematically richer than a flat multiplier would have been.

**Fairness.** In multiplayer all factions share one host and one fertility value, so the
governor is inherently even-handed. The remaining design question is whether fertility
should also vary *spatially* — fertile basins and barren steppe — as an authored
property multiplied by the global governor value. Recommended: yes. It creates worth
fighting over, and abundance that attracts both migrants and raiders is self-balancing.

### 2.5 Resource dynamics — land that changes

Resources are not a static layer to be strip-mined. They **deplete where they are worked
and recover or appear elsewhere**, so the economically valuable parts of the map move
over time. This is the engine behind migration and resettlement (§2.7).

**Renewable — recovers in place**

- **Woodland.** Logging clears tiles. Cleared land regrows, but only from *adjacent*
  woodland, spreading outward at a slow rate. Consequence: selective logging regrows
  indefinitely, while clear-cutting an entire area removes the seed stock and the forest
  never returns. Ecological restraint becomes a genuine strategy rather than a moral.
- **Soil fertility.** A local per-tile value, drawn down by continuous farming and
  recovering while fallow. Effective farm yield is **local soil × global governor value**
  (§2.4). This pushes farms to rotate and relocate on their own.
- **Forage and game.** Minor wild food, regenerating, matters most to unaligned people
  and armies in the field.
- **Fish.** Renewable stock in rivers, lakes, and coastal water, depleted by overfishing
  and recovering when rested. Because it is independent of soil, it is the one food
  source that survives a soil crisis — and the reason lakeside ghost towns may never
  fully empty.

Floodplain cells adjacent to rivers carry a **higher base soil fertility and faster
recovery rate** (§2.8), which is what makes river valleys the map's prime farmland and
draws settlement onto water without any rule requiring it.

**Non-renewable — finite, but not fully known**

- **Coal, iron ore, gold ore, stone.** Deposits are finite and deplete permanently.
- **Prospecting.** Most deposits begin *undiscovered*. A **survey post** employs
  prospectors who reveal deposits within a radius over time. So mineral wealth does not
  regrow — but it does keep *appearing*, in places nobody had reason to look.

The two mechanisms differ honestly: forests come back, ore does not. What both produce is
the same pressure — **the map's centre of economic gravity drifts**, and no settlement
site is permanently correct.

### 2.6 Structure decay and ruins

See §5.2. Structures degrade fastest when **unused**, so a settlement that loses its
reason to exist visibly rots rather than persisting as tidy abandoned geometry.

### 2.7 The settlement cycle

The systems above combine into the game's principal long arc. Nothing implements this
cycle directly; it falls out of depletion, the job market, decay, and regrowth all
running at once:

1. **Discovery.** A deposit is prospected or good land is found. Structures go up.
2. **Boom.** Wages there are high, so the job utility function (§3.5) pulls people in.
   Homes are built, families form, children are born, roots deepen.
3. **Exhaustion.** The seam runs out or the soil is spent. Output falls, structures can
   no longer fund wages, and the same utility function that filled the town now empties
   it.
4. **Abandonment.** Unstaffed structures decay fast (§5.2). The settlement becomes a
   **ghost town** — a cluster of ruins, visible on the map, with graves still in it.
5. **Recovery.** Over decades, woodland creeps back and soil rests. The land becomes
   worth something again.
6. **Resettlement.** Ruins are cheap to rebuild on (§5.2), so the old site is the
   natural place to return to — and descendants of the people who left still carry
   root affinity to the ground their ancestors are buried in (§3.7).

Then it repeats. **This is what makes an eternal world worth running forever**: a player
who stays long enough watches the same valley fill, empty, and fill again with the
grandchildren of the people who left it.

> **The anti-snowball property.** This cycle interacts with roots (§3.7) in a way that
> was not designed in, but is worth protecting. Depletion forces every faction to
> eventually relocate — and **roots make relocation expensive in proportion to how long
> you have been successful.** A young faction with shallow roots migrates easily. An old,
> dominant faction is anchored by forty-year veterans, established homes, and family
> graves sitting on exhausted ground. Success creates its own drag, without any
> artificial rubber-banding. This is the best answer the design currently has to the
> snowball problem.

### 2.8 Water — rivers, lakes, and coasts

Water is the single most valuable terrain feature this design can have, because a torus
supplies no geography of its own (§2.1). Rivers and lakes carry an unusual amount of
weight here:

**They create the chokepoints the map otherwise lacks.** River width derives from flow
accumulation, which gives a free difficulty gradient:

| Water | Crossing |
|---|---|
| Stream | Fordable anywhere, slight slowdown |
| River | Fordable only at shallows; otherwise needs a bridge |
| Major river | Bridge or boat only |
| Lake / sea | Boat only |

**They make encirclement real.** Capture (§7.2) requires trapping people with no escape
route. On featureless ground that is hard to arrange; against a river bend, or on the
wrong side of a single bridge, it becomes a genuine tactic. Water is what makes the
central conquest mechanic reliably achievable.

**Bridges become strategic objects.** A bridge is a structure — built by a construction
site, employing builders, degrading with disuse, capturable, and destructible. Control of
a crossing controls a frontier. This is the most concentrated strategic value on the map.

**They make land unequal in a useful way.** Floodplain cells adjacent to rivers get
higher base soil fertility and faster soil recovery (§2.5). River valleys therefore
become prime farmland, settlements cluster along water without any rule saying they must,
and river ghost towns get resettled sooner than dry ones because their land heals faster.

**They give boats a purpose.** Boats already exist as a workshop good. Water transport
along rivers and across lakes moves goods faster and cheaper than overland hauling, so
river towns trade advantageously and confluences become natural market sites.

**They add a renewable food source.** A **fishery** on water yields food that does not
depend on soil, which makes coastal and lakeside settlement viable on otherwise poor
ground — and gives famine a second escape valve.

Farms should require proximity to fresh water for irrigation. This is a hard placement
constraint rather than a modifier, and it is what makes water genuinely contested rather
than merely pleasant.

### 2.9 Map generation

The world is generated once, from a **seed**, deterministically: the same seed always
produces the same world (§9.2). A world is therefore identifiable and reproducible by
seed alone, which matters for testing and for tuning runs.

Generation runs at world creation only, never per tick, so it may be expensive. Its cost
budget is entirely separate from the tick budget the load governor manages (§2.4).

**Everything must wrap.** This is the constraint that shapes the whole pipeline.
Ordinary Perlin or simplex noise sampled on a plane will not tile, and a visible seam on
a toroidal world is a permanent, unfixable scar. The correct technique is to sample
**4D noise on two circles**, which is exactly seamless in both axes rather than
approximately so:

```go
// Seamless 2D noise on a torus, via a 4D slice.
func TileableNoise(x, y float64, freq float64) float64 {
    u := 2 * math.Pi * x / WorldW
    v := 2 * math.Pi * y / WorldH
    r := freq / (2 * math.Pi)
    return Noise4D(r*math.Cos(u), r*math.Sin(u),
                   r*math.Cos(v), r*math.Sin(v))
}
```

Every neighbor lookup in every stage below uses `WrapCell` (§9.3).

**Pipeline**

1. **Elevation.** Multi-octave tileable noise (fBm), with domain warping so landmasses
   are ragged rather than blobby.
2. **Sea level.** Choose the threshold that hits a target land fraction (~60%) by binary
   search rather than fixing it by hand, so land area is stable across seeds. Cells
   below it are ocean, and ocean is the terminal sink for all drainage.
3. **Depression filling.** Priority-flood (Barnes et al., O(n log n)) over land.
   Basins with no outlet fill to their lowest rim and **become lakes**; the fill level is
   the lake surface.
4. **Flow direction.** D8 steepest descent to the lowest of eight wrapped neighbors.
5. **Flow accumulation.** Queue-based accumulation of upstream drainage area, wrapped.
6. **Rivers.** Cells above an accumulation threshold are river; **width scales with
   accumulation**, which produces the crossing-difficulty gradient in §2.8 for free, and
   also produces confluences, deltas, and lake outflows without special-casing.
7. **Moisture.** Distance to water plus prevailing-wind rain shadow behind high ground.
8. **Temperature.** A wrapping latitude band — `cos(2πy/H)` gives one warm belt and one
   cold belt with no seam — plus an elevation lapse rate and noise. This gives real
   climate variety while remaining perfectly toroidal.
9. **Biomes.** Derived from temperature × moisture × elevation.
10. **Terrain fields.** Soil fertility and initial woodland density from biome, with the
    floodplain bonus applied along rivers (§2.8).
11. **Ore deposits.** Placed by rule rather than raw noise — iron and coal in hills and
    ridges, gold in mountains and in river gravel downstream of them. Most are created
    **undiscovered**, awaiting prospecting (§2.5).
12. **Validation.** Reject and reseed if the map fails its checks.

**Validation matters more here than in most games.** The world is fixed and eternal, so a
bad map is not a bad match — it is a permanent defect. Check at minimum:

- sufficient contiguous land, and no large landmass unreachable without boats
- resource totals within band, and not all clustered in one quadrant
- **gold present and reachable.** A world with no gold has no money supply, since
  panning is the faucet the whole economy starts from (§4.2). Choosing gold by quantile
  rather than by an absolute threshold makes this reliable — a threshold left the amount
  hostage to whether a world happened to have mountains, and produced worlds with
  literally none
- enough distinct viable settlement sites for the settlement cycle (§2.7) to have
  somewhere to migrate *to*
- for multiplayer, start positions that are well separated and comparably provisioned

**A generation goal worth stating explicitly:** the map must stay interesting for a very
long time. Uniform terrain produces one boom and then a static equilibrium. What the
settlement cycle needs is *variety in when and where land becomes valuable* — scattered
deposits of differing size, river valleys of differing quality, marginal land that only
becomes worth settling once the good land is exhausted.

### 2.10 Time and persistence

The world runs continuously in real time. A faction whose player is offline keeps
living, working, aging — and can be attacked.

Because logging off must not be a death sentence, the design owes players **standing
orders**: defensive postures, garrison assignments, supply thresholds, auto-restocking,
and rules of engagement that persist and execute without supervision. A well-configured
faction should survive a night. A neglected one should not.

#### The master constant

**1 real day = 1 in-world year.** Every other rate in the design — fertility, the skill
curve constant `T`, structure decay, woodland regrowth, soil recovery — is tuned against
this one number, so it is fixed early and changed reluctantly.

The year runs 360 days rather than 365 so the tick arithmetic divides exactly.

| Quantity | Value |
|---|---|
| Tick rate | 10 Hz |
| In-world hour | 100 ticks — 10 real seconds |
| In-world day | 2,400 ticks — **4 real minutes** |
| In-world year | 864,000 ticks — **exactly 24 real hours** |
| Adulthood (age 15) | 15 real days |
| Full life (age 60) | 60 real days (~2 months) |
| Settlement cycle (§2.7) | tuned by regrowth, target ~1 month |

Three things this rate buys, in order of how much they matter:

**It is fast enough to watch.** This is the argument that decided it. A simulation about
people living needs their living to be legible: at four real minutes to the in-world day,
a working day takes two, and a villager crosses town in about twenty-five seconds. At the
rate first chosen — 48 real seconds to the day — people effectively teleported, and the
daily rhythm the design is built around could not be seen at all.

**It is kind to intermittent players.** The world runs whether or not anyone is watching
(§2.10), so the cost of absence is set entirely by this constant. A week away costs seven
in-world years: substantial, survivable. At five times the speed it would have cost a
quarter of a lifetime, which punishes exactly the players a persistent world most needs to
keep.

**It is legible without arithmetic.** "A year a day" is a rate a player can hold in their
head and plan around.

> **Cycle length is independent of lifespan.** An earlier draft argued for a faster clock
> on the grounds that the settlement cycle spans three or four lifetimes, which at this
> rate would put it half a year away. That was an assumption, not a constraint. The cycle
> is governed by **regrowth rates**, which tune separately — real soil recovers over a few
> fallow years and woodland over twenty to fifty. Set woodland regrowth near 25 in-world
> years and the cycle completes in about a month of real time, comfortably inside any
> player's tenure. Do not couple the two knobs.

#### Development time compression

Tuning demography is impossible at wall-clock rate — a single generation takes days. The
simulation therefore accepts a **speed multiplier** that consumes N ticks per frame.

This must be understood as changing only *how fast ticks are consumed*, never what a tick
does. The simulation itself has no notion of real time, so a century run at 10,000× is
bit-identical to the same century run live (§9.2). If that ever stops being true,
something has leaked wall-clock into the tick and determinism is already broken.

---

## 3. Characters

### 3.1 Attributes

| Attribute | Range | Notes |
|---|---|---|
| Age | 0–~80 | Drives every life stage |
| Health | 0–100 | Zero is death |
| Hunger | 0–100 | Rises constantly; eating resets it |
| Fatigue | 0–100 | Rises while working; sleep resets it |
| Morale | 0–100 | Drives loyalty and defection |
| Gold | 0+ | Personal wealth, carried |
| Skills | per-role float | Tenure-based efficiency, see 3.4 |
| Home | ref or null | Null causes health decay |
| Faction | ref or null | Null = unaligned |
| Partner | ref or null | See 3.3 |

### 3.2 Life cycle

| Stage | Age | Behavior |
|---|---|---|
| Child | 0–14 | Cannot work. Consumes food. Requires a guardian. |
| Adult | 15–55 | Works, mates, fights, can be taxed |
| Elder | 56+ | Works at reduced rate, no longer fertile, mortality climbs |

- **Fertility window:** 18–45. A partnered pair in the window with adequate food, a
  home with capacity, and morale above a floor produces a child on a probabilistic
  roll (tune toward roughly one child per pair per 2–4 in-world years).
- **Dependency:** children must be assigned to a home with a resident adult. Orphans
  lose morale and health and are prime candidates for capture or defection.
- **Mortality:** near-zero from 15–50, then a rising curve. Starvation, injury,
  homelessness, and low morale all add to it.

Population dynamics are the most failure-prone system in the game. Tune for **slow
growth under good conditions** — exponential blowup is far easier to trigger than you
expect, and the load governor should not be the only thing standing between the design
and a runaway.

### 3.3 Partnership

Adults in the fertility window seek partners, weighted by proximity, shared faction,
morale, and material security (home + income). Partnership is durable; on a partner's
death the survivor may re-partner after a mourning period, at reduced likelihood.

Partnership matters mechanically because it gates reproduction, and emotionally because
it creates families the player can recognize and lose.

### 3.4 Skill and tenure

The longer a character works a role, the more efficient they become, with diminishing
returns:

```
efficiency = 1 + k * ln(1 + tenure_hours / T)
```

Tuned so a lifetime of work approaches roughly 2.5–3× a novice's output, with most of
the gain in the first several in-world years. Skill is **per structure type** and decays
slowly when unused, so moving a master farmer to a barracks is a genuine sacrifice — and
a structure destroyed is a lifetime of accumulated skill left with nowhere to apply it.

### 3.5 Job selection — the utility function

This is the engine of the whole simulation and deserves more care than any other
function in the codebase. Each unemployed adult scores **every reachable structure with
an opening**:

```
score = wage
      * skill_fit(character, structure.type)
      * distance_penalty(character, structure)   // toroidal distance — see §9.3
      * need_urgency(character)
      * faction_affinity(character, structure.owner)
      * policy_weight(structure.type)
```

- `wage` — offered by the structure, funded from its own gold
- `skill_fit` — the character's accumulated efficiency at that structure type
- `distance_penalty` — decays with travel time, **measured on the torus**
- `need_urgency` — a starving character takes worse work; a comfortable one holds out
- `faction_affinity` — resistance to leaving one's people; see §3.7
- `policy_weight` — the player's thumb on the scale (see §8.1)

Everything the player does economically expresses itself as pressure on this one
function. There is no separate job system to maintain.

**Unemployment** is therefore a central event, not an edge case. A structure destroyed,
demolished, or unable to make payroll turns out its workers, who re-enter the market
immediately — and may be hired by a rival, or by nobody. Mass unemployment is what a
collapsing faction actually looks like.

Everything interesting emerges from this one function: labor shortages, wage competition
between factions, economic migration, brain drain, and the slow bleed of a faction that
underpays. **Unaligned characters joining whoever employs them is the primary peaceful
growth mechanic**, and it is this function that implements it.

### 3.6 Loyalty and defection

Morale accrues from being fed, housed, paid, and safe; it drains from hunger,
homelessness, unpaid work, defeat, and taxation. Below a threshold, characters emigrate
— and a captured or unaligned character with high morale toward a neighboring faction
will join it. Factions can therefore be hollowed out without a single battle.

### 3.7 Roots — why people are not commodities

If loyalty were purely a matter of wage, anyone could be outbid, nobody would be
irreplaceable, and the first design pillar would be false. **Roots** are the
counterweight, and they are what `faction_affinity` actually measures.

Affinity accrues from things money cannot quickly replace:

- **Tenure** — years served under one flag
- **Home** — a dwelling lived in for a long time
- **Partner and children** — family settled nearby
- **Birthplace** — being where one was born
- **Graves** — parents and partners buried in this ground

A new hire is cheap to poach. A forty-year smith with three children in the town and a
wife in the ground is close to unbuyable. This produces exactly the gradient the design
wants: **your veterans stay because of their lives, not their wages** — and it is what
makes losing one hurt.

It also gives capture its purpose. You cannot outbid someone's entire life, but you can
surround them. Uprooting people by force is the only way to take those who would never
leave willingly, which is precisely why conquest still has a place in an economy this
open.

---

## 4. Economy

### 4.1 Resources

**Raw:** lumber, stone, coal, iron ore, gold ore, food
**Refined:** planks, blocks, iron, tools
**Goods:** horse carriages, swords, bows, catapults, boats

### 4.2 Closing the gold loop

The original sketch had structures paying characters, and players taxing characters to
fund structures — a closed circuit with no source and no drain, which will either
deflate to zero or inflate without bound.

**Taxation is not a faucet.** It only redistributes gold between characters and the
faction treasury. Real faucets and sinks are required:

**Faucets (gold enters the world)**
- **Panning** — the primary faucet, and the one that makes the whole economy start.
  See below.
- **Minting** — gold ore mined in quantity and struck into coin at a mint. The
  industrial version of panning, and the faction's controllable source.
- **Exports** — selling goods to neutral factions or other players draws gold in
  from outside the faction.

#### Panning: the counter-cyclical faucet

**Gold enters the economy through the unemployed.** A character with no work can pan for
gold in river gravel or dig at an exposed seam. It pays worse than a wage, so nobody
prefers it, but it is always available to someone with nothing else to do.

This is the single most important mechanism in the economy, because it is what stops the
money supply from being conserved — and **a conserved money supply deadlocks**. Every
failure of a closed circulation takes the same form: the gold ends up somewhere it cannot
leave, and no amount of price or wage adjustment moves it. Three such states were reached
in practice before this mechanism existed:

| Deadlock | How panning breaks it |
|---|---|
| Full granaries, nobody with money to buy | The jobless pan and buy; gold reaches the granary, which can then pay farms |
| Money in hand, empty granaries | Demand exists, so farms can afford to hire and produce |
| Both, and nobody employed at all | Panning creates income, income creates demand, demand creates hiring |

Its great virtue is that it is **automatically counter-cyclical**. Money enters the world
*only* through people the economy has failed to employ. A healthy faction has nobody
panning and a stable money supply; a collapsing one floods itself with new coin, which
restarts trade, which puts people back to work, which closes the faucet. Nothing decides
this — it falls out of who has nothing better to do. A natural rate of unemployment
emerges as a consequence rather than a setting.

**Gold decides where settlements go.** A prospector walks about four cells an hour and
comes home at dusk, so a working day reaches roughly ten cells out and back with time
left to pan. Gold sits in well under one per cent of cells. Settlement siting therefore
has to weigh reachable gold alongside soil and fresh water, or a village is founded with
no way to mint a coin.

It is a bonus rather than a requirement, and the distinction matters. Food and water
decide whether a village can live at all; gold decides only whether it can make its own
money. A settlement out of panning range is not doomed — it is **dependent on trade for
coin**, which is a genuinely different strategic position rather than a failed one, and
one of the more interesting ways for two settlements to differ.

**Keeping gold scarce enough.** The mechanism only works if coin cannot be conjured
freely, and the design already carries the levers:

- **Geology limits it.** Gold sits in mountains and in river gravel downstream of them
  (§2.9). Most of the map yields nothing at all, so scarcity is a property of place
  rather than a tuned number.
- **Deposits deplete.** Panning draws a cell down, so any gold rush exhausts its own
  riverbed and closes the local faucet.
- **It must pay worse than work** — on the order of half to two-thirds of a wage — so
  that it is never chosen over employment, only fallen back on.

**Home gardens are a separate mechanism, and must stay separate.** An unemployed
household grows a little food for itself, which keeps the jobless alive. Panning supplies
*money*. Confusing the two would break the stabiliser: if the jobless could eat without
touching the money economy, unemployment would no longer expand the money supply, and the
deadlocks would return.

#### The remaining risk is the sink, not the faucet

The faucet above self-limits; nothing yet drains gold at a comparable rate, so the
long-run drift is inflationary. The sinks below have to scale with economic activity as
convincingly as panning scales with the lack of it. Two additions matter:

- **Inheritance.** A dead character's gold should pass to partner and children, with only
  a portion lost. Hoards that vanish shrink the supply arbitrarily; hoards that circulate
  keep it honest.
- **Somewhere to spend.** Characters that can only buy food accumulate coin they have no
  use for, which withdraws it from circulation as surely as burning it. Workshops, stores,
  and manufactured goods are a money sink as much as a content addition.

**Sinks (gold leaves the world)**
- **Structure upkeep** — every building costs gold per tick to maintain. Unmaintained
  structures decay and eventually collapse.
- **Research** — technology costs are *burned*, not transferred.
- **Imports** — buying from other factions sends gold out.
- **Attrition** — a portion of a dead character's carried gold is lost.
- **Tribute and reparations** — diplomatic outflows.

Tune so that a healthy faction runs near equilibrium and a stagnant one slowly deflates.
Deflation should feel like decline, not like a bug.

### 4.3 Wages and prices

Structures set wages from their own gold reserves, adjusted by demand: a structure that
cannot fill its positions raises its offer; one with a queue lowers it. Prices at stores
float on stock levels. This produces regional price differences, which produces trade,
which gives the diplomacy layer something concrete to be about.

---

## 5. Structures

**Structures are the only employers**, so every kind of work in the game must have a
structure that offers it. Each holds gold, has a build cost, carries **per-tick upkeep**,
offers a number of **positions** at a **wage**, and can be captured.

| Structure | Employs people to | Notes |
|---|---|---|
| **Home** | *(not an employer)* | Assigned residents up to capacity. Unhoused characters lose health. Stores and serves food. |
| **Farm** | Grow food | Yield scaled by the `fertility` multiplier (§2.4). **Requires fresh water nearby** for irrigation (§2.8) |
| **Lumber camp** | Fell timber | Placed on woodland; depletes it |
| **Quarry** | Cut stone | Placed on rock |
| **Mine** | Extract coal, iron ore, gold ore | Placed on a deposit; deposits deplete |
| **Workshop** | Refine materials, make goods, research | Consumes materials; the industrial core |
| **Mint** | Strike gold ore into coin | The economy's primary faucet (§4.2) |
| **Store** | Sell goods to characters | The retail end of the economy |
| **Granary** | Store and distribute food | Buffers famine; essential to surviving governor downswings |
| **Dining hall** | Cook and serve meals | A fixed forward kitchen at a work site. Buys food wholesale from a granary and sells meals (§5.1) |
| **Mobile kitchen** | Carry food to work sites and armies | The travelling version, for forces on the move |
| **Barracks** | Soldier | Houses and trains; anchors defensive standing orders |
| **Fishery** | Fish | Renewable food from river, lake, or coast; independent of soil (§2.8) |
| **Home garden** | *(not an employer)* | Part of a home. Feeds an unemployed household a little, so joblessness is poverty rather than death (§4.2) |
| **Bridge** | *(not an employer once built)* | Spans a river. The most strategically concentrated object on the map (§2.8); capturable and destructible |
| **Survey post** | Prospect for deposits | Reveals undiscovered ore within a radius over time (§2.5) |
| **Construction site** | Build | A transient structure that employs builders until complete, then becomes the finished building |

**Construction site** is the piece that makes the unification work: rather than
"construction" being an activity type, a planned building exists immediately as a site
that hires labor and consumes materials. It competes for workers in the job market like
any other employer, which means an under-funded build simply cannot attract anyone — a
much better failure mode than a silent stall.

### 5.1 Feeding people at their work

Farms and homes can be sited by preference; **extraction cannot**. Timber, stone, and ore
are where geology put them, so a settlement that wants any of them must send people beyond
comfortable reach of its granary. Those people have to eat.

A **dining hall** is a fixed kitchen placed at a work site. It buys food wholesale from a
granary and sells meals, exactly like any other link in the chain, so a supply line that
fails does so visibly — an unstocked hall means hungry workers, not a silent stall. The
**mobile kitchen** is the travelling equivalent, for armies, which move.

Two constraints matter more than they look:

- **A forward store is a few days of meals for its crew, not a second granary.** Sized as
  a rival to the village store, two kitchens will between them swallow the entire food
  supply and starve everyone at home.
- **The village eats first.** A hall draws only on the granary's surplus above a reserve,
  so outlying works cannot feed themselves at the expense of the people who grew the food.
  Under a lord this is simply how stores are apportioned; it is an obvious later candidate
  for a player policy lever (§8.1).

Without this, the only way to stop people starving at remote sites is to forbid them the
job — which caps how far a settlement can ever reach, and works directly against the
migration the settlement cycle depends on (§2.7).

### 5.2 Condition, decay, and ruin

Every structure carries a **condition** value from 0 to 100. It falls continuously and
is restored by maintenance.

**Decay accelerates with disuse.** A staffed, funded structure is being looked after by
the people in it and decays slowly. An empty one rots. Suggested drivers, multiplied
together:

- **Baseline weathering** — always present, slow
- **Disuse multiplier** — scales sharply as staffing falls below capacity; an entirely
  unstaffed structure decays several times faster than a full one
- **Unpaid upkeep** — a structure that cannot meet its per-tick gold upkeep decays faster
  still

Homes decay the same way: a home with no residents rots.

**Thresholds**

| Condition | Effect |
|---|---|
| 100–60 | Full function |
| 59–25 | Output and capacity progressively reduced |
| 24–1 | Non-functional; cannot employ or house anyone |
| 0 | Collapses into a **ruin** |

**Ruins matter.** They are not merely scenery:

- They can be **salvaged** for a fraction of their original materials.
- Building on a ruin is **materially cheaper than building new** — the foundations,
  cleared ground, and roads are still there.

That second property is what closes the settlement cycle (§2.7). A ghost town is not
dead ground; it is a discount on the next settlement, waiting for the land around it to
recover. Combined with ancestral graves still conferring root affinity (§3.7), ruins
become the natural place for a later generation to return to.

Aesthetically this should be legible at a glance: a decaying town should visibly sag,
empty, and fall in, so a player scanning the map can read a region's economic history
from its architecture.

---

## 6. Technology

The original "2× cost for a random improvement" is a treadmill with no decision in it.
Replace it with **branching specialization**:

- Technologies sit on a tree with **mutually exclusive branches**. A faction cannot have
  everything.
- Categories: **speed, damage, durability, range**, plus economic lines (yield,
  efficiency, capacity, logistics).
- Each tier costs more than the last, but the choice of *branch* is the interesting part.

Because no faction can self-sufficiently research everything, factions become
**dependent on each other for goods they cannot make**. That dependency is what gives
trade and diplomacy real stakes, and it turns the diplomacy layer from flavor into
strategy.

---

## 7. Factions, Diplomacy, and Capture

### 7.1 Relations

- **Unaligned** — belongs to no faction; joins one upon accepting employment
- **Neutral** — may trade; will neither defend nor attack
- **Allied** — will defend if their ally is attacked
- **Hostile** — will attack

### 7.2 Capture — the central conquest mechanic

A non-combat character who is surrounded or trapped — no escape route, and no friendly
or allied combat unit nearby — becomes **captured**.

After a period in captivity, a captured character becomes **neutral and unaligned**, and
is then free to be *employed* — by anyone, including their captor.

This deserves to be the spine of the game rather than a footnote. It means:

- Conquest is fundamentally about **taking people, not killing them**
- Armies are instruments of encirclement, not annihilation
- The most effective aggression may be economic — pay better, and their people walk
- Captives are a resource with a moral texture: treatment during captivity should
  influence which faction they choose on release

The wrapped world materially strengthens this mechanic: with no map edge to back
against, encirclement is a real tactical possibility everywhere rather than an accident
of terrain.

Captured *structures* work similarly: they keep functioning, but their output belongs to
whoever holds the ground.

---

## 8. Player Agency

Four layers, roughly ordered from slow to fast.

### 8.1 Policy and economy

Tax rates; budget allocation across structure types; build orders; research priorities;
wage floors and ceilings; subsidies to specific structures; pension policy; food
distribution rules. These express themselves as `policy_weight` in the job utility
function (§3.5) — the player's thumb on the labor market's scale.

Note the deliberate limit: the player is a **government, not a manager**. Wages emerge
from structures competing for labor (§4.3); the player regulates that market — floors,
ceilings, subsidies, taxes — rather than setting every wage by hand. Policy shapes
conditions; people still choose.

**Retirement and pensions.** If a faction is its payroll, then aging out of work is a
real event with real consequences. Elders are simultaneously the most skilled and the
most rooted people you have. A pension is a gold sink that buys deep loyalty and keeps
skilled elders in place to raise grandchildren; withholding one frees gold and creates
a population of impoverished, high-affinity old people whose morale is collapsing.
Neither choice is obviously right, which is what makes it a good knob.

### 8.1a The town hall — where policy becomes a building

Policy needs a place to live, a payroll to live on, and a purse to spend from. The town
hall is all three: the faction's first civic structure, and the point where §8.1 stops
being a menu and becomes machinery. Before a player exists it runs on standing defaults,
exactly as PolicyWeight stood in for a government before there was one.

**It is an employer.** A hall has staff — a clerk, a reeve — hired through the ordinary
labour market at ordinary wages, because the spine (§1) admits no other relationship.
A faction that cannot pay its clerks has no government, which is the correct failure
mode.

**Its purse is the treasury, and every coin in it is accounted for.** Three sources,
none conjured:

- **Tax.** A small levy, assessed where wealth actually sits (measured: in a few
  households holding thousands while the median holds tens). A wealth levy above a
  generous floor, not an income tax — subsistence earners pay nothing, hoards pay. Tax
  is the institutionalised alternative to the mob: the civilised form of the leveling
  that every society applies to hoarded wealth one way or another, with a rate dial
  instead of torches.
- **Escheat.** Estates with no heirs pass to the hall rather than vanishing. The ledger
  measured inheritance destroying over one per cent of the money supply a year with the
  faucet shut; escheat converts that leak into civic revenue, and is historically exact.
- **Fees, later.** Market stall rents, mill soke, tolls — each arrives with the
  structure it belongs to.

**It spends on exactly three things, each answering a measured failure:**

1. **Relief.** A dole for the destitute, paid in coin at the hall (no behavioural
   machinery: the two-day record shows interventions fail by displacing behaviour, so
   relief moves money, never people). The rate is the policy: set against the lowest
   wage, it is the less-eligibility dial — generous relief risks idleness, mean relief
   lets the poor die beside full granaries, and the player owns that trade-off rather
   than the simulation asserting an answer. Relief is bounded by the treasury: a hall
   with no money feeds nobody, which keeps conservation honest.
2. **Public works.** Founding businesses private capital will not touch — the granary
   nobody profits enough from, the dining hall at the remote mine, eventually roads.
   The founding machinery (business.go) already exists; the hall is the founder of last
   resort when the price signal says build but every private purse declines.
3. **Pensions (§8.1).** The retirement knob, funded from the same purse so that
   generosity to elders competes with relief and works — one budget, real trade-offs.

**Defaults before players.** Standing policy: modest wealth levy, relief at
two-thirds of the lowest prevailing wage, public works only on famine-class signals.
These are the same kind of placeholder as PolicyWeight — documented stand-ins for
choices that become the player's the day a player arrives (§10 Phase 3).

**What it deliberately is not.** Not a food store (the granary trades; the hall pays
coin and the poor buy like anyone — relief flows through the market, propping demand
rather than bypassing it). Not a barracks, not a court; conscription and law arrive
with armies and crime, on their own designs.

### 8.1b Health — clinics, and why they wait for epidemics

Disease is the dominant killer of children in the measured village, so a clinic is the
obvious civic building — and the record says to sequence it carefully: twice, reducing
disease while food was binding merely relabelled the death certificates as hunger.
Clinics enter when they can matter, which is with §8.3's epidemics.

A clinic is an employer (healers, skill-bearing like any trade), it reduces the
disease hazard within a radius rather than switching it off, and it must be paid for —
which is a policy fork the town hall makes real: fee-for-service, where the poor die
untreated and the pressure builds toward the second option; or tax-funded, where the
treasury strains and relief competes with medicine. Historically honest in either
direction, and the choice is the player's, not the simulation's.

### 8.2 Diplomacy and trade

Relation changes, treaties, trade routes, tribute, alliance commitments, prisoner
exchange. Made meaningful by technology specialization (§6) — you trade because you
genuinely cannot make the thing yourself.

### 8.3 Crisis response

The world generates events demanding a decision now: famine, raid, plague, a strike over
unpaid wages, a defection cascade, a structure collapse, a neighbor's army massing on the
border. **This is what gives the game a pulse** and prevents it from being a spreadsheet
you watch. Crises should be frequent enough to keep attention and consequential enough
to fear.

### 8.4 Direct command

The player may direct groups — soldier units especially — for tactical moments: combat,
escorts, and capture operations. This is the fast layer, and the reason encirclement
mechanics are worth having.

**Micromanagement is bounded by the world, not by rules.** This layer looks like it
conflicts with pillar 3, but the conflict is largely self-resolving: the world is
continuous, persistent, and far too large to hand-operate, and the player cannot always
be present. Attention is the scarce resource, and no artificial penalty is needed to
enforce what scale and absence already enforce.

So direct command is deliberately left unpunished. The expected pattern is that players
micromanage a **few chosen things at decisive moments** — a battle, an encirclement, an
escort — and govern everything else by policy and standing orders, because there is no
alternative.

One ergonomic principle is still worth keeping: **command groups, not individuals.**
Orders address a unit and its officer rather than puppeting a named character. This is
partly to protect the fiction, but mostly because individual-level control of a
population this size is not a usable interface.

### 8.5 Presentation

**Fullscreen world view with an overlay HUD** — the map fills the display, and controls
sit as translucent panels over it rather than as fixed chrome shrinking a viewport.

The wrapped world means the camera can scroll indefinitely in any direction and never
hit a boundary. Minimap and camera both wrap; scrolling never stops or clamps.

---

## 9. Architecture

### 9.1 The core decision: split simulation from rendering

**The simulation and the renderer are separated by an interface from day one — but they
run in the same process by default.** Even with the mesh dropped, the boundary remains
the most important structural choice available:

- Go owns the simulation — its genuine strength
- The client can be anything, and can be *replaced*
- **2D → 3D becomes a client swap, not a rewrite**
- Multi-user falls out naturally: many clients, one authoritative simulation
- Headless servers, automated tuning runs, and testing all come free

**But the boundary is an interface, not a socket.** Two implementations sit behind it:

| Transport | Used by | Cost |
|---|---|---|
| **Direct** | Single-player, tests, tuning runs | Function calls; zero serialization |
| **Network** | Multi-user (Phase 4) | Serialized deltas |

This keeps every benefit above while paying nothing in the single-player case. Forcing
traffic through a socket to talk to yourself is exactly the kind of distributed-systems
overhead the mesh required and a single host does not.

Go's 3D ecosystem is weak, with no mature engine. Do not let that constrain a 3D future —
architect so the eventual 3D client can be Godot or anything else, while the Go
investment survives intact.

### 9.2 Simulation core

- **Fixed tick loop.** Start at 10 ticks/second.
- **Deterministic.** Same inputs, same seed, same state — always. Even without a mesh
  this pays for itself: reproducible replay, time-travel debugging, regression tests
  over whole simulated centuries, and — critically — the ability to tune population
  dynamics by re-running identical scenarios with one variable changed. Nearly
  impossible to retrofit, nearly free up front.
  - seeded PRNG, threaded explicitly through the tick — never a global
  - **no map iteration order dependence** (Go randomizes it; this will bite you)
  - **`float64` is sufficient** — fixed-point exists to survive *cross-machine* lockstep,
    which died with the mesh. IEEE 754 is deterministic for one binary on one platform,
    so replay and tuning runs are exact. What is given up is bit-identical replay across
    different CPUs, OSes, or Go versions, which nothing in this design needs
- **Data-oriented storage.** Structs of arrays over arrays of structs; characters in
  contiguous slices indexed by ID.
- **Parallelism by region, not by entity.** Each region ticks on one goroutine.

> **Do not use goroutine-per-character.** It is the obvious move in Go and it is a trap:
> it destroys determinism, makes replay impossible, and turns debugging into guesswork.
> Characters are data processed by a loop, not concurrent actors.

### 9.3 Toroidal geometry — one source of truth

Wrapping touches nearly every system, and inconsistent handling is the single largest
bug source in wrapped-world games. **Implement the primitives once and forbid raw
coordinate arithmetic anywhere else in the codebase.**

**Two layers, both wrapping.** Per §2.2 there is a continuous layer (float positions,
movement, steering, rendering) and a discrete layer (terrain cells: soil, wood, ore).
Both wrap, by different arithmetic, and conversion between them must go through one
function:

```go
const (
    WorldW, WorldH = 1024.0, 1024.0 // continuous extent
    CellsX, CellsY = 1024, 1024     // terrain grid — one cell per unit
)

// --- Continuous layer ---

// Wrap a coordinate into [0, size).
func Wrap(v, size float64) float64 {
    r := math.Mod(v, size)
    if r < 0 { r += size }
    return r
}

// Shortest signed delta from a to b across the wrap.
func Delta(a, b, size float64) float64 {
    d := math.Mod(b-a, size)
    if d > size/2  { d -= size }
    if d < -size/2 { d += size }
    return d
}

// Shortest distance on the torus.
func Dist(a, b Vec2, w, h float64) float64 {
    return math.Hypot(Delta(a.X, b.X, w), Delta(a.Y, b.Y, h))
}

// --- Discrete layer ---

// Wrap a cell index into [0, n). Note Go's % is truncating, not modular.
func WrapCell(i, n int) int { return ((i % n) + n) % n }

// Shortest signed cell delta across the wrap.
func CellDelta(a, b, n int) int {
    d := WrapCell(b-a, n)
    if d > n/2 { d -= n }
    return d
}

// --- The only bridge between them ---

func CellAt(p Vec2) (int, int) {
    return WrapCell(int(math.Floor(p.X)), CellsX),
           WrapCell(int(math.Floor(p.Y)), CellsY)
}
```

The eight-neighbor lookup used by woodland regrowth and soil recovery (§2.5) must use
`WrapCell`, or forests will fail to spread across the seam — a bug that stays invisible
until a player happens to settle there.

Every one of these must use these primitives:

- **Pathfinding** — flow fields are computed over the terrain grid with wrapped neighbor
  lookups; characters then sample the field and steer continuously. This is the
  designed pairing of the two layers (§2.2).
- **Terrain simulation** — woodland spread and soil recovery, via `WrapCell`
- **Job selection** — the `distance_penalty` term (§3.5)
- **Combat** — range checks and targeting
- **Partner selection** — proximity weighting
- **Structure service radii** — homes, stores, kitchens
- **Prospecting radius** — survey posts revealing deposits
- **Spatial hashing / broadphase** — character bucket indices wrap modularly
- **Area queries and selection boxes** — a box may straddle the seam
- **Steering and facing** — direction vectors take the short way around
- **Rendering** — entities near an edge must be drawn on *both* sides; up to four
  times near a corner. Render by tiling the world modularly across the viewport.
- **Camera** — scrolls forever, never clamps (§8.5)

A useful discipline: make the position type a distinct `TorusPos` that has no `-`
operator, so the compiler prevents accidental Euclidean subtraction. The same applies to
cell coordinates — a `Cell` type is worth having so grid and world units cannot be
silently mixed.

### 9.4 Optimizing for a single host

Dropping the mesh removes real overhead, and it is worth being deliberate about
collecting the refund rather than carrying distributed-system machinery that no longer
buys anything.

**What the mesh required and a single host does not**

| Removed | Was for | Now |
|---|---|---|
| Cross-peer state hashing in the tick | Desync detection | Debug build only |
| Region serialization and handoff | Moving entities between hosts | Regions share memory; a "handoff" is an index change |
| Versioned broadcast of the governor value | Replica agreement | A plain field |
| Protocol round-trip for local play | Peer communication | Direct interface call (§9.1) |
| Trust, reputation, spot-checking | Untrusted peers | Gone entirely |

**Regions are now purely a parallelism device.** They carry no authority, no borders that
mean anything to gameplay, and no serialization cost. Size them for cache behaviour and
core count, not for network topology.

#### Where the time actually goes

Three systems dominate, and each has a specific fix.

**1. Job market evaluation** — naively O(unemployed × structures) every tick, which is
the single worst hot spot in the design because §3.5 is evaluated constantly.

- **Spatially limit** candidates to structures within travel radius, via the spatial index
- **Stagger deterministically**: a character re-evaluates only when `tick % N == id % N`.
  This cuts cost by a factor of N and stays perfectly deterministic — unlike anything
  keyed to camera or wall-clock
- **Prefer triggers to polling**: re-evaluate on becoming unemployed, on a nearby wage
  change, or on a need crossing a threshold

**2. Pathfinding** — many characters walking to the same structure.

- **Share flow fields by destination.** Compute once per destination, cache, and
  invalidate on terrain or structure change. A hundred people walking to one farm cost
  one field, not a hundred paths. This is the main reason flow fields beat per-agent A*
  here, and it is worth building the cache in from the start.

**3. Terrain cellular automata** — woodland spread and soil recovery over 65k cells at
256², 1M at 1024².

- **Do not run per tick.** Once per in-world day is ample for processes that take years
- **Keep a dirty set** — only cells that changed, or neighbour a change, need visiting
- **Amortize** — process a fraction of the grid per cadence step rather than all of it

#### Go-specific wins

- **Keep entity arrays pointer-free.** Reference characters and structures by `int32`
  IDs, never `*Character`. A large `[]struct` containing no pointers is skipped entirely
  by the GC scanner, which matters a great deal at high population. This is the single
  most valuable Go-specific optimization available here.
- **Dense slices with generation counters** for entity handles, rather than maps keyed by
  ID in hot paths.
- **Pool transient allocations**; avoid interface dispatch inside tick loops.

#### Parallelism without locks

Region-parallel ticking now shares memory, and the job market legitimately reads across
region borders. Rather than locking, **double-buffer the simulation state**: every region
reads the previous tick's snapshot and writes into the next. Reads are then lock-free and
order-independent by construction, which keeps parallel execution deterministic — the
property §9.2 depends on — at the cost of holding two copies of mutable state.

#### Measurement

The load governor (§2.4) targets 70% of tick budget, which presupposes the budget is
actually known. Wire `pprof` and per-system tick timing in from the beginning; the
governor is only as good as its measurement, and an unprofiled tick loop will mean tuning
the wrong thing.

### 9.5 Networking

Client/server, authoritative simulation. Clients send intents (policy changes, orders);
the server simulates and broadcasts state deltas. Interest management: clients receive
only what is near their camera and their holdings.

Because the simulation is deterministic and tick-based, client-side prediction and
smooth interpolation between ticks are straightforward to add later.

### 9.6 Persistence

Append-only event log plus periodic snapshots. Restore = load snapshot, replay events.
This falls directly out of determinism and gives time-travel debugging as a side effect.

---

## 10. Build Order

### Phase 1 — Single-process 2D, one world, no network
Ebitengine client. Characters, needs, employment, the utility function, wages, food,
homes. Deterministic tick loop, both toroidal layers (§9.3), the terrain grid, flow-field
pathfinding with continuous steering, region-partitioned data, and the sim/render split —
all present from the start even though nothing is distributed yet.

Map generation belongs here too, at least through **stage 6** of §2.9 — elevation, sea
level, lakes, and rivers. Water is not decoration to be added later: farms need fresh
water, pathfinding must route around lakes and across bridges, and settlements cluster on
floodplains. Building the village sim on a featureless plain and introducing water
afterwards would invalidate most of what Phase 1 was meant to prove.
**Goal: is watching a village survive interesting?**

### Phase 2 — The full simulation loop
Life cycle, partnership, children, aging, death, roots. Full economy with faucets and
sinks. All structure types, technology tree, the load governor with its damping. Resource
depletion, woodland regrowth, soil recovery, structure decay and ruins.
**Goal: does a population stay stable across generations without babysitting — and does
the settlement cycle (§2.7) actually turn?**

### Phase 3 — Factions and conflict
Multiple factions, relations, combat, encirclement and capture, defection, standing
orders. Still single-process — factions can be AI-run.
**Goal: is capture-driven conquest as good as it sounds on paper?**

### Phase 4 — Client/server split
Extract the headless simulation. Client talks over the protocol. Multiple clients on one
sim. Interest management, persistence, replay.
**Goal: multi-user.**

### Phase 5 — 3D client
A new renderer against the same protocol. The simulation does not change.

---

## 11. Open Questions

- **Combat resolution** — deterministic tactical model, entirely unspecified so far
- **How does a new player enter?** Found a faction on unclaimed land, or inherit an
  abandoned one? With a fixed world, land pressure makes this a real problem
- **Does the map need erosion?** Hydraulic erosion before drainage (§2.9) yields far more
  believable valleys and river networks, at real generation cost and complexity. Probably
  worth it for a world this permanent, but not required for a first pass
- **Bridge destruction** — a defender who burns their own bridges gets safety at the cost
  of trade and their own mobility. Good tension, mechanics unspecified
- **Do rivers make good faction borders in practice?** They should, but whether territory
  claims should recognise them explicitly or let it emerge is open
- **Is the anti-snowball property strong enough?** Depletion plus roots (§2.7) creates
  real drag on established factions, but whether it is sufficient in an eternal world
  with no reset is untested
- **Captivity treatment** — does it become a real mechanic, or stay abstract?
- **Spatial fertility** — authored fertile and barren zones multiplied by the global
  governor value; recommended, not yet specified
- **Does anything hold a faction together besides pay and roots?** Culture, religion,
  and language would each deepen affinity, but each is a whole system
- **Cycle timing** — regrowth and soil recovery must be slow enough that abandonment
  hurts, fast enough that a long-running player sees a valley refill at least once.
  Calibrating this against the ~2.5-real-month lifespan is a real tuning problem
- **Do ghost towns need to be readable as history?** Ruins that record who lived there
  and why they left would be evocative, but it is a whole layer of bookkeeping

## Appendix A — What the simulation taught about population growth

Established by measurement between the first standing village and the first growing
one: an arc from R0 0.244 to sustained compounding growth (68 -> 401 over 120 years on
one seed, every seed growing). Each principle below was paid for with instrumented
runs, and most with a failed intervention or three. They are design constraints for
everything built on top — factions, colonies, policy — not suggestions.

**A.1 Population has a critical mass, below which nothing else matters.** Marriage is
a coincidence process: at 34 settlers, 2,957 midnight samples of unmarried fertile
adults found an eligible partner within pairing range zero times. Below the threshold,
every economic improvement is spent into matching failure; above it the same code
grows. Consequence, already in force: villages found at 68, and daughter settlements
must leave home above critical mass — a colony founded small is founded dying.

**A.2 Production must scale with population, or every life saved is a death moved.**
Three clean competing-risks substitutions: cure disease and children starve; save
young adults and toddlers starve; add food and births rise to meet it. At fixed
capacity the death total is conserved and only the certificates change. A fix is real
when counts fall, not when causes shift.

**A.3 Dependants need a food channel that bypasses money.** In a closed wage loop,
total wages equal total food spending — an identity — so wages can never feed
non-earners. Children starved by construction until the kitchen garden carried them.
Some provisioning must not flow through the labour market.

**A.4 Labour allocation beats every price signal.** Farms paid double the other trades
and stood half empty; harvests tracked staffing linearly; the largest production gain
came from seasonally releasing servants to the fields — moving people, not wages.

**A.5 Transfers of money are safe; transfers of behaviour are ruinous.** Eight
interventions failed, every one by displacement — errands, expeditions, unpaid
loitering — because a subsistence family's scarcest asset is its labour hours. This is
the town hall's standing rule (§8.1a): relief moves coin, never people.

**A.6 Most premature death was rules, not scarcity.** The granary deadlock, the
0.32-gold till loop, the tool-shop pilgrimage, the 0.93-meal refusal: absorbing states
where an individual repeats a failing action until dead. Removing them produced more
growth than any added mechanism. A growing world is first one whose rules have no
lethal fixed points for individuals.

**A.7 The constraints are simultaneous, not sequential.** Every lever alone failed or
drowned in noise; the same levers stacked — critical mass, seasonal labour, child
food, calibrated mortality — produced growth, because R0 is a product and a
single-factor fix multiplies against unchanged bottlenecks.

**A.8 Instruments before interventions.** The founding transient reads as health; the
aggregate hides the individual; small-cohort statistics read as signal; completed-life
measures turn pessimistic in the presence of success (right-censoring). None of the
above was findable without the census, the ledger, the diaries, and multi-seed pooled
demography built first — and the two worst weeks of wrong conclusions each traced to
an instrument, not the world. (The full set of working lessons lives in
docs/method.md.)
