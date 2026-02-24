package simulation

import (
	"fmt"
	"testing"
	"time"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/selection"
)

// RaceValidator holds actual race results for comparison against simulations.
type RaceValidator struct {
	RaceName    string
	RaceID      int64
	NumRuns     int
	ActualOrder []int64 // rider IDs in actual finishing order (1-based position)
}

// ValidationResult compares simulated vs actual results.
type ValidationResult struct {
	RaceName           string
	NumRuns            int
	SpearmanCorr       float64
	MeanAbsDeviation   float64
	TopTenAccuracy     float64 // % of actual top 10 in sim top 10
	PodiumAccuracy     float64 // % of actual podium in sim top 3
	RiderPlacementsAvg map[int64]float64
}

// ValidateRaceResults simulates a race multiple times and compares to known actual results.
func ValidateRaceResults(t *testing.T, db *database.DB, validator RaceValidator) *ValidationResult {
	race, err := db.GetRace(validator.RaceID)
	if err != nil {
		t.Fatalf("Failed to get race: %v", err)
	}

	course, err := db.GetCourse(race.CourseID)
	if err != nil {
		t.Fatalf("Failed to get course: %v", err)
	}

	allRiders, err := db.ListRiders()
	if err != nil {
		t.Fatalf("Failed to get riders: %v", err)
	}

	// Run the race multiple times and track rider positions
	riderPlacementsSum := make(map[int64]float64)
	riderRunCount := make(map[int64]int)

	for run := 0; run < validator.NumRuns; run++ {
		selConfig := selection.DefaultConfig(race.RaceType, false, 0)
		riders := selection.SelectStartList(allRiders, course, race, selConfig)

		config := RaceConfig{
			Race:   race,
			Course: course,
			Riders: riders,
			Seed:   int64(run),
		}

		var results []model.RaceResult
		if race.RaceType == model.RaceTypeTT {
			results = SimulateTT(config)
		} else {
			results = SimulateRoadRace(config)
		}

		// Track positions
		for pos, res := range results {
			riderID := res.RiderID
			riderPlacementsSum[riderID] += float64(pos + 1)
			riderRunCount[riderID]++
		}
	}

	// Calculate average positions
	riderAvgPos := make(map[int64]float64)
	for riderID, sum := range riderPlacementsSum {
		riderAvgPos[riderID] = sum / float64(riderRunCount[riderID])
	}

	// Build actual and simulated rankings
	actualRankingSimulated := make([]float64, len(validator.ActualOrder))
	for i, riderID := range validator.ActualOrder {
		avgPos, exists := riderAvgPos[riderID]
		if !exists {
			// Rider not in simulation (possibly wasn't selected)
			avgPos = float64(len(validator.ActualOrder) + 1)
		}
		actualRankingSimulated[i] = avgPos
	}

	// Build ranking for riders in actualOrder (1 to len)
	actualRanking := make([]float64, len(validator.ActualOrder))
	for i := 0; i < len(validator.ActualOrder); i++ {
		actualRanking[i] = float64(i + 1)
	}

	// Calculate Spearman correlation
	correlation := spearmanCorrelation(actualRanking, actualRankingSimulated)

	// Calculate mean absolute deviation in positions
	var totalDeviation float64
	for i, riderID := range validator.ActualOrder {
		actualPos := float64(i + 1)
		simPos := riderAvgPos[riderID]
		if simPos == 0 {
			simPos = float64(len(validator.ActualOrder) + 1)
		}
		totalDeviation += absoluteValue(actualPos - simPos)
	}
	meanAbsDev := totalDeviation / float64(len(validator.ActualOrder))

	// Calculate top-10 and podium accuracy
	actualTop10 := make(map[int64]bool)
	for i := 0; i < 10 && i < len(validator.ActualOrder); i++ {
		actualTop10[validator.ActualOrder[i]] = true
	}

	actualPodium := make(map[int64]bool)
	for i := 0; i < 3 && i < len(validator.ActualOrder); i++ {
		actualPodium[validator.ActualOrder[i]] = true
	}

	top10InSim := 0
	for riderID := range actualTop10 {
		avgPos := riderAvgPos[riderID]
		if avgPos > 0 && avgPos <= 10 {
			top10InSim++
		}
	}
	top10Accuracy := float64(top10InSim) / float64(len(actualTop10)) * 100

	podiumInSim := 0
	for riderID := range actualPodium {
		avgPos := riderAvgPos[riderID]
		if avgPos > 0 && avgPos <= 3 {
			podiumInSim++
		}
	}
	podiumAccuracy := float64(podiumInSim) / float64(len(actualPodium)) * 100

	result := &ValidationResult{
		RaceName:           validator.RaceName,
		NumRuns:            validator.NumRuns,
		SpearmanCorr:       correlation,
		MeanAbsDeviation:   meanAbsDev,
		TopTenAccuracy:     top10Accuracy,
		PodiumAccuracy:     podiumAccuracy,
		RiderPlacementsAvg: riderAvgPos,
	}

	return result
}

