package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
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
	case "riders":
		cmdListRiders(db)
	case "courses":
		cmdListCourses(db)
	case "races":
		cmdListRaces(db)
	case "run":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: cycling-sim run <race-id>")
			os.Exit(1)
		}
		raceID, err := strconv.ParseInt(os.Args[2], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid race ID: %v\n", err)
			os.Exit(1)
		}
		cmdRunRace(db, raceID)
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
		srID, _ := strconv.ParseInt(os.Args[2], 10, 64)
		stage, _ := strconv.Atoi(os.Args[3])
		cmdShowGC(db, srID, stage)
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
  riders                         List all riders
  courses                        List all courses
  races                          List all races
  run <race-id>                  Simulate a race
  run-stage-race <stage-race-id> Simulate a stage race
  results <race-id>              Show race results
  gc <stage-race-id> <stage>     Show GC standings after a stage

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

	// Classic courses
	courses := []struct {
		course   model.Course
		segments []model.Segment
	}{
		{
			course: model.Course{Name: "Milan-San Remo", TotalDistanceKm: 290},
			segments: []model.Segment{
				{SegmentOrder: 1, Name: "Flat approach", DistanceKm: 230, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 2, Name: "Cipressa", DistanceKm: 5.6, GradientPct: 4.1, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 3, Name: "Cipressa descent", DistanceKm: 6, GradientPct: -4.0, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 4, Name: "Poggio", DistanceKm: 3.7, GradientPct: 3.7, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 5, Name: "Poggio descent", DistanceKm: 4, GradientPct: -5.0, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 6, Name: "Via Roma finish", DistanceKm: 1, SurfaceType: model.SurfaceRoad},
			},
		},
		{
			course: model.Course{Name: "Alpe d'Huez Mountain Stage", TotalDistanceKm: 165},
			segments: []model.Segment{
				{SegmentOrder: 1, Name: "Valley approach", DistanceKm: 80, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 2, Name: "Col du Galibier", DistanceKm: 18, GradientPct: 6.9, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 3, Name: "Galibier descent", DistanceKm: 20, GradientPct: -6.0, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 4, Name: "Valley road", DistanceKm: 20, GradientPct: 0.5, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 5, Name: "Alpe d'Huez", DistanceKm: 13.8, GradientPct: 8.1, SurfaceType: model.SurfaceRoad},
			},
		},
		{
			course: model.Course{Name: "Paris-Roubaix", TotalDistanceKm: 257},
			segments: []model.Segment{
				{SegmentOrder: 1, Name: "Road to Trouée", DistanceKm: 95, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 2, Name: "Trouée d'Arenberg", DistanceKm: 2.3, SurfaceType: model.SurfaceCobbles, IsTechnical: true},
				{SegmentOrder: 3, Name: "Road section", DistanceKm: 30, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 4, Name: "Mons-en-Pévèle", DistanceKm: 3.0, GradientPct: 1.5, SurfaceType: model.SurfaceCobbles, IsTechnical: true},
				{SegmentOrder: 5, Name: "Road to Carrefour", DistanceKm: 40, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 6, Name: "Carrefour de l'Arbre", DistanceKm: 2.1, SurfaceType: model.SurfaceCobbles, IsTechnical: true},
				{SegmentOrder: 7, Name: "Road to Roubaix", DistanceKm: 15, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 8, Name: "Velodrome finish", DistanceKm: 0.5, SurfaceType: model.SurfaceRoad},
			},
		},
		{
			course: model.Course{Name: "40km Individual Time Trial", TotalDistanceKm: 40},
			segments: []model.Segment{
				{SegmentOrder: 1, Name: "Flat section 1", DistanceKm: 12, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 2, Name: "Rolling hills", DistanceKm: 8, GradientPct: 2.0, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 3, Name: "Flat section 2", DistanceKm: 10, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 4, Name: "False flat return", DistanceKm: 10, GradientPct: -0.5, SurfaceType: model.SurfaceRoad},
			},
		},
		{
			course: model.Course{Name: "Flat Sprint Stage", TotalDistanceKm: 200},
			segments: []model.Segment{
				{SegmentOrder: 1, Name: "Flat road", DistanceKm: 160, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 2, Name: "Gentle rolling", DistanceKm: 30, GradientPct: 0.5, SurfaceType: model.SurfaceRoad},
				{SegmentOrder: 3, Name: "Sprint finish", DistanceKm: 10, SurfaceType: model.SurfaceRoad},
			},
		},
	}

	for _, cd := range courses {
		c := cd.course
		if err := db.CreateCourse(&c); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating course %s: %v\n", c.Name, err)
			continue
		}
		for _, seg := range cd.segments {
			seg.CourseID = c.ID
			if err := db.CreateSegment(&seg); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating segment %s: %v\n", seg.Name, err)
			}
		}
	}
	fmt.Printf("  Created %d courses\n", len(courses))

	// Create races
	races := []model.Race{
		{Name: "Milan-San Remo", CourseID: 1, RaceType: model.RaceTypeRoad, Date: time.Date(2025, 3, 22, 0, 0, 0, 0, time.UTC), Temperature: 15, WindSpeedKmh: 10},
		{Name: "Alpe d'Huez Stage", CourseID: 2, RaceType: model.RaceTypeRoad, Date: time.Date(2025, 7, 17, 0, 0, 0, 0, time.UTC), Temperature: 28, WindSpeedKmh: 5},
		{Name: "Paris-Roubaix", CourseID: 3, RaceType: model.RaceTypeRoad, Date: time.Date(2025, 4, 13, 0, 0, 0, 0, time.UTC), Temperature: 12, WindSpeedKmh: 20},
		{Name: "World Championship TT", CourseID: 4, RaceType: model.RaceTypeTT, Date: time.Date(2025, 9, 21, 0, 0, 0, 0, time.UTC), Temperature: 22, WindSpeedKmh: 8},
		{Name: "Sprint Stage", CourseID: 5, RaceType: model.RaceTypeRoad, Date: time.Date(2025, 7, 5, 0, 0, 0, 0, time.UTC), Temperature: 30, WindSpeedKmh: 5},
	}
	for i := range races {
		if err := db.CreateRace(&races[i]); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating race %s: %v\n", races[i].Name, err)
		}
	}
	fmt.Printf("  Created %d races\n", len(races))

	// Create a mini stage race
	sr := model.StageRace{Name: "Mini Tour (3 stages)"}
	if err := db.CreateStageRace(&sr); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating stage race: %v\n", err)
	} else {
		// Stage 1: Sprint stage, Stage 2: Mountain, Stage 3: TT
		db.AddStageToRace(&model.StageRaceStage{StageRaceID: sr.ID, RaceID: races[4].ID, StageNumber: 1})
		db.AddStageToRace(&model.StageRaceStage{StageRaceID: sr.ID, RaceID: races[1].ID, StageNumber: 2})
		db.AddStageToRace(&model.StageRaceStage{StageRaceID: sr.ID, RaceID: races[3].ID, StageNumber: 3})
		fmt.Printf("  Created stage race: %s (3 stages)\n", sr.Name)
	}

	fmt.Println("Seed complete!")
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

