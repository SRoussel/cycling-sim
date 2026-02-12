# Accuracy Improvement Roadmap

## Phase 1 - Data and Calibration
- Build a calibration dataset with real race results, startlists, and course profiles.
- Fit core model parameters (handling effect, drafting, fatigue) via regression or Bayesian tuning.

## Phase 2 - Validation Suite
- Expand automated validation metrics:
  - Top-10 overlap
  - Rank correlation
  - Time-gap error
- Add per-terrain breakdowns (cobbles, climbs, flat).

## Phase 3 - Selection Realism
- Improve startlist selection using real team rosters and role balance.
- Bias selection by race type and season context.

## Phase 4 - Tactics Model
- Add race-type tactics (breakaway success rates, sprint lead-outs, GC protection).
- Add team strategy profiles with controlled randomness.

Rider archetypes: Define tactical profiles (sprinter, GC leader, breakaway specialist, domestique) with decision weights for risk, aggression, and energy budgeting.
Team objectives: Encode team goals per race type (stage win vs GC protection) and let roles coordinate (lead‑out trains, chase control, protect leader).
Stateful decision model: Track race phase, gaps, remaining terrain, and W′ to drive decisions (attack, sit in, chase, bridge, conserve).
Opponent modeling: Add simple estimators of rival strength and intent (e.g., “dangerous breakaway,” “sprinter teams chasing”).
Event-driven tactics: Trigger behavior on key terrain (cobbles sectors, summit within X km, final 10 km).
Energy realism: Incorporate W′ and fatigue into probability of attack and success; discourage repeated attacks when depleted.
Breakaway dynamics: Model cooperation/defection and when peloton allows a break.
Calibration hooks: Add tunable parameters and log tactical outcomes for replay analysis and tuning.

## Phase 5 - Course Fidelity
- Enrich course segments with wind, surface transitions, and technicality.
- Validate segment distributions against real races.

## Phase 6 - Rider Form and Variability
- Introduce season form curves and cumulative fatigue.
- Add day-to-day variance anchored to rider consistency.

## Phase 7 - Incident Modeling
- Calibrate crash and puncture rates by terrain and weather.
- Add neutralizations or time penalties.

## Phase 8 - Benchmarks and Gates
- Add golden run fixtures for key races.
- Set accuracy thresholds required for release.