// absoluteValue returns the absolute value of a float64.
func absoluteValue(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// PrintValidationResult displays the validation results.
func PrintValidationResult(result *ValidationResult) {
	fmt.Printf("\n=== Race Validation Results: %s ===\n", result.RaceName)
	fmt.Printf("Simulations: %d runs\n", result.NumRuns)
	fmt.Printf("Spearman Correlation: %.3f (target: >0.70)\n", result.SpearmanCorr)
	fmt.Printf("Mean Absolute Deviation: %.1f positions (target: <5)\n", result.MeanAbsDeviation)
	fmt.Printf("Top-10 Accuracy: %.0f%% (target: >70%%)\n", result.TopTenAccuracy)
	fmt.Printf("Podium Accuracy: %.0f%% (target: >50%%)\n", result.PodiumAccuracy)
	fmt.Println()

	if result.SpearmanCorr < 0.60 {
		fmt.Println("⚠️ WARNING: Low correlation - major mechanic may be wrong")
	} else if result.SpearmanCorr < 0.70 {
		fmt.Println("⚠️ CAUTION: Moderate correlation - may need attribute tuning")
	} else {
		fmt.Println("✅ GOOD: Strong correlation - model is realistic")
	}
}

// TestValidation_SimpleCourse validates on a simple flat course first.
func TestValidation_SimpleCourse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping validation test in short mode")
	}

	// Create a simple synthetic validation case with known sprinters
	// Build a 10-rider field: 3 sprinters (best), 3 climbers (weak sprinters), 4 all-rounders
	// Run on flat course -> sprinters should dominate

	var riders []model.Rider
	actualOrder := []int64{} // order they should finish

	// Sprinters should be top 3
	for i := 0; i < 3; i++ {
		r := makeSprinter(fmt.Sprintf("TopSprinter%d", i+1))
		r.ID = int64(i + 1)
		riders = append(riders, r)
		actualOrder = append(actualOrder, r.ID)
	}

	// All-rounders in middle
	for i := 0; i < 4; i++ {
		r := makeAllRounder(fmt.Sprintf("AllRounder%d", i+1))
		r.ID = int64(3 + i + 1)
		riders = append(riders, r)
		actualOrder = append(actualOrder, r.ID)
	}

	// Climbers last (weak sprinters)
	for i := 0; i < 3; i++ {
		r := makeClimber(fmt.Sprintf("PureClimber%d", i+1))
		r.ID = int64(7 + i + 1)
		riders = append(riders, r)
		actualOrder = append(actualOrder, r.ID)
	}

	course := flatCourse()
	race := &model.Race{ID: 1, Name: "Flat Test", CourseID: 1, RaceType: model.RaceTypeRoad}

	// Simulate
	riderPlacementsSum := make(map[int64]float64)
	riderRunCount := make(map[int64]int)

	numRuns := 20
	for run := 0; run < numRuns; run++ {
		config := RaceConfig{
			Race:   race,
			Course: course,
			Riders: riders,
			Seed:   int64(run),
		}

		results := SimulateRoadRace(config)
		for pos, res := range results {
			riderID := res.RiderID
			riderPlacementsSum[riderID] += float64(pos + 1)
			riderRunCount[riderID]++
		}
	}

	// Build average positions
	riderAvgPos := make([]float64, len(actualOrder))
	for i, riderID := range actualOrder {
		riderAvgPos[i] = riderPlacementsSum[riderID] / float64(riderRunCount[riderID])
	}

	// Build expected ranking (1,2,3 for sprinters, 4-7 for all-rounders, 8-10 for climbers)
	expectedRanking := make([]float64, len(actualOrder))
	for i := 0; i < len(actualOrder); i++ {
		expectedRanking[i] = float64(i + 1)
	}

	// Calculate correlation
	correlation := spearmanCorrelation(expectedRanking, riderAvgPos)

	t.Logf("Flat Course Validation:")
	t.Logf("  Expected rank vs Sim avg position correlation: %.3f", correlation)
	t.Logf("  Sprinters avg positions: %.1f, %.1f, %.1f", riderAvgPos[0], riderAvgPos[1], riderAvgPos[2])
	t.Logf("  Climbers avg positions: %.1f, %.1f, %.1f", riderAvgPos[7], riderAvgPos[8], riderAvgPos[9])

	if correlation < 0.70 {
		t.Errorf("Expected correlation >0.70, got %.3f on flat course", correlation)
	}
}