func cmdRunRace(db *database.DB, raceID int64) {
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

	riders, err := db.ListRiders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting riders: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Simulating: %s (%s, %.1f km)\n", race.Name, course.Name, course.TotalDistanceKm)
	fmt.Printf("Riders: %d | Type: %s\n\n", len(riders), race.RaceType)

	config := simulation.RaceConfig{
		Race:   race,
		Course: course,
		Riders: riders,
		Seed:   time.Now().UnixNano(),
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

	riders, err := db.ListRiders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting riders: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Simulating stage race: %s (%d stages, %d riders)\n\n", sr.Name, len(sr.Stages), len(riders))

	var stages []simulation.StageConfig
	for _, s := range sr.Stages {
		race, err := db.GetRace(s.RaceID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting stage race: %v\n", err)
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
	}

	allResults, allStandings := simulation.SimulateStageRace(sr, stages, riders, time.Now().UnixNano())

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

// loadRidersFromCSV reads riders from a CSV file and inserts them into the database.
// CSV columns: Id,Name,Team,Nationality,Weight_kg,Height_cm,FTP_watts,Sprint_watts,
//
//	Threshold_watts,W_kg,Climbing,Sprint,TT,Classics,Endurance,Recovery,
//	Acceleration,Consistency
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
		if len(row) < 18 {
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

		rider := model.Rider{
			Name:              row[1],
			Team:              row[2],
			Nationality:       row[3],
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
		}

		if err := db.CreateRider(&rider); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: skipping %s: %v\n", rider.Name, err)
			continue
		}
		count++
	}
	return count, nil
}
