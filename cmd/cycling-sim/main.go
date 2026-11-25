package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"cycling_sim/internal/calibrate"
	"cycling_sim/internal/database"
	"cycling_sim/internal/importer"
	"cycling_sim/internal/model"
	"cycling_sim/internal/seed"
	"cycling_sim/internal/service"
	"cycling_sim/internal/simulation"
	"cycling_sim/internal/stagefile"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	dbPath := database.DefaultPath()

	cmd := os.Args[1]
	args, incidents, seedVal := parseFlags(os.Args[2:])

	db, err := database.Open(dbPath)
	if err != nil {
		fatalf("Error opening database: %v\n", err)
	}
	defer db.Close()

	switch cmd {
	case "seed":
		cmdSeed(db)
	case "clear":
		cmdClear(db, dbPath)
	case "riders":
		cmdListRiders(db)
	case "courses":
		cmdListCourses(db)
	case "races":
		cmdListRaces(db)
	case "one-day-races":
		cmdListOneDayRaces(db)
	case "stage-races":
		cmdListStageRaces(db)
	case "run":
		raceID := idArg(args, 0, "Usage: cycling-sim run <race-id> [--incidents] [--seed N]")
		cmdRunRace(db, raceID, incidents, seedVal)
	case "run-stage-race":
		srID := idArg(args, 0, "Usage: cycling-sim run-stage-race <stage-race-id> [--seed N]")
		cmdRunStageRace(db, srID, seedVal)
	case "validate":
		if len(args) > 0 && args[0] == "--quick" {
			// A fast look (one seed set, a third of the runs) at the 2026 season.
			simulation.QuickValidation()
			cmdValidateSets(db, 1, false, true)
			break
		}
		if len(args) > 1 && args[0] == "--sets" {
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 1 {
				fatalf("Usage: cycling-sim validate --sets N [--cross-season | --season 2026 [--holdout]]\n")
			}
			season2026 := len(args) > 3 && args[2] == "--season" && args[3] == "2026"
			cmdValidateSets(db, n, len(args) > 2 && args[2] == "--cross-season", season2026)
			if season2026 && len(args) > 4 && args[4] == "--holdout" {
				printHoldout(db, n)
			}
			break
		}
		cmdValidate(db, len(args) > 0 && (args[0] == "-v" || args[0] == "--verbose"))
	case "import-riders":
		if len(args) == 0 {
			fatalf("Usage: cycling-sim import-riders <WorldDB mod dir or OfficialRelease.cdb>\n")
		}
		n, err := seed.ImportWorldDB(args[0], seed.DataPath(seed.WorldDBFile))
		if err != nil {
			fatalf("Error importing riders: %v\n", err)
		}
		fmt.Printf("Wrote %d WorldTeam and ProTeam riders to %s (local only; not committed).\n", n, seed.DataPath(seed.WorldDBFile))
		fmt.Println("Run `cycling-sim seed` to add the missing riders and `cycling-sim apply-ratings` to update ages.")
	case "import-stages":
		if len(args) == 0 {
			fatalf("Usage: cycling-sim import-stages <stage-files-dir>\n")
		}
		cmdImportStages(db, args[0])
	case "apply-ratings":
		n, err := seed.ApplyRatings(db)
		if err != nil {
			fatalf("Error applying ratings: %v\n", err)
		}
		fmt.Printf("Updated %d riders from riders.csv + %s (saved results kept).\n", n, strings.Join(seed.RatingLayers, " + "))
	case "fit-ratings":
		out := seed.DataPath(seed.FittedFile)
		classics, season2026 := false, false
		for _, a := range args {
			if a == "--classics" {
				classics = true
			} else if a == "--season=2026" || a == "--season2026" {
				season2026 = true
			} else {
				out = a
			}
		}
		cmdFitRatings(db, out, classics, season2026)
	case "import-results":
		dir := "data/results"
		if len(args) > 0 {
			dir = args[0]
		}
		cmdImportResults(db, dir)
	case "results":
		raceID := idArg(args, 0, "Usage: cycling-sim results <race-id>")
		cmdShowResults(db, raceID)
	case "season":
		cmdSeason(db, args, seedVal)
	case "gc":
		usage := "Usage: cycling-sim gc <stage-race-id> <after-stage>"
		srID := idArg(args, 0, usage)
		stage := idArg(args, 1, usage)
		if stage < 1 {
			fatalf("Invalid stage number: %d\n", stage)
		}
		cmdShowGC(db, srID, int(stage))
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

// parseFlags pulls --incidents and --seed N out of args, returning the rest.
func parseFlags(in []string) (args []string, incidents bool, seedVal int64) {
	for i := 0; i < len(in); i++ {
		switch a := in[i]; {
		case a == "--incidents":
			incidents = true
		case a == "--seed" && i+1 < len(in):
			v, err := strconv.ParseInt(in[i+1], 10, 64)
			if err != nil {
				fatalf("Invalid --seed value: %v\n", err)
			}
			seedVal = v
			i++
		case strings.HasPrefix(a, "--seed="):
			v, err := strconv.ParseInt(strings.TrimPrefix(a, "--seed="), 10, 64)
			if err != nil {
				fatalf("Invalid --seed value: %v\n", err)
			}
			seedVal = v
		default:
			args = append(args, a)
		}
	}
	return args, incidents, seedVal
}

func idArg(args []string, i int, usage string) int64 {
	if len(args) <= i {
		fatalf("%s\n", usage)
	}
	v, err := strconv.ParseInt(args[i], 10, 64)
	if err != nil {
		fatalf("Invalid ID %q: %v\n", args[i], err)
	}
	return v
}

func printUsage() {
	fmt.Println(`Cycling Race Simulator

Usage:
  cycling-sim <command> [arguments]

Commands:
  seed                           Load seed data (WorldTour riders + classic courses)
  clear                          Delete the database file (run seed after)
  riders                         List all riders
  courses                        List all courses
  races                          List all races
  one-day-races                  List one-day races (road and time trial)
  stage-races                    List all stage races
  run <race-id> [--incidents]    Simulate a race
  run-stage-race <stage-race-id> Simulate a stage race
                                 (both accept --seed N for reproducible runs)
  validate [-v]                  Run synthetic + real-race validation (-v: per rider)
  import-riders <dir>            Read riders from a local WorldDB mod (real ages, missing
                                 WorldTeam/ProTeam riders) into data/local/
  validate --sets N [--cross-season]  2024 references over N seed sets (judge changes);
  validate --sets N --season 2026     2026 references (data/reference_2026.csv, start lists);
  validate --quick                    2026 references, one seed set, a third of the runs (iterating);
                                 --cross-season adds the 2023 races
  import-results [dir]           Derive ratings from saved result pages in
                                 dir/<sprint|hilly|mountain|cobbles|tt>/
                                 (default data/results); writes
                                 data/rider_derived.csv. Re-seed to apply.
  import-stages <dir>            Import real route profiles for the courses in
                                 data/stage_sources.csv from a local stage-file
                                 dir; writes data/local/ and updates the DB
  apply-ratings                  Re-apply riders.csv + rating layers to the
                                 riders in the DB, keeping saved results
  fit-ratings [--classics] [file] Fit rider thresholds to the 2024 Tour GC
  fit-ratings --season=2026 [file]  Fit to every 2026 reference (ridge prior, ±10%)
                                 (--classics: also the one-day races) by
                                 simulation; writes file (default
                                 data/rider_fitted.csv) and updates the DB
  results <race-id>              Show race results
  gc <stage-race-id> <stage>     Show GC standings after a stage

Environment:
  CYCLING_SIM_DB                 Database file path (default: the app-data folder, e.g. ~/.config/CyclingSim/cycling_sim.db)`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func cmdSeed(db *database.DB) {
	fmt.Println("Seeding database with WorldTour riders and courses...")
	res, err := seed.Seed(db)
	if err != nil {
		fatalf("Error seeding: %v\n", err)
	}
	if res.Riders == 0 && res.Courses == 0 && res.Races == 0 && res.StageRaces == 0 {
		fmt.Println("Database already contains data. Skipping seed.")
		return
	}
	fmt.Printf("  Created %d riders\n  Created %d courses\n  Created %d races\n  Created %d stage races\n",
		res.Riders, res.Courses, res.Races, res.StageRaces)
	if res.Warnings > 0 {
		fmt.Printf("  (%d warnings)\n", res.Warnings)
	}
	fmt.Println("Seed complete!")
}

func cmdClear(db *database.DB, dbPath string) {
	fmt.Println("Deleting database file...")
	db.Close()
	if err := seed.ClearDBFile(dbPath); err != nil {
		fatalf("Error deleting database: %v\n", err)
	}
	fmt.Printf("Database file deleted: %s\n", dbPath)
	fmt.Println("You can now run 'cycling-sim seed' to create a fresh database.")
}

func cmdListRiders(db *database.DB) {
	riders, err := db.ListRiders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%-4s %-25s %-30s %-5s %5s %4s %4s %4s\n",
		"ID", "Name", "Team", "Nat", "Wt", "FTP", "Clmb", "Sprt")
	fmt.Println(strings.Repeat("-", 105))
	for _, r := range riders {
		fmt.Printf("%-4d %-25s %-30s %-5s %5.0f %4d %4d %4d\n",
			r.ID, r.Name, r.Team, r.Nationality, r.WeightKg, r.FTPWatts, r.ClimbingAbility, r.SprintAbility)
	}
}

func cmdListCourses(db *database.DB) {
	courses, err := db.ListCourses()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%-4s %-35s %8s\n", "ID", "Name", "Distance")
	fmt.Println(strings.Repeat("-", 50))
	for _, c := range courses {
		fmt.Printf("%-4d %-35s %6.1f km\n", c.ID, c.Name, c.TotalDistanceKm)
	}
}

func cmdListRaces(db *database.DB) {
	races, err := db.ListRaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%-4s %-30s %-6s %4s\n", "ID", "Name", "Type", "Course")
	fmt.Println(strings.Repeat("-", 50))
	for _, r := range races {
		fmt.Printf("%-4d %-30s %-6s %4d\n", r.ID, r.Name, r.RaceType, r.CourseID)
	}
}

func cmdListOneDayRaces(db *database.DB) {
	races, err := service.OneDayRaces(db)
	if err != nil {
		fatalf("Error: %v\n", err)
	}
	fmt.Printf("%-4s %-30s %-6s %4s\n", "ID", "Name", "Type", "Course")
	fmt.Println(strings.Repeat("-", 50))
	for _, r := range races {
		fmt.Printf("%-4d %-30s %-6s %4d\n", r.ID, r.Name, r.RaceType, r.CourseID)
	}
	fmt.Printf("\nTotal: %d one-day races\n", len(races))
}

func cmdListStageRaces(db *database.DB) {
	srs, err := db.ListStageRaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%-4s %-35s %8s\n", "ID", "Name", "Stages")
	fmt.Println(strings.Repeat("-", 50))
	for _, sr := range srs {
		fmt.Printf("%-4d %-35s %8d\n", sr.ID, sr.Name, len(sr.Stages))
	}
}

func cmdRunRace(db *database.DB, raceID int64, incidentsEnabled bool, seedVal int64) {
	cfg, err := service.LoadRaceConfig(db, raceID, incidentsEnabled, nil)
	if err != nil {
		fatalf("Error: %v\n", err)
	}
	if seedVal == 0 {
		seedVal = time.Now().UnixNano()
	}
	cfg.Seed = seedVal
	fmt.Printf("Simulating: %s (%s, %.1f km)\n", cfg.Race.Name, cfg.Course.Name, cfg.Course.TotalDistanceKm)
	fmt.Printf("Riders: %d | Type: %s | Seed: %d\n", len(cfg.Riders), cfg.Race.RaceType, seedVal)
	if incidentsEnabled {
		fmt.Printf("Incidents: ENABLED (crashes/punctures can occur)\n")
	}
	fmt.Println()

	results := service.RunRace(cfg, nil)
	if err := db.SaveRaceResults(results); err != nil {
		fatalf("Error saving results: %v\n", err)
	}
	printResults(results, service.RiderNames(cfg.Riders))
}

func cmdRunStageRace(db *database.DB, srID int64, seedVal int64) {
	plan, err := service.PrepareStageRace(db, srID, true)
	if err != nil {
		fatalf("Error: %v\n", err)
	}
	if seedVal == 0 {
		seedVal = time.Now().UnixNano()
	}
	fmt.Printf("Simulating stage race: %s (%d stages, %d riders, seed %d)\n\n",
		plan.StageRace.Name, len(plan.Stages), len(plan.Riders), seedVal)

	out := service.RunStageRace(plan, seedVal, nil, nil)
	if err := service.SaveStageRaceOutput(db, out); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: saving results: %v\n", err)
	}

	names := service.RiderNames(plan.Riders)
	for i, results := range out.Results {
		fmt.Printf("=== Stage %d: %s ===\n", plan.Stages[i].StageNumber, plan.Stages[i].Race.Name)
		printResults(results, names)
		fmt.Println()
	}
	fmt.Println("=== Final General Classification ===")
	printGC(out.GC[len(out.GC)-1], names)
}

func cmdShowResults(db *database.DB, raceID int64) {
	race, err := db.GetRace(raceID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	results, err := db.GetRaceResults(raceID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(results) == 0 {
		fmt.Println("No results found. Run the race first.")
		return
	}

	riders, _ := db.ListRiders()
	riderMap := make(map[int64]string)
	for _, r := range riders {
		riderMap[r.ID] = r.Name
	}

	fmt.Printf("Results: %s\n\n", race.Name)
	printResults(results, riderMap)
}

func cmdShowGC(db *database.DB, srID int64, afterStage int) {
	standings, err := db.GetGCStandings(srID, afterStage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(standings) == 0 {
		fmt.Println("No GC standings found.")
		return
	}

	riders, _ := db.ListRiders()
	riderMap := make(map[int64]string)
	for _, r := range riders {
		riderMap[r.ID] = r.Name
	}

	fmt.Printf("GC Standings after Stage %d\n\n", afterStage)
	printGC(standings, riderMap)
}

func printResults(results []model.RaceResult, riderMap map[int64]string) {
	fmt.Printf("%-4s %-25s %12s %10s\n", "Pos", "Rider", "Time", "Gap")
	fmt.Println(strings.Repeat("-", 55))
	for _, r := range results {
		name := riderMap[r.RiderID]
		if name == "" {
			name = fmt.Sprintf("Rider %d", r.RiderID)
		}
		if r.Position == 0 {
			fmt.Printf("%-4s %-25s %12s %10s\n", "DNF", name, "-", "-")
			continue
		}
		timeStr := formatDuration(r.FinishTimeSeconds)
		gapStr := ""
		if r.Position == 1 {
			gapStr = "-"
		} else {
			gapStr = "+" + formatDuration(r.TimeGapSeconds)
		}
		fmt.Printf("%-4d %-25s %12s %10s\n", r.Position, name, timeStr, gapStr)
	}
}

func printGC(standings []model.GCStanding, riderMap map[int64]string) {
	fmt.Printf("%-4s %-25s %12s %10s\n", "Pos", "Rider", "Total Time", "Gap")
	fmt.Println(strings.Repeat("-", 55))
	leaderTime := standings[0].TotalTimeSeconds
	for _, s := range standings {
		name := riderMap[s.RiderID]
		if name == "" {
			name = fmt.Sprintf("Rider %d", s.RiderID)
		}
		timeStr := formatDuration(s.TotalTimeSeconds)
		gapStr := ""
		if s.GCPosition == 1 {
			gapStr = "-"
		} else {
			gapStr = "+" + formatDuration(s.TotalTimeSeconds-leaderTime)
		}
		fmt.Printf("%-4d %-25s %12s %10s\n", s.GCPosition, name, timeStr, gapStr)
	}
}

func formatDuration(seconds float64) string {
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := int(seconds) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02d'%02d\"", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%d'%02d\"", m, s)
	}
	return fmt.Sprintf("%d\"", s)
}

// clock formats a GC gap as m'ss" (or h:mm:ss over an hour).
func clock(secs float64) string {
	t := int(math.Round(secs))
	if t >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", t/3600, t%3600/60, t%60)
	}
	return fmt.Sprintf("%d'%02d\"", t/60, t%60)
}

// cmdValidateSets scores the reference races over n independent seed sets:
// a change is real when it moves the mean by more than the spread.
func cmdValidateSets(db *database.DB, n int, crossSeason, season2026 bool) {
	fmt.Printf("Reference races over %d seed sets (gap error: mean, min-max; real winner's win rate)\n", n)
	var total float64
	var results []simulation.SeedSetResult
	shapeRefs, shapeField := simulation.KnownRealShapes(), simulation.StageRaceField
	var setShapes [][]simulation.ShapeResult // 2026: shapes from the GC runs
	if season2026 {
		riders, _ := db.ListRiders()
		oneDay, stage, err := simulation.Load2026References(seed.DataPath("reference_2026.csv"), seed.DataPath("startlists_2026.csv"), riders)
		if err != nil {
			fatalf("Error reading the 2026 references: %v\n", err)
		}
		results, setShapes = simulation.RunSeedSetsFor(db, n, oneDay, stage, simulation.KnownRealShapes2026())
		shapeRefs, shapeField = simulation.KnownRealShapes2026(), simulation.FieldOf(stage)
	} else {
		results = simulation.RunSeedSets(db, n, crossSeason)
	}
	for _, r := range results {
		mean, lo, hi := 0.0, math.Inf(1), math.Inf(-1)
		for _, v := range r.MAE {
			mean += v / float64(len(r.MAE))
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
		total += mean
		med := ""
		if len(r.MedianMAE) > 0 && r.MedianMAE[0] > 0 {
			m := 0.0
			for _, v := range r.MedianMAE {
				m += v / float64(len(r.MedianMAE))
			}
			med = fmt.Sprintf("  median-gap %4.0fs", m)
		}
		fmt.Printf("  %-30s %5.0fs  (%4.0f-%4.0f)  winner %3.0f%%%s\n", r.Name, mean, lo, hi, r.RealWinnerPc, med)
	}
	fmt.Printf("  %-30s %5.0fs\n", "Sum of mean gap errors", total)
	fmt.Println("GC spread (median gap of 2nd and 10th place, sim vs real; who holds the place doesn't matter)")
	for _, r := range results {
		if len(r.Spread) == 0 {
			continue
		}
		var s2, s10 float64
		for _, s := range r.Spread {
			s2 += s.Sim2 / float64(len(r.Spread))
			s10 += s.Sim10 / float64(len(r.Spread))
		}
		fmt.Printf("  %-30s 2nd %8s (real %8s)   10th %8s (real %8s)\n", r.Name, clock(s2), clock(r.Spread[0].Real2), clock(s10), clock(r.Spread[0].Real10))
	}
	fmt.Println("One-day finishes (share of runs solo / small group / bunch; median winning margin, sim vs real)")
	var nSolo, realSolo, nRaces float64
	for _, r := range results {
		if len(r.Finish) == 0 {
			continue
		}
		var f simulation.FinishShape
		for _, x := range r.Finish {
			k := float64(len(r.Finish))
			f.Solo += x.Solo / k
			f.Small += x.Small / k
			f.Bunch += x.Bunch / k
			f.Margin += x.Margin / k
		}
		real := r.Finish[0]
		fmt.Printf("  %-26s solo %3.0f%%  small %3.0f%%  bunch %3.0f%%  margin %6s (real %-11s %s)\n", r.Name,
			f.Solo*100, f.Small*100, f.Bunch*100, clock(f.Margin), real.RealKind+",", clock(real.RealMargin))
		nSolo += f.Solo
		nRaces++
		if real.RealKind == "solo" {
			realSolo++
		}
	}
	if nRaces > 0 {
		fmt.Printf("  %-26s solo %3.0f%% of races (real %3.0f%%)\n", "All one-day races", nSolo/nRaces*100, realSolo/nRaces*100)
	}
	printCalibration(results)
	printHistory(results)
	fmt.Println("Race shape: how stages are won, averaged per race kind (judge on these); per race as detail")
	var detail []string
	for set := 0; set < n; set++ {
		simulation.ValidationSeedBase = int64(set) * 1000
		var shapes []simulation.ShapeResult
		if set < len(setShapes) {
			shapes = setShapes[set]
		} else {
			shapes = simulation.RunShapeValidationFor(db, shapeRefs, shapeField)
		}
		for _, a := range simulation.ShapeAverages(shapes) {
			fmt.Printf("  set %d %-16s (%d) %s\n", set+1, a.Kind, a.Races, a.Summary())
		}
		for _, r := range shapes {
			detail = append(detail, fmt.Sprintf("  set %d %-36s %s", set+1, r.Name, r.Summary()))
		}
	}
	for _, d := range detail {
		fmt.Println(d)
	}
	simulation.ValidationSeedBase = 0
}

func cmdValidate(db *database.DB, verbose bool) {
	names := map[int64]string{}
	if verbose {
		if all, err := db.ListRiders(); err == nil {
			names = service.RiderNames(all)
		}
	}
	detail := func(label string, c simulation.RiderCheck) {
		real := "  -"
		if c.RealPos > 0 {
			real = fmt.Sprintf("%3d", c.RealPos)
		}
		realGap := "     ?"
		if c.RealGap >= 0 {
			realGap = fmt.Sprintf("%6.0f", c.RealGap)
		}
		fmt.Printf("    %-9s %-26s real %s %ss | sim %5.1f %6.0fs | top10 %3.0f%% win %3.0f%%\n",
			label, names[c.RiderID], real, realGap, c.SimPos, c.SimGap, c.Top10Pct, c.WinPct)
	}
	print := func(title string, rows []simulation.ValidationSummary) int {
		fmt.Printf("== %s ==\n", title)
		passed := 0
		for _, r := range rows {
			mark := "FAIL"
			if r.Passed {
				mark = "PASS"
				passed++
			}
			fmt.Printf("[%s] %-28s %s\n", mark, r.Name, r.Details)
			if verbose {
				for _, c := range r.Riders {
					detail("", c)
				}
				for _, c := range r.Intruders {
					detail("intruder", c)
				}
			}
		}
		fmt.Println()
		return passed
	}
	synth := simulation.RunValidationSuite()
	real := append(simulation.RunRealRaceValidation(db), simulation.RunRealStageRaceValidation(db)...)
	if real == nil {
		fmt.Println("(no riders in DB: run 'cycling-sim seed' for real-race validation)")
	}
	holdout := simulation.RunHoldoutValidation(db)
	speeds := simulation.RunSpeedValidation(db)
	p1 := print("Synthetic fields", synth)
	p2 := print("2024 races (the ratings' season; fit-ratings fits the Tour and Giro GC)", real)
	p3 := print("Cross-season (2023, informational: form changes between seasons)", holdout)
	p4 := print("Race speeds", speeds)
	fmt.Println("== Race shape (how stages were won) ==")
	shapes := simulation.RunShapeValidation(db)
	for _, a := range simulation.ShapeAverages(shapes) {
		fmt.Printf("%-34s (%d) %s\n", a.Kind, a.Races, a.Summary())
	}
	for _, r := range shapes {
		fmt.Printf("%-34s %s\n", r.Name, r.Summary())
		if verbose {
			for i, per := range r.PerStage {
				fmt.Printf("    stage %2d real %-9s sim sprint %3.0f%% break %3.0f%% selection %3.0f%% tt %3.0f%%\n", i+1, r.Stages[i],
					per[simulation.OutcomeSprint]*100, per[simulation.OutcomeBreakaway]*100, per[simulation.OutcomeSelection]*100, per[simulation.OutcomeTT]*100)
			}
		}
	}
	fmt.Println()
	fmt.Printf("Passed %d/%d synthetic, %d/%d 2024, %d/%d cross-season, %d/%d speeds\n",
		p1, len(synth), p2, len(real), p3, len(holdout), p4, len(speeds))
}

func cmdImportStages(db *database.DB, dir string) {
	sources, err := stagefile.LoadSources(seed.DataPath("stage_sources.csv"))
	if err != nil {
		fatalf("Error reading stage_sources.csv: %v\n", err)
	}
	courses, err := db.ListCourses()
	if err != nil || len(courses) == 0 {
		fatalf("No courses in the database: run 'cycling-sim seed' first.\n")
	}
	dist := map[string]float64{}
	ids := map[string]int64{}
	for _, c := range courses {
		dist[c.Name] = c.TotalDistanceKm
		ids[c.Name] = c.ID
	}
	profiles, problems := stagefile.Build(dir, sources, dist)
	for _, p := range problems {
		fmt.Printf("  skipped %s\n", p)
	}
	if len(profiles) == 0 {
		fatalf("No stage files could be imported from %s.\n", dir)
	}
	out := seed.DataPath(seed.LocalProfilesFile)
	if err := stagefile.Save(out, profiles); err != nil {
		fatalf("Error writing %s: %v\n", out, err)
	}
	for _, p := range profiles {
		segs := seed.CourseSegments(p.Course, dist[p.Course], p.ModelSegments())
		if err := db.ReplaceSegments(ids[p.Course], segs); err != nil {
			fatalf("Error updating %s: %v\n", p.Course, err)
		}
		cob := 0.0
		for _, s := range segs {
			if s.SurfaceType.Unpaved() {
				cob += s.DistanceKm
			}
		}
		fmt.Printf("  %-24s <- %-32s %3d segments, %5.0f m climbing, %4.1f km cobbles/gravel\n",
			p.Course, p.Stage, len(segs), stagefile.ClimbingM(segs), cob)
	}
	fmt.Printf("Wrote %s (local only, not committed) and updated %d courses in the database.\n", out, len(profiles))
}

func cmdFitRatings(db *database.DB, path string, classics, season2026 bool) {
	riders, err := db.ListRiders()
	if err != nil || len(riders) == 0 {
		fatalf("No riders in the database: run 'cycling-sim seed' first.\n")
	}
	// Start from the ratings without any fitted correction (never from the
	// database, which already carries the last fit, so fits don't compound).
	unfitted, err := seed.UnfittedFTPs()
	if err != nil {
		fatalf("Error building base ratings: %v\n", err)
	}
	base := make([]model.Rider, len(riders))
	copy(base, riders)
	for i := range base {
		if ftp, ok := unfitted[seed.NormalizeName(base[i].Name)]; ok {
			base[i].FTPWatts = ftp
		}
	}
	opt := calibrate.DefaultOptions()
	opt.Classics = classics
	opt.Log = func(format string, args ...any) { fmt.Printf(format, args...) }
	what := "the 2024 Tour GC"
	if classics {
		what += " and 2024 classics"
	}
	if season2026 {
		// Every 2026 reference, one-day and GC, with a ridge prior (the base
		// counts as one race) and wider bounds; races on an approximate course
		// (no 2026 route seeded) count half.
		oneDay, stage, err := simulation.Load2026References(seed.DataPath("reference_2026.csv"), seed.DataPath("startlists_2026.csv"), riders)
		if err != nil {
			fatalf("Error reading the 2026 references: %v\n", err)
		}
		opt.OneDay, opt.Stage = oneDay, stage
		opt.Weight = map[string]float64{}
		for _, e := range append(oneDay, stage...) {
			if !strings.HasSuffix(e.InternalName, " 2026") {
				opt.Weight[e.DisplayName] = 0.5
			}
		}
		opt.Prior, opt.MaxChange = 1, 0.10
		opt.Iterations, opt.Runs, opt.StageRuns = 20, 30, 16
		// Start from the installed fit (the prior still pulls towards the
		// unfitted base) and stop when the average settles: a routine refit
		// starts near its answer (Nov 2026: ~10-15 min instead of 30-40).
		if fitted, err := seed.LoadOverrides(seed.DataPath(seed.FittedFile)); err == nil && len(fitted) > 0 {
			opt.Start = map[int64]float64{}
			for _, r := range base {
				if o, ok := fitted[seed.NormalizeName(r.Name)]; ok {
					if v, err := strconv.ParseFloat(o.Fields["FTP_scale"], 64); err == nil && v > 0 {
						opt.Start[r.ID] = v
					}
				}
			}
			opt.MinPasses = 6
			fmt.Printf("Starting from the installed fit (%d riders).\n", len(opt.Start))
		}
		what = "every 2026 reference (one-day and GC)"
	}
	fmt.Printf("Fitting thresholds to %s (this takes a minute or two)...\n", what)
	adj, err := calibrate.Fit(db, base, opt)
	if err != nil {
		fatalf("Fit failed: %v\n", err)
	}
	if err := calibrate.Write(path, adj); err != nil {
		fatalf("Error writing %s: %v\n", path, err)
	}
	baseFTP := map[int64]int{}
	for _, r := range base {
		baseFTP[r.ID] = r.FTPWatts
	}
	ftp := map[int64]int{}
	for _, r := range riders {
		ftp[r.ID] = baseFTP[r.ID] // undo old corrections...
	}
	implausible := 0
	for _, a := range adj {
		ftp[a.RiderID] = int(math.Round(float64(baseFTP[a.RiderID]) * a.Scale)) // ...apply new ones
		flag := ""
		if a.Implausible {
			flag = "  ← clearly out-climbs the strongest rider: likely compensating for a model gap"
			implausible++
		}
		spread := ""
		if a.Spread > 0 {
			spread = fmt.Sprintf(" ±%.1f", a.Spread*100)
		}
		fmt.Printf("  %-28s %+5.1f%%%s  (%d W, %.2f W/kg, %.2f on climbs)%s\n", a.Name, (a.Scale-1)*100, spread, ftp[a.RiderID], a.WKg, a.ClimbWKg, flag)
	}
	if implausible > 0 {
		fmt.Printf("Warning: %d rider(s) fitted to implausible W/kg; review before keeping %s.\n", implausible, path)
	}
	if err := db.SetRiderFTPs(ftp); err != nil {
		fatalf("Error updating riders: %v\n", err)
	}
	fmt.Printf("Wrote %s (%d riders) and updated the database.\n", path, len(adj))
	fmt.Println("Check generalisation with: cycling-sim validate  (see the 2023 hold-out section)")
}

func cmdImportResults(db *database.DB, dir string) {
	lib, err := importer.LoadDir(dir)
	if err != nil {
		fatalf("Error reading %s: %v\n", dir, err)
	}
	for f, why := range lib.Skipped {
		fmt.Printf("  skipped %s: %s\n", f, why)
	}
	if len(lib.Files) == 0 {
		fmt.Printf("No result files found. Save results pages (FirstCycling or ProCyclingStats,\n"+
			"browser \"Save page as\") or CSVs (position,rider[,gap]) into %s/<profile>/,\n"+
			"where <profile> is one of: %s\n", dir, strings.Join(importer.Profiles, ", "))
		return
	}
	for _, p := range importer.Profiles {
		if lib.Races[p] > 0 {
			fmt.Printf("  %-9s %d race files\n", p, lib.Races[p])
		}
	}
	riders, err := db.ListRiders()
	if err != nil || len(riders) == 0 {
		fatalf("No riders in the database: run 'cycling-sim seed' first\n")
	}

	known := map[string]bool{}
	for _, r := range riders {
		known[importer.NameKey(r.Name)] = true
	}
	unmatched := map[string]int{}
	for _, byName := range lib.Results {
		for key, positions := range byName {
			if !known[key] {
				unmatched[lib.Names[key]] += len(positions)
			}
		}
	}
	if len(unmatched) > 0 {
		type um struct {
			name string
			n    int
		}
		var list []um
		for n, c := range unmatched {
			list = append(list, um{n, c})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
		fmt.Printf("  %d rider names not in the database (most frequent):", len(list))
		for i := 0; i < len(list) && i < 8; i++ {
			fmt.Printf(" %s(%d)", list[i].name, list[i].n)
		}
		fmt.Println()
	}

	ratings := importer.Derive(lib, riders)
	out := seed.DataPath("rider_derived.csv")
	note := fmt.Sprintf("%d result files in %s", len(lib.Files), dir)
	if err := importer.WriteDerived(out, ratings, note); err != nil {
		fatalf("Error writing %s: %v\n", out, err)
	}
	fmt.Printf("Derived ratings for %d riders -> %s\n", len(ratings), out)
	fmt.Println("Apply with: cycling-sim clear && cycling-sim seed  (curated rider_overrides.csv still wins)")
}

// printCalibration reports whether the simulation's probabilities are
// honest: every starter's top-10 and win chances against what he did, over
// all reference races and seed sets (calibration.go).
// printHoldout runs the older references the 2026 fit never sees (the 2024
// classics and stage races, the 2023 hold-out) and prints their calibration:
// a warning light for overfitting the 2026 references, not a target (the
// ratings describe 2026 riders, so these fields are a season off).
func printHoldout(db *database.DB, n int) {
	fmt.Println()
	fmt.Println("Held out: 2024 and 2023 references, never fitted (ratings are 2026's; a warning light, not a target)")
	printCalibration(simulation.RunSeedSets(db, n, true))
}

func printCalibration(results []simulation.SeedSetResult) {
	var oneDay, stage []simulation.CalibPoint
	var races [][]simulation.CalibPoint
	for _, r := range results {
		for _, set := range r.Calib {
			if len(set) == 0 {
				continue
			}
			races = append(races, set)
			if strings.Contains(r.Name, "(GC)") {
				stage = append(stage, set...)
			} else {
				oneDay = append(oneDay, set...)
			}
		}
	}
	if len(races) == 0 {
		return
	}
	fmt.Println("Calibration: do the simulation's chances come true as often as it says? (every starter, all races)")
	for _, part := range []struct {
		name string
		pts  []simulation.CalibPoint
	}{{"One-day top 10", oneDay}, {"Stage-race GC top 10", stage}} {
		if len(part.pts) == 0 {
			continue
		}
		fmt.Printf("  %-22s chance band:", part.name)
		for _, b := range simulation.Reliability(part.pts, false) {
			if b.N == 0 {
				continue
			}
			fmt.Printf("  %2.0f-%3.0f%%: said %4.1f%%, happened %4.1f%% (n=%d)", b.Lo*100, b.Hi*100, b.Predicted*100, b.Actual*100, b.N)
		}
		fmt.Printf("  | Brier %.4f\n", simulation.Brier10(part.pts))
	}
	all := append(append([]simulation.CalibPoint{}, oneDay...), stage...)
	fmt.Printf("  %-22s chance band:", "Winner")
	for _, b := range simulation.Reliability(all, true) {
		if b.N == 0 {
			continue
		}
		fmt.Printf("  %2.0f-%3.0f%%: said %4.1f%%, happened %4.1f%% (n=%d)", b.Lo*100, b.Hi*100, b.Predicted*100, b.Actual*100, b.N)
	}
	fmt.Println()
	score, blind, n := simulation.WinnerLogScore(races)
	fmt.Printf("  Real winners got on average 2^%.2f = %.1f%% from the simulation (field-blind guess 2^%.2f = %.1f%%; %d races)\n",
		score, math.Pow(2, score)*100, blind, math.Pow(2, blind)*100, n)
}

// printHistory sets the simulated race character against each race's long
// run (data/history_results.csv): one-day finish types and margins, stage
// races' GC gaps. A value outside the history's range is flagged.
func printHistory(results []simulation.SeedSetResult) {
	hist, err := simulation.LoadHistory(seed.DataPath("history_results.csv"))
	if err != nil || len(hist) == 0 {
		return
	}
	clk := func(s float64) string { return clock(s) }
	flag := func(v, lo, hi float64) string {
		if v < lo || v > hi {
			return " ←"
		}
		return ""
	}
	fmt.Println("Against history (each race's last ~10 editions: median, range; ← outside the range)")
	fmt.Println("  Solo shares with 95% ranges (history over ~10 editions, the simulation over its runs);")
	fmt.Println("  ← only when the two ranges don't overlap: a clear difference, not noise.")
	var simSolo, realSolo, nRaces float64
	var simK, simN, realK, realN float64
	for _, r := range results {
		h := hist[simulation.HistoryName(r.Name)]
		if h == nil {
			continue
		}
		if len(r.Finish) > 0 {
			var solo, margin float64
			runs := 0
			for _, f := range r.Finish {
				solo += f.Solo / float64(len(r.Finish))
				margin += f.Margin / float64(len(r.Finish))
				runs += f.Runs
			}
			n := float64(len(h.Gap2))
			med, lo, hi := simulation.Range(h.Gap2)
			hLo, hHi := simulation.Wilson95(float64(h.Solo), n)
			sLo, sHi := simulation.Wilson95(solo*float64(runs), float64(runs))
			soloFlag := ""
			if runs > 0 && (sHi < hLo || sLo > hHi) {
				soloFlag = " ←"
			}
			fmt.Printf("  %-26s solo %3.0f%% (%2.0f-%3.0f%%; history %3.0f%%, %d of %d: %2.0f-%3.0f%%)%-2s  margin %6s (history %s, %s-%s)%s\n",
				simulation.HistoryName(r.Name), solo*100, sLo*100, sHi*100, float64(h.Solo)/n*100, h.Solo, len(h.Gap2), hLo*100, hHi*100, soloFlag, clk(margin), clk(med), clk(lo), clk(hi), flag(margin, lo, hi))
			simSolo += solo
			realSolo += float64(h.Solo) / n
			nRaces++
			simK += solo * float64(runs)
			simN += float64(runs)
			realK += float64(h.Solo)
			realN += n
		}
		if len(r.Spread) > 0 {
			var s2, s10 float64
			for _, sp := range r.Spread {
				s2 += sp.Sim2 / float64(len(r.Spread))
				s10 += sp.Sim10 / float64(len(r.Spread))
			}
			m2, lo2, hi2 := simulation.Range(h.Gap2)
			m10, lo10, hi10 := simulation.Range(h.Gap10)
			fmt.Printf("  %-26s GC 2nd %6s (history %s, %s-%s)%-2s  10th %6s (history %s, %s-%s)%s\n",
				simulation.HistoryName(r.Name), clk(s2), clk(m2), clk(lo2), clk(hi2), flag(s2, lo2, hi2), clk(s10), clk(m10), clk(lo10), clk(hi10), flag(s10, lo10, hi10))
		}
	}
	if nRaces > 0 {
		sLo, sHi := simulation.Wilson95(simK, simN)
		hLo, hHi := simulation.Wilson95(realK, realN)
		fmt.Printf("  %-26s solo %3.0f%% of races (%2.0f-%2.0f%%; history %3.0f%%, %.0f of %.0f editions: %2.0f-%2.0f%%)\n", "All one-day races", simSolo/nRaces*100, sLo*100, sHi*100, realSolo/nRaces*100, realK, realN, hLo*100, hHi*100)
	}
}