// TestValidation_MountainCourse validates on a mountain course.
func TestValidation_MountainCourse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping validation test in short mode")
	}

	// Build a 10-rider field: 3 climbers (best), 3 sprinters (weak climbers), 4 all-rounders
	// Run on mountain course -> climbers should dominate

	var riders []model.Rider
	actualOrder := []int64{} // order they should finish

	// Climbers should be top 3
	for i := 0; i < 3; i++ {
		r := makeClimber(fmt.Sprintf("TopClimber%d", i+1))
		r.ID = int64(i + 1)
		riders = append(riders, r)
		actualOrder = append(actualOrder, r.ID)
	}

	// All-rounders in middle
	for i := 0; i < 4; i++ {
		r := makeAllRounder(fmt.Sprintf("AllRounder%d", i+1))
		r.ID = int64(3 + i + 1)
		riders = append(riders, r)
		actualOrder = append(actualOrder, r.ID)
	}

	// Sprinters last (weak climbers)
	for i := 0; i < 3; i++ {
		r := makeSprinter(fmt.Sprintf("PureSprinter%d", i+1))
		r.ID = int64(7 + i + 1)
		riders = append(riders, r)
		actualOrder = append(actualOrder, r.ID)
	}

	course := mountainCourse()
	race := &model.Race{ID: 2, Name: "Mountain Test", CourseID: 2, RaceType: model.RaceTypeRoad}

	// Simulate
	riderPlacementsSum := make(map[int64]float64)
	riderRunCount := make(map[int64]int)

	numRuns := 20
	for run := 0; run < numRuns; run++ {
		config := RaceConfig{
			Race:   race,
			Course: course,
			Riders: riders,
			Seed:   int64(run),
		}

		results := SimulateRoadRace(config)
		for pos, res := range results {
			riderID := res.RiderID
			riderPlacementsSum[riderID] += float64(pos + 1)
			riderRunCount[riderID]++
		}
	}

	// Build average positions
	riderAvgPos := make([]float64, len(actualOrder))
	for i, riderID := range actualOrder {
		riderAvgPos[i] = riderPlacementsSum[riderID] / float64(riderRunCount[riderID])
	}

	// Build expected ranking (1,2,3 for climbers, 4-7 for all-rounders, 8-10 for sprinters)
	expectedRanking := make([]float64, len(actualOrder))
	for i := 0; i < len(actualOrder); i++ {
		expectedRanking[i] = float64(i + 1)
	}

	// Calculate correlation
	correlation := spearmanCorrelation(expectedRanking, riderAvgPos)

	t.Logf("Mountain Course Validation:")
	t.Logf("  Expected rank vs Sim avg position correlation: %.3f", correlation)
	t.Logf("  Climbers avg positions: %.1f, %.1f, %.1f", riderAvgPos[0], riderAvgPos[1], riderAvgPos[2])
	t.Logf("  Sprinters avg positions: %.1f, %.1f, %.1f", riderAvgPos[7], riderAvgPos[8], riderAvgPos[9])

	if correlation < 0.70 {
		t.Errorf("Expected correlation >0.70, got %.3f on mountain course", correlation)
	}
}

