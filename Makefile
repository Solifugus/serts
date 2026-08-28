# How to run this project's tests.
#
# There are three tiers, because the thing under test is a simulation and simulating a
# century is not free. Left undivided, the cost of the slowest work sets the cost of the
# cheapest check, the suite stops being run, and the fast tests stop catching anything
# either (docs/method.md, note 15).

GO ?= go

# The gate: every invariant test, none of the century-scale acceptance runs. Fast enough
# that there is no reason not to run it on every change.
#
# The acceptance tests opt out of this tier themselves, by calling acceptance(t) — see
# internal/sim/sim_test.go. Naming them here instead was the first attempt and was wrong
# twice over: it missed one of the two, and it would have gone quietly stale the day
# somebody renamed a test. The test says what kind of test it is; the build file does not
# have to keep a list.
#
# They are skipped, never shortened or relaxed. They still run in `make accept`.
.PHONY: test
test:
	$(GO) test -short ./...

# The acceptance run: the whole suite including the century-scale tests, which encode the
# goal of the project and are expected to be red whenever the goal is unmet (method note
# 7). Run this before committing anything that touches the simulation.
#
# Measured, on four cores: TestVillageReplacesItself alone is 47 minutes (four seeds ×
# a hundred years, ending at populations of 120 to 177 from 68 settlers).
# TestVitalStatisticsArePlausible runs six seeds over the same horizon. Go applies
# -timeout per package, so internal/sim must accommodate both together — call it two
# hours of real work.
#
# Four hours, then. The number is deliberately far above the measurement: a timeout is a
# net for a genuine hang, not a budget for the expected run. Set it near the true cost and
# the first honest slowdown reads as a deadlock. Go's ten-minute default is a figure
# chosen for unit tests and says nothing about how long a thousand simulated years takes.
.PHONY: accept
accept:
	$(GO) test -timeout 4h ./...

# A probe: an expensive measurement that prints findings and asserts nothing. Kept out of
# the default build behind a tag, and named explicitly here so that running one is a
# deliberate act.
#
#	make probe P=TestMinimumViableFounding
.PHONY: probe
probe:
	@test -n "$(P)" || { echo "usage: make probe P=TestName"; exit 2; }
	$(GO) test -tags probe ./internal/sim -run '$(P)' -timeout 3h -v

.PHONY: vet
vet:
	$(GO) vet ./...
	$(GO) vet -tags probe ./internal/sim

.PHONY: build
build:
	$(GO) build ./...
