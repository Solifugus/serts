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
