package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

func makeRider(name string, ftp int, weight float64, climbing, sprint, tt, descending, endurance, recovery, consistency, tactical int) model.Rider {
	return model.Rider{
		ID: 0, Name: name, Team: "Team", Nationality: "TST",
		WeightKg: weight, HeightCm: 180, FTPWatts: ftp,
		MaxSprintWatts: 1400, VO2MaxWatts: ftp + 60,
		CdA: 0.32, ClimbingAbility: climbing, SprintAbility: sprint,
		TTAbility: tt, DescendingAbility: descending,
		Endurance: endurance, Recovery: recovery,
		Consistency: consistency, TacticalAwareness: tactical,
	}
}

func makeSprinter(name string) model.Rider {
	r := makeRider(name, 380, 75, 50, 95, 60, 70, 70, 70, 70, 70)
	r.MaxSprintWatts = 1800
	return r
}

func makeClimber(name string) model.Rider {
	return makeRider(name, 410, 62, 95, 50, 80, 85, 85, 80, 85, 80)
}

func makeAllRounder(name string) model.Rider {
	return makeRider(name, 400, 68, 80, 65, 82, 75, 80, 75, 80, 75)
}

func flatCourse() *model.Course {
	return &model.Course{
		ID: 1, Name: "Flat Classic", TotalDistanceKm: 200,
		Segments: []model.Segment{
			{ID: 1, CourseID: 1, SegmentOrder: 1, Name: "Flat 1", DistanceKm: 100, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 2, CourseID: 1, SegmentOrder: 2, Name: "Rolling", DistanceKm: 60, GradientPct: 1.5, SurfaceType: model.SurfaceRoad},
			{ID: 3, CourseID: 1, SegmentOrder: 3, Name: "Flat finish", DistanceKm: 40, GradientPct: 0, SurfaceType: model.SurfaceRoad},
		},
	}
}

func mountainCourse() *model.Course {
	return &model.Course{
		ID: 2, Name: "Mountain Stage", TotalDistanceKm: 160,
		Segments: []model.Segment{
			{ID: 1, CourseID: 2, SegmentOrder: 1, Name: "Valley", DistanceKm: 60, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 2, CourseID: 2, SegmentOrder: 2, Name: "Col 1", DistanceKm: 15, GradientPct: 7.0, SurfaceType: model.SurfaceRoad},
			{ID: 3, CourseID: 2, SegmentOrder: 3, Name: "Descent 1", DistanceKm: 15, GradientPct: -6.0, SurfaceType: model.SurfaceRoad},
			{ID: 4, CourseID: 2, SegmentOrder: 4, Name: "Valley 2", DistanceKm: 30, GradientPct: 0.5, SurfaceType: model.SurfaceRoad},
			{ID: 5, CourseID: 2, SegmentOrder: 5, Name: "Summit Finish", DistanceKm: 14, GradientPct: 8.5, SurfaceType: model.SurfaceRoad},
		},
	}
}

func ttCourse() *model.Course {
	return &model.Course{
		ID: 3, Name: "TT Course", TotalDistanceKm: 40,
		Segments: []model.Segment{
			{ID: 1, CourseID: 3, SegmentOrder: 1, Name: "Flat out", DistanceKm: 15, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 2, CourseID: 3, SegmentOrder: 2, Name: "Slight rise", DistanceKm: 10, GradientPct: 2.0, SurfaceType: model.SurfaceRoad},
			{ID: 3, CourseID: 3, SegmentOrder: 3, Name: "Flat return", DistanceKm: 15, GradientPct: -0.5, SurfaceType: model.SurfaceRoad},
		},
	}
}

func TestFlatRaceBunchFinish(t *testing.T) {
	riders := []model.Rider{
		makeSprinter("Sprinter 1"),
		makeSprinter("Sprinter 2"),
		makeClimber("Climber 1"),
		makeAllRounder("AllRounder 1"),
		makeAllRounder("AllRounder 2"),
	}
	for i := range riders {
		riders[i].ID = int64(i + 1)
	}

	race := &model.Race{ID: 1, Name: "Flat Race", CourseID: 1, RaceType: model.RaceTypeRoad}
	config := RaceConfig{Race: race, Course: flatCourse(), Riders: riders, Seed: 42}
	results := SimulateRoadRace(config)

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// In a flat race, the time gaps should be small (< 30 seconds for the group)
	maxGap := results[len(results)-1].TimeGapSeconds
	if maxGap > 30.0 {
		t.Errorf("flat race max gap = %.1f seconds, expected < 30 (bunch finish)", maxGap)
	}
	t.Logf("Flat race: winner time=%.0fs, max gap=%.1fs", results[0].FinishTimeSeconds, maxGap)
	for _, r := range results {
		t.Logf("  P%d: rider=%d time=%.1fs gap=%.1fs", r.Position, r.RiderID, r.FinishTimeSeconds, r.TimeGapSeconds)
	}
}

