package simulation

import (
	"fmt"
	"math"
	"sort"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
)

// ValidationSummary captures a quick validation result for the UI.
type ValidationSummary struct {
	Name                 string
	Passed               bool
	Correlation          float64
	Top10Accuracy        float64
	MeanTimeGapErrorSecs float64 // MAE of time gaps for matched riders (0 if N/A)
	MatchedRiders        int     // number of riders matched between reference and internal DB
	IsRealRace           bool    // true when backed by real FirstCycling reference data
	Details              string
	// Riders compares each matched reference rider with the simulation, and
	// Intruders lists riders who often finish top 10 in the simulation but
	// were not in the real top 10 (real-race cases only).
	Riders    []RiderCheck `json:",omitempty"`
	Intruders []RiderCheck `json:",omitempty"`
}

// RiderCheck is one rider's real result next to their simulated average.
type RiderCheck struct {
	RiderID  int64
	RealPos  int     // 0 when not in the real top 10
	RealGap  float64 // gapUnknown when unknown
	SimPos   float64 // average simulated position
	SimGap   float64 // average simulated gap (s)
	Top10Pct float64 // share of runs finishing top 10
	WinPct   float64
}

// ValidationSeedBase offsets every validation seed (run i uses base+i+1).
// Different bases give independent seed sets, to tell a real change from
// noise (see RunSeedSets).
var ValidationSeedBase int64

// SeedSetResult is one reference race scored over several seed sets.
type SeedSetResult struct {
	Name         string
	MAE          []float64 // gap error per seed set
	RealWinnerPc float64   // mean share of runs the real winner wins
}

// RunSeedSets scores every reference race (2024 and the 2023 hold-out) over
// sets independent seed sets of the usual run counts.
func RunSeedSets(db *database.DB, sets int) []SeedSetResult {
	riders, err := db.ListRiders()
	if err != nil || len(riders) == 0 {
		return nil
	}
	defer func(b int64) { ValidationSeedBase = b }(ValidationSeedBase)
	var out []SeedSetResult
	index := map[string]int{}
	for set := 0; set < sets; set++ {
		ValidationSeedBase = int64(set) * 1000
		var rows []ValidationSummary
		rows = append(rows, RunRealRaceValidationFor(db, riders, KnownRealRaces(), realValidationRuns)...)
		rows = append(rows, RunRealStageRaceValidationFor(db, riders, KnownRealStageRaces(), realStageRaceRuns)...)
		rows = append(rows, RunRealRaceValidationFor(db, riders, KnownRealRaces2023(), realValidationRuns)...)
		rows = append(rows, RunRealStageRaceValidationFor(db, riders, KnownRealStageRaces2023(), realStageRaceRuns)...)
		for _, r := range rows {
			i, ok := index[r.Name]
			if !ok {
				i = len(out)
				index[r.Name] = i
				out = append(out, SeedSetResult{Name: r.Name})
			}
			out[i].MAE = append(out[i].MAE, r.MeanTimeGapErrorSecs)
			for _, c := range r.Riders {
				if c.RealPos == 1 {
					out[i].RealWinnerPc += c.WinPct / float64(sets)
				}
			}
		}
	}
	return out
}

// RunValidationSuite runs lightweight validation checks for the UI.
func RunValidationSuite() []ValidationSummary {
	results := []ValidationSummary{
		runFlatSprintValidation(),
		runMountainClimbValidation(),
		runTTValidation(),
	}
	return results
}

func runFlatSprintValidation() ValidationSummary {
	return runCategoryValidation(validationCaseConfig{
		Name:           "Flat sprint dominance",
		RaceType:       model.RaceTypeRoad,
		Course:         validationFlatCourse(),
		NumRuns:        20,
		ExpectedTopIDs: buildGroupIDs(1, 10),
		// Sprinters first; behind them all-rounders and climbers share the same
		// sprint power, so their order depends on who did the team's work.
		ExpectedTiers:     [][]int64{buildGroupIDs(1, 10), append(buildGroupIDs(11, 10), buildGroupIDs(21, 10)...)},
		CorrelationTarget: 0.65,
		Top10Target:       60,
		Riders:            buildFlatField(),
	})
}

