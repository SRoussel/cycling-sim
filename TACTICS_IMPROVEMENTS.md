# Phase 4 - Tactics Model (Implementation Details)

This document refines the Phase 4 tactics improvements into concrete implementation steps, data structures, and integration points.

## 1) Stateful Decision Model

### Goal
Drive decisions with race state, terrain, gaps, and W'.

### Implementation
- Extend `RiderState` with:
  - `CurrentGroup`, `WPrime`, `Phase` (early, mid, final), `GapToFront`.
- Add a lightweight `RaceState` snapshot updated per segment:
  - `KmRemaining`, `TerrainType`, `GroupSizes`, `BreakawayTimeGap`.
- Decision function signature:
  - `DecideAction(rs *RiderState, team *TeamContext, state *RaceState) Action`.

## 2) Breakaway Dynamics

### Goal
Model cooperation/defection and peloton response.

### Implementation
- Add group behavior weights:
  - `CooperationScore` per rider (based on existing rider/team stats).
- Breakaway stability:
  - Compute `Cohesion` from cooperation average.
  - Higher cohesion => more stable gap, lower chase success.
- Peloton response:
  - Use `BreakawayLeashKm` and `GCProtectionPriority` to decide chase.

## 3) Calibration Hooks

### Goal
Make tactics tunable and observable.

### Implementation
- Add `TacticsParams` struct to `simulation` package:
  - `BreakawaySuccessBias`, `SprintTrainEfficiency`, `GCChaseAggression`, etc.
- Add logging hooks:
  - `TacticsEvent` with `type`, `riderID`, `km`, `reason`.
  - Keep a ring buffer for UI replay or later export.

## Integration Plan

1. Create `internal/simulation/tactics_model.go` with `RaceState`, decision helpers, and calibration params.
2. Extend `RiderState` with the minimal fields required for decisions.
3. Apply decisions inside `SimulateSegment` to influence power usage, drafting, and attack attempts.
4. Add a `TacticsParams` default set and surface it via Wails in the Settings page.
5. Add unit tests for decision logic using synthetic rider groups.

## Notes

- Keep tactics deterministic under seed for reproducible validation runs.
- Avoid large CPU cost by reusing `RaceState` snapshots and avoiding per-rider expensive scans.

## Deferred Ideas

- Race-type tactics
- Energy realism extensions (W' cooldowns, attack fatigue)

## Plan - Team Strategy Profiles

- Add `TeamStrategy` struct with `PrimaryObjective`, `AggressionBias`, `RiskTolerance`, `EnergyBudget`.
- Create `TeamContext` at race start from roster mix (leader/sprinter count).
- Integrate into `DecideAction` as a multiplier on attack/chase/conserve.
- Add seed-based randomness so strategies vary between races but are deterministic.

## Plan - Opponent Modeling

- Build `RivalAssessment` map keyed by team with `DangerousInBreak` and `SprintTeamsChasing`.
- Populate once per race from rider stats (top GC and sprint threats).
- Update mid-race when breakaways form and gaps exceed threshold.
- Feed into chase decision weights and breakaway suppression.

## Plan - Event-Driven Tactics

- Add event hooks in `SimulateSegment` for cobbles, summits, and final 10 km.
- Apply event modifiers to `TacticsParams` (temporary boosts/penalties).
- Log `TacticsEvent` entries with reason and km for later analysis.
