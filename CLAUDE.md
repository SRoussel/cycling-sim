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
- **Ability modifiers** (1-100 scale, 70 = WT average): `effectivePower = base * (1 + (ability-70)*0.002)`, giving ±4% range
- **Drafting model**: Position-based CdA reduction (solo=1.0, deep peloton=0.25). Naturally produces bunch finishes on flat terrain.
- **Climb splitting**: On gradients >4%, riders split by W/kg with minimal draft benefit
- **Speed solver**: Bisection on cubic `0.5*CdA*rho*v³ + staticForce*v - P = 0` (robust for all gradients including descents)
- **W' balance**: Depletes above FTP at `(P-FTP)*dt`, reconstitutes exponentially below FTP
- **Stage race fatigue**: `cumulativeFatigue += stress * (1 - recovery/150)`, reduces FTP by `fatigue*0.001`

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
./cycling-sim results <race-id>       # view saved results
./cycling-sim gc <sr-id> <stage>      # view GC standings
```

## Frontend
- `gui/src/sim/useTeamControl.ts` holds all live-race state/commands; `gui/src/components/race/` holds the shared panels. Both `Simulation.tsx` and `StageRaceSimulation.tsx` use them.
- Typecheck with `cd gui && npx tsc --noEmit`.

## Data notes
- Course segments must sum to the stated distance (`fitToDistance` in seed, enforced by a test). Changing seed data requires `clear` + `seed`.
- `data/riders.csv`: ~370 of 521 riders share templated ability profiles; ratings are the main realism bottleneck.