func runMountainClimbValidation() ValidationSummary {
	return runCategoryValidation(validationCaseConfig{
		Name:              "Mountain climber dominance",
		RaceType:          model.RaceTypeRoad,
		Course:            validationMountainCourse(),
		NumRuns:           20,
		ExpectedTopIDs:    buildGroupIDs(11, 10),
		ExpectedOrderIDs:  append(buildGroupIDs(11, 10), append(buildGroupIDs(21, 10), buildGroupIDs(1, 10)...)...),
		CorrelationTarget: 0.65,
		Top10Target:       60,
		Riders:            buildMountainField(),
	})
}

func runTTValidation() ValidationSummary {
	return runCategoryValidation(validationCaseConfig{
		Name:              "TT specialists dominate",
		RaceType:          model.RaceTypeTT,
		Course:            validationTTCourse(),
		NumRuns:           20,
		ExpectedTopIDs:    buildGroupIDs(21, 10),
		ExpectedOrderIDs:  append(buildGroupIDs(21, 10), append(buildGroupIDs(11, 10), buildGroupIDs(1, 10)...)...),
		CorrelationTarget: 0.6,
		Top10Target:       50,
		Riders:            buildTTField(),
	})
}

type validationCaseConfig struct {
	Name              string
	RaceType          model.RaceType
	Course            *model.Course
	NumRuns           int
	ExpectedTopIDs    []int64
	ExpectedOrderIDs  []int64
	ExpectedTiers     [][]int64 // alternative to ExpectedOrderIDs: ties within a tier
	CorrelationTarget float64
	Top10Target       float64
	Riders            []model.Rider
}

func runCategoryValidation(cfg validationCaseConfig) ValidationSummary {
	race := &model.Race{ID: 1, Name: cfg.Name, CourseID: 1, RaceType: cfg.RaceType}
	avgPositions := simulateAveragePositions(race, cfg.Course, cfg.Riders, cfg.NumRuns)

	simOrder := rankByAveragePosition(avgPositions)
	top10 := topNAccuracy(cfg.ExpectedTopIDs, simOrder, 10)
	corr := validationSpearman(cfg.ExpectedOrderIDs, avgPositions)
	if cfg.ExpectedTiers != nil {
		corr = validationTierCorrelation(cfg.ExpectedTiers, avgPositions)
	}

	passed := corr >= cfg.CorrelationTarget && top10 >= cfg.Top10Target
	details := fmt.Sprintf("Corr %.2f (>=%.2f), Top-10 %.0f%% (>=%.0f%%)", corr, cfg.CorrelationTarget, top10, cfg.Top10Target)

	return ValidationSummary{
		Name:          cfg.Name,
		Passed:        passed,
		Correlation:   corr,
		Top10Accuracy: top10,
		Details:       details,
	}
}

func simulateAveragePositions(race *model.Race, course *model.Course, riders []model.Rider, numRuns int) map[int64]float64 {
	positionSums := make(map[int64]float64, len(riders))
	positionCounts := make(map[int64]int, len(riders))

	for run := 0; run < numRuns; run++ {
		config := RaceConfig{
			Race:   race,
			Course: course,
			Riders: riders,
			Seed:   ValidationSeedBase + int64(run+1),
		}
		var results []model.RaceResult
		if race.RaceType == model.RaceTypeTT {
			results = SimulateTT(config)
		} else {
			results = SimulateRoadRace(config)
		}
		for _, res := range results {
			positionSums[res.RiderID] += float64(res.Position)
			positionCounts[res.RiderID]++
		}
	}

	avgPositions := make(map[int64]float64, len(riders))
	for _, r := range riders {
		if count := positionCounts[r.ID]; count > 0 {
			avgPositions[r.ID] = positionSums[r.ID] / float64(count)
		} else {
			avgPositions[r.ID] = float64(len(riders) + 1)
		}
	}
	return avgPositions
}

func rankByAveragePosition(avgPositions map[int64]float64) []int64 {
	type entry struct {
		id  int64
		pos float64
	}
	entries := make([]entry, 0, len(avgPositions))
	for id, pos := range avgPositions {
		entries = append(entries, entry{id: id, pos: pos})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].pos < entries[j].pos })

	ranked := make([]int64, len(entries))
	for i, e := range entries {
		ranked[i] = e.id
	}
	return ranked
}

