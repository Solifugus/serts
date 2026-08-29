# Working notes: how this simulation lies to you

Lessons paid for in debugging time, recorded so they are not paid for twice. Each entry
has a real instance behind it from this project.

---

## 1. Aggregates hide individual failure. Watch one character.

Every genuine fault found so far was invisible in summary statistics and obvious within
seconds of examining a single person.

| The summary said | The individual said |
|---|---|
| average hunger 19 | quarrymen at hunger 100, three gold, twenty cells from food |
| employment 100% | every job was a farm; industry could not staff itself at all |
| "walking to the diggings" | position unchanged to the centimetre for 2,000 ticks |
| average hunger 14 | 21 adults starved that year |

An average over a population is a statement about nobody. When the aggregate looks healthy
and the outcome does not, the aggregate is the thing to distrust.

**Practice:** before theorising about a population, print one member of it in full — every
field, not the ones that seem relevant. The answer has repeatedly been a field nobody
thought to look at.

---

## 2. Checking the wrong consequence falsifies a correct hypothesis.

"The granary is too small" was raised early, tested by watching whether the **wage gap**
closed, and discarded when it did not. It was right. The consequence to check was whether
people could **afford a meal**.

A hypothesis is only disproved by the consequence it actually predicts. Before running the
experiment, write down what should change if the theory is true — and make sure that is the
thing being measured.

---

## 3. Behaviour bugs read as reasonable code and describe absurd human action.

The decision cascade looked fine on the page every time. Stated as behaviour it was
ridiculous:

- A man walks past his own full pantry to go and starve at a quarry.
- A prospector sets out for gold each morning, turns round at dusk, and in three years
  reaches it three times.
- A farmhand walks twenty cells to the granary, buys one dinner, and walks back.
- Nobody in the village ever looks for better work, while farms stand empty offering fifty
  times subsistence.

**Practice:** read each branch aloud as a sentence about a person. If no person would do
it, the code is wrong however well it compiles.

---

## 4. Buffers must exceed the lead time of what they buffer.

Repeatedly the same error at different scales:

- Food coverage target of 25 days against a 240-day growing season.
- Granary holding 400 units — nine days for thirty people — against a 25-day target, so
  the price mechanism read permanent famine and pinned itself at maximum forever.
- Founding villages provisioned with eight days of food.
- Dining halls given 220 meals each when the whole village store held 400.

**Practice:** size every store as *days of the thing it supplies*, and check that figure
against how long it takes to replace. A buffer smaller than the lead time is not a thin
margin; it is a guaranteed failure that no downstream mechanism can recover from.

---

## 5. Measurement bugs masquerade as balance bugs.

Hours went into tuning wage gains, price elasticities and yields against a demand figure
that was measuring the wrong thing entirely — every meal eaten, including food grown in
kitchen gardens that never touched the market. The economy was not badly balanced. The
instrument was broken, and every adjustment was made against a false reading.

**Practice:** when tuning stops producing sensible movement, suspect the measurement before
the model. And instrument the instrument: a debug counter that reports zero because it was
never wired up looks exactly like a mechanism that never fires.

---

## 6. One variable at a time, or the result means nothing.

Several changes were made together more than once, and each time the outcome could not be
attributed. The clean experiments — switching industry off entirely to separate a food
problem from a trade problem, reverting a single constant to isolate a regression — were
the ones that produced answers.

---

## 7. A test that encodes the goal is worth more than a passing suite.

`TestVillageSurvivesItsFirstDecades` has repeatedly refused work that was individually
correct — the growing season, job mobility — because each destabilised a balance that had
quietly been resting on the bug it fixed. That is the test doing its job. Correct changes
that break it belong on a branch with the reason recorded, not merged with the test
relaxed.

---

## 8. Confident explanations are cheap. Count how often they are right.

Tally for one working session: roughly one diagnosis in five was correct. The wrong ones —
the granary being too small (tested wrongly), prospectors wandering out of range, eviction
at fifteen, foraging yield — were each coherent, argued fluently, and wrong. Every correct
one came from looking at a single character rather than reasoning from totals.

Fluency of explanation carries no information about its truth. When an explanation arrives
faster than the evidence for it, that is the moment to go and measure something.

---

## 9. An instrument has a floor. Read its noise before its readings.

