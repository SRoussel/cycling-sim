package main

import (
	"fmt"
	"os"
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

	// WorldTour riders (representative profiles)
	riderData := []model.Rider{
		{Name: "Tadej Pogačar", Team: "UAE Team Emirates", Nationality: "SLO", WeightKg: 66, HeightCm: 176, FTPWatts: 415, MaxSprintWatts: 1400, VO2MaxWatts: 490, CdA: 0.32, ClimbingAbility: 97, SprintAbility: 75, TTAbility: 92, DescendingAbility: 88, Endurance: 93, Recovery: 90, Consistency: 95, TacticalAwareness: 92},
		{Name: "Jonas Vingegaard", Team: "Visma-Lease a Bike", Nationality: "DEN", WeightKg: 60, HeightCm: 175, FTPWatts: 410, MaxSprintWatts: 1200, VO2MaxWatts: 495, CdA: 0.31, ClimbingAbility: 96, SprintAbility: 55, TTAbility: 90, DescendingAbility: 80, Endurance: 95, Recovery: 88, Consistency: 90, TacticalAwareness: 85},
		{Name: "Remco Evenepoel", Team: "Soudal-QuickStep", Nationality: "BEL", WeightKg: 61, HeightCm: 171, FTPWatts: 412, MaxSprintWatts: 1300, VO2MaxWatts: 485, CdA: 0.30, ClimbingAbility: 90, SprintAbility: 65, TTAbility: 96, DescendingAbility: 72, Endurance: 88, Recovery: 85, Consistency: 88, TacticalAwareness: 82},
		{Name: "Primož Roglič", Team: "Red Bull-BORA-hansgrohe", Nationality: "SLO", WeightKg: 65, HeightCm: 177, FTPWatts: 408, MaxSprintWatts: 1250, VO2MaxWatts: 475, CdA: 0.31, ClimbingAbility: 93, SprintAbility: 60, TTAbility: 91, DescendingAbility: 75, Endurance: 88, Recovery: 82, Consistency: 82, TacticalAwareness: 88},
		{Name: "Wout van Aert", Team: "Visma-Lease a Bike", Nationality: "BEL", WeightKg: 78, HeightCm: 187, FTPWatts: 420, MaxSprintWatts: 1750, VO2MaxWatts: 470, CdA: 0.35, ClimbingAbility: 72, SprintAbility: 92, TTAbility: 93, DescendingAbility: 85, Endurance: 90, Recovery: 85, Consistency: 88, TacticalAwareness: 90},
		{Name: "Mathieu van der Poel", Team: "Alpecin-Deceuninck", Nationality: "NED", WeightKg: 75, HeightCm: 184, FTPWatts: 415, MaxSprintWatts: 1700, VO2MaxWatts: 465, CdA: 0.34, ClimbingAbility: 75, SprintAbility: 90, TTAbility: 78, DescendingAbility: 90, Endurance: 88, Recovery: 82, Consistency: 85, TacticalAwareness: 88},
		{Name: "Jasper Philipsen", Team: "Alpecin-Deceuninck", Nationality: "BEL", WeightKg: 76, HeightCm: 181, FTPWatts: 385, MaxSprintWatts: 1850, VO2MaxWatts: 440, CdA: 0.34, ClimbingAbility: 45, SprintAbility: 97, TTAbility: 60, DescendingAbility: 70, Endurance: 72, Recovery: 70, Consistency: 78, TacticalAwareness: 75},
		{Name: "Jonathan Milan", Team: "Lidl-Trek", Nationality: "ITA", WeightKg: 82, HeightCm: 190, FTPWatts: 395, MaxSprintWatts: 1900, VO2MaxWatts: 445, CdA: 0.36, ClimbingAbility: 40, SprintAbility: 95, TTAbility: 72, DescendingAbility: 68, Endurance: 70, Recovery: 68, Consistency: 75, TacticalAwareness: 70},
		{Name: "Filippo Ganna", Team: "INEOS Grenadiers", Nationality: "ITA", WeightKg: 82, HeightCm: 193, FTPWatts: 425, MaxSprintWatts: 1500, VO2MaxWatts: 460, CdA: 0.35, ClimbingAbility: 50, SprintAbility: 70, TTAbility: 98, DescendingAbility: 75, Endurance: 82, Recovery: 78, Consistency: 88, TacticalAwareness: 72},
		{Name: "Juan Ayuso", Team: "UAE Team Emirates", Nationality: "ESP", WeightKg: 63, HeightCm: 175, FTPWatts: 400, MaxSprintWatts: 1300, VO2MaxWatts: 470, CdA: 0.31, ClimbingAbility: 88, SprintAbility: 62, TTAbility: 85, DescendingAbility: 78, Endurance: 85, Recovery: 82, Consistency: 80, TacticalAwareness: 78},
		{Name: "Carlos Rodríguez", Team: "INEOS Grenadiers", Nationality: "ESP", WeightKg: 62, HeightCm: 174, FTPWatts: 398, MaxSprintWatts: 1200, VO2MaxWatts: 468, CdA: 0.30, ClimbingAbility: 90, SprintAbility: 52, TTAbility: 80, DescendingAbility: 82, Endurance: 85, Recovery: 80, Consistency: 82, TacticalAwareness: 80},
		{Name: "Egan Bernal", Team: "INEOS Grenadiers", Nationality: "COL", WeightKg: 63, HeightCm: 175, FTPWatts: 395, MaxSprintWatts: 1200, VO2MaxWatts: 475, CdA: 0.31, ClimbingAbility: 92, SprintAbility: 55, TTAbility: 78, DescendingAbility: 85, Endurance: 82, Recovery: 75, Consistency: 75, TacticalAwareness: 82},
		{Name: "Adam Yates", Team: "UAE Team Emirates", Nationality: "GBR", WeightKg: 58, HeightCm: 172, FTPWatts: 390, MaxSprintWatts: 1100, VO2MaxWatts: 465, CdA: 0.30, ClimbingAbility: 90, SprintAbility: 48, TTAbility: 75, DescendingAbility: 80, Endurance: 85, Recovery: 78, Consistency: 82, TacticalAwareness: 82},
		{Name: "Tom Pidcock", Team: "INEOS Grenadiers", Nationality: "GBR", WeightKg: 58, HeightCm: 170, FTPWatts: 392, MaxSprintWatts: 1350, VO2MaxWatts: 470, CdA: 0.29, ClimbingAbility: 88, SprintAbility: 72, TTAbility: 78, DescendingAbility: 92, Endurance: 82, Recovery: 78, Consistency: 78, TacticalAwareness: 85},
		{Name: "Biniam Girmay", Team: "Intermarché-Wanty", Nationality: "ERI", WeightKg: 73, HeightCm: 179, FTPWatts: 388, MaxSprintWatts: 1780, VO2MaxWatts: 445, CdA: 0.33, ClimbingAbility: 55, SprintAbility: 93, TTAbility: 62, DescendingAbility: 75, Endurance: 78, Recovery: 72, Consistency: 76, TacticalAwareness: 78},
		{Name: "Mads Pedersen", Team: "Lidl-Trek", Nationality: "DEN", WeightKg: 76, HeightCm: 183, FTPWatts: 400, MaxSprintWatts: 1700, VO2MaxWatts: 450, CdA: 0.34, ClimbingAbility: 60, SprintAbility: 88, TTAbility: 82, DescendingAbility: 82, Endurance: 82, Recovery: 78, Consistency: 82, TacticalAwareness: 85},
		{Name: "Richard Carapaz", Team: "EF Education-EasyPost", Nationality: "ECU", WeightKg: 62, HeightCm: 170, FTPWatts: 395, MaxSprintWatts: 1150, VO2MaxWatts: 472, CdA: 0.30, ClimbingAbility: 92, SprintAbility: 50, TTAbility: 72, DescendingAbility: 80, Endurance: 90, Recovery: 82, Consistency: 85, TacticalAwareness: 88},
		{Name: "Simon Yates", Team: "Team Jayco-AlUla", Nationality: "GBR", WeightKg: 58, HeightCm: 172, FTPWatts: 388, MaxSprintWatts: 1150, VO2MaxWatts: 462, CdA: 0.30, ClimbingAbility: 90, SprintAbility: 50, TTAbility: 70, DescendingAbility: 78, Endurance: 82, Recovery: 75, Consistency: 72, TacticalAwareness: 80},
		{Name: "Mikel Landa", Team: "Soudal-QuickStep", Nationality: "ESP", WeightKg: 60, HeightCm: 173, FTPWatts: 392, MaxSprintWatts: 1100, VO2MaxWatts: 468, CdA: 0.30, ClimbingAbility: 92, SprintAbility: 48, TTAbility: 68, DescendingAbility: 82, Endurance: 80, Recovery: 75, Consistency: 70, TacticalAwareness: 78},
		{Name: "Stefan Küng", Team: "Groupama-FDJ", Nationality: "SUI", WeightKg: 80, HeightCm: 189, FTPWatts: 420, MaxSprintWatts: 1450, VO2MaxWatts: 458, CdA: 0.35, ClimbingAbility: 48, SprintAbility: 65, TTAbility: 95, DescendingAbility: 78, Endurance: 80, Recovery: 75, Consistency: 85, TacticalAwareness: 72},
	}

	for i := range riderData {
		if err := db.CreateRider(&riderData[i]); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating rider %s: %v\n", riderData[i].Name, err)
			continue
		}
	}
	fmt.Printf("  Created %d riders\n", len(riderData))

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