func topNAccuracy(expected []int64, simOrder []int64, n int) float64 {
	if len(expected) == 0 {
		return 0
	}
	limit := n
	if len(simOrder) < limit {
		limit = len(simOrder)
	}
	inTop := make(map[int64]bool, limit)
	for i := 0; i < limit; i++ {
		inTop[simOrder[i]] = true
	}
	count := 0
	for _, id := range expected {
		if inTop[id] {
			count++
		}
	}
	return float64(count) / float64(len(expected)) * 100
}

// validationSpearman is the rank correlation between an expected order and
// simulated average positions.
func validationSpearman(expectedOrder []int64, avgPositions map[int64]float64) float64 {
	tiers := make([][]int64, len(expectedOrder))
	for i, id := range expectedOrder {
		tiers[i] = []int64{id}
	}
	return validationTierCorrelation(tiers, avgPositions)
}

// validationTierCorrelation is Spearman's correlation (Pearson on tie-averaged
// ranks) between expected tiers — riders in a tier are expected to finish in
// any order among themselves — and simulated average positions.
func validationTierCorrelation(tiers [][]int64, avgPositions map[int64]float64) float64 {
	var expected, values []float64
	for t, tier := range tiers {
		for _, id := range tier {
			expected = append(expected, float64(t))
			values = append(values, avgPositions[id])
		}
	}
	if len(values) < 2 {
		return 0
	}
	return pearson(validationRanks(expected), validationRanks(values))
}

func pearson(x, y []float64) float64 {
	n := float64(len(x))
	var mx, my float64
	for i := range x {
		mx += x[i] / n
		my += y[i] / n
	}
	var sxy, sxx, syy float64
	for i := range x {
		sxy += (x[i] - mx) * (y[i] - my)
		sxx += (x[i] - mx) * (x[i] - mx)
		syy += (y[i] - my) * (y[i] - my)
	}
	if sxx == 0 || syy == 0 {
		return 0
	}
	return sxy / math.Sqrt(sxx*syy)
}

// validationRanks returns 1-based ranks, ties sharing their average rank.
func validationRanks(vals []float64) []float64 {
	idx := make([]int, len(vals))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return vals[idx[a]] < vals[idx[b]] })
	ranks := make([]float64, len(vals))
	for a := 0; a < len(idx); {
		b := a
		for b+1 < len(idx) && vals[idx[b+1]] == vals[idx[a]] {
			b++
		}
		for k := a; k <= b; k++ {
			ranks[idx[k]] = float64(a+b)/2 + 1
		}
		a = b + 1
	}
	return ranks
}

func sequenceFloats(n int) []float64 {
	vals := make([]float64, n)
	for i := 0; i < n; i++ {
		vals[i] = float64(i + 1)
	}
	return vals
}

func buildGroupIDs(start int64, count int) []int64 {
	ids := make([]int64, count)
	for i := 0; i < count; i++ {
		ids[i] = start + int64(i)
	}
	return ids
}

func buildFlatField() []model.Rider {
	riders := make([]model.Rider, 0, 30)
	riders = append(riders, buildSprinterGroup(1, 10)...)
	riders = append(riders, buildAllRounderGroup(11, 10)...)
	riders = append(riders, buildClimberGroup(21, 10)...)
	return riders
}

func buildMountainField() []model.Rider {
	riders := make([]model.Rider, 0, 30)
	riders = append(riders, buildClimberGroup(11, 10)...)
	riders = append(riders, buildAllRounderGroup(21, 10)...)
	riders = append(riders, buildSprinterGroup(1, 10)...)
	return riders
}

func buildTTField() []model.Rider {
	riders := make([]model.Rider, 0, 30)
	riders = append(riders, buildTTGroup(21, 10)...)
	riders = append(riders, buildAllRounderGroup(11, 10)...)
	riders = append(riders, buildSprinterGroup(1, 10)...)
	return riders
}

func buildSprinterGroup(startID int64, count int) []model.Rider {
	riders := make([]model.Rider, count)
	for i := 0; i < count; i++ {
		id := startID + int64(i)
		riders[i] = baseValidationRider(fmt.Sprintf("Sprinter %d", i+1), id, 78, 395, 92, 60, 70, 74)
		riders[i].MaxSprintWatts = 1900
	}
	return riders
}

func buildClimberGroup(startID int64, count int) []model.Rider {
	riders := make([]model.Rider, count)
	for i := 0; i < count; i++ {
		id := startID + int64(i)
		riders[i] = baseValidationRider(fmt.Sprintf("Climber %d", i+1), id, 64, 405, 60, 92, 70, 72)
		riders[i].Handling = 70
	}
	return riders
}