Heritable traits were added partly as a measurement device: if temperament is under
selection, the surviving lineages say which temperaments the world rewards. The first run
came back with caution at 1.24 against a founding mean of 1.00 — a twenty-four per cent
drift in twenty years, and an immediate explanation arrived with it, that dangerous work
was not paying its risk premium.

Five more seeds gave 1.17, 0.98, 1.05, 0.95, 1.06. Mean 1.04. The village collapses to
four to eight survivors, and the standard error on a trait mean at that sample size is
about 0.07, so the whole first reading fitted inside the noise of drawing seven people out
of a uniform distribution.

The instrument is not wrong; it is below its detection floor. It will start telling the
truth about selection only once a population survives in numbers large enough for
selection to outrun sampling. Until then its readings must be quoted with the seed count
beside them.

The general form: before believing an instrument, run it where you already know the answer
is nothing, and see what it says.

---

## 10. Ask over the horizon the question actually has.

`TestVillageSurvivesItsFirstDecades` checked the headcount at year twenty and passed
comfortably for hours while every village studied was in terminal decline. Measured to 120
years, six seeds out of six died — five to zero, one to a single survivor — and all six
looked healthy at year twenty.

A founding settlement grows for a decade or so because its settlers arrive adult, healthy
and with savings. That transient is indistinguishable from health at short range, and I
read "population 41 and rising" off a ten-year window and reported it as growth.

This is the third time the measurement rather than the simulation was the thing that needed
fixing: the test world at half the shipped size, the vital statistics on a single seed, and
now the survival horizon. In each case a green reading came from asking a question the
instrument could not answer.

The general form: a metric evaluated inside a transient measures the transient. Before
trusting one, work out how long the process it describes actually takes, and measure at
least that long.

---

## 11. An intervention is measured on the system, not on its target.

The provisioning errand sent parents to the granary whenever the larder ran below half
the children's share, so that infants would not starve between their parents' shopping
trips. It was aimed at the largest single death category and it made every figure worse
at once: R0 0.621 to 0.465, survival-to-five 53% to 41%, and deaths of old age 31 to 7 —
while infant hunger, its actual target, did not improve at all.

The trigger was chronically true in exactly the households it meant to help, so it
converted their parents' working hours into walking, and the lost wages starved the
family faster than the errand fed it. Helping the target group through a channel that
costs labour time can be net-negative for the target group itself.

The general form: before shipping a behavioural intervention, ask what it displaces.
And when the measurement says harm, revert the same hour — the cost of a wrong idea is
only the run time it took to measure, unless it is left in.

---

## 12. Two individually-principled fixes can compound into harm.

The famine price signal (empty shelves read as scarcity, not neutrality) and the
meal-denominated reserve (real quantities in real units) are each defensible alone, and
each was adopted on its own argument. Together they were measured at R0 0.483 against a
baseline of 0.621: the famine signal spiked the price, the indexed reserve spiked with
it, and parents stopped provisioning larders in exactly the famine. The glut side of the
same experiment (floor 0.08) failed the other way — deflation collapsed every nominal
quantity against thresholds stated in gold.

A price is not a number; it is the unit half the economy is denominated in. Changing how
it moves changes every threshold, reserve, and wage that hangs off it, and those must be
audited as one system or not touched. The experiment cost three decomposition runs and
was worth it once: the interactions are now written down instead of latent.

---

## 13. A local optimum defends itself; respect what the ugly mechanism is doing.

Three reasonable changes in one day — panning for the merely short of money, pay indexed
to skill, wages in arrears instead of instant dismissal — were each implemented cleanly,
measured on the standard instrument, found to cost 0.08–0.12 of R0, and reverted the
same day. Each replaced something that looked broken and was quietly load-bearing:
instant dismissal ejected workers from dying employers back to paying work; flat pay
subsidised the family-founding age; the narrow panning gate kept parents home. The
baseline configuration is a local optimum whose parts protect each other, and a
mechanism that looks pathological in a diary may be cheap in practice — the thrash
loop's cost was hours of walking, and the cure cost days of unpaid work.

The discipline that made the day cheap instead of ruinous: one change, one measurement,
revert on harm, record the finding at the site. Ideas are disposable; the instrument
readings are the asset.

---

## 14. Assume the crude mechanism is load-bearing until measured otherwise.

Four times now a rule has looked plainly wrong, been replaced with something better
reasoned, and cost dearly:

