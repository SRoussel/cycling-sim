# Cycling Race Simulator

## Project Overview
A cycling race simulator in Go that produces realistic race results for men's WorldTour and ProSeries level racing. Uses a hybrid physics-based model with abstract ability modifiers.

## Build & Test
```bash
go build ./cmd/cycling-sim/       # build binary
go test ./...                      # run all tests
go test ./internal/simulation/ -v  # verbose simulation tests
```

Requires Go 1.24+ and uses `modernc.org/sqlite` (pure Go, no CGO needed).

## Project Structure
- `main.go`, `app.go`, `interactive_*.go` — Wails desktop app backend (package main); `gui/` is the React/Vite frontend
- `cmd/cycling-sim/main.go` — CLI entry point
- `cmd/dbcheck/` — small DB inspection utility
- `internal/service/` — race loading, start-list selection, running and saving; shared by CLI and GUI (use it, don't duplicate)
- `internal/seed/` — seed data (riders from `data/riders.csv`, courses, races, stage races)
- `internal/selection/` — start-list selection per race profile
- `internal/model/` — Domain structs (Rider, Course, Segment, Race, Result, GCStanding) with validation
- `internal/simulation/` — Core simulation engine:
  - `physics.go` — Speed-from-power solver (bisection method), drag/gravity/rolling forces, drafting
  - `fatigue.go` — W' balance (within-stage) + cumulative multi-stage fatigue
  - `peloton.go` — Group dynamics, climb splitting, descent speed caps
  - `tactics.go` — Breakaway logic, sprint finish resolution, daily form variance
  - `engine.go`, `runner.go` — Orchestrate road races, TTs, and stage races (runner adds progress callbacks)
  - `validation.go` — synthetic-field validation; `validation_real.go` — 2024 real-race reference results
- `internal/database/` — SQLite schema, migrations, CRUD operations

## Key Design Decisions
Guiding rule: express behaviour as physics or rider intent, never as hardcoded gradient/distance thresholds.

- **Ability modifiers** (1-100 scale, 70 = WT average): `effectivePower = base * (1 + (ability-70)*0.002)`, giving ±4% range. W/kg (FTP/weight) is the main performance lever.
- **Speed solver**: bisection on `0.5·ρ·CdA·(v+w)|v+w|·v + staticForce·v = P` (w = headwind; robust on descents).
- **Steps**: segments are simulated in ≤2 km steps; tactical/incident probabilities are per 5 km (`ScaleToDistance`).
- **Groups** (`peloton_follow.go`): one rule on all terrain. Pace comes from a rotating pace line of workers (riders chasing/attacking/driving; small groups share). Followers must hold it with positional draft + spendable W'; else they're dropped. Groups line out at ≥95% threshold (no deep bunch shelter). Cooperative chases of a break spend W' N/k faster (pool depth). Groups can't pass each other; riders in a group close up to its front.
- **Contact**: riders within 10 s are one road group. Breakaway state is read from the road.
- **Effort** (`riderEffort`): each mode draws on W' once over one horizon — attack: the step; committed chase / finale (last 20 km): to the line; team driving: rest of the climb. Domestiques pace at threshold until the 3 km lead-out.
- **W'**: depletes above FTP; recovers with Skiba (2012) τ = 546·e^(−0.01·(CP−P)) + 316 s.
- **Durability**: in-race threshold fades ~2.5%/1000 kJ (Endurance-scaled, cap 15%).
- **Stage fatigue**: `cumulativeFatigue += stress·(1 − recovery/150)`, FTP × (1 − 0.004·fatigue), floor 85% (~4% over a Grand Tour). Time cuts by climbing density (waived if >20% outside).
- **Wind**: segments have headings (synthetic in seed data); headwind enters the solver; crosswind removes gutter shelter beyond a 10-rider echelon by yaw angle; roads meander ±60° per step.
- **Terrain**: `ClimbFactor` (share of power lifting weight) classifies terrain for tactics.
- **Team strategy**: a team drives when its leader's threshold pace would break the group's fastest finisher even in the draft. Leaders race in the finale; shelter needs a teammate.

## Database
SQLite with WAL mode, foreign keys enabled. Schema auto-migrates on open. Use `CYCLING_SIM_DB` env var to set path (default: `cycling_sim.db`).

## CLI Usage
```bash
./cycling-sim seed                    # load WorldTour riders + classic courses
./cycling-sim riders                  # list riders
./cycling-sim courses                 # list courses
./cycling-sim races                   # list races
./cycling-sim run <race-id> [--incidents] [--seed N]   # simulate a race
./cycling-sim run-stage-race <sr-id> [--seed N]        # simulate stage race with GC
./cycling-sim validate                # synthetic + real-race validation report
./cycling-sim import-results [dir]    # derive ratings from saved result pages
./cycling-sim results <race-id>       # view saved results
./cycling-sim gc <sr-id> <stage>      # view GC standings
```

## Frontend
- `gui/src/sim/useTeamControl.ts` holds all live-race state/commands; `gui/src/components/race/` holds the shared panels. Both `Simulation.tsx` and `StageRaceSimulation.tsx` use them.
- Typecheck with `cd gui && npx tsc --noEmit`.

## Data notes
- Course segments must sum to the stated distance (`fitToDistance` in seed, enforced by a test). Changing seed data requires `clear` + `seed`.
- `data/riders.csv`: ~370 of 521 riders share templated ability profiles; ratings are the main realism bottleneck.
- Rating layers at seed time: `riders.csv` → `rider_derived.csv` (from `cycling-sim import-results`, saved pages in `data/results/<profile>/`) → `rider_overrides.csv` (curated; GC tier calibrated to 2024 Tour gaps). Never scrape FirstCycling/PCS (robots.txt / bot protection).
- `cycling-sim validate` scores synthetic fields, six 2024 classics and the 2024 Tour GC; real non-starters are excluded.