func buildAllRounderGroup(startID int64, count int) []model.Rider {
	riders := make([]model.Rider, count)
	for i := 0; i < count; i++ {
		id := startID + int64(i)
		riders[i] = baseValidationRider(fmt.Sprintf("All-Rounder %d", i+1), id, 71, 390, 78, 78, 78, 75)
	}
	return riders
}

func buildTTGroup(startID int64, count int) []model.Rider {
	riders := make([]model.Rider, count)
	for i := 0; i < count; i++ {
		id := startID + int64(i)
		base := baseValidationRider(fmt.Sprintf("TT %d", i+1), id, 72, 400, 65, 70, 92, 76)
		base.CdA = 0.26
		base.MaxSprintWatts = 1400
		riders[i] = base
	}
	return riders
}

func baseValidationRider(name string, id int64, weightKg float64, ftp int, sprint int, climb int, tt int, handling int) model.Rider {
	return model.Rider{
		ID:                id,
		Name:              name,
		Team:              fmt.Sprintf("Team %d", id%10), // each team gets one rider of each type
		Nationality:       "TST",
		WeightKg:          weightKg,
		HeightCm:          180,
		FTPWatts:          ftp,
		MaxSprintWatts:    1600,
		VO2MaxWatts:       int(math.Max(320, float64(ftp)-40)),
		CdA:               0.3,
		ClimbingAbility:   climb,
		SprintAbility:     sprint,
		TTAbility:         tt,
		DescendingAbility: 74,
		Endurance:         80,
		Recovery:          75,
		Consistency:       75,
		TacticalAwareness: 75,
		Handling:          handling,
	}
}

func validationFlatCourse() *model.Course {
	return &model.Course{
		ID:              1,
		Name:            "Flat Validation",
		TotalDistanceKm: 180,
		Segments: []model.Segment{
			{ID: 1, CourseID: 1, SegmentOrder: 1, Name: "Flat 1", DistanceKm: 60, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 2, CourseID: 1, SegmentOrder: 2, Name: "Flat 2", DistanceKm: 60, GradientPct: 0.2, SurfaceType: model.SurfaceRoad},
			{ID: 3, CourseID: 1, SegmentOrder: 3, Name: "Flat Finish", DistanceKm: 60, GradientPct: -0.3, SurfaceType: model.SurfaceRoad},
		},
	}
}

func validationMountainCourse() *model.Course {
	return &model.Course{
		ID:              2,
		Name:            "Mountain Validation",
		TotalDistanceKm: 170,
		Segments: []model.Segment{
			{ID: 1, CourseID: 2, SegmentOrder: 1, Name: "Valley", DistanceKm: 40, GradientPct: 0.5, SurfaceType: model.SurfaceRoad},
			{ID: 2, CourseID: 2, SegmentOrder: 2, Name: "Climb 1", DistanceKm: 18, GradientPct: 7.2, SurfaceType: model.SurfaceRoad},
			{ID: 3, CourseID: 2, SegmentOrder: 3, Name: "Ridge", DistanceKm: 30, GradientPct: 2.0, SurfaceType: model.SurfaceRoad},
			{ID: 4, CourseID: 2, SegmentOrder: 4, Name: "Climb 2", DistanceKm: 16, GradientPct: 8.1, SurfaceType: model.SurfaceRoad},
			{ID: 5, CourseID: 2, SegmentOrder: 5, Name: "Descent", DistanceKm: 32, GradientPct: -3.0, SurfaceType: model.SurfaceRoad, IsTechnical: true},
			{ID: 6, CourseID: 2, SegmentOrder: 6, Name: "Final Rise", DistanceKm: 34, GradientPct: 4.5, SurfaceType: model.SurfaceRoad},
		},
	}
}

func validationTTCourse() *model.Course {
	return &model.Course{
		ID:              3,
		Name:            "TT Validation",
		TotalDistanceKm: 40,
		Segments: []model.Segment{
			{ID: 1, CourseID: 3, SegmentOrder: 1, Name: "TT Out", DistanceKm: 20, GradientPct: 0.8, SurfaceType: model.SurfaceRoad},
			{ID: 2, CourseID: 3, SegmentOrder: 2, Name: "TT Back", DistanceKm: 20, GradientPct: -0.4, SurfaceType: model.SurfaceRoad},
		},
	}
}