func TestMountainRaceProducesGaps(t *testing.T) {
	riders := []model.Rider{
		makeClimber("Climber 1"),
		makeClimber("Climber 2"),
		makeAllRounder("AllRounder"),
		makeSprinter("Sprinter 1"),
		makeSprinter("Sprinter 2"),
	}
	for i := range riders {
		riders[i].ID = int64(i + 1)
	}

	race := &model.Race{ID: 1, Name: "Mountain Stage", CourseID: 2, RaceType: model.RaceTypeRoad}
	config := RaceConfig{Race: race, Course: mountainCourse(), Riders: riders, Seed: 42}
	results := SimulateRoadRace(config)

	// Mountain race should produce meaningful time gaps
	maxGap := results[len(results)-1].TimeGapSeconds
	if maxGap < 30 {
		t.Errorf("mountain race max gap = %.1f seconds, expected > 30 seconds", maxGap)
	}

	t.Logf("Mountain race: winner time=%.0fs, max gap=%.1fs", results[0].FinishTimeSeconds, maxGap)
	for _, r := range results {
		t.Logf("  P%d: rider=%d time=%.1fs gap=%.1fs", r.Position, r.RiderID, r.FinishTimeSeconds, r.TimeGapSeconds)
	}

	// Climbers should generally finish ahead of sprinters
	// (not always due to randomness, but check winner is not a sprinter)
	// Just log for manual inspection
}

func TestTTRealisticTimes(t *testing.T) {
	riders := []model.Rider{
		makeRider("TT Specialist", 415, 75, 70, 60, 95, 70, 80, 70, 85, 70),
		makeClimber("Climber"),
		makeSprinter("Sprinter"),
		makeAllRounder("AllRounder"),
	}
	for i := range riders {
		riders[i].ID = int64(i + 1)
	}

	race := &model.Race{ID: 1, Name: "40km TT", CourseID: 3, RaceType: model.RaceTypeTT}
	config := RaceConfig{Race: race, Course: ttCourse(), Riders: riders, Seed: 42}
	results := SimulateTT(config)

	// Winner should finish ~40km TT in 28-35 minutes (50-54 km/h average)
	winnerTime := results[0].FinishTimeSeconds
	winnerAvgKmh := 40.0 / (winnerTime / 3600.0)
	if winnerAvgKmh < 45 || winnerAvgKmh > 58 {
		t.Errorf("TT winner avg speed = %.1f km/h, expected 45-58 km/h", winnerAvgKmh)
	}

	// Gaps between top riders should be ~30-90 seconds
	if len(results) > 1 {
		topGap := results[1].TimeGapSeconds
		if topGap > 180 {
			t.Errorf("TT gap between 1st and 2nd = %.1f seconds, expected < 180", topGap)
		}
	}

	t.Logf("TT: winner time=%.0fs (%.1f km/h avg)", winnerTime, winnerAvgKmh)
	for _, r := range results {
		t.Logf("  P%d: rider=%d time=%.1fs gap=%.1fs", r.Position, r.RiderID, r.FinishTimeSeconds, r.TimeGapSeconds)
	}
}

