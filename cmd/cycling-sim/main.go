package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"cycling_sim/internal/database"
	"cycling_sim/internal/fcimport"
	"cycling_sim/internal/model"
	"cycling_sim/internal/selection"
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

	db, err := database.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	switch os.Args[1] {
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
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: cycling-sim run <race-id> [--incidents]")
			os.Exit(1)
		}
		raceID, err := strconv.ParseInt(os.Args[2], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid race ID: %v\n", err)
			os.Exit(1)
		}
		incidents := len(os.Args) > 3 && os.Args[3] == "--incidents"
		cmdRunRace(db, raceID, incidents)
	case "run-stage-race":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: cycling-sim run-stage-race <stage-race-id>")
			os.Exit(1)
		}
		srID, err := strconv.ParseInt(os.Args[2], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid stage race ID: %v\n", err)
			os.Exit(1)
		}
		cmdRunStageRace(db, srID)
	case "results":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: cycling-sim results <race-id>")
			os.Exit(1)
		}
		raceID, err := strconv.ParseInt(os.Args[2], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid race ID: %v\n", err)
			os.Exit(1)
		}
		cmdShowResults(db, raceID)
	case "gc":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: cycling-sim gc <stage-race-id> <after-stage>")
			os.Exit(1)
		}
		srID, err := strconv.ParseInt(os.Args[2], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid stage race ID: %v\n", err)
			os.Exit(1)
		}
		stage, err := strconv.Atoi(os.Args[3])
		if err != nil || stage < 1 {
			fmt.Fprintf(os.Stderr, "Invalid stage number: %v\n", err)
			os.Exit(1)
		}
		cmdShowGC(db, srID, stage)
	case "fc-fetch":
		cmdFCFetch(db, os.Args[2:])
	case "fc-list":
		cmdFCList(db)
	case "fc-match":
		cmdFCMatch(db)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
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
  run <race-id>                  Simulate a race
  run-stage-race <stage-race-id> Simulate a stage race
  results <race-id>              Show race results
  gc <stage-race-id> <stage>     Show GC standings after a stage

FirstCycling import:
  fc-fetch --race <id> --year <year> [--stage <n>|--stages <from>-<to>]
                                 Fetch reference results from firstcycling.com
  fc-list                        List all fetched FC reference data
  fc-match                       Auto-match FC rider names to internal riders