- Instant dismissal over one unpayable tick looked like churn; it was ejecting workers
  from dying employers back into paying work. Wage arrears cost 0.08 of R0.
- Flat pay ignored skill; skill-indexed pay taxed the family-founding age and cost 0.12.
- A village-wide levy to fund colonies looked like confiscation; narrowing it to the
  departing party cost two-thirds of the world's population, because sixty settlers
  cannot raise a purse between them.
- Famine as a reason to send a founding party looked backwards, since harvests track
  staffing; removing it cost more than half the world, because a famine party opens new
  land rather than redistributing food.

The pattern is not that crude rules are good. It is that a rule which has survived
measurement is holding up something not yet understood, and the replacement is reasoned
from a model of the system that is by definition incomplete — otherwise the load would
have been visible. Rewrite them, by all means; but measure before believing, and expect
to be wrong about half the time.

---

## 15. Probes are not tests. Keep them out of the suite.

`go test ./...` stopped completing. The cause was not a hang: `internal/sim` held six
research probes — the minimum-viable-founding sweep, the scale hypothesis, the
demographic decomposition, the diary dump — that between them simulated several thousand
village-years on every invocation. One of them carries a comment recording that it once
"timed out at 2h30m still mid-simulation". They were written to answer a specific
question, run once by hand with a long timeout, and never removed, because a file ending
in `_test.go` has nowhere else to live.

They are easy to tell apart from tests by a mechanical rule: **every one of them had zero
assertions.** A test asserts and can fail; a probe measures and prints, and has no opinion
about what the answer should be, which is exactly why it is useful for investigation and
useless as a regression gate. Running them on every change bought nothing and cost the
ability to run the suite at all.

There is a second-order cost worth naming. When the suite cannot finish, it stops being
run, and then the tests that *would* have caught a regression are no longer catching
anything either. A slow suite does not degrade gracefully into a slower suite; it degrades
into no suite.

Probes now sit behind `//go:build probe` and are invoked deliberately:

	go test -tags probe ./internal/sim -run TestMinimumViableFounding -timeout 3h -v

Keep writing them — every real diagnosis in this project came from one. Just do not leave
them in the path of every build.

---

## 16. A change that survives one measurement has not survived measurement.

A rule was added so that hungry villagers would not walk to a granary holding a single
meal. It is a good rule. It describes something real: the shelf empties before the crowd
arrives, and the people who set out are worse off than when they left. Four seeds said it
cut hunger deaths by nine per cent.

Twelve seeds said it cut nothing. Hunger deaths moved from 434 to 439 — the wrong way —
and population from 1,072 to 1,034, a mean of −3.2 per seed against a standard error of
3.4. The nine per cent was noise, and the sign flipped on widening the sample.

The reason four seeds could not see it was visible before the run and worth stating as a
rule: **the between-seed spread bounds what any number of seeds can resolve.** Identical
code produces villages of 59 and 111 people on different maps. An effect of five or ten is
invisible underneath that no matter how confidently it is reasoned, and half the sample
was structurally incapable of responding at all — six of twelve seeds came back
byte-identical, because their granaries never ran thin enough for the rule to have a
choice to make.

Two practices follow. Estimate the noise floor from the between-seed spread *before*
running, and say out loud what result would count as nothing. And when a first measurement
agrees with the hypothesis, treat that as the moment to widen the sample rather than the
moment to commit — a fluent explanation plus one confirming measurement is exactly the
combination that feels like knowledge and is not (note 8).

The rule was reverted. The finding is recorded at NearestFoodSource so that the next
person to have the same good idea finds the measurement before spending the afternoon.

---

## 17. A dull representation can hide a failure that a human one makes unmissable.

Diaries are keyed by character ID. Character IDs are slots, and newChar hands the slots of
the dead to the newborn, so a slot's second occupant appended their life to their
predecessor's. One name was born, died at twenty-two, was born again to different parents,
was orphaned at eight, and died at eleven.

That bug was present from the day diaries were written and survived every reading of them.
It survived because the entries read `character 147 took work at structure 2`. Nothing
about a recycled slot looks wrong in a column of anonymous state changes — there is no
claim being made that can be false. The moment names and parents were printed beside the
same data it became absurd on sight: this woman gave birth after she died.

