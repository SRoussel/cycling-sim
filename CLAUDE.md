# Cycling Race Simulator

A cycling race simulator in Go producing realistic men's WorldTour/ProSeries racing, played as a "race director" game (Wails desktop app) with a season mode. A physics model (power, drag, gravity, drafting, anaerobic reserve) with rider intent and team tactics on top.

This file describes the model **as it is now** and how to work on it. The full development log (every change, its measured effect, and the many variants tried and rejected) is in **`docs/model-history.md`**: read the relevant part before retrying an idea, and append dated entries there for new work.

## Working rules (from the user)
- **Physics or rider intent, never hardcoded gradient/distance/ability thresholds.** Deliberate special logic is fine when it's explicit; no test-chasing tweaks. One change at a time.
- **Judge on race character, not on recreating one edition.** A real race is one draw. Use calibration (do the sim's chances come true as often as it says) and each race's ~10-edition history (finish types, margins, GC gaps). Gap-error sums and real-winner rates are diagnostics only. A rider overperforming once is expected; the fit must not turn it into a rating.
- **2026 is the validation season** (`validate --sets 2 --season 2026`); 2024/2023 references are kept but not used for decisions. `validate --quick` (~10 min, noisy) for iterating.
- **Keep correct bug fixes**, even if they regress something; fix what was tuned around the bug.
- **Show the numbers** (per-rider tables) before committing model changes or new fitting rules. A routine refit after an approved model change is applied first and its table shown after (stop and ask first if any rider moves more than ~3 points or a favourite's correction changes sign). Back up the DB before `apply-ratings` (`sqlite3 cycling_sim.db ".backup 'data/local/backups/<name>.db'"`; plain `cp` misses the WAL). Ask before `clear` + `seed` on the real DB.
- **Data sources:** never scrape FirstCycling/PCS (bot protection, robots.txt); Wikipedia is fine. PCM/WorldDB mod data is the mod authors': read it from the local install, never commit it or bulk-derived data (`data/local/` is gitignored). WorldDB values are one developer's attempt at realism: use them for facts (age, height, weight) and rank order only, never copy their numbers.
- Never call an LLM inside the race loop. Don't push without asking.
- Slow test runs are a concern: prefer exact speed-ups; avoid approximate caches that could shift results.

## Build & Test
```bash
export PATH=$HOME/go_install/go/bin:$HOME/go/bin:$PATH
go build ./cmd/cycling-sim/       # CLI
go test ./...                      # all tests (desktop app: go test -tags webkit2_41 ./...)
cd gui && npx tsc --noEmit         # frontend typecheck
wails generate module -tags webkit2_41   # after adding Go methods the GUI calls
```
Go 1.24+, `modernc.org/sqlite` (pure Go). DB path: `CYCLING_SIM_DB` (default `cycling_sim.db`; WAL, auto-migrating schema). The `run-cycling-sim` skill drives the GUI headless. Profiling: `data/local/zz_prof_test.go.txt` (BenchmarkZZRaces); trace harnesses `data/local/zz_*_test.go.txt` (copy into `internal/simulation/` to run; `traceChase`/`traceEffort` hooks are nil in normal runs).

## Project structure
- `main.go`, `app.go`, `interactive_*.go`, `season_api.go`, `rider_profile_api.go` — Wails backend (package main); `gui/` React/Vite frontend
- `cmd/cycling-sim/` — CLI; `cmd/dbcheck/` — DB inspection
- `internal/service/` — race loading, start lists, running and saving (shared by CLI and GUI; use it, don't duplicate)
- `internal/seed/` — riders and rating layers, WorldDB merge, courses, races, stage races (incl. real-route and 2026 editions, stand-ins)
- `internal/selection/` — start-list selection; `internal/calibrate/` — `fit-ratings`
- `internal/stagefile/` — PCM stage files (`.cds`) → segments; `internal/pcmdb/` — PCM database reader
- `internal/season/` — season mode; `internal/director/` — decision moments
- `internal/model/` — domain structs; `internal/database/` — SQLite CRUD
- `internal/simulation/` — the engine: `physics.go` (speed solver, forces, drafting), `power_ceiling.go`, `power_curve.go`, `fatigue.go` (W', durability, stage fatigue), `fuel.go`, `form.go`, `wind.go`, `altitude.go`, `peloton_follow.go` (groups, pace lines, draft), `peloton_groups.go` (effort, teams, attacks/moves), `peloton_segment.go` (steps), `tactics*.go`, `escape.go` (attacks, commitment, finishes), `chase.go` (race control), `engine.go`/`runner.go`, validation files (`validation_2026.go`, `calibration.go`, `validation_shape.go`, …)

## CLI
```bash
./cycling-sim seed | riders | courses | races
./cycling-sim run <race-id> [--incidents] [--seed N]
./cycling-sim run-stage-race <sr-id> [--seed N]
./cycling-sim validate --sets 2 --season 2026   # the check for decisions (~30 min)
./cycling-sim validate --quick                  # one seed set, a third of the runs (~10 min)
./cycling-sim validate [-v] | --sets N [--cross-season]   # older 2024/2023 references
./cycling-sim fit-ratings --season=2026 [file]  # fit thresholds to every 2026 reference
./cycling-sim apply-ratings                     # re-apply rating layers to the DB in place (keeps results)
./cycling-sim import-stages <mod>/Stages        # real route profiles from the local WorldDB mod
./cycling-sim import-riders <mod dir>           # WorldDB riders → data/local/riders_worlddb.csv
./cycling-sim season ... | results <race-id> | gc <sr-id> <stage>
```

## How the model works

### Physics
- **Speed:** solve 0.5·ρ·CdA·(v+w)|v+w|·v + (gravity + rolling)·v = P for v (bracketed Newton, `solveSpeed`; unique positive root). Headwind w from segment headings and race wind (roads meander ±60°); altitude from integrated gradients, air density by standard atmosphere, aerobic power by Bassett 1999 (~92% at 2000 m).
- **Drafting:** CdA × `DraftFactor(position)`: 0.65 on one wheel → 0.25 deep in a packed bunch. A fast bunch keeps its depth of shelter (riders deep in it are not capped at the 5th wheel). Single file only on gravel (no shelter beyond the 5th wheel, 70% of the draft kept) and in crosswind echelons (riders beyond a 10-rider echelon lose shelter in the gutter when the front rides hard). Cobbles keep 40% of the draft. Pace lines rotate up to `maxRotation` riders.
- **Rough roads:** cobbles Crr by sector stars (0.008–0.020), gravel ~0.007–0.011, each × the rider's rough-road factor (`roughResistanceFactor`): −1%/point of Handling around 70 and (mass/72 kg)^−0.5 for vibration (half on gravel). Handling never changes power.
- **Descents:** a speed cap from Descending and technical roads.

### Riders
- **Threshold (FTP)** in watts; W/kg is the main lever. Ability modifiers (±0.2%/point around 70): Climbing (blended in by the climb factor), TT, Sprint (sprint power).
- **Anaerobic reserve W'**: body mass × 300 J/kg × (1 + 0.03·(Punch − 75)) (~14–28 kJ across the field, biggest for punchy and fast riders). Reconstitution (Skiba) faster with Repeatability. Fades with the day's work like threshold.
- **Short-effort ceiling:** above threshold a rider rides threshold + W' spread over the effort, never above maximal aerobic power + the fading part of his sprint: MAP + (sprint − MAP)·e^(−(t−5 s)/40 s), MAP = threshold × (1.17 + 0.007·(Punch − 75)) (~1.11 to 1.31). All bursts go through `maxPowerFor`.
- **Durability:** threshold and W' fade ~2.5%/1000 kJ of work (Endurance-scaled, cap 15%). **Fuel:** a carbohydrate store burned by intensity, threshold falling below 30% of it (70% at empty). **Surges:** riders deep in a bunch pay to regain speed lost to braking (accordion).
- **Form:** daily form ±~1% plus off days (1 in 15 for Consistency 75, 3–8% down); a race form drawn once per stage race; seasonal form (peaks by rider type) for real editions and the season mode. **Stage fatigue:** cumulative, FTP × (1 − 0.004·fatigue), floor 85%, reduced by Recovery.
- **Ratings pipeline** (`seed.BuildRiders`): `riders.csv` → `rider_llm.csv` → `rider_derived.csv` → `rider_overrides.csv` → thresholds scaled with mass^0.75 → WorldDB (real ages/heights; Endurance, Recovery, Handling, Descending, Tactical, Punch, Repeatability re-dealt in WorldDB's rank order, our values; Punch ranked by hill + acceleration − mountain on a normal spread, sd 6; threshold W/kg, sprint watts and CdA re-dealt by rank, CdA from the body by Heil frontal area ±4% by TT rank) → `rider_fitted.csv` last (FTP_scale corrections).
- **Fitting** (`fit-ratings --season=2026`): every 2026 reference (10 GC, 11 one-day), races on stand-in routes at half weight; ridge prior (the base counts as one race), bound ±10% shrinking with evidence (one race ±5%), per-pass rate limit; the result is the average of the last half of the passes, printed with each rider's spread.

### Groups and steps
- Segments are simulated in ≤2 km steps; one-day races after the early phase use 250 m steps with a 3 s contact range (10 s otherwise): riders within contact are one road group.
- A group's pace comes from a rotating pace line of workers (chasing, attacking, driving); a big group with nobody working holds a tempo set by its middle riders until the finale. Followers hold the pace with their draft and W', or are dropped. Cooperative chases pool W' (N workers sharing k slots spend it N/k faster). An attacker sets his group's pace at his burst; when a favourite attacks (or anyone in the final 20 km) the group answers at that pace.

### One-day races
- **Phases:** early (calm: nobody drives, a non-threatening break goes and the bunch eases), then the race opens where the remaining hard sections are half the course's hard riding (`openingKm`, never later than 60 km); final 20 km.
- **Leaders and favourites:** a team's leader is its rider fastest over the course's hard sections (`decisiveTime`); favourites are the 5 fastest over the course at threshold.
- **The finish** is judged on the finish: `finishTime`, each rider's time over the last km with the power he has left (the sprint decides flat finishes, W/kg uphill ones).
- **Attacks** (`attackPays`): only a leader who'd lose the finish to a rival leader in his group attacks (the best finisher waits); the strongest rival must be unable to hold the attack, ridden over the rest of the climb or sector (`attackKm`), and the solo time to the line must beat the chase, projected with its helpers pooled (`pooledTimeToLine`). On a climb attacks come as late as they still drop as many rivals. Leaders who'd lose the finish also attack on hard sections when they have as much W' left as their rivals and the attack drops one of them (`sprintLoserAttacks`) and press (`pressPays`); a favourite's team wears the field down (`wearDown`) against a better finisher it outrides. In a safe front selection (`frontSafe`) favourites don't press and an attack only has to beat the strongest rival; no attack a faster finisher can follow.
- **Driving:** a favourite's team sets the pace its leader can hold in the draft at the phase intensity (90% mid-race), but each domestique on the front is capped by what he can hold to the end of his job (the finale mid-race, the lead-out in the finale).
- **Commitment and chases:** a leader ahead commits while his group holds off a threatening rival (`escapeHolds`); behind, teams chase for their leaders (planned chase: a favourite's team wants the race; sprint teams chase a finish their sprinter can contest). A group behind a front no pace line can catch (`winGone`) races for places (riders who'd win its sprint sit on, grudging two-rider turns) — never against the main bunch (a group behind a bigger group rejoins it), the front projected as disorganised when it holds rival leaders of 2+ teams, the chase with its helpers pooled. Riders dropped behind the main field chase to rejoin it.

### Stage races (race control, `chase.go`)
- Teams chase a break only if it threatens what they ride for: the race leader's team defends the jersey at any cost; teams defending places 2–10 only when a steady tempo (`stageHuntMax` 85%) keeps the break within their allowance; a favourite's team hunts the stage when it decided to that morning (50% on ordinary stages, 100% at prestige 1, halved per stage won, lower with a big lead) and a tempo catches the break; a sprint team chases at any cost only if its sprinter survives the favourites racing the last 20 km (`sprinterSurvivesFinale`), otherwise like a hunter.
- With nobody chasing, the bunch rides easy (70%); a break with a group behind rides at its sustainable pace. GC contenders (leaders or GC top 20 within reach) are followed when they attack; a contender with a rival up the road limits his losses. Each team's GC leader is its best climber by W/kg; strong domestiques may be freed as co-leaders by a team plan drawn per race. Free riders may want the break (`setBreakIntent`). Time bonuses 10/6/4 s. Time cuts by climbing density.

### Validation (2026)
- References: `data/reference_2026.csv` (one-day top 10s, GC top 10s with gaps) and `data/startlists_2026.csv` (real fields), both from Wikipedia. Routes: real 2026 stage files for the Giro, Tirreno, Romandie, Strade, E3, Gent-Wevelgem, Flanders, Roubaix and Flèche; stand-ins (newest real route the mod has) for Paris-Nice (2025), the Basque Country and the Dauphiné (2023) and the Tour (the real 2023 route); generic courses for Suisse (2026 was 5 stages) and the Vuelta (no file in any year). Check for newer WorldDB updates (only spring 2026 routes exist so far).
- The check reports per race: gap errors (diagnostic), finish types and margins against **history** (`data/history_results.csv`, ~10 editions per race; ← flags values outside the range), GC spread, **calibration** (every starter's top-10 and win chances against what happened, by band; Brier; winners' log score) and **race shape** per race kind (how stages were won: sprint / breakaway / selection / TT; Giro, Tirreno, Romandie 2026 and the Tour 2023), with crashes, punctures and mechanicals on.

### Season mode
`internal/season` (CLI `cycling-sim season ...`, GUI `/season`, `season_api.go`): a save game with its own copy of every rider (contract, salary, morale, racing load, potential, next-year signing), the seeded calendar in the season's year (reference editions and stand-alone TTs left out), results with simplified UCI points and planned starts. Yearly programmes by prestige × fit (`planYear`: targets, ≤2 Grand Tours, load and race-day budgets, ≤2 leaders per team per event); racing applies load and morale factors and seasonal form. Rider value (`Values`: half physics — hard-section time, TT time, sprint — half ratings) sets salaries and asks. Board objectives and confidence move budgets; contracts, transfers, free agents, retirements, ageing and neo-pros at the year's end. Races never touch `race_results`.

### Desktop app
- **Look ("broadcast", Nov 2026):** tokens in `gui/src/styles/index.css` — near-black page, raised panels (hairline border, soft shadow), Archivo in sentence case with JetBrains Mono for numbers, one yellow accent (`--color-primary`, dark ink on it) for the player's things, the race front and the main action; team colours only as thin bars beside names; no emoji in the UI. Mockups of every screen: the "TV broadcast redesign" canvas (local copy in `data/local/design/`).
- Live race screen (`RaceDashboard`, `RaceTopBar`, `RoadView`, `TeamDock`); all live-race state in `gui/src/sim/useTeamControl.ts`, shared panels in `gui/src/components/race/`. `ProfileView` measures itself and draws at its real pixel size (gradient bands, named climbs, elevation scale).
- Team orders (`TeamOrderProvider`: chase, break, pace, protect, lead-out, stage ambition) and the player's rider commands reach the engine; a rider's own command always wins. AI teams, the CLI and validation are unaffected.
- Race director (`internal/director`): decision moments read from `simulation.RaceView` (rival favourite attacks, a card dropped, decisive climbs and cobbled sectors ahead, the finale, breaks); a race plan before one-day races and each road stage (leader, break, where to drive, where to attack). The team panel shows every order and command with who set it and why (`interactive_live.go`).
- Rider panel (`gui/src/components/RiderProfile.tsx`, `GetRiderProfile`): the model's power curve against the field's 10th–90th percentile band, a percentile strip, and every rating and body number as percentile bars; compare two riders.

## Data notes
- Course segments must sum to the stated distance (`fitToDistance`). `seed` on an existing DB adds missing courses, races and stage races by name (keeps results); changing existing seed data needs `clear` + `seed` (ask first). `apply-ratings` updates ratings and ages in place.
- Route profiles come from the locally installed WorldDB mod (Steam Workshop 2511310/3647555648, folder `Stages`): `data/stage_sources.csv` maps courses to files (optional `gravel` column); `import-stages` writes `data/local/course_profiles.json` (never commit). PCM units: distance 10 m, altitude 7.5 m.
- WorldDB riders: `import-riders` reads the mod database (`internal/pcmdb`); riders the base files lack get ratings from ridge fits of PCM attributes. Names match ignoring accents, middle names, double surnames and one-letter variants.
- `data/rider_llm.csv` (offline LLM-drafted profiles for templated riders, `data/llm/`); `data/stage_prestige.csv` (stage prestige); `data/results/` (saved pages for `import-results`).

## Open problems (Nov 2026, in priority order; baseline in docs/model-history.md, "Baseline after the refit")
1. **One-day solo shares by race:** overall right (37% vs history 41%) but Lombardia now too rarely solo (28% vs 73%, margin 0'00" vs 0'31") and Roubaix too (12% vs 40%) since attacks are judged against a pooled chase; Liège too often (93% vs 64%), Milan-San Remo still 40% (9%).
2. **Flèche** still splits ~60-75 km out in some runs (finish correct: favourites together on the Mur).
3. **Stage-race GC spread:** Giro 10th ~28' (history 9-21'), Dauphiné 10th 13'40" (4-10'); week-long 2nd places too big at Romandie/Suisse/Paris-Nice and too small at the Basque Country/Tirreno.
4. **Calibration:** one-day top 10 overconfident (riders given 50-75% make it 31%); GC top 10 given 75-100% make it 66%.
5. **Week-long races** have too many sprints (2.4 vs 1.5 per race) and too few selections (2.6 vs 3.5).
6. **Grand Tour breakaway wins** 6.9-7.2 per race (real 8.5); sprints 7.1-7.4 (6.5).
