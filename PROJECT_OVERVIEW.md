# Cycling Sim Project Overview

## Summary
Cycling Simulator is a Go-based race simulation engine with a Wails (Go + WebView) desktop UI built using React and Vite.

## Architecture
- Go backend: data access, simulation engine, selection logic, and Wails bindings.
- Wails v2: exposes backend methods to the frontend and hosts the UI.
- React + Vite frontend: race selection, simulation view, results, settings, and validation UI.

## Key Entry Points
- Backend app: `app.go` (Wails bindings and API)
- Wails launcher: `main.go`
- CLI tool: `cmd/cycling-sim/main.go`
- Frontend entry: `gui/src/main.tsx`
- Frontend routes: `gui/src/App.tsx`

## Data and Config
- CSV seed data: `data/riders.csv`
- Default DB file: `cycling_sim.db`
- Override DB path: set `CYCLING_SIM_DB`

## Common Commands
- Seed DB: `go run ./cmd/cycling-sim seed`
- Clear DB: `go run ./cmd/cycling-sim clear`
- Run race: `go run ./cmd/cycling-sim run <race-id> [--incidents]`
- Run stage race: `go run ./cmd/cycling-sim run-stage-race <stage-race-id>`
- Wails dev (Ubuntu 24.04 / webkit2gtk 4.1): `wails dev -tags webkit2_41`
- Wails build: `wails build -tags webkit2_41`
- Go tests: `go test ./...`

## Project Structure (excluding node_modules)
```
./.gitignore
./CLAUDE.md
./HANDLING_ANALYSIS.md
./PROJECT_OVERVIEW.md
./app.go
./cmd/cycling-sim/cycling_sim.db
./cmd/cycling-sim/cycling_sim.db-shm
./cmd/cycling-sim/cycling_sim.db-wal
./cmd/cycling-sim/main.go
./cmd/cycling_sim.db
./cmd/cycling_sim.db-shm
./cmd/cycling_sim.db-wal
./cycling-sim
./cycling_sim.db
./cycling_sim.db-shm
./cycling_sim.db-wal
./data/original.csv
./data/riders.csv
./go.mod
./go.sum
./gui/dist/assets/index-B4ymN51s.js
./gui/dist/assets/index-DCeqGoFs.css
./gui/dist/index.html
./gui/index.html
./gui/package-lock.json
./gui/package.json
./gui/package.json.md5
./gui/src/App.tsx
./gui/src/components/Layout.css
./gui/src/components/Layout.tsx
./gui/src/main.tsx
./gui/src/pages/Home.css
./gui/src/pages/Home.tsx
./gui/src/pages/RaceSetup.css
./gui/src/pages/RaceSetup.tsx
./gui/src/pages/Results.css
./gui/src/pages/Results.tsx
./gui/src/pages/Settings.css
./gui/src/pages/Settings.tsx
./gui/src/pages/Simulation.css
./gui/src/pages/Simulation.tsx
./gui/src/pages/StageRaceGC.tsx
./gui/src/pages/StageRaceSetup.tsx
./gui/src/pages/Validation.css
./gui/src/pages/Validation.tsx
./gui/src/styles/index.css
./gui/src/wailsjs/runtime.d.ts
./gui/tsconfig.json
./gui/tsconfig.node.json
./gui/vite.config.ts
./gui/wailsjs/go/main/App.d.ts
./gui/wailsjs/go/main/App.js
./gui/wailsjs/go/models.ts
./gui/wailsjs/runtime/package.json
./gui/wailsjs/runtime/runtime.d.ts
./gui/wailsjs/runtime/runtime.js
./internal/database/courses.go
./internal/database/db.go
./internal/database/db_test.go
./internal/database/races.go
./internal/database/results.go
./internal/database/riders.go
./internal/model/course.go
./internal/model/model_test.go
./internal/model/race.go
./internal/model/result.go
./internal/model/rider.go
./internal/selection/profile.go
./internal/selection/selector.go
./internal/selection/selector_test.go
./internal/simulation/accuracy_test.go
./internal/simulation/engine.go
./internal/simulation/engine_test.go
./internal/simulation/fatigue.go
./internal/simulation/fatigue_test.go
./internal/simulation/helpers_test.go
./internal/simulation/peloton.go
./internal/simulation/physics.go
./internal/simulation/physics_test.go
./internal/simulation/tactics.go
./internal/simulation/tactics_test.go
./internal/simulation/validation_test.go
./main.go
./wails.json
```

Notes:
- The list excludes `gui/node_modules` to keep the structure readable.
- `gui/dist` and `*.db` files are generated artifacts.
