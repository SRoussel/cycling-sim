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
- `internal/calibrate/` — `fit-ratings`: fits rider thresholds to real results by simulation
- `internal/stagefile/` — reads Pro Cycling Manager stage files (`.cds`) installed locally, converts route profile + cobbled/gravel sectors into segments
- `internal/model/` — Domain structs (Rider, Course, Segment, Race, Result, GCStanding) with validation
- `internal/simulation/` — Core simulation engine:
  - `physics.go` — Speed-from-power solver (bisection method), drag/gravity/rolling forces, drafting
  - `fatigue.go` — W' balance (within-stage) + cumulative multi-stage fatigue
  - `peloton_*.go` — groups (`peloton_follow.go`), effort/teams (`peloton_groups.go`), steps (`peloton_segment.go`)
  - `wind.go`, `altitude.go` — wind/echelons, climb factor, altitude; `PrepareSegments` readies a course for a race
  - `tactics.go` — Breakaway logic, sprint finish resolution, daily form variance
  - `engine.go`, `runner.go` — Orchestrate road races, TTs, and stage races (runner adds progress callbacks)
  - `validation.go` — synthetic fields; `validation_real.go` — 2024 references + 2023 hold-out; `validation_speed.go` — winning speeds
- `internal/database/` — SQLite schema, migrations, CRUD operations

## Key Design Decisions
Guiding rule: express behaviour as physics or rider intent, never as hardcoded gradient/distance thresholds.

- **Ability modifiers** (1-100 scale, 70 = WT average): `effectivePower = base * (1 + (ability-70)*0.002)`, giving ±4% range. W/kg (FTP/weight) is the main performance lever. Climbing ability blends in with the climb factor (full effect on real climbs, none on the flat). `EffectivePower` is the rider's threshold on that terrain *and* the W' threshold (`thresholdIntOn`) — keep them the same, or riding "at threshold" drains/refills W' and cancels abilities.
- **Speed solver**: bisection on `0.5·ρ·CdA·(v+w)|v+w|·v + staticForce·v = P` (w = headwind; robust on descents).
- **Steps**: segments are simulated in ≤2 km steps; tactical/incident probabilities are per 5 km (`ScaleToDistance`).
- **Groups** (`peloton_follow.go`): one rule on all terrain. Pace comes from a rotating pace line of workers (riders chasing/attacking/driving; small groups share). A big group with nobody working holds a collective tempo set by its middle riders (`tempoRiders`), not its strongest, until the finale. Followers must hold it with positional draft + spendable W'; else they're dropped. Groups line out at ≥95% threshold (no deep bunch shelter). Cooperative chases of a break spend W' N/k faster (pool depth). Groups can't pass each other; riders in a group close up to its front.
- **Contact**: riders within 10 s are one road group. Breakaway state is read from the road.
- **Effort** (`riderEffort`): each mode draws on W' once over one horizon — attack: the step; committed chase / finale (last 20 km): to the line; team driving: rest of the climb. Domestiques pace at threshold until the 3 km lead-out.
- **W'**: anaerobic capacity from peak power (`wPrimeSeconds` 14 s × sprint watts, ~15–28 kJ): puncheurs and sprinters carry more than pure climbers, which decides short climbs. Depletes above FTP; recovers with Skiba (2012) τ = 546·e^(−0.01·(CP−P)) + 316 s.
- **Durability**: in-race threshold fades ~2.5%/1000 kJ (Endurance-scaled, cap 15%). Sprint power fades the same way and loses up to 10% with an emptied W' (`FatiguedSprintPower`), so lead-out riders finish behind.
- **Altitude**: elevation integrated from gradients (course low point = 0 m); air density by standard atmosphere; aerobic power by Bassett et al. 1999 (acclimatised): ~92% at 2000 m.
- **Drafting**: `DraftFactor` is a smooth curve: 0.65 on one wheel → 0.25 deep in a bunch.
- **Stage fatigue**: `cumulativeFatigue += stress·(1 − recovery/150)`, FTP × (1 − 0.004·fatigue), floor 85% (~4% over a Grand Tour), always applied to the rider's base FTP (a bug once compounded it stage on stage, ~40% by Paris). Time cuts by climbing density (waived if >20% outside).
- **Wind**: segments have headings (synthetic in seed data); headwind enters the solver; crosswind removes gutter shelter beyond a 10-rider echelon by yaw angle; roads meander ±60° per step.
- **Terrain**: `ClimbFactor` (share of power lifting weight) classifies terrain for tactics.
- **Team strategy**: a team drives when its leader's threshold pace would break the group's fastest finisher even in the draft; its domestiques then set the pace the leader can hold on the wheel at the phase intensity (`leaderTempoSpeed`: a tempo mid-race, threshold in the finale), not an all-out effort. Leaders race in the finale; shelter needs a teammate.
- **Stage races** (`AssignCoLeaders`, `gcRivalAhead`): each team's GC leader is its best climber by W/kg (not the ability-rating average `AssignTeamRoles` uses); domestiques who are among the 10 strongest climbers in the field, or in the GC top 10, become co-leaders. A GC leader with a rival up the road who would pass them on GC limits losses (chases to the line); chasing GC leaders take turns instead of sitting in.