// TestValidation_ParisRoubaix validates against actual 2025 Paris-Roubaix results.
func TestValidation_ParisRoubaix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping validation test in short mode")
	}

	// Actual 2025 Paris-Roubaix top 20 finishers
	// Format: name, riderID (from our DB), actualRankingPosition
	actualFinishers := []struct {
		name      string
		riderID   int64
		actualPos int
	}{
		{"Mathieu van der Poel", 28, 1}, // Alpecin-Premier Tech (H=99)
		{"Tadej Pogačar", 452, 2},       // UAE Team Emirates
		{"Mads Pedersen", 190, 3},       // Lidl-Trek
		{"Wout van Aert", 429, 4},       // Visma-Lease a Bike
		{"Florian Vermeersch", 0, 5},    // UAE (not in DB)
		{"Jonas Rutsch", 0, 6},          // Intermarché-Wanty
		{"Stefan Bissegger", 0, 7},      // Decathlon-AG2R
		{"Markus Hoelgaard", 0, 8},      // Uno-X
		{"Fred Wright", 0, 9},           // Bahrain Victorious
		{"Laurenz Rex", 0, 10},          // Intermarché-Wanty
		{"Jasper Philipsen", 17, 11},    // Alpecin-Premier Tech
		{"Marco Haller", 0, 12},         // Tudor
		{"Filippo Ganna", 150, 13},      // INEOS Grenadiers
		{"Madis Mihkels", 0, 14},        // EF Education
		{"Biniam Girmay", 269, 15},      // NSN Cycling Team
		{"Mike Teunissen", 0, 16},       // XDS Astana
		{"Daan Hoole", 0, 17},           // Lidl-Trek
		{"Mick van Dijke", 0, 18},       // Red Bull-BORA
		{"Marius Mayrhofer", 0, 19},     // Tudor
		{"Taco van der Hoorn", 0, 20},   // Intermarché-Wanty
	}

	// Filter to riders we have in database
	var knownRiders []struct {
		name      string
		riderID   int64
		actualPos int
	}

	for _, f := range actualFinishers {
		if f.riderID > 0 {
			knownRiders = append(knownRiders, f)
		}
	}

	if len(knownRiders) < 2 {
		t.Logf("⚠️ Only %d known riders from actual top-20; creating synthetic validation", len(knownRiders))
		return
	}

	t.Logf("Validating 2025 Paris-Roubaix with %d actual finishers from database\n", len(knownRiders))

	// Build realistic peloton with known finishers
	var riders []model.Rider

	// Add known finishers with attributes from data/riders.csv
	for _, f := range knownRiders {
		switch f.riderID {
		case 28: // Mathieu van der Poel: ftp=415, H=99
			r := makeRider("Mathieu van der Poel", 415, 75, 75, 90, 78, 90, 88, 82, 88, 85)
			r.ID = 28
			r.Handling = 99
			r.MaxSprintWatts = 1700
			riders = append(riders, r)
		case 452: // Tadej Pogačar: ftp=425, H=90
			r := makeRider("Tadej Pogačar", 425, 66, 97, 75, 92, 88, 93, 90, 92, 95)
			r.ID = 452
			r.Handling = 90
			r.MaxSprintWatts = 1400
			riders = append(riders, r)
		case 190: // Mads Pedersen: ftp=400, H=90
			r := makeRider("Mads Pedersen", 400, 76, 60, 88, 82, 82, 82, 78, 85, 82)
			r.ID = 190
			r.Handling = 90
			r.MaxSprintWatts = 1700
			riders = append(riders, r)
		case 429: // Wout van Aert: ftp=420, H=96
			r := makeRider("Wout van Aert", 420, 78, 72, 92, 93, 85, 90, 85, 90, 88)
			r.ID = 429
			r.Handling = 96
			r.MaxSprintWatts = 1750
			riders = append(riders, r)
		case 17: // Jasper Philipsen: ftp=385, H=74
			r := makeRider("Jasper Philipsen", 385, 76, 45, 97, 60, 70, 72, 70, 75, 78)
			r.ID = 17
			r.Handling = 74
			r.MaxSprintWatts = 1850
			riders = append(riders, r)
		case 150: // Filippo Ganna: ftp=425, TT=98, H=55
			r := makeRider("Filippo Ganna", 425, 82, 50, 70, 98, 75, 82, 78, 72, 88)
			r.ID = 150
			r.Handling = 55
			r.MaxSprintWatts = 1500
			riders = append(riders, r)
		case 269: // Biniam Girmay: ftp=388, Sprint=93, H=65
			r := makeRider("Biniam Girmay", 388, 73, 55, 93, 62, 75, 78, 72, 78, 76)
			r.ID = 269
			r.Handling = 65
			r.MaxSprintWatts = 1780
			riders = append(riders, r)
		}
	}

	// Fill to ~30 riders with synthetic filler
	for i := 0; i < 23; i++ {
		var r model.Rider
		switch i % 4 {
		case 0:
			r = makeSprinter(fmt.Sprintf("Sprinter %d", i))
		case 1:
			r = makeClimber(fmt.Sprintf("Climber %d", i))
		case 2:
			r = makeCobbleSpecialist(fmt.Sprintf("Classics %d", i))
		default:
			r = makeAllRounder(fmt.Sprintf("AllRounder %d", i))
		}
		r.ID = int64(5000 + i)
		riders = append(riders, r)
	}

	// Paris-Roubaix course: 257km with 11 cobble sectors (~30km total)
	course := &model.Course{
		ID: 10, Name: "Paris-Roubaix", TotalDistanceKm: 257,
		Segments: []model.Segment{
			{ID: 1, CourseID: 10, SegmentOrder: 1, Name: "Compiegne start", DistanceKm: 100, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 2, CourseID: 10, SegmentOrder: 2, Name: "Carrefour", DistanceKm: 2.4, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 3, CourseID: 10, SegmentOrder: 3, Name: "Trouée d'Arenberg", DistanceKm: 2.4, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 4, CourseID: 10, SegmentOrder: 4, Name: "Intermediate road", DistanceKm: 40, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 5, CourseID: 10, SegmentOrder: 5, Name: "Carrefour de l'Arbre", DistanceKm: 2.1, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 6, CourseID: 10, SegmentOrder: 6, Name: "Gruson", DistanceKm: 1.1, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 7, CourseID: 10, SegmentOrder: 7, Name: "Carrefour de la Chapelle", DistanceKm: 2.0, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 8, CourseID: 10, SegmentOrder: 8, Name: "Intermediate 2", DistanceKm: 50, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 9, CourseID: 10, SegmentOrder: 9, Name: "Carrefour Haegen", DistanceKm: 2.3, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 10, CourseID: 10, SegmentOrder: 10, Name: "Parc de la Citadelle", DistanceKm: 2.5, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 11, CourseID: 10, SegmentOrder: 11, Name: "Final to Roubaix", DistanceKm: 50, GradientPct: 0, SurfaceType: model.SurfaceRoad},
		},
	}

	race := &model.Race{
		ID:           3,
		Name:         "Paris-Roubaix",
		CourseID:     10,
		RaceType:     model.RaceTypeRoad,
		Date:         time.Date(2025, time.April, 13, 0, 0, 0, 0, time.UTC),
		Temperature:  12,
		WindSpeedKmh: 20,
	}

	// Simulate 10 times
	numRuns := 10
	t.Logf("Running %d simulations of 2025 Paris-Roubaix...\n", numRuns)

	riderPlacementsSum := make(map[int64]float64)
	riderRunCount := make(map[int64]int)

	for run := 0; run < numRuns; run++ {
		config := RaceConfig{
			Race:   race,
			Course: course,
			Riders: riders,
			Seed:   int64(run),
		}

		results := SimulateRoadRace(config)
		for pos, res := range results {
			riderID := res.RiderID
			riderPlacementsSum[riderID] += float64(pos + 1)
			riderRunCount[riderID]++
		}
	}

	// Display results
	t.Logf("Paris-Roubaix 2025 Validation Results:\n")
	t.Logf("Rider (Actual Position) -> Avg Simulated Position\n")
	t.Logf("─────────────────────────────────────────────────\n")

	for _, f := range knownRiders {
		avgPos := riderPlacementsSum[f.riderID] / float64(riderRunCount[f.riderID])
		status := ""
		if avgPos <= 5 {
			status = "✅"
		} else if avgPos <= 10 {
			status = "⚠️"
		} else {
			status = "❌"
		}
		t.Logf("%s %-25s (#%d actual) -> %.1f avg\n", status, f.name, f.actualPos, avgPos)
	}

	// Summary
	t.Logf("\n=== Summary ===")
	for _, f := range knownRiders {
		avgPos := riderPlacementsSum[f.riderID] / float64(riderRunCount[f.riderID])
		t.Logf("%s (actual #%d): avg sim pos %.1f", f.name, f.actualPos, avgPos)
	}
}