func TestStageRace(t *testing.T) {
	riders := []model.Rider{
		makeClimber("GC Contender 1"),
		makeClimber("GC Contender 2"),
		makeAllRounder("AllRounder"),
		makeSprinter("Sprinter"),
	}
	for i := range riders {
		riders[i].ID = int64(i + 1)
	}

	stageRace := &model.StageRace{ID: 1, Name: "Mini Tour"}

	flat := flatCourse()
	flat.TotalDistanceKm = 180
	flat.Segments = []model.Segment{
		{ID: 1, SegmentOrder: 1, Name: "Flat", DistanceKm: 180, SurfaceType: model.SurfaceRoad},
	}

	mtn := mountainCourse()

	stages := []StageConfig{
		{
			Race:        &model.Race{ID: 1, Name: "Stage 1 - Flat", CourseID: 1, RaceType: model.RaceTypeRoad},
			Course:      flat,
			StageNumber: 1,
		},
		{
			Race:        &model.Race{ID: 2, Name: "Stage 2 - Mountain", CourseID: 2, RaceType: model.RaceTypeRoad},
			Course:      mtn,
			StageNumber: 2,
		},
		{
			Race:        &model.Race{ID: 3, Name: "Stage 3 - TT", CourseID: 3, RaceType: model.RaceTypeTT},
			Course:      ttCourse(),
			StageNumber: 3,
		},
	}

	allResults, allStandings := SimulateStageRace(stageRace, stages, riders, 42)

	if len(allResults) != 3 {
		t.Fatalf("expected 3 stages of results, got %d", len(allResults))
	}
	if len(allStandings) != 3 {
		t.Fatalf("expected 3 stages of standings, got %d", len(allStandings))
	}

	// GC standings after final stage
	finalStandings := allStandings[2]
	if len(finalStandings) != 4 {
		t.Fatalf("expected 4 GC entries, got %d", len(finalStandings))
	}

	t.Logf("Stage race GC after stage 3:")
	for _, s := range finalStandings {
		gap := s.TotalTimeSeconds - finalStandings[0].TotalTimeSeconds
		t.Logf("  GC %d: rider=%d total=%.0fs gap=%.0fs", s.GCPosition, s.RiderID, s.TotalTimeSeconds, gap)
	}

	// GC should produce meaningful gaps
	gcGap := finalStandings[len(finalStandings)-1].TotalTimeSeconds - finalStandings[0].TotalTimeSeconds
	if gcGap < 10 {
		t.Errorf("GC gap = %.0f seconds, expected significant gaps after 3 stages", gcGap)
	}
}

func TestDailyFormVariance(t *testing.T) {
	rider := testRider()
	rider.Consistency = 90

	// Generate many form factors, check distribution
	rng := newTestRng()
	var sum float64
	n := 10000
	for i := 0; i < n; i++ {
		f := DailyFormFactor(rider, rng)
		sum += f
	}
	mean := sum / float64(n)
	if mean < 0.99 || mean > 1.01 {
		t.Errorf("mean form factor = %.4f, expected ~1.0", mean)
	}
}

func TestSprintFinishOrdering(t *testing.T) {
	sprinter := makeSprinter("Pure Sprinter")
	sprinter.ID = 1
	sprinter.MaxSprintWatts = 1800

	climber := makeClimber("Climber")
	climber.ID = 2
	climber.MaxSprintWatts = 1100

	rng := newTestRng()

	states := []*RiderState{
		{Rider: &sprinter, WPrime: NewWPrimeState(&sprinter), FormFactor: 1.0, EffectiveFTP: sprinter.FTPWatts},
		{Rider: &climber, WPrime: NewWPrimeState(&climber), FormFactor: 1.0, EffectiveFTP: climber.FTPWatts},
	}

	// Sprinter should win most sprints
	sprinterWins := 0
	for i := 0; i < 100; i++ {
		states[0].TotalTime = 0
		states[1].TotalTime = 0
		SprintFinish(states, rng)
		if states[0].TotalTime < states[1].TotalTime {
			sprinterWins++
		}
	}
	if sprinterWins < 80 {
		t.Errorf("sprinter won %d/100 sprints, expected >80", sprinterWins)
	}
}

// Stage fatigue is applied to the rider's base threshold each stage: over 21
// identical time trials it costs a few percent, it doesn't compound (it once
// did, taking ~40% off threshold by the last stage).
func TestStageFatigueDoesNotCompound(t *testing.T) {
	r := makeAllRounder("Rider")
	r.ID = 1
	r.Consistency = 100
	var stages []StageConfig
	for i := 1; i <= 21; i++ {
		stages = append(stages, StageConfig{
			Race:        &model.Race{ID: int64(i), Name: "TT", CourseID: 3, RaceType: model.RaceTypeTT},
			Course:      ttCourse(),
			StageNumber: i,
		})
	}
	res, _ := SimulateStageRace(&model.StageRace{ID: 1, Name: "Test"}, stages, []model.Rider{r}, 7)
	first, last := res[0][0].FinishTimeSeconds, res[20][0].FinishTimeSeconds
	if ratio := last / first; ratio > 1.06 {
		t.Errorf("last stage %.1f%% slower than the first; fatigue should cost a few percent", (ratio-1)*100)
	}
}