## Special cases still to replace (audit, Nov 2026)
These break the guiding rule (gradient/distance/ability thresholds instead of physics or intent). Replace one at a time and judge with `validate --sets 3`:
- `DecideAction` (tactics_model.go): stage-race mountain attacks from a probability table (km < 15/8/25/12, 0.08–0.22, ×0.7 jersey, ×(1+deficit/1800)); GC leaders conserve on flat terrain (85% in the final); "one-day flat/rolling: always chase" (comment cites the Poggio); "cobbles: dangerous riders always chase" (cites Roubaix); one-day mountain attacks at 8%/5 km; random team objectives with their own rules.
- `breakawayProbability` (tactics.go): ~10 multipliers keyed to km windows (×1.5 at 30–60 km, ×0.05 beyond 80 km on the flat, ×0.35 below, ×0.3 inside 10 km, ×0.7 beyond 100 km), sprinter ×0.3 unless handling > 85 on cobbles, capped handling boost.
- `BuildRivalAssessment`: "dangerous" = top 5 by an ability score that weights Handling 3× on cobbles (comment lists named riders' scores).
- `PhaseFromKm`: phases at 60 km / 20 km drive holding intensity, driving, rivalAhead and the finale.
- Experiment branch `experiment/physics-attacks-v2` (on top of W′ from peak power): one-day attacks by physics replace the 8% roll — go when the strongest rival leader can't hold the wheel at your burst speed here and your solo time to the line beats the group's chase pace line; a group commits while its pace line holds off a threatening rival behind. Winners improve (Lombardia, Roubaix, Liège) but the gap sum is 2889 s vs 2749 s on main: once a selection goes, riders behind lose minutes (MSR 2023 Ganna 379 s vs 15 s real). Fix the chase dynamics behind a selection before merging; don't tune the attack rule to compensate.

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
./cycling-sim validate [-v]           # synthetic, 2024 real races, 2023 hold-out, race speeds (-v: per rider)
./cycling-sim validate --sets N       # reference races over N independent seed sets (judge model changes with this)
./cycling-sim fit-ratings [--classics] [file]  # fit thresholds to 2024 refs by simulation
./cycling-sim apply-ratings           # re-apply all rating layers to the DB in place (keeps results)
./cycling-sim import-stages <dir>     # real route profiles from local PCM stage files (keeps results)
./cycling-sim import-results [dir]    # derive ratings from saved result pages
./cycling-sim results <race-id>       # view saved results
./cycling-sim gc <sr-id> <stage>      # view GC standings
```

## Frontend
- `gui/src/sim/useTeamControl.ts` holds all live-race state/commands; `gui/src/components/race/` holds the shared panels. Both `Simulation.tsx` and `StageRaceSimulation.tsx` use them.
- Typecheck with `cd gui && npx tsc --noEmit`.

## Data notes
- Course segments must sum to the stated distance (`fitToDistance` in seed, enforced by a test). `seed` on an existing DB adds courses/races/stage races that are missing by name (keeps results); changing existing seed data requires `clear` + `seed`.
- `data/riders.csv`: ~370 of 521 riders share templated profiles. `data/rider_llm.csv` replaces 338 of them with LLM-drafted physical profiles (specialty, weight, W/kg; confidence high/medium/low), written offline from `data/llm/draft_spec.txt` by `python3 data/llm/expand.py [--review]`. Never call an LLM inside the race loop (speed, reproducibility). Leakage caveat: an LLM's sense of a specialty is shaped by results it has seen, so the 2023 hold-out isn't fully independent of it.
- Rating layers at seed time (`seed.RatingLayers`): `riders.csv` → `rider_llm.csv` → `rider_derived.csv` (from `cycling-sim import-results`, saved pages in `data/results/<profile>/`) → `rider_overrides.csv` (curated; GC tier calibrated to 2024 Tour gaps). Never scrape FirstCycling/PCS (robots.txt / bot protection).
- `cycling-sim validate` scores synthetic fields, six 2024 classics and the 2024 Tour GC (verified non-starters excluded via `DidNotStart`; reference riders always start), a 2023 hold-out (never fitted) and winners' average speeds.
- `data/rider_fitted.csv` (from `fit-ratings`, Tour GC only) is a 4th layer of `FTP_scale` multipliers (≤ ±4%). Don't fit the classics: with 1-2 results per rider it overfits (the 2023 hold-out gets worse). The fitter flags riders pushed above the strongest rider's climbing W/kg — that means it is compensating for a model gap (as it did before co-leader tactics), so fix the model instead.
- Route profiles: `data/stage_sources.csv` maps the 11 seeded classics and the 21 Tour 2023 stages to stage files of the locally installed "WorldDB" PCM mod (Steam Workshop). `import-stages` writes derived segments to `data/local/course_profiles.json` (gitignored: the source files are the mod author's; never commit them or bulk-derived data) and seeding uses it when present. Format notes are in `internal/stagefile/stagefile.go`; distance is in 10 m units and altitude in 7.5 m units — fixed from the climb records every file carries (start/end/length/gain in metres, which match published climbs). Reading both as one unit (before Nov 2026) made every gradient and all climbing 33% too steep; no smoothing is needed with the right units. `Tour de France 2023` (`internal/seed/tdf2023.go`) is the real 2023 route — stage distances, ITT and rest days from Wikipedia, generic template segments until `import-stages` supplies the real profiles — and is the stage-race hold-out. The generic `Tour de France` remains the 2024 reference (the mod has no 2024 Tour).
- Co-leaders: each stage race draws a team plan once (`RaceConfig.TeamPlan`); a domestique is freed with a probability falling smoothly around 10th in the field's climbing ranking (`freedByPlan`, `planSpread`). The old hard top-10 cut swung riders ~500 s on ±1% rating changes. Judge GC changes over several seed sets: one set of 32 runs once showed 84 s for a setup that averages ~780 s. `fit-ratings` always starts from every rating layer except the fitted one (`seed.UnfittedFTPs`).
- **One-day races** (`rivalAhead`): after the early phase, a rider with another team's leader up the road (and no leader of their own ahead) races for placings — chases to the line and takes turns — instead of riding the holding tempo.
- Known gaps: on real routes, top-10 gaps in hilly classics are still too large (Liège 2023 ~360 s MAE, Strade Bianche 2023 ~530 s). In the Tour 2023 hold-out Pogačar beats Vingegaard (ratings reflect 2024 form); Grand Tour GC gaps are noisy at the validation's run count. Tested and rejected (Nov 2026): raising the whole puncheur/classics tier by +0.12/+0.25 W/kg — hold-out gaps improved but 2024 gaps and real-winner rates got worse, and results were noise-dominated at 20 runs. Paris-Roubaix runs ~9% slow because of the synthetic 20 km/h wind on synthetic headings (44.9 km/h in still air); cobble resistance and cobble drafting were tested and are not the cause. Also tested and rejected (Nov 2026): a one-day leader who gets clear of rival leaders committing to the line (mirror of `rivalAhead`) — real-winner rates rose (Strade 2024 10→53%, Lombardia 2023 25→75%) but gap errors grew in 9 of 13 races (Milan-San Remo: Pogačar solo 90%, real race ends in a group). A gated version (Nov 2026: commit only if the projected time to the line at threshold beats the chase's pace line plus the gap) was neutral within noise once it required being clear of rival leaders in one's own road group (without that, leaders inside a front group of rivals committed and Milan-San Remo got worse), because leaders rarely get clear: attacks are one-step bursts. The weak point is the attack model, not commitment.
