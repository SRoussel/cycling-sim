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
- `cmd/cycling-sim/main.go` — CLI entry point (seed, run, results, gc commands)
- `internal/model/` — Domain structs (Rider, Course, Segment, Race, Result, GCStanding) with validation
- `internal/simulation/` — Core simulation engine:
  - `physics.go` — Speed-from-power solver (bisection method), drag/gravity/rolling forces, drafting
  - `fatigue.go` — W' balance (within-stage) + cumulative multi-stage fatigue
  - `peloton.go` — Group dynamics, climb splitting, descent speed caps
  - `tactics.go` — Breakaway logic, sprint finish resolution, daily form variance
  - `engine.go` — Orchestrates road races, TTs, and stage races
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
./cycling-sim run <race-id>           # simulate a race
./cycling-sim run-stage-race <sr-id>  # simulate stage race with GC
./cycling-sim results <race-id>       # view saved results
./cycling-sim gc <sr-id> <stage>      # view GC standings
```