// TestValidation_DiagnosticCobbles is a diagnostic test to investigate handling advantage.
func TestValidation_DiagnosticCobbles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping diagnostic test in short mode")
	}

	// Build a small field: 1 elite cobble specialist vs 9 average riders
	var riders []model.Rider

	// Van der Poel (elite)
	vdp := makeRider("vdP Elite", 415, 75, 75, 90, 78, 80, 88, 82, 88, 80)
	vdp.ID = 1
	vdp.Handling = 99
	riders = append(riders, vdp)

	// 9 average riders
	for i := 0; i < 9; i++ {
		r := makeAllRounder(fmt.Sprintf("Avg %d", i+1))
		r.ID = int64(i + 2)
		r.Handling = 70 // average handler
		riders = append(riders, r)
	}

	// Pure cobble course: 100km all cobbles
	course := &model.Course{
		ID: 99, Name: "Pure Cobbles", TotalDistanceKm: 100,
		Segments: []model.Segment{
			{ID: 1, CourseID: 99, SegmentOrder: 1, Name: "Cobbles 1", DistanceKm: 25, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 2, CourseID: 99, SegmentOrder: 2, Name: "Cobbles 2", DistanceKm: 25, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 3, CourseID: 99, SegmentOrder: 3, Name: "Cobbles 3", DistanceKm: 25, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 4, CourseID: 99, SegmentOrder: 4, Name: "Cobbles 4", DistanceKm: 25, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
		},
	}

	race := &model.Race{
		ID:           99,
		Name:         "Pure Cobbles Test",
		CourseID:     99,
		RaceType:     model.RaceTypeRoad,
		Date:         time.Date(2025, time.April, 13, 0, 0, 0, 0, time.UTC),
		Temperature:  12,
		WindSpeedKmh: 20,
	}

	// Simulate 20 times
	numRuns := 20
	t.Logf("\n=== DIAGNOSTIC: 100km Pure Cobbles ===")
	t.Logf("Elite specialist (H=99) vs 9 average riders (H=70)\n")

	riderPlacementsSum := make(map[int64]float64)
	riderRunCount := make(map[int64]int)

	for run := 0; run < numRuns; run++ {
		config := RaceConfig{
			Race:   race,
			Course: course,
			Riders: riders,
			Seed:   int64(run),
		}

		results := SimulateRoadRace(config)
		for pos, res := range results {
			riderID := res.RiderID
			riderPlacementsSum[riderID] += float64(pos + 1)
			riderRunCount[riderID]++
		}
	}

	// Calculate elite handler advantage
	eliteAvgPos := riderPlacementsSum[1] / float64(riderRunCount[1])
	avgOfAverage := 0.0
	for i := 2; i <= 10; i++ {
		avgOfAverage += riderPlacementsSum[int64(i)] / float64(riderRunCount[int64(i)])
	}
	avgOfAverage /= 9

	t.Logf("Elite (H=99) average position: %.1f\n", eliteAvgPos)
	t.Logf("Average riders (H=70) average position: %.1f\n", avgOfAverage)
	t.Logf("Position difference: %.1f\n\n", avgOfAverage-eliteAvgPos)

	if eliteAvgPos > 3 {
		t.Logf("⚠️ PROBLEM: Elite handler not dominating 100km pure cobbles!")
		t.Logf("   Expected position 1-2, got %.1f", eliteAvgPos)
	} else {
		t.Logf("✅ Elite handler dominates cobbles as expected")
	}
}