Note 1 says aggregates hide individual failure. This is the neighbouring lesson: among
views of an individual, the more human representation has more surface for a contradiction
to catch on. A record that names people, ages, and relations asserts things a reader
already knows the rules for — the dead do not have children, a mother is older than her
child — and those background rules do the checking for free. A record of opaque IDs and
state transitions asserts almost nothing, so almost nothing can contradict it.

The practical form: when a diagnostic is hard to read, that is not only a cost in comfort.
It is a loss of error-detecting power, and the fix pays twice. Making the diaries legible
this session turned up three defects nobody was looking for — a child count lost through a
pointer into a reallocated slice, this one, and a marriage market that remarries the
widowed the next day.

---

## 18. Diagnose by counting, not by reasoning about the mechanism.

The job churn — villagers changing work sixty-nine times a year, median tenure one day —
was diagnosed three times.

First, distance: the score weighs how far a post is from where the character is *standing*,
and people walk, so the same person should re-decide differently every two hours. Fluent,
mechanical, wrong. Distance had a geometric mean of 0.922 across sixteen thousand real
switches, pulling the other way, and was the largest factor in 15% of them.

Second, employers failing payroll. Right mechanism — and dismissed on the strength of a
probe that sampled solvency **once per day** when payroll runs every working tick. The
instrument reported 1.4% of employer-days and pointed away from the truth already in hand.
A measurement at the wrong cadence is not weak evidence; it is confident evidence for the
wrong answer.

Third, a counter on every quit site, tagged with its cause. One run, and the argument was
over: 54% unpaid, 44% for better work, everything else noise.

The counters cost twenty minutes to add and would have been decisive at any point in the
preceding two hours. The two failed diagnoses each cost longer than that, and the second
produced a *number*, which is worse than producing nothing — a wrong number is believed.

So: when a system has several paths to the same outcome, instrument the paths before
reasoning about them. Prefer a counter at the site over an inference from a sample; prefer
a sample at the event's own cadence over one at a convenient cadence. This codebase already
had the pattern in ColonyBlocked and it did not occur to me to reach for it until two
hypotheses had died.

And the coda, which is note 14 again: the correct diagnosis did not yield a correct fix.
Barring insolvent employers from hiring removed 86% of the unpaid quits and cost 26.7% of
the world's population, because a farm with an empty till still needs harvesting. Knowing
why a mechanism misbehaves does not tell you what it is holding up.

---

## 19. Mobilising idle capital is not the same as having somewhere to put it.

The measurement was real and the inference from it was wrong.

At year three the village held 13,823 gold in purses, 9,815 of it above what a year's food
costs, one hoard alone at 2,315 — while every employer's till in the world held 1,264
between them, and businesses cycling three hundred gold a year ran on floats of under
three, dismissing their staff whenever an outflow preceded an inflow. Both preconditions
for banking were present without having been designed in: idle capital, and solvent
borrowers. The reasoning was that a bank would return the hoards to circulation and attack
the poverty itself, not merely the churn.

A bank was built: deposits, working-capital loans, repayment leaving a buffer, an interest
spread paying its clerks, default and write-off past a debt cap, bank runs left to happen.
Twelve seeds, fifty years:

	population    1,072 -> 944   (-11.9%; mean -10.7/seed, stderr 5.2, t = -2.06,
	                              worse in 8 of 12 seeds)
	hunger deaths   434 -> 487   (+12.2%)
	tick cost     7,998 -> 13,910 ns  (+73.9%)

The flaw in the argument is visible only once stated plainly: the hoard was large and the
borrowing need was small. Employers needed a few gold, for a few hours, a few dozen times
a year. A bank sized to the hoard is a hole in the money supply; a bank sized to the need
is too small to change anything. The first version drew 13,084 gold out of circulation and
had 289 of it on loan, and cost a quarter of the village. Correcting that made the bank
harmless on one seed at one year, which is not the same as useful, and the twelve-seed run
said so.

Two habits follow.

State the acceptance criteria before running the measurement. They were written down here
— near 7,998 ns, better than 1,072 and 434 — and both failed, which made the decision a
reading rather than a negotiation. A large build creates its own pressure to ship; the only
defence is a number agreed in advance.

And when a quantity is idle, ask what would use it before building the thing that moves it.
"There is a lot of X sitting still" and "there is unmet demand for X" are different claims,
and only the second justifies an institution.

The code is at commit 3a200ba if the economy ever grows a real appetite for credit — more
trades, businesses founded on borrowed money, stock bought on credit. What it cannot do is
make a small need worth an institution.
