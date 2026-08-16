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