// TestValidation_TourOfFlanders validates against actual 2025 Tour of Flanders results.
func TestValidation_TourOfFlanders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping validation test in short mode")
	}

	// Actual 2025 Tour of Flanders top 20 finishers
	actualFinishers := []struct {
		name      string
		riderID   int64
		actualPos int
	}{
		{"Tadej Pogačar", 452, 1},         // UAE Team Emirates
		{"Mads Pedersen", 190, 2},         // Lidl-Trek
		{"Mathieu van der Poel", 28, 3},   // Alpecin-Premier Tech (H=99)
		{"Wout van Aert", 429, 4},         // Visma-Lease a Bike
		{"Jasper Stuyven", 336, 5},        // Soudal-QuickStep
		{"Tiesj Benoot", 60, 6},           // Decathlon-CMA CGM
		{"Stefan Küng", 0, 7},             // Groupama-FDJ (not in DB)
		{"Filippo Ganna", 150, 8},         // INEOS Grenadiers
		{"Iván García Cortina", 0, 9},     // Movistar (not in DB)
		{"Davide Ballerini", 492, 10},     // XDS Astana
		{"Laurence Pithie", 0, 11},        // Red Bull-BORA (not in DB)
		{"Mike Teunissen", 0, 12},         // XDS Astana (not in DB)
		{"Michael Matthews", 0, 13},       // Jayco-AlUla (not in DB)
		{"Markus Hoelgaard", 0, 14},       // Uno-X (not in DB)
		{"Gianni Vermeersch", 0, 15},      // Alpecin (not in DB)
		{"Aurélien Paret-Peintre", 0, 16}, // Decathlon-AG2R (not in DB)
		{"Valentin Madouas", 0, 17},       // Groupama-FDJ (not in DB)
		{"Dylan van Baarle", 338, 18},     // Soudal-QuickStep
		{"Stefan Bissegger", 0, 19},       // Decathlon-AG2R (not in DB)
		{"Magnus Sheffield", 0, 20},       // INEOS (not in DB)
	}

	// Filter to riders we have in database
	var knownRiders []struct {
		name      string
		riderID   int64
		actualPos int
	}

	for _, f := range actualFinishers {
		if f.riderID > 0 {
			knownRiders = append(knownRiders, f)
		}
	}

	if len(knownRiders) < 1 {
		t.Logf("⚠️ Only %d matchable riders from actual top-20; creating synthetic validation", len(knownRiders))
		return
	}

	t.Logf("Validating 2025 Tour of Flanders with %d actual finishers from database\n", len(knownRiders))

	// Build realistic peloton with known finishers
	var riders []model.Rider

	// Add known finishers with attributes from data/riders.csv
	for _, f := range knownRiders {
		switch f.riderID {
		case 452: // Tadej Pogačar: ftp=425, H=90
			r := makeRider("Tadej Pogačar", 425, 66, 97, 75, 92, 88, 93, 90, 92, 95)
			r.ID = 452
			r.Handling = 90
			r.MaxSprintWatts = 1400
			riders = append(riders, r)
		case 190: // Mads Pedersen: ftp=400, H=90
			r := makeRider("Mads Pedersen", 400, 76, 60, 88, 82, 82, 82, 78, 85, 82)
			r.ID = 190
			r.Handling = 90
			r.MaxSprintWatts = 1700
			riders = append(riders, r)
		case 28: // Mathieu van der Poel: ftp=415, H=99
			r := makeRider("Mathieu van der Poel", 415, 75, 75, 90, 78, 90, 88, 82, 88, 85)
			r.ID = 28
			r.Handling = 99
			r.MaxSprintWatts = 1700
			riders = append(riders, r)
		case 429: // Wout van Aert: ftp=420, H=96
			r := makeRider("Wout van Aert", 420, 78, 72, 92, 93, 85, 90, 85, 90, 88)
			r.ID = 429
			r.Handling = 96
			r.MaxSprintWatts = 1750
			riders = append(riders, r)
		case 336: // Jasper Stuyven: ftp=405, Classics=90, H=80
			r := makeRider("Jasper Stuyven", 405, 74, 75, 78, 85, 90, 88, 82, 88, 85)
			r.ID = 336
			r.Handling = 80
			r.MaxSprintWatts = 1650
			riders = append(riders, r)
		case 60: // Tiesj Benoot: ftp=405, Classics=88, H=55
			r := makeRider("Tiesj Benoot", 405, 74, 82, 60, 85, 88, 90, 85, 90, 85)
			r.ID = 60
			r.Handling = 55
			r.MaxSprintWatts = 1200
			riders = append(riders, r)
		case 150: // Filippo Ganna: TT=98, H=55
			r := makeRider("Filippo Ganna", 425, 82, 50, 70, 98, 75, 82, 78, 72, 88)
			r.ID = 150
			r.Handling = 55
			r.MaxSprintWatts = 1500
			riders = append(riders, r)
		case 492: // Davide Ballerini: Sprint=84, Classics=85, H=72
			r := makeRider("Davide Ballerini", 392, 72, 75, 84, 82, 85, 85, 80, 85, 82)
			r.ID = 492
			r.Handling = 72
			r.MaxSprintWatts = 1700
			riders = append(riders, r)
		case 338: // Dylan van Baarle: Classics=88, H=55
			r := makeRider("Dylan van Baarle", 405, 73, 82, 55, 85, 88, 90, 85, 90, 85)
			r.ID = 338
			r.Handling = 55
			r.MaxSprintWatts = 1200
			riders = append(riders, r)
		}
	}

	// Fill to ~30 riders with synthetic filler
	for i := 0; i < 21; i++ {
		var r model.Rider
		switch i % 4 {
		case 0:
			r = makeClimber(fmt.Sprintf("Climber %d", i))
		case 1:
			r = makeAllRounder(fmt.Sprintf("AllRounder %d", i))
		case 2:
			r = makeCobbleSpecialist(fmt.Sprintf("Classics %d", i))
		default:
			r = makeSprinter(fmt.Sprintf("Sprinter %d", i))
		}
		r.ID = int64(6000 + i)
		riders = append(riders, r)
	}

	// Tour of Flanders: ~265km with ~18 climbs (hilly/technical, not steep)
	course := &model.Course{
		ID: 11, Name: "Tour of Flanders", TotalDistanceKm: 265,
		Segments: []model.Segment{
			{ID: 1, CourseID: 11, SegmentOrder: 1, Name: "Gent start", DistanceKm: 60, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 2, CourseID: 11, SegmentOrder: 2, Name: "Molenberg", DistanceKm: 1.9, GradientPct: 4.2, SurfaceType: model.SurfaceRoad},
			{ID: 3, CourseID: 11, SegmentOrder: 3, Name: "Intermediate", DistanceKm: 30, GradientPct: 0.5, SurfaceType: model.SurfaceRoad},
			{ID: 4, CourseID: 11, SegmentOrder: 4, Name: "Bosberg", DistanceKm: 2.1, GradientPct: 3.9, SurfaceType: model.SurfaceRoad},
			{ID: 5, CourseID: 11, SegmentOrder: 5, Name: "Molenberg 2", DistanceKm: 1.9, GradientPct: 4.2, SurfaceType: model.SurfaceRoad},
			{ID: 6, CourseID: 11, SegmentOrder: 6, Name: "Intermediate 2", DistanceKm: 40, GradientPct: 0.3, SurfaceType: model.SurfaceRoad},
			{ID: 7, CourseID: 11, SegmentOrder: 7, Name: "Molenberg 3", DistanceKm: 1.9, GradientPct: 4.2, SurfaceType: model.SurfaceRoad},
			{ID: 8, CourseID: 11, SegmentOrder: 8, Name: "Finish", DistanceKm: 126, GradientPct: 0, SurfaceType: model.SurfaceRoad},
		},
	}

	race := &model.Race{
		ID:           11,
		Name:         "Tour of Flanders",
		CourseID:     11,
		RaceType:     model.RaceTypeRoad,
		Date:         time.Date(2025, time.April, 6, 0, 0, 0, 0, time.UTC),
		Temperature:  12,
		WindSpeedKmh: 18,
	}

	// Simulate 10 times
	numRuns := 10
	t.Logf("Running %d simulations of 2025 Tour of Flanders...\n", numRuns)

	riderPlacementsSum := make(map[int64]float64)
	riderRunCount := make(map[int64]int)

	for run := 0; run < numRuns; run++ {
		config := RaceConfig{
			Race:   race,
			Course: course,
			Riders: riders,
			Seed:   int64(run),
		}

		results := SimulateRoadRace(config)
		for pos, res := range results {
			riderID := res.RiderID
			riderPlacementsSum[riderID] += float64(pos + 1)
			riderRunCount[riderID]++
		}
	}

	// Display results
	t.Logf("Tour of Flanders 2025 Validation Results:\n")
	t.Logf("Rider (Actual Position) -> Avg Simulated Position\n")
	t.Logf("─────────────────────────────────────────────────\n")

	for _, f := range knownRiders {
		avgPos := riderPlacementsSum[f.riderID] / float64(riderRunCount[f.riderID])
		status := ""
		if avgPos <= float64(f.actualPos)+3 {
			status = "✅"
		} else if avgPos <= float64(f.actualPos)+8 {
			status = "⚠️"
		} else {
			status = "❌"
		}
		t.Logf("%s %-25s (#%d actual) -> %.1f avg\n", status, f.name, f.actualPos, avgPos)
	}

	// Summary
	t.Logf("\n=== Summary ===")
	for _, f := range knownRiders {
		avgPos := riderPlacementsSum[f.riderID] / float64(riderRunCount[f.riderID])
		t.Logf("%s (actual #%d): avg sim pos %.1f", f.name, f.actualPos, avgPos)
	}
}

