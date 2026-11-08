package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"cycling_sim/internal/database"
	"cycling_sim/internal/importer"
	"cycling_sim/internal/model"
	"cycling_sim/internal/seed"
	"cycling_sim/internal/service"
	"cycling_sim/internal/simulation"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	dbPath := "cycling_sim.db"
	if envDB := os.Getenv("CYCLING_SIM_DB"); envDB != "" {
		dbPath = envDB
	}

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
		cmdValidate(db)
	case "import-results":
		dir := "data/results"
		if len(args) > 0 {
			dir = args[0]
		}
		cmdImportResults(db, dir)
	case "results":
		raceID := idArg(args, 0, "Usage: cycling-sim results <race-id>")
		cmdShowResults(db, raceID)
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
  validate                       Run synthetic + real-race (2024) validation
  import-results [dir]           Derive ratings from saved result pages in
                                 dir/<sprint|hilly|mountain|cobbles|tt>/
                                 (default data/results); writes
                                 data/rider_derived.csv. Re-seed to apply.
  results <race-id>              Show race results
  gc <stage-race-id> <stage>     Show GC standings after a stage

Environment:
  CYCLING_SIM_DB                 Database file path (default: cycling_sim.db)`)
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

func cmdValidate(db *database.DB) {
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
		}
		fmt.Println()
		return passed
	}
	synth := simulation.RunValidationSuite()
	real := simulation.RunRealRaceValidation(db)
	if real == nil {
		fmt.Println("(no riders in DB: run 'cycling-sim seed' for real-race validation)")
	}
	p1 := print("Synthetic fields", synth)
	p2 := print("Real races (2024)", real)
	fmt.Printf("Passed %d/%d synthetic, %d/%d real\n", p1, len(synth), p2, len(real))
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