Environment:
  CYCLING_SIM_DB                 Database file path (default: cycling_sim.db)`)
}

func cmdSeed(db *database.DB) {
	// Check if already seeded
	riders, _ := db.ListRiders()
	if len(riders) > 0 {
		fmt.Println("Database already contains data. Skipping seed.")
		return
	}

	fmt.Println("Seeding database with WorldTour riders and courses...")

	// Load riders from CSV
	csvPath := findDataFile("riders.csv")
	riderCount, err := loadRidersFromCSV(db, csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading riders: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Created %d riders\n", riderCount)

	// Create all courses and track their IDs by name
	courseIDs := seedCourses(db)
	fmt.Printf("  Created %d courses\n", len(courseIDs))

	// Create all one-day races and stage race stages, track IDs
	raceIDs := seedRaces(db, courseIDs)
	fmt.Printf("  Created %d races\n", len(raceIDs))

	// Create stage races
	srCount := seedStageRaces(db, raceIDs)
	fmt.Printf("  Created %d stage races\n", srCount)

	fmt.Println("Seed complete!")
}

func cmdClear(db *database.DB, dbPath string) {
	fmt.Println("Deleting database file...")

	// Close the database connection first
	db.Close()

	// Delete the database file
	if err := os.Remove(dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting database: %v\n", err)
		os.Exit(1)
	}

	// Also delete WAL and SHM files if they exist (SQLite auxiliary files)
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")

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
	fmt.Println(repeat("-", 105))
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
	fmt.Println(repeat("-", 50))
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
	fmt.Println(repeat("-", 50))
	for _, r := range races {
		fmt.Printf("%-4d %-30s %-6s %4d\n", r.ID, r.Name, r.RaceType, r.CourseID)
	}
}

func cmdListOneDayRaces(db *database.DB) {
	races, err := db.ListRaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get all stage races and collect their race IDs
	stageRaces, err := db.ListStageRaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	stagRaceIDs := make(map[int64]bool)
	for _, sr := range stageRaces {
		fullSR, err := db.GetStageRace(sr.ID)
		if err != nil {
			continue
		}
		for _, stage := range fullSR.Stages {
			stagRaceIDs[stage.RaceID] = true
		}
	}

	fmt.Printf("%-4s %-30s %-6s %4s\n", "ID", "Name", "Type", "Course")
	fmt.Println(repeat("-", 50))
	count := 0
	for _, r := range races {
		// Exclude stages from stage races and races with RaceTypeStage
		if !stagRaceIDs[r.ID] && r.RaceType != model.RaceTypeStage {
			fmt.Printf("%-4d %-30s %-6s %4d\n", r.ID, r.Name, r.RaceType, r.CourseID)
			count++
		}
	}
	fmt.Printf("\nTotal: %d one-day races\n", count)
}

func cmdListStageRaces(db *database.DB) {
	srs, err := db.ListStageRaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%-4s %-35s %8s\n", "ID", "Name", "Stages")
	fmt.Println(repeat("-", 50))
	for _, sr := range srs {
		fmt.Printf("%-4d %-35s %8d\n", sr.ID, sr.Name, len(sr.Stages))
	}
}

func cmdRunRace(db *database.DB, raceID int64, incidentsEnabled bool) {
	race, err := db.GetRace(raceID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting race: %v\n", err)
		os.Exit(1)
	}

	course, err := db.GetCourse(race.CourseID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting course: %v\n", err)
		os.Exit(1)
	}

	allRiders, err := db.ListRiders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting riders: %v\n", err)
		os.Exit(1)
	}

	selConfig := selection.DefaultConfig(race.RaceType, false, 0)
	riders := selection.SelectStartList(allRiders, course, race, selConfig)
	fmt.Printf("Simulating: %s (%s, %.1f km)\n", race.Name, course.Name, course.TotalDistanceKm)
	fmt.Printf("Riders: %d (%d teams × %d) | Type: %s\n", len(riders), selConfig.NumTeams, selConfig.RidersPerTeam, race.RaceType)
	if incidentsEnabled {
		fmt.Printf("Incidents: ENABLED (crashes/punctures can occur)\n")
	}
	fmt.Println()

	config := simulation.RaceConfig{
		Race:             race,
		Course:           course,
		Riders:           riders,
		Seed:             time.Now().UnixNano(),
		IncidentsEnabled: incidentsEnabled,
	}

	var results []model.RaceResult
	if race.RaceType == model.RaceTypeTT {
		results = simulation.SimulateTT(config)
	} else {
		results = simulation.SimulateRoadRace(config)
	}

	if err := db.SaveRaceResults(results); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving results: %v\n", err)
		os.Exit(1)
	}

	// Build rider name lookup
	riderMap := make(map[int64]string)
	for _, r := range riders {
		riderMap[r.ID] = r.Name
	}

	printResults(results, riderMap)
}

func cmdRunStageRace(db *database.DB, srID int64) {
	sr, err := db.GetStageRace(srID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting stage race: %v\n", err)
		os.Exit(1)
	}
	if err := db.ClearGCStandings(srID); err != nil {
		fmt.Fprintf(os.Stderr, "Error clearing GC standings: %v\n", err)
		os.Exit(1)
	}
	_ = db.ClearPointsStandings(srID)
	_ = db.ClearMountainsStandings(srID)
	_ = db.ClearYouthStandings(srID)

	allRiders, err := db.ListRiders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting riders: %v\n", err)
		os.Exit(1)
	}

	var stages []simulation.StageConfig
	var stageInfos []selection.StageInfo
	for _, s := range sr.Stages {
		race, err := db.GetRace(s.RaceID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting stage race: %v\n", err)
			os.Exit(1)
		}
		if err := db.ClearRaceResults(race.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Error clearing stage results: %v\n", err)
			os.Exit(1)
		}
		course, err := db.GetCourse(race.CourseID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting course: %v\n", err)
			os.Exit(1)
		}
		stages = append(stages, simulation.StageConfig{
			Race: race, Course: course,
			StageNumber: s.StageNumber, RestDayBefore: s.RestDayBefore,
		})
		stageInfos = append(stageInfos, selection.StageInfo{Course: course, RaceType: race.RaceType})
	}

	selConfig := selection.DefaultConfig(model.RaceTypeRoad, true, len(stages))
	riders := selection.SelectStageRaceStartList(allRiders, stageInfos, selConfig)
	fmt.Printf("Simulating stage race: %s (%d stages, %d riders)\n\n", sr.Name, len(sr.Stages), len(riders))

	allResults, allStandings := simulation.SimulateStageRace(sr, stages, riders, time.Now().UnixNano())
	pointsByStage, komByStage, youthByStage := simulation.ComputeStageRaceCompetitions(sr.ID, stages, riders, allResults)

	riderMap := make(map[int64]string)
	for _, r := range riders {
		riderMap[r.ID] = r.Name
	}

	for i, results := range allResults {
		fmt.Printf("=== Stage %d: %s ===\n", stages[i].StageNumber, stages[i].Race.Name)
		if err := db.SaveRaceResults(results); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving stage results: %v\n", err)
		}
		printResults(results, riderMap)
		fmt.Println()
	}

	// Save and display final GC
	for _, standings := range allStandings {
		if err := db.SaveGCStandings(standings); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving GC standings: %v\n", err)
		}
	}
	for _, ps := range pointsByStage {
		_ = db.SavePointsStandings(ps)
	}
	for _, ks := range komByStage {
		_ = db.SaveMountainsStandings(ks)
	}
	for _, ys := range youthByStage {
		_ = db.SaveYouthStandings(ys)
	}

	fmt.Println("=== Final General Classification ===")
	finalGC := allStandings[len(allStandings)-1]
	printGC(finalGC, riderMap)
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
	fmt.Println(repeat("-", 55))
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
	fmt.Println(repeat("-", 55))
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

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// courseData is a helper for defining seed courses.
type courseData struct {
	course   model.Course
	segments []model.Segment
}

func cd(name string, distKm float64, segments []model.Segment) courseData {
	return courseData{
		course:   model.Course{Name: name, TotalDistanceKm: distKm},
		segments: segments,
	}
}

func seg(order int, name string, distKm, gradPct float64, surface model.SurfaceType, technical bool) model.Segment {
	return model.Segment{
		SegmentOrder: order, Name: name, DistanceKm: distKm,
		GradientPct: gradPct, SurfaceType: surface, IsTechnical: technical,
	}
}

func road(order int, name string, distKm, gradPct float64) model.Segment {
	return seg(order, name, distKm, gradPct, model.SurfaceRoad, false)
}

func cobbles(order int, name string, distKm, gradPct float64) model.Segment {
	return seg(order, name, distKm, gradPct, model.SurfaceCobbles, true)
}

func seedCourses(db *database.DB) map[string]int64 {
	courses := []courseData{
		// ===== 5 Monuments =====
		// 1: Milan-San Remo
		cd("Milan-San Remo", 290, []model.Segment{
			road(1, "Flat approach", 230, 0),
			road(2, "Cipressa", 5.6, 4.1),
			road(3, "Cipressa descent", 6, -4.0),
			road(4, "Poggio", 3.7, 3.7),
			road(5, "Poggio descent", 4, -5.0),
			road(6, "Via Roma finish", 1, 0),
		}),
		// 2: Tour of Flanders
		cd("Tour of Flanders", 270, []model.Segment{
			road(1, "Flat approach", 140, 0),
			cobbles(2, "Oude Kwaremont", 2.2, 4.0),
			road(3, "Road section", 20, 0.3),
			cobbles(4, "Koppenberg", 0.6, 11.6),
			road(5, "Road section", 30, 0.3),
			cobbles(6, "Paterberg", 0.4, 12.9),
			road(7, "Road section", 15, 0.3),
			cobbles(8, "Oude Kwaremont (2nd)", 2.2, 4.0),
			road(9, "Road section", 10, 0.3),
			cobbles(10, "Paterberg (2nd)", 0.4, 12.9),
			road(11, "Run-in to Oudenaarde", 18, 0),
		}),
		// 3: Paris-Roubaix (actual coverage ~28km cobbles out of 257km total = 11%)
		// Key: Major cobbles in final 10-15km to ensure handling impacts the finish
		cd("Paris-Roubaix", 257, []model.Segment{
			road(1, "Road to Cancanson", 94, 0.2),
			cobbles(2, "Cancanson-Vieux-Reng", 1.8, 0.8),
			road(3, "Road section", 18, 0.1),
			cobbles(4, "Saint-Python-Viesly", 2.1, 1.2),
			road(5, "Road section", 25, 0.3),
			cobbles(6, "Trouee d'Arenberg", 2.3, 0),
			road(7, "Road section", 30, 0.2),
			cobbles(8, "Mons-en-Pevele", 3.0, 1.5),
			road(9, "Road section", 20, 0.1),
			cobbles(10, "Bouvines", 2.1, 0.3),
			road(11, "Road section", 18, 0.2),
			cobbles(12, "Saint-Leger-les-Dooms", 2.3, 0.6),
			road(13, "Road section", 35, 0.1),
			cobbles(14, "Modules sector", 3.2, 0.8),
			road(15, "Road section", 12, 0.2),
			cobbles(16, "Petite Forets", 3.4, 1.1),
			road(17, "Road to Carrefour", 7, 0.1),
			cobbles(18, "Carrefour de l'Arbre", 2.1, 0),
			road(19, "Road section", 5, 0.1),
			cobbles(20, "Gruson-Roubaix", 1.8, 0.5),
			road(21, "Velodrome finish", 0.5, 0),
		}),
		// 4: Liege-Bastogne-Liege
		cd("Liege-Bastogne-Liege", 258, []model.Segment{
			road(1, "Liege to Bastogne", 100, 0.3),
			road(2, "Bastogne return", 60, 0.5),
			road(3, "Cote de la Redoute", 2.0, 8.8),
			road(4, "Valley road", 20, -1.0),
			road(5, "Roche aux Faucons", 1.5, 11.0),
			road(6, "Road section", 15, -0.5),
			road(7, "Cote de Saint-Nicolas", 1.2, 8.6),
			road(8, "Descent to Liege", 3, -4.0),
			road(9, "Liege finish", 2, 0),
		}),
		// 5: Il Lombardia
		cd("Il Lombardia", 243, []model.Segment{
			road(1, "Lake Como approach", 100, 0.3),
			road(2, "Civiglio", 4.2, 9.7),
			road(3, "Civiglio descent", 5, -6.0),
			road(4, "Valley road", 40, 0.3),
			road(5, "San Fermo della Battaglia", 2.7, 7.2),
			road(6, "Descent and flat", 30, -1.0),
			road(7, "Run-in to Como", 10, 0),
		}),

		// ===== One-Day Classics =====
		// 6: Strade Bianche
		cd("Strade Bianche", 184, []model.Segment{
			road(1, "Road section", 50, 0.3),
			seg(2, "Sterrato sector 1", 3, 2.0, model.SurfaceCobbles, true),
			road(3, "Road section", 30, 0.5),
			seg(4, "Sterrato sector 2", 4, 3.0, model.SurfaceCobbles, true),
			road(5, "Road section", 25, 0.5),
			seg(6, "Sterrato sector 3", 5, 2.5, model.SurfaceCobbles, true),
			road(7, "Road to Siena", 20, 1.0),
			road(8, "Piazza del Campo climb", 3, 7.0),
		}),
		// 7: E3 Saxo Classic
		cd("E3 Saxo Classic", 204, []model.Segment{
			road(1, "Flat approach", 100, 0),
			cobbles(2, "Paterberg", 0.4, 12.9),
			road(3, "Road section", 25, 0.3),
			cobbles(4, "Oude Kwaremont", 2.2, 4.0),
			road(5, "Road section", 20, 0.3),
			cobbles(6, "Tiegemberg", 0.8, 6.8),
			road(7, "Road to finish", 30, 0),
		}),
		// 8: Gent-Wevelgem
		cd("Gent-Wevelgem", 261, []model.Segment{
			road(1, "Flat approach", 140, 0),
			road(2, "Kemmelberg", 0.8, 9.3),
			road(3, "Descent", 2, -5.0),
			road(4, "Road section", 30, 0),
			road(5, "Kemmelberg (2nd)", 0.8, 9.3),
			road(6, "Descent", 2, -5.0),
			road(7, "Flat run-in", 50, 0),
			road(8, "Sprint finish", 5, 0),
		}),
		// 9: Amstel Gold Race
		cd("Amstel Gold Race", 254, []model.Segment{
			road(1, "Rolling approach", 80, 0.5),
			road(2, "Cauberg (1st)", 1.2, 5.8),
			road(3, "Road section", 40, 0.5),
			road(4, "Keutenberg", 0.7, 9.4),
			road(5, "Road section", 40, 0.5),
			road(6, "Gulperberg", 0.8, 9.3),
			road(7, "Road section", 30, 0.5),
			road(8, "Kruisberg", 1.0, 8.0),
			road(9, "Road to Cauberg", 20, 0.5),
			road(10, "Cauberg finish", 1.2, 5.8),
		}),
		// 10: La Fleche Wallonne
		cd("La Fleche Wallonne", 202, []model.Segment{
			road(1, "Road section", 100, 0.3),
			road(2, "Cote de Cherave", 1.3, 8.1),
			road(3, "Road section", 35, 0.3),
			road(4, "Mur de Huy (1st)", 1.3, 9.6),
			road(5, "Loop road", 40, 0.3),
			road(6, "Cote de Cherave (2nd)", 1.3, 8.1),
			road(7, "Road to Huy", 10, 0),
			road(8, "Mur de Huy finish", 1.3, 9.6),
		}),
		// 11: Clasica San Sebastian
		cd("Clasica San Sebastian", 228, []model.Segment{
			road(1, "Coastal road", 60, 0.3),
			road(2, "Jaizkibel", 8.1, 5.4),
			road(3, "Descent", 8, -5.0),
			road(4, "Road section", 50, 0.3),
			road(5, "Jaizkibel (2nd)", 8.1, 5.4),
			road(6, "Descent", 8, -5.0),
			road(7, "Road section", 30, 0.3),
			road(8, "Murgil Tontorra", 2.8, 8.4),
			road(9, "Descent and finish", 12, -2.0),
		}),

		// ===== Grand Tour Stage Templates =====
		// 12: Flat Sprint Stage
		cd("Flat Sprint Stage", 200, []model.Segment{
			road(1, "Flat road", 160, 0),
			road(2, "Gentle rolling", 30, 0.5),
			road(3, "Sprint finish", 10, 0),
		}),
		// 13: Flat Sprint Stage (short)
		cd("Flat Sprint Stage (short)", 170, []model.Segment{
			road(1, "Flat road", 130, 0),
			road(2, "Gentle rolling", 30, 0.3),
			road(3, "Sprint finish", 10, 0),
		}),
		// 14: Rolling/Hilly Stage
		cd("Rolling Stage", 185, []model.Segment{
			road(1, "Road section", 60, 0.3),
			road(2, "Cat 3 climb", 4, 5.0),
			road(3, "Descent", 5, -4.0),
			road(4, "Road section", 40, 0.5),
			road(5, "Cat 3 climb", 5, 4.5),
			road(6, "Descent", 5, -4.0),
			road(7, "Road section", 30, 0.3),
			road(8, "Cat 4 climb", 2.5, 4.0),
			road(9, "Descent and finish", 8, -1.0),
		}),
		// 15: Medium Mountain Stage
		cd("Medium Mountain Stage", 175, []model.Segment{
			road(1, "Road section", 50, 0.3),
			road(2, "Cat 2 climb", 8, 5.8),
			road(3, "Descent", 10, -5.0),
			road(4, "Valley road", 30, 0.3),
			road(5, "Cat 2 climb", 10, 5.5),
			road(6, "Descent", 12, -5.0),
			road(7, "Valley road", 20, 0.3),
			road(8, "Cat 3 climb", 4, 5.0),
			road(9, "Descent to finish", 6, -3.0),
		}),
		// 16: High Mountain Stage
		cd("High Mountain Stage", 165, []model.Segment{
			road(1, "Valley approach", 40, 0.5),
			road(2, "HC climb", 15, 7.0),
			road(3, "Descent", 18, -6.0),
			road(4, "Valley road", 20, 0.3),
			road(5, "Cat 1 climb", 12, 6.5),
			road(6, "Descent", 14, -5.5),
			road(7, "Valley road", 15, 0.3),
			road(8, "Cat 1 climb", 10, 6.8),
			road(9, "Descent to finish", 6, -3.0),
		}),
		// 17: Summit Finish Stage
		cd("Summit Finish Stage", 155, []model.Segment{
			road(1, "Valley approach", 40, 0.3),
			road(2, "Cat 1 climb", 12, 6.5),
			road(3, "Descent", 15, -6.0),
			road(4, "Valley road", 20, 0.3),
			road(5, "Cat 1 climb", 14, 6.8),
			road(6, "Descent", 16, -5.5),
			road(7, "Short valley", 8, 0.3),
			road(8, "HC summit finish", 14, 7.8),
		}),
		// 18: Alpine Summit Finish
		cd("Alpine Summit Finish", 170, []model.Segment{
			road(1, "Valley approach", 50, 0.3),
			road(2, "Col du Galibier", 18, 6.9),
			road(3, "Galibier descent", 20, -6.0),
			road(4, "Valley road", 20, 0.5),
			road(5, "Alpe d'Huez", 13.8, 8.1),
		}),
		// 19: Dolomites Summit Finish
		cd("Dolomites Summit Finish", 160, []model.Segment{
			road(1, "Valley approach", 35, 0.3),
			road(2, "Passo Fedaia", 14, 7.6),
			road(3, "Descent", 12, -6.5),
			road(4, "Valley road", 15, 0.3),
			road(5, "Passo Pordoi", 11.8, 6.8),
			road(6, "Descent", 10, -5.5),
			road(7, "Passo Giau", 10, 9.3),
		}),
		// 20: Pyrenees Summit Finish
		cd("Pyrenees Summit Finish", 150, []model.Segment{
			road(1, "Valley approach", 30, 0.3),
			road(2, "Col du Tourmalet", 17.1, 7.3),
			road(3, "Descent", 18, -6.5),
			road(4, "Valley road", 15, 0.3),
			road(5, "Hautacam", 13.6, 7.8),
		}),
		// 21: Prologue TT
		cd("Prologue TT", 8, []model.Segment{
			road(1, "Urban circuit", 8, 0),
		}),
		// 22: Short ITT
		cd("Short ITT", 25, []model.Segment{
			road(1, "Flat section", 10, 0),
			road(2, "Rolling section", 8, 1.5),
			road(3, "Flat return", 7, -0.5),
		}),
		// 23: Long ITT
		cd("Long ITT", 40, []model.Segment{
			road(1, "Flat section 1", 12, 0),
			road(2, "Rolling hills", 8, 2.0),
			road(3, "Flat section 2", 10, 0),
			road(4, "False flat return", 10, -0.5),
		}),
		// 24: Mountain TT
		cd("Mountain TT", 30, []model.Segment{
			road(1, "Valley start", 8, 0.5),
			road(2, "Mountain climb", 12, 6.5),
			road(3, "False flat", 4, 2.0),
			road(4, "Final ramp", 6, 7.0),
		}),
		// 25: Flat Sprint Stage (long)
		cd("Flat Sprint Stage (long)", 220, []model.Segment{
			road(1, "Flat road", 180, 0),
			road(2, "Gentle rolling", 30, 0.3),
			road(3, "Sprint finish", 10, 0),
		}),
		// 26: Hilly Stage with Uphill Finish
		cd("Hilly Stage with Uphill Finish", 180, []model.Segment{
			road(1, "Road section", 60, 0.3),
			road(2, "Cat 3 climb", 5, 5.0),
			road(3, "Descent", 6, -4.0),
			road(4, "Road section", 50, 0.5),
			road(5, "Cat 3 climb", 4, 5.5),
			road(6, "Descent", 5, -4.0),
			road(7, "Road section", 20, 0.3),
			road(8, "Uphill finish", 3, 6.5),
		}),
		// 27: Basque Country Mountain Stage
		cd("Basque Country Mountain Stage", 165, []model.Segment{
			road(1, "Road section", 40, 0.5),
			road(2, "Cat 2 climb", 6, 7.0),
			road(3, "Descent", 8, -5.5),
			road(4, "Valley road", 30, 0.5),
			road(5, "Cat 2 climb", 7, 6.5),
			road(6, "Descent", 8, -5.5),
			road(7, "Valley road", 15, 0.3),
			road(8, "Cat 1 summit finish", 10, 7.5),
		}),
		// 28: Swiss Mountain Stage
		cd("Swiss Mountain Stage", 175, []model.Segment{
			road(1, "Lake road", 50, 0.3),
			road(2, "Cat 1 alpine pass", 14, 6.5),
			road(3, "Descent", 16, -5.5),
			road(4, "Valley road", 25, 0.3),
			road(5, "Cat 1 alpine pass", 12, 7.0),
			road(6, "Descent", 10, -6.0),
			road(7, "Run-in", 8, 0),
		}),
		// 29: Romandie Rolling Stage
		cd("Romandie Rolling Stage", 165, []model.Segment{
			road(1, "Lake road", 50, 0.3),
			road(2, "Cat 3 climb", 4, 5.0),
			road(3, "Descent", 5, -4.0),
			road(4, "Road section", 40, 0.5),
			road(5, "Cat 3 climb", 5, 4.5),
			road(6, "Descent", 5, -4.0),
			road(7, "Run-in", 20, 0.3),
		}),
		// 30: Vuelta Mountain Stage
		cd("Vuelta Mountain Stage", 175, []model.Segment{
			road(1, "Road section", 40, 0.5),
			road(2, "Puerto climb", 12, 6.2),
			road(3, "Descent", 14, -5.5),
			road(4, "Valley road", 25, 0.3),
			road(5, "Cat 1 climb", 14, 7.0),
			road(6, "Descent", 12, -5.5),
			road(7, "Valley road", 15, 0.3),
			road(8, "Cat 1 summit finish", 12, 7.5),
		}),
		// 31: Vuelta Steep Summit
		cd("Vuelta Steep Summit", 155, []model.Segment{
			road(1, "Valley approach", 35, 0.3),
			road(2, "Cat 1 climb", 10, 7.2),
			road(3, "Descent", 12, -6.0),
			road(4, "Valley road", 20, 0.3),
			road(5, "Cat 1 climb", 12, 7.5),
			road(6, "Descent", 14, -5.5),
			road(7, "Short valley", 5, 0.3),
			road(8, "Angliru / Lagos finish", 12.5, 9.8),
		}),
		// 32: Short Prologue TT (for Romandie)
		cd("Short Prologue TT", 5, []model.Segment{
			road(1, "Urban circuit", 5, 0.3),
		}),
	}

	ids := make(map[string]int64)
	for _, cd := range courses {
		c := cd.course
		if err := db.CreateCourse(&c); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating course %s: %v\n", c.Name, err)
			continue
		}
		ids[c.Name] = c.ID
		for _, seg := range cd.segments {
			seg.CourseID = c.ID
			if err := db.CreateSegment(&seg); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating segment %s: %v\n", seg.Name, err)
			}
		}
	}
	return ids
}

func seedRaces(db *database.DB, cIDs map[string]int64) map[string]int64 {
	d := func(y, m, day int) time.Time {
		return time.Date(y, time.Month(m), day, 0, 0, 0, 0, time.UTC)
	}

	races := []model.Race{
		// ===== Monuments =====
		{Name: "Milan-San Remo", CourseID: cIDs["Milan-San Remo"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 22), Temperature: 15, WindSpeedKmh: 10},
		{Name: "Tour of Flanders", CourseID: cIDs["Tour of Flanders"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 6), Temperature: 12, WindSpeedKmh: 18},
		{Name: "Paris-Roubaix", CourseID: cIDs["Paris-Roubaix"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 13), Temperature: 12, WindSpeedKmh: 20},
		{Name: "Liege-Bastogne-Liege", CourseID: cIDs["Liege-Bastogne-Liege"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 27), Temperature: 14, WindSpeedKmh: 12},
		{Name: "Il Lombardia", CourseID: cIDs["Il Lombardia"], RaceType: model.RaceTypeRoad, Date: d(2025, 10, 11), Temperature: 16, WindSpeedKmh: 8},

		// ===== One-Day Classics =====
		{Name: "Strade Bianche", CourseID: cIDs["Strade Bianche"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 8), Temperature: 12, WindSpeedKmh: 10},
		{Name: "E3 Saxo Classic", CourseID: cIDs["E3 Saxo Classic"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 28), Temperature: 10, WindSpeedKmh: 15},
		{Name: "Gent-Wevelgem", CourseID: cIDs["Gent-Wevelgem"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 30), Temperature: 10, WindSpeedKmh: 22},
		{Name: "Amstel Gold Race", CourseID: cIDs["Amstel Gold Race"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 20), Temperature: 14, WindSpeedKmh: 12},
		{Name: "La Fleche Wallonne", CourseID: cIDs["La Fleche Wallonne"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 23), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Clasica San Sebastian", CourseID: cIDs["Clasica San Sebastian"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 2), Temperature: 25, WindSpeedKmh: 8},

		// ===== Tour de France (21 stages) =====
		{Name: "TdF Stage 1 - Prologue TT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 7, 5), Temperature: 22, WindSpeedKmh: 8},
		{Name: "TdF Stage 2 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 6), Temperature: 24, WindSpeedKmh: 10},
		{Name: "TdF Stage 3 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 7), Temperature: 25, WindSpeedKmh: 8},
		{Name: "TdF Stage 4 - Hilly", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 8), Temperature: 26, WindSpeedKmh: 10},
		{Name: "TdF Stage 5 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (long)"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 9), Temperature: 27, WindSpeedKmh: 12},
		{Name: "TdF Stage 6 - Cobbles", CourseID: cIDs["E3 Saxo Classic"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 10), Temperature: 22, WindSpeedKmh: 15},
		{Name: "TdF Stage 7 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 11), Temperature: 24, WindSpeedKmh: 8},
		{Name: "TdF Stage 8 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 12), Temperature: 22, WindSpeedKmh: 8},
		{Name: "TdF Stage 9 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 13), Temperature: 28, WindSpeedKmh: 10},
		// Rest day after stage 9
		{Name: "TdF Stage 10 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 15), Temperature: 20, WindSpeedKmh: 5},
		{Name: "TdF Stage 11 - Pyrenees Summit", CourseID: cIDs["Pyrenees Summit Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 16), Temperature: 18, WindSpeedKmh: 8},
		{Name: "TdF Stage 12 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 17), Temperature: 30, WindSpeedKmh: 5},
		{Name: "TdF Stage 13 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 18), Temperature: 28, WindSpeedKmh: 10},
		{Name: "TdF Stage 14 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 19), Temperature: 24, WindSpeedKmh: 8},
		{Name: "TdF Stage 15 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 20), Temperature: 18, WindSpeedKmh: 5},
		// Rest day after stage 15
		{Name: "TdF Stage 16 - ITT", CourseID: cIDs["Long ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 7, 22), Temperature: 25, WindSpeedKmh: 10},
		{Name: "TdF Stage 17 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (long)"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 23), Temperature: 28, WindSpeedKmh: 8},
		{Name: "TdF Stage 18 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 24), Temperature: 18, WindSpeedKmh: 5},
		{Name: "TdF Stage 19 - Alpine Summit", CourseID: cIDs["Alpine Summit Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 25), Temperature: 16, WindSpeedKmh: 8},
		{Name: "TdF Stage 20 - Mountain", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 26), Temperature: 18, WindSpeedKmh: 5},
		{Name: "TdF Stage 21 - Paris Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 27), Temperature: 25, WindSpeedKmh: 8},

		// ===== Giro d'Italia (21 stages) =====
		{Name: "Giro Stage 1 - Prologue TT", CourseID: cIDs["Prologue TT"], RaceType: model.RaceTypeTT, Date: d(2025, 5, 10), Temperature: 18, WindSpeedKmh: 5},
		{Name: "Giro Stage 2 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 11), Temperature: 20, WindSpeedKmh: 8},
		{Name: "Giro Stage 3 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 12), Temperature: 22, WindSpeedKmh: 10},
		{Name: "Giro Stage 4 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 13), Temperature: 22, WindSpeedKmh: 8},
		{Name: "Giro Stage 5 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 14), Temperature: 20, WindSpeedKmh: 10},
		{Name: "Giro Stage 6 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (long)"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 15), Temperature: 24, WindSpeedKmh: 8},
		{Name: "Giro Stage 7 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 16), Temperature: 18, WindSpeedKmh: 5},
		{Name: "Giro Stage 8 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 17), Temperature: 22, WindSpeedKmh: 10},
		{Name: "Giro Stage 9 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 18), Temperature: 20, WindSpeedKmh: 8},
		// Rest day after stage 9
		{Name: "Giro Stage 10 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 20), Temperature: 16, WindSpeedKmh: 5},
		{Name: "Giro Stage 11 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 21), Temperature: 24, WindSpeedKmh: 8},
		{Name: "Giro Stage 12 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 22), Temperature: 18, WindSpeedKmh: 8},
		{Name: "Giro Stage 13 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 23), Temperature: 22, WindSpeedKmh: 10},
		{Name: "Giro Stage 14 - ITT", CourseID: cIDs["Long ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 5, 24), Temperature: 22, WindSpeedKmh: 8},
		{Name: "Giro Stage 15 - Dolomites Summit", CourseID: cIDs["Dolomites Summit Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 25), Temperature: 14, WindSpeedKmh: 5},
		// Rest day after stage 15
		{Name: "Giro Stage 16 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 27), Temperature: 16, WindSpeedKmh: 5},
		{Name: "Giro Stage 17 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 28), Temperature: 26, WindSpeedKmh: 8},
		{Name: "Giro Stage 18 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 29), Temperature: 24, WindSpeedKmh: 10},
		{Name: "Giro Stage 19 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 30), Temperature: 14, WindSpeedKmh: 5},
		{Name: "Giro Stage 20 - Mountain TT", CourseID: cIDs["Mountain TT"], RaceType: model.RaceTypeTT, Date: d(2025, 5, 31), Temperature: 16, WindSpeedKmh: 5},
		{Name: "Giro Stage 21 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 1), Temperature: 26, WindSpeedKmh: 5},

		// ===== Vuelta a Espana (21 stages) =====
		{Name: "Vuelta Stage 1 - Short ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 8, 23), Temperature: 30, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 2 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 24), Temperature: 32, WindSpeedKmh: 10},
		{Name: "Vuelta Stage 3 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 25), Temperature: 30, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 4 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 26), Temperature: 33, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 5 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 27), Temperature: 28, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 6 - Vuelta Mountain", CourseID: cIDs["Vuelta Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 28), Temperature: 26, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 7 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (long)"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 29), Temperature: 32, WindSpeedKmh: 10},
		{Name: "Vuelta Stage 8 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 30), Temperature: 28, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 9 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 31), Temperature: 22, WindSpeedKmh: 5},
		// Rest day after stage 9
		{Name: "Vuelta Stage 10 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 2), Temperature: 30, WindSpeedKmh: 10},
		{Name: "Vuelta Stage 11 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 3), Temperature: 26, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 12 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 4), Temperature: 28, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 13 - Hilly", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 5), Temperature: 26, WindSpeedKmh: 10},
		{Name: "Vuelta Stage 14 - Vuelta Mountain", CourseID: cIDs["Vuelta Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 6), Temperature: 24, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 15 - Steep Summit", CourseID: cIDs["Vuelta Steep Summit"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 7), Temperature: 20, WindSpeedKmh: 5},
		// Rest day after stage 15
		{Name: "Vuelta Stage 16 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 9), Temperature: 22, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 17 - ITT", CourseID: cIDs["Long ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 9, 10), Temperature: 26, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 18 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 11), Temperature: 24, WindSpeedKmh: 10},
		{Name: "Vuelta Stage 19 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 12), Temperature: 20, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 20 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 13), Temperature: 18, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 21 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 14), Temperature: 26, WindSpeedKmh: 8},

		// ===== Paris-Nice (8 stages) =====
		{Name: "Paris-Nice Stage 1 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 9), Temperature: 8, WindSpeedKmh: 15},
		{Name: "Paris-Nice Stage 2 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 10), Temperature: 10, WindSpeedKmh: 12},
		{Name: "Paris-Nice Stage 3 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 11), Temperature: 10, WindSpeedKmh: 10},
		{Name: "Paris-Nice Stage 4 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 12), Temperature: 12, WindSpeedKmh: 8},
		{Name: "Paris-Nice Stage 5 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 13), Temperature: 12, WindSpeedKmh: 8},
		{Name: "Paris-Nice Stage 6 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 14), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Paris-Nice Stage 7 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 15), Temperature: 10, WindSpeedKmh: 5},
		{Name: "Paris-Nice Stage 8 - ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 3, 16), Temperature: 14, WindSpeedKmh: 8},

		// ===== Tirreno-Adriatico (7 stages) =====
		{Name: "Tirreno Stage 1 - Short ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 3, 10), Temperature: 12, WindSpeedKmh: 8},
		{Name: "Tirreno Stage 2 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 11), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Tirreno Stage 3 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 12), Temperature: 14, WindSpeedKmh: 8},
		{Name: "Tirreno Stage 4 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 13), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Tirreno Stage 5 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 14), Temperature: 12, WindSpeedKmh: 5},
		{Name: "Tirreno Stage 6 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 15), Temperature: 10, WindSpeedKmh: 8},
		{Name: "Tirreno Stage 7 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 16), Temperature: 14, WindSpeedKmh: 10},

		// ===== Tour de Suisse (8 stages) =====
		{Name: "Suisse Stage 1 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 8), Temperature: 18, WindSpeedKmh: 8},
		{Name: "Suisse Stage 2 - Rolling", CourseID: cIDs["Romandie Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 9), Temperature: 20, WindSpeedKmh: 8},
		{Name: "Suisse Stage 3 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 10), Temperature: 22, WindSpeedKmh: 10},
		{Name: "Suisse Stage 4 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 11), Temperature: 20, WindSpeedKmh: 8},
		{Name: "Suisse Stage 5 - Swiss Mountain", CourseID: cIDs["Swiss Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 12), Temperature: 16, WindSpeedKmh: 5},
		{Name: "Suisse Stage 6 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 13), Temperature: 18, WindSpeedKmh: 8},
		{Name: "Suisse Stage 7 - Swiss Mountain", CourseID: cIDs["Swiss Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 14), Temperature: 14, WindSpeedKmh: 5},
		{Name: "Suisse Stage 8 - ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 6, 15), Temperature: 20, WindSpeedKmh: 8},

		// ===== Criterium du Dauphine (8 stages) =====
		{Name: "Dauphine Stage 1 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 8), Temperature: 20, WindSpeedKmh: 8},
		{Name: "Dauphine Stage 2 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 9), Temperature: 22, WindSpeedKmh: 10},
		{Name: "Dauphine Stage 3 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 10), Temperature: 22, WindSpeedKmh: 8},
		{Name: "Dauphine Stage 4 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 11), Temperature: 18, WindSpeedKmh: 5},
		{Name: "Dauphine Stage 5 - ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 6, 12), Temperature: 20, WindSpeedKmh: 8},
		{Name: "Dauphine Stage 6 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 13), Temperature: 16, WindSpeedKmh: 5},
		{Name: "Dauphine Stage 7 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 14), Temperature: 14, WindSpeedKmh: 5},
		{Name: "Dauphine Stage 8 - Alpine Summit", CourseID: cIDs["Alpine Summit Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 15), Temperature: 14, WindSpeedKmh: 8},

		// ===== Tour of the Basque Country (6 stages) =====
		{Name: "Basque Stage 1 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 7), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Basque Stage 2 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 8), Temperature: 14, WindSpeedKmh: 8},
		{Name: "Basque Stage 3 - Basque Mountain", CourseID: cIDs["Basque Country Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 9), Temperature: 12, WindSpeedKmh: 8},
		{Name: "Basque Stage 4 - ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 4, 10), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Basque Stage 5 - Basque Mountain", CourseID: cIDs["Basque Country Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 11), Temperature: 12, WindSpeedKmh: 5},
		{Name: "Basque Stage 6 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 12), Temperature: 10, WindSpeedKmh: 8},

		// ===== Tour de Romandie (5 stages + prologue) =====
		{Name: "Romandie Prologue", CourseID: cIDs["Short Prologue TT"], RaceType: model.RaceTypeTT, Date: d(2025, 4, 29), Temperature: 14, WindSpeedKmh: 5},
		{Name: "Romandie Stage 1 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 30), Temperature: 16, WindSpeedKmh: 8},
		{Name: "Romandie Stage 2 - Rolling", CourseID: cIDs["Romandie Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 1), Temperature: 16, WindSpeedKmh: 10},
		{Name: "Romandie Stage 3 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 2), Temperature: 14, WindSpeedKmh: 5},
		{Name: "Romandie Stage 4 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 3), Temperature: 12, WindSpeedKmh: 5},
		{Name: "Romandie Stage 5 - ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 5, 4), Temperature: 16, WindSpeedKmh: 8},
	}

	ids := make(map[string]int64)
	for i := range races {
		if err := db.CreateRace(&races[i]); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating race %s: %v\n", races[i].Name, err)
			continue
		}
		ids[races[i].Name] = races[i].ID
	}
	return ids
}

func seedStageRaces(db *database.DB, rIDs map[string]int64) int {
	type srDef struct {
		name   string
		stages []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}
	}

	stg := func(name string, num int, rest bool) struct {
		raceName      string
		stageNum      int
		restDayBefore bool
	} {
		return struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{name, num, rest}
	}

	stageRaces := []srDef{
		{name: "Tour de France", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("TdF Stage 1 - Prologue TT", 1, false),
			stg("TdF Stage 2 - Flat Sprint", 2, false),
			stg("TdF Stage 3 - Flat Sprint", 3, false),
			stg("TdF Stage 4 - Hilly", 4, false),
			stg("TdF Stage 5 - Flat Sprint", 5, false),
			stg("TdF Stage 6 - Cobbles", 6, false),
			stg("TdF Stage 7 - Hilly", 7, false),
			stg("TdF Stage 8 - Medium Mountain", 8, false),
			stg("TdF Stage 9 - Flat Sprint", 9, false),
			stg("TdF Stage 10 - High Mountain", 10, true),
			stg("TdF Stage 11 - Pyrenees Summit", 11, false),
			stg("TdF Stage 12 - Flat Sprint", 12, false),
			stg("TdF Stage 13 - Rolling", 13, false),
			stg("TdF Stage 14 - Medium Mountain", 14, false),
			stg("TdF Stage 15 - Summit Finish", 15, false),
			stg("TdF Stage 16 - ITT", 16, true),
			stg("TdF Stage 17 - Flat Sprint", 17, false),
			stg("TdF Stage 18 - High Mountain", 18, false),
			stg("TdF Stage 19 - Alpine Summit", 19, false),
			stg("TdF Stage 20 - Mountain", 20, false),
			stg("TdF Stage 21 - Paris Sprint", 21, false),
		}},
		{name: "Giro d'Italia", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Giro Stage 1 - Prologue TT", 1, false),
			stg("Giro Stage 2 - Flat Sprint", 2, false),
			stg("Giro Stage 3 - Rolling", 3, false),
			stg("Giro Stage 4 - Flat Sprint", 4, false),
			stg("Giro Stage 5 - Hilly", 5, false),
			stg("Giro Stage 6 - Flat Sprint", 6, false),
			stg("Giro Stage 7 - Medium Mountain", 7, false),
			stg("Giro Stage 8 - Flat Sprint", 8, false),
			stg("Giro Stage 9 - Rolling", 9, false),
			stg("Giro Stage 10 - Summit Finish", 10, true),
			stg("Giro Stage 11 - Flat Sprint", 11, false),
			stg("Giro Stage 12 - Medium Mountain", 12, false),
			stg("Giro Stage 13 - Hilly", 13, false),
			stg("Giro Stage 14 - ITT", 14, false),
			stg("Giro Stage 15 - Dolomites Summit", 15, false),
			stg("Giro Stage 16 - High Mountain", 16, true),
			stg("Giro Stage 17 - Flat Sprint", 17, false),
			stg("Giro Stage 18 - Rolling", 18, false),
			stg("Giro Stage 19 - Summit Finish", 19, false),
			stg("Giro Stage 20 - Mountain TT", 20, false),
			stg("Giro Stage 21 - Flat Sprint", 21, false),
		}},
		{name: "Vuelta a Espana", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Vuelta Stage 1 - Short ITT", 1, false),
			stg("Vuelta Stage 2 - Flat Sprint", 2, false),
			stg("Vuelta Stage 3 - Hilly", 3, false),
			stg("Vuelta Stage 4 - Flat Sprint", 4, false),
			stg("Vuelta Stage 5 - Medium Mountain", 5, false),
			stg("Vuelta Stage 6 - Vuelta Mountain", 6, false),
			stg("Vuelta Stage 7 - Flat Sprint", 7, false),
			stg("Vuelta Stage 8 - Rolling", 8, false),
			stg("Vuelta Stage 9 - Summit Finish", 9, false),
			stg("Vuelta Stage 10 - Flat Sprint", 10, true),
			stg("Vuelta Stage 11 - Medium Mountain", 11, false),
			stg("Vuelta Stage 12 - Flat Sprint", 12, false),
			stg("Vuelta Stage 13 - Hilly", 13, false),
			stg("Vuelta Stage 14 - Vuelta Mountain", 14, false),
			stg("Vuelta Stage 15 - Steep Summit", 15, false),
			stg("Vuelta Stage 16 - High Mountain", 16, true),
			stg("Vuelta Stage 17 - ITT", 17, false),
			stg("Vuelta Stage 18 - Rolling", 18, false),
			stg("Vuelta Stage 19 - Summit Finish", 19, false),
			stg("Vuelta Stage 20 - High Mountain", 20, false),
			stg("Vuelta Stage 21 - Flat Sprint", 21, false),
		}},
		{name: "Paris-Nice", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Paris-Nice Stage 1 - Flat Sprint", 1, false),
			stg("Paris-Nice Stage 2 - Flat Sprint", 2, false),
			stg("Paris-Nice Stage 3 - Rolling", 3, false),
			stg("Paris-Nice Stage 4 - Hilly", 4, false),
			stg("Paris-Nice Stage 5 - Medium Mountain", 5, false),
			stg("Paris-Nice Stage 6 - Flat Sprint", 6, false),
			stg("Paris-Nice Stage 7 - Summit Finish", 7, false),
			stg("Paris-Nice Stage 8 - ITT", 8, false),
		}},
		{name: "Tirreno-Adriatico", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Tirreno Stage 1 - Short ITT", 1, false),
			stg("Tirreno Stage 2 - Flat Sprint", 2, false),
			stg("Tirreno Stage 3 - Rolling", 3, false),
			stg("Tirreno Stage 4 - Hilly", 4, false),
			stg("Tirreno Stage 5 - Medium Mountain", 5, false),
			stg("Tirreno Stage 6 - Summit Finish", 6, false),
			stg("Tirreno Stage 7 - Flat Sprint", 7, false),
		}},
		{name: "Tour de Suisse", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Suisse Stage 1 - Flat Sprint", 1, false),
			stg("Suisse Stage 2 - Rolling", 2, false),
			stg("Suisse Stage 3 - Flat Sprint", 3, false),
			stg("Suisse Stage 4 - Hilly", 4, false),
			stg("Suisse Stage 5 - Swiss Mountain", 5, false),
			stg("Suisse Stage 6 - Medium Mountain", 6, false),
			stg("Suisse Stage 7 - Swiss Mountain", 7, false),
			stg("Suisse Stage 8 - ITT", 8, false),
		}},
		{name: "Criterium du Dauphine", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Dauphine Stage 1 - Flat Sprint", 1, false),
			stg("Dauphine Stage 2 - Rolling", 2, false),
			stg("Dauphine Stage 3 - Hilly", 3, false),
			stg("Dauphine Stage 4 - Medium Mountain", 4, false),
			stg("Dauphine Stage 5 - ITT", 5, false),
			stg("Dauphine Stage 6 - High Mountain", 6, false),
			stg("Dauphine Stage 7 - Summit Finish", 7, false),
			stg("Dauphine Stage 8 - Alpine Summit", 8, false),
		}},
		{name: "Tour of the Basque Country", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Basque Stage 1 - Rolling", 1, false),
			stg("Basque Stage 2 - Hilly", 2, false),
			stg("Basque Stage 3 - Basque Mountain", 3, false),
			stg("Basque Stage 4 - ITT", 4, false),
			stg("Basque Stage 5 - Basque Mountain", 5, false),
			stg("Basque Stage 6 - Summit Finish", 6, false),
		}},
		{name: "Tour de Romandie", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Romandie Prologue", 1, false),
			stg("Romandie Stage 1 - Flat Sprint", 2, false),
			stg("Romandie Stage 2 - Rolling", 3, false),
			stg("Romandie Stage 3 - Medium Mountain", 4, false),
			stg("Romandie Stage 4 - Summit Finish", 5, false),
			stg("Romandie Stage 5 - ITT", 6, false),
		}},
	}

	count := 0
	for _, srDef := range stageRaces {
		sr := model.StageRace{Name: srDef.name}
		if err := db.CreateStageRace(&sr); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating stage race %s: %v\n", srDef.name, err)
			continue
		}
		for _, s := range srDef.stages {
			raceID, ok := rIDs[s.raceName]
			if !ok {
				fmt.Fprintf(os.Stderr, "  Warning: race %q not found for %s\n", s.raceName, srDef.name)
				continue
			}
			db.AddStageToRace(&model.StageRaceStage{
				StageRaceID:   sr.ID,
				RaceID:        raceID,
				StageNumber:   s.stageNum,
				RestDayBefore: s.restDayBefore,
			})
		}
		fmt.Printf("  Created stage race: %s (%d stages)\n", srDef.name, len(srDef.stages))
		count++
	}
	return count
}

// findDataFile locates a data file relative to the binary or source directory.
func findDataFile(name string) string {
	// Try relative to working directory
	candidates := []string{
		filepath.Join("data", name),
		filepath.Join("..", "..", "data", name),
	}
	// Try relative to the source file location
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		srcDir := filepath.Dir(thisFile)
		candidates = append(candidates,
			filepath.Join(srcDir, "..", "..", "data", name),
		)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join("data", name) // fallback
}

// ---------------------------------------------------------------------------
// FirstCycling import commands
// ---------------------------------------------------------------------------

// cmdFCFetch fetches race results from FirstCycling and stores them.
//
// Usage:
//
//	fc-fetch --race <id> --year <year> [--stage <n>] [--stages <from>-<to>]
func cmdFCFetch(db *database.DB, args []string) {
	fs := flag.NewFlagSet("fc-fetch", flag.ExitOnError)
	race := fs.Int("race", 0, "FirstCycling race ID (e.g. 17 = Tour de France)")
	year := fs.Int("year", 0, "Year (e.g. 2024)")
	stage := fs.Int("stage", 0, "Specific stage number (0 = overall/GC)")
	stages := fs.String("stages", "", "Stage range to fetch, e.g. 1-21")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if *race == 0 || *year == 0 {
		fmt.Fprintln(os.Stderr, "Usage: cycling-sim fc-fetch --race <id> --year <year> [--stage <n>|--stages <from>-<to>]")
		os.Exit(1)
	}

	imp := fcimport.NewImporter(os.Stdout)

	// Collect the list of requests to fetch.
	var requests []fcimport.FetchRequest
	switch {
	case *stages != "":
		// Parse "from-to" range.
		parts := strings.SplitN(*stages, "-", 2)
		if len(parts) != 2 {
			fmt.Fprintln(os.Stderr, "Invalid --stages format; use e.g. 1-21")
			os.Exit(1)
		}
		from, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		to, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || from < 1 || to < from {
			fmt.Fprintln(os.Stderr, "Invalid --stages range")
			os.Exit(1)
		}
		for s := from; s <= to; s++ {
			requests = append(requests, fcimport.FetchRequest{FCRaceID: *race, Year: *year, Stage: s})
		}
	default:
		requests = []fcimport.FetchRequest{{FCRaceID: *race, Year: *year, Stage: *stage}}
	}

	total := 0
	for _, req := range requests {
		result, err := imp.Fetch(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching: %v\n", err)
			os.Exit(1)
		}
		if len(result.Results) == 0 {
			fmt.Printf("  Warning: no results parsed for race=%d year=%d stage=%d\n",
				req.FCRaceID, req.Year, req.Stage)
			continue
		}
		if err := db.UpsertFCResults(req, result.Results); err != nil {
			fmt.Fprintf(os.Stderr, "Error storing results: %v\n", err)
			os.Exit(1)
		}
		total += len(result.Results)
	}

	// After storing, auto-match any new FC rider names.
	matched, err := db.AutoMatchFCRiders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: auto-match failed: %v\n", err)
	} else if matched > 0 {
		fmt.Printf("Auto-matched %d new rider(s) to internal riders.\n", matched)
	}

	fmt.Printf("Done. Stored %d result rows total.\n", total)
}

// cmdFCList prints a summary of all fetched FirstCycling reference data.
func cmdFCList(db *database.DB) {
	summaries, err := db.ListFCFetched()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(summaries) == 0 {
		fmt.Println("No FirstCycling data fetched yet. Use: cycling-sim fc-fetch --race <id> --year <year>")
		return
	}
	fmt.Printf("%-12s %-6s %-8s %-8s %s\n", "FC Race ID", "Year", "Stage", "Results", "Fetched At")
	fmt.Println(strings.Repeat("-", 55))
	for _, s := range summaries {
		stageStr := "overall"
		if s.Stage > 0 {
			stageStr = fmt.Sprintf("stage %d", s.Stage)
		}
		fmt.Printf("%-12d %-6d %-8s %-8d %s\n", s.FCRaceID, s.Year, stageStr, s.ResultCount, s.FetchedAt)
	}
}

// cmdFCMatch attempts to resolve all unmatched FC rider names against internal riders.
func cmdFCMatch(db *database.DB) {
	matched, err := db.AutoMatchFCRiders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Matched %d rider(s).\n", matched)
}

// ---------------------------------------------------------------------------
// CSV seed helpers
// ---------------------------------------------------------------------------

// loadRidersFromCSV reads riders from a CSV file and inserts them into the database.
// CSV columns: Id,Name,Team,Nationality,Weight_kg,Height_cm,FTP_watts,Sprint_watts,
//
//	Threshold_watts,W_kg,Climbing,Sprint,TT,Classics,Endurance,Recovery,
//	Acceleration,Consistency,Handling
func loadRidersFromCSV(db *database.DB, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("reading CSV: %w", err)
	}

	if len(records) < 2 {
		return 0, fmt.Errorf("CSV has no data rows")
	}

	count := 0
	for _, row := range records[1:] { // skip header
		if len(row) < 19 {
			continue
		}
		weightKg, _ := strconv.ParseFloat(row[4], 64)
		heightCm, _ := strconv.ParseFloat(row[5], 64)
		ftpWatts, _ := strconv.Atoi(row[6])
		sprintWatts, _ := strconv.Atoi(row[7])
		vo2maxWatts, _ := strconv.Atoi(row[8])
		cda, _ := strconv.ParseFloat(row[9], 64)
		climbing, _ := strconv.Atoi(row[10])
		sprint, _ := strconv.Atoi(row[11])
		tt, _ := strconv.Atoi(row[12])
		classics, _ := strconv.Atoi(row[13])
		endurance, _ := strconv.Atoi(row[14])
		recovery, _ := strconv.Atoi(row[15])
		acceleration, _ := strconv.Atoi(row[16])
		consistency, _ := strconv.Atoi(row[17])
		handling, _ := strconv.Atoi(row[18])

		rider := model.Rider{
			Name:              row[1],
			Team:              row[2],
			Nationality:       row[3],
			Age:               simulation.DefaultAgeFromName(row[1]),
			WeightKg:          weightKg,
			HeightCm:          heightCm,
			FTPWatts:          ftpWatts,
			MaxSprintWatts:    sprintWatts,
			VO2MaxWatts:       vo2maxWatts,
			CdA:               cda,
			ClimbingAbility:   climbing,
			SprintAbility:     sprint,
			TTAbility:         tt,
			DescendingAbility: classics,
			Endurance:         endurance,
			Recovery:          recovery,
			Consistency:       consistency,
			TacticalAwareness: acceleration,
			Handling:          handling,
		}

		if err := db.CreateRider(&rider); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: skipping %s: %v\n", rider.Name, err)
			continue
		}
		count++
	}
	return count, nil
}