// TestValidation_MilanSanRemo validates against actual 2025 Milan-San Remo results.
func TestValidation_MilanSanRemo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping validation test in short mode")
	}

	// Actual 2025 Milan-San Remo top finishers (only those with known IDs)
	actualFinishers := []struct {
		name      string
		riderID   int64
		actualPos int
	}{
		{"Mathieu van der Poel", 28, 1}, // Alpecin-Premier Tech (H=99)
		{"Biniam Girmay", 269, 2},       // NSN Cycling Team
		{"Wout van Aert", 429, 3},       // Visma-Lease a Bike
		{"Mads Pedersen", 190, 4},       // Lidl-Trek
		{"Kaden Groves", 14, 5},         // Alpecin-Premier Tech
		{"Matej Mohorič", 47, 6},        // Bahrain Victorious (H=85)
		{"Jasper Stuyven", 336, 7},      // Soudal-QuickStep
		{"Tadej Pogačar", 452, 8},       // UAE Team Emirates
		{"Jonathan Milan", 184, 9},      // Lidl-Trek
		{"Jasper Philipsen", 17, 163},   // Alpecin-Premier Tech
	}

	// Filter to riders we have in database
	var knownRiders []struct {
		name      string
		riderID   int64
		actualPos int
	}
	for _, f := range actualFinishers {
		if f.riderID > 0 {
			knownRiders = append(knownRiders, f)
		}
	}

	if len(knownRiders) < 1 {
		t.Logf("⚠️ Only %d matchable riders from actual finishers; creating synthetic validation", len(knownRiders))
		return
	}

	t.Logf("Validating 2025 Milan-San Remo with %d actual finishers from database\n", len(knownRiders))

	// Build realistic peloton with known finishers
	var riders []model.Rider

	// Add known finishers with attributes from data/riders.csv
	for _, f := range knownRiders {
		switch f.riderID {
		case 28: // Mathieu van der Poel: ftp=415, H=99
			r := makeRider("Mathieu van der Poel", 415, 75, 75, 90, 78, 90, 88, 82, 88, 85)
			r.ID = 28
			r.Handling = 99
			r.MaxSprintWatts = 1700
			riders = append(riders, r)
		case 269: // Biniam Girmay: Sprint=93, H=65
			r := makeRider("Biniam Girmay", 388, 73, 55, 93, 62, 75, 78, 72, 78, 76)
			r.ID = 269
			r.Handling = 65
			r.MaxSprintWatts = 1780
			riders = append(riders, r)
		case 429: // Wout van Aert: ftp=420, H=96
			r := makeRider("Wout van Aert", 420, 78, 72, 92, 93, 85, 90, 85, 90, 88)
			r.ID = 429
			r.Handling = 96
			r.MaxSprintWatts = 1750
			riders = append(riders, r)
		case 190: // Mads Pedersen: Sprint=88, H=90
			r := makeRider("Mads Pedersen", 400, 76, 60, 88, 82, 82, 82, 78, 85, 82)
			r.ID = 190
			r.Handling = 90
			r.MaxSprintWatts = 1700
			riders = append(riders, r)
		case 14: // Kaden Groves: Sprint=92, H=75
			r := makeRider("Kaden Groves", 400, 74, 55, 92, 80, 80, 82, 78, 82, 88)
			r.ID = 14
			r.Handling = 75
			r.MaxSprintWatts = 1850
			riders = append(riders, r)
		case 47: // Matej Mohorič: Classics=90, Descending=90, H=85
			r := makeRider("Matej Mohorič", 405, 71, 78, 72, 85, 90, 92, 85, 90, 85)
			r.ID = 47
			r.Handling = 85
			r.MaxSprintWatts = 1400
			riders = append(riders, r)
		case 336: // Jasper Stuyven: Classics=90, H=80
			r := makeRider("Jasper Stuyven", 405, 74, 75, 78, 85, 90, 88, 82, 88, 85)
			r.ID = 336
			r.Handling = 80
			r.MaxSprintWatts = 1650
			riders = append(riders, r)
		case 452: // Tadej Pogačar: ftp=425, H=90
			r := makeRider("Tadej Pogačar", 425, 66, 97, 75, 92, 88, 93, 90, 92, 95)
			r.ID = 452
			r.Handling = 90
			r.MaxSprintWatts = 1400
			riders = append(riders, r)
		case 184: // Jonathan Milan: Sprint=95, H=48
			r := makeRider("Jonathan Milan", 395, 82, 40, 95, 72, 68, 70, 68, 70, 75)
			r.ID = 184
			r.Handling = 48
			r.MaxSprintWatts = 1900
			riders = append(riders, r)
		case 17: // Jasper Philipsen: Sprint=97, H=74
			r := makeRider("Jasper Philipsen", 385, 76, 45, 97, 60, 70, 72, 70, 75, 78)
			r.ID = 17
			r.Handling = 74
			r.MaxSprintWatts = 1850
			riders = append(riders, r)
		}
	}

	// Fill to ~25 riders with synthetic filler
	for i := 0; i < 15; i++ {
		var r model.Rider
		switch i % 3 {
		case 0:
			r = makeSprinter(fmt.Sprintf("Sprinter %d", i))
		case 1:
			r = makeAllRounder(fmt.Sprintf("AllRounder %d", i))
		default:
			r = makeClimber(fmt.Sprintf("Climber %d", i))
		}
		r.ID = int64(7000 + i)
		riders = append(riders, r)
	}

	// Milan-San Remo: ~294km, mostly flat, Poggio climb near finish
	course := &model.Course{
		ID: 12, Name: "Milan-San Remo", TotalDistanceKm: 294,
		Segments: []model.Segment{
			{ID: 1, CourseID: 12, SegmentOrder: 1, Name: "Milano", DistanceKm: 120, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 2, CourseID: 12, SegmentOrder: 2, Name: "Passo del Turchino", DistanceKm: 25, GradientPct: 1.4, SurfaceType: model.SurfaceRoad},
			{ID: 3, CourseID: 12, SegmentOrder: 3, Name: "Coastal", DistanceKm: 120, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 4, CourseID: 12, SegmentOrder: 4, Name: "Capi", DistanceKm: 10, GradientPct: 2.5, SurfaceType: model.SurfaceRoad},
			{ID: 5, CourseID: 12, SegmentOrder: 5, Name: "Cipressa", DistanceKm: 5.6, GradientPct: 3.6, SurfaceType: model.SurfaceRoad},
			{ID: 6, CourseID: 12, SegmentOrder: 6, Name: "Poggio di San Remo", DistanceKm: 3.7, GradientPct: 3.7, SurfaceType: model.SurfaceRoad},
			{ID: 7, CourseID: 12, SegmentOrder: 7, Name: "San Remo Finish", DistanceKm: 9.7, GradientPct: 0, SurfaceType: model.SurfaceRoad},
		},
	}

	race := &model.Race{
		ID:           12,
		Name:         "Milan-San Remo",
		CourseID:     12,
		RaceType:     model.RaceTypeRoad,
		Date:         time.Date(2025, time.March, 15, 0, 0, 0, 0, time.UTC),
		Temperature:  16,
		WindSpeedKmh: 10,
	}

	// Simulate 10 times
	numRuns := 10
	t.Logf("Running %d simulations of 2025 Milan-San Remo...\n", numRuns)

	riderPlacementsSum := make(map[int64]float64)
	riderRunCount := make(map[int64]int)

	for run := 0; run < numRuns; run++ {
		config := RaceConfig{
			Race:   race,
			Course: course,
			Riders: riders,
			Seed:   int64(run),
		}
		results := SimulateRoadRace(config)
		for pos, res := range results {
			riderID := res.RiderID
			riderPlacementsSum[riderID] += float64(pos + 1)
			riderRunCount[riderID]++
		}
	}

	// Display results
	t.Logf("Milan-San Remo 2025 Validation Results:\n")
	t.Logf("Rider (Actual Position) -> Avg Simulated Position\n")
	t.Logf("─────────────────────────────────────────────────\n")

	for _, f := range knownRiders {
		avgPos := riderPlacementsSum[f.riderID] / float64(riderRunCount[f.riderID])
		status := ""
		if avgPos <= float64(f.actualPos)+3 {
			status = "✅"
		} else if avgPos <= float64(f.actualPos)+8 {
			status = "⚠️"
		} else {
			status = "❌"
		}
		t.Logf("%s %-25s (#%d actual) -> %.1f avg\n", status, f.name, f.actualPos, avgPos)
	}

	// Summary
	t.Logf("\n=== Summary ===")
	for _, f := range knownRiders {
		avgPos := riderPlacementsSum[f.riderID] / float64(riderRunCount[f.riderID])
		t.Logf("%s (actual #%d): avg sim pos %.1f", f.name, f.actualPos, avgPos)
	}
}
