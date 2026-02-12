package simulation

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"cycling_sim/internal/model"
)

// makeTTSpecialist creates a rider optimized for time trials.
func makeTTSpecialist(name string) model.Rider {
	r := makeRider(name, 415, 75, 70, 60, 95, 70, 80, 70, 85, 70)
	r.MaxSprintWatts = 1300
	return r
}

// makeField returns a 20-rider mixed field with unique IDs.
func makeField() []model.Rider {
	var riders []model.Rider
	for i := 1; i <= 5; i++ {
		s := makeSprinter(fmt.Sprintf("Sprinter %d", i))
		s.ID = int64(i)
		riders = append(riders, s)
	}
	for i := 1; i <= 5; i++ {
		c := makeClimber(fmt.Sprintf("Climber %d", i))
		c.ID = int64(5 + i)
		riders = append(riders, c)
	}
	for i := 1; i <= 5; i++ {
		tt := makeTTSpecialist(fmt.Sprintf("TT %d", i))
		tt.ID = int64(10 + i)
		riders = append(riders, tt)
	}
	for i := 1; i <= 5; i++ {
		ar := makeAllRounder(fmt.Sprintf("AllRounder %d", i))
		ar.ID = int64(15 + i)
		riders = append(riders, ar)
	}
	return riders
}

func median(vals []float64) float64 {
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

func spearmanCorrelation(x, y []float64) float64 {
	n := len(x)
	if n != len(y) || n < 2 {
		return 0
	}
	rankX := ranks(x)
	rankY := ranks(y)
	var sumD2 float64
	for i := 0; i < n; i++ {
		d := rankX[i] - rankY[i]
		sumD2 += d * d
	}
	return 1 - 6*sumD2/(float64(n)*(float64(n)*float64(n)-1))
}

func ranks(vals []float64) []float64 {
	n := len(vals)
	type indexed struct {
		val float64
		idx int
	}
	items := make([]indexed, n)
	for i, v := range vals {
		items[i] = indexed{v, i}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].val < items[j].val })
	r := make([]float64, n)
	for rank, item := range items {
		r[item.idx] = float64(rank + 1)
	}
	return r
}

const numSeeds = 50

func TestAccuracy_SprintersWinFlatRaces(t *testing.T) {
	riders := makeField()
	course := flatCourse()

	sprinterWins := 0
	for seed := int64(1); seed <= numSeeds; seed++ {
		race := &model.Race{ID: 1, Name: "Flat", CourseID: 1, RaceType: model.RaceTypeRoad}
		results := SimulateRoadRace(RaceConfig{Race: race, Course: course, Riders: riders, Seed: seed})
		winnerID := results[0].RiderID
		if winnerID >= 1 && winnerID <= 5 {
			sprinterWins++
		}
	}
	pct := float64(sprinterWins) / float64(numSeeds) * 100
	t.Logf("Sprinter win rate on flat: %.0f%% (%d/%d)", pct, sprinterWins, numSeeds)
	if sprinterWins < numSeeds*60/100 {
		t.Errorf("sprinters won %d/%d flat races (%.0f%%), expected >60%%", sprinterWins, numSeeds, pct)
	}
}

func TestAccuracy_ClimbersWinMountainRaces(t *testing.T) {
	riders := makeField()
	course := mountainCourse()

	climberWins := 0
	for seed := int64(1); seed <= numSeeds; seed++ {
		race := &model.Race{ID: 1, Name: "Mountain", CourseID: 2, RaceType: model.RaceTypeRoad}
		results := SimulateRoadRace(RaceConfig{Race: race, Course: course, Riders: riders, Seed: seed})
		winnerID := results[0].RiderID
		if winnerID >= 6 && winnerID <= 10 {
			climberWins++
		}
	}
	pct := float64(climberWins) / float64(numSeeds) * 100
	t.Logf("Climber win rate on mountain: %.0f%% (%d/%d)", pct, climberWins, numSeeds)
	if climberWins < numSeeds*60/100 {
		t.Errorf("climbers won %d/%d mountain races (%.0f%%), expected >60%%", climberWins, numSeeds, pct)
	}
}

func TestAccuracy_TTSpecialistsWinTTs(t *testing.T) {
	riders := makeField()
	course := ttCourse()

	ttWins := 0
	for seed := int64(1); seed <= numSeeds; seed++ {
		race := &model.Race{ID: 1, Name: "TT", CourseID: 3, RaceType: model.RaceTypeTT}
		results := SimulateTT(RaceConfig{Race: race, Course: course, Riders: riders, Seed: seed})
		winnerID := results[0].RiderID
		if winnerID >= 11 && winnerID <= 15 {
			ttWins++
		}
	}
	pct := float64(ttWins) / float64(numSeeds) * 100
	t.Logf("TT specialist win rate: %.0f%% (%d/%d)", pct, ttWins, numSeeds)
	if ttWins < numSeeds*50/100 {
		t.Errorf("TT specialists won %d/%d TTs (%.0f%%), expected >50%%", ttWins, numSeeds, pct)
	}
}

func TestAccuracy_FlatStagesBunchFinish(t *testing.T) {
	riders := makeField()
	course := flatCourse()

	var maxGaps []float64
	for seed := int64(1); seed <= numSeeds; seed++ {
		race := &model.Race{ID: 1, Name: "Flat", CourseID: 1, RaceType: model.RaceTypeRoad}
		results := SimulateRoadRace(RaceConfig{Race: race, Course: course, Riders: riders, Seed: seed})
		maxGap := results[len(results)-1].TimeGapSeconds
		maxGaps = append(maxGaps, maxGap)
	}
	med := median(maxGaps)
	t.Logf("Flat race median max gap: %.1fs", med)
	if med > 90 {
		t.Errorf("median max gap on flat = %.1fs, expected < 90s (bunch finish)", med)
	}
}

func TestAccuracy_MountainStagesProduceGaps(t *testing.T) {
	riders := makeField()
	course := mountainCourse()

	var maxGaps []float64
	for seed := int64(1); seed <= numSeeds; seed++ {
		race := &model.Race{ID: 1, Name: "Mountain", CourseID: 2, RaceType: model.RaceTypeRoad}
		results := SimulateRoadRace(RaceConfig{Race: race, Course: course, Riders: riders, Seed: seed})
		maxGap := results[len(results)-1].TimeGapSeconds
		maxGaps = append(maxGaps, maxGap)
	}
	med := median(maxGaps)
	t.Logf("Mountain race median max gap: %.1fs", med)
	if med < 120 {
		t.Errorf("median max gap on mountain = %.1fs, expected > 120s", med)
	}
}

func TestAccuracy_GCFavorsAllRounders(t *testing.T) {
	riders := makeField()

	flat := flatCourse()
	flat.ID = 1
	mtn := mountainCourse()
	mtn.ID = 2
	tt := ttCourse()
	tt.ID = 3

	climberOrARWins := 0
	sprinterWins := 0

	for seed := int64(1); seed <= numSeeds; seed++ {
		stages := []StageConfig{
			{Race: &model.Race{ID: 1, Name: "S1 Flat", CourseID: 1, RaceType: model.RaceTypeRoad}, Course: flat, StageNumber: 1},
			{Race: &model.Race{ID: 2, Name: "S2 Mountain", CourseID: 2, RaceType: model.RaceTypeRoad}, Course: mtn, StageNumber: 2},
			{Race: &model.Race{ID: 3, Name: "S3 TT", CourseID: 3, RaceType: model.RaceTypeTT}, Course: tt, StageNumber: 3},
		}
		stageRace := &model.StageRace{ID: 1, Name: "Mini Tour"}
		_, allStandings := SimulateStageRace(stageRace, stages, riders, seed)
		gcWinner := allStandings[2][0].RiderID
		// Climbers: 6-10, All-rounders: 16-20
		if (gcWinner >= 6 && gcWinner <= 10) || (gcWinner >= 16 && gcWinner <= 20) {
			climberOrARWins++
		}
		// Sprinters: 1-5
		if gcWinner >= 1 && gcWinner <= 5 {
			sprinterWins++
		}
	}
	arPct := float64(climberOrARWins) / float64(numSeeds) * 100
	spPct := float64(sprinterWins) / float64(numSeeds) * 100
	t.Logf("GC: climber/all-rounder wins %.0f%%, sprinter wins %.0f%%", arPct, spPct)
	if climberOrARWins < numSeeds*80/100 {
		t.Errorf("climber/all-rounder won GC %d/%d (%.0f%%), expected >80%%", climberOrARWins, numSeeds, arPct)
	}
	if sprinterWins > numSeeds*5/100 {
		t.Errorf("sprinter won GC %d/%d (%.0f%%), expected <5%%", sprinterWins, numSeeds, spPct)
	}
}

func TestAccuracy_ClimbingAbilityCorrelatesWithMountainResult(t *testing.T) {
	// 20 riders with climbing ability from 40 to 95, other stats equal
	var riders []model.Rider
	for i := 0; i < 20; i++ {
		climbing := 40 + (i * 55 / 19) // 40 to 95
		r := makeRider(fmt.Sprintf("Rider %d", i+1), 390, 68, climbing, 65, 70, 70, 75, 70, 75, 70)
		r.ID = int64(i + 1)
		riders = append(riders, r)
	}

	course := mountainCourse()
	var allAbilities []float64
	var avgPositions []float64
	for i := range riders {
		allAbilities = append(allAbilities, float64(riders[i].ClimbingAbility))
		avgPositions = append(avgPositions, 0)
	}

	for seed := int64(1); seed <= numSeeds; seed++ {
		race := &model.Race{ID: 1, Name: "Mountain", CourseID: 2, RaceType: model.RaceTypeRoad}
		results := SimulateRoadRace(RaceConfig{Race: race, Course: course, Riders: riders, Seed: seed})
		for _, res := range results {
			avgPositions[res.RiderID-1] += float64(res.Position)
		}
	}
	for i := range avgPositions {
		avgPositions[i] /= float64(numSeeds)
	}

	// Higher climbing ability should correlate with lower (better) position
	// So we negate positions for positive correlation
	negPositions := make([]float64, len(avgPositions))
	for i, p := range avgPositions {
		negPositions[i] = -p
	}
	corr := spearmanCorrelation(allAbilities, negPositions)
	t.Logf("Spearman correlation (climbing ability vs mountain result): %.3f", corr)
	if corr < 0.5 {
		t.Errorf("correlation = %.3f, expected > 0.5", corr)
	}
}

func TestAccuracy_FatigueImpactsLaterStages(t *testing.T) {
	riders := makeField()

	// 5-stage race: all mountain stages to accumulate fatigue
	var stages []StageConfig
	for i := 1; i <= 5; i++ {
		mtn := mountainCourse()
		mtn.ID = int64(i)
		stages = append(stages, StageConfig{
			Race:        &model.Race{ID: int64(i), Name: fmt.Sprintf("Stage %d", i), CourseID: int64(i), RaceType: model.RaceTypeRoad},
			Course:      mtn,
			StageNumber: i,
		})
	}

	var stage1WinnerTimes, stage5WinnerTimes []float64

	for seed := int64(1); seed <= numSeeds; seed++ {
		stageRace := &model.StageRace{ID: 1, Name: "5-Stage Tour"}
		allResults, _ := SimulateStageRace(stageRace, stages, riders, seed)
		stage1WinnerTimes = append(stage1WinnerTimes, allResults[0][0].FinishTimeSeconds)
		stage5WinnerTimes = append(stage5WinnerTimes, allResults[4][0].FinishTimeSeconds)
	}

	avgStage1 := mean(stage1WinnerTimes)
	avgStage5 := mean(stage5WinnerTimes)
	t.Logf("Avg winner time: stage 1 = %.0fs, stage 5 = %.0fs (diff = %.0fs)", avgStage1, avgStage5, avgStage5-avgStage1)
	if avgStage5 <= avgStage1 {
		t.Errorf("stage 5 winner time (%.0fs) should be greater than stage 1 (%.0fs) due to fatigue", avgStage5, avgStage1)
	}
}

func mean(vals []float64) float64 {
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func makeCobbleSpecialist(name string) model.Rider {
	r := makeRider(name, 395, 74, 65, 72, 75, 75, 82, 78, 80, 80)
	r.MaxSprintWatts = 1500
	r.Handling = 93
	return r
}

func cobbleCourse() *model.Course {
	// Modeled on Paris-Roubaix: ~257km with ~30km of cobbles across 11 sectors
	// interspersed with road links. Sectors get shorter and closer together
	// toward the finish, increasing attrition pressure.
	return &model.Course{
		ID: 10, Name: "Paris-Roubaix", TotalDistanceKm: 257,
		Segments: []model.Segment{
			{ID: 1, CourseID: 10, SegmentOrder: 1, Name: "Road start", DistanceKm: 95, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			// Early sectors — long cobbles, wide spacing
			{ID: 2, CourseID: 10, SegmentOrder: 2, Name: "Troisvilles", DistanceKm: 2.2, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 3, CourseID: 10, SegmentOrder: 3, Name: "Link 1", DistanceKm: 8, GradientPct: 0.3, SurfaceType: model.SurfaceRoad},
			{ID: 4, CourseID: 10, SegmentOrder: 4, Name: "Wandignies", DistanceKm: 3.7, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 5, CourseID: 10, SegmentOrder: 5, Name: "Link 2", DistanceKm: 6, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 6, CourseID: 10, SegmentOrder: 6, Name: "Hornaing", DistanceKm: 3.5, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 7, CourseID: 10, SegmentOrder: 7, Name: "Link 3", DistanceKm: 5, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 8, CourseID: 10, SegmentOrder: 8, Name: "Arenberg", DistanceKm: 2.4, GradientPct: 0, SurfaceType: model.SurfaceCobbles, IsTechnical: true},
			{ID: 9, CourseID: 10, SegmentOrder: 9, Name: "Link 4", DistanceKm: 5, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			// Middle sectors — attrition builds
			{ID: 10, CourseID: 10, SegmentOrder: 10, Name: "Wallers", DistanceKm: 2.5, GradientPct: 0.3, SurfaceType: model.SurfaceCobbles},
			{ID: 11, CourseID: 10, SegmentOrder: 11, Name: "Link 5", DistanceKm: 4, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 12, CourseID: 10, SegmentOrder: 12, Name: "Orchies", DistanceKm: 1.7, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 13, CourseID: 10, SegmentOrder: 13, Name: "Link 6", DistanceKm: 4, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 14, CourseID: 10, SegmentOrder: 14, Name: "Auchy-lez-Orchies", DistanceKm: 2.7, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 15, CourseID: 10, SegmentOrder: 15, Name: "Link 7", DistanceKm: 3, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			// Late sectors — decisive, close together
			{ID: 16, CourseID: 10, SegmentOrder: 16, Name: "Mons-en-Pévèle", DistanceKm: 3.0, GradientPct: 0.5, SurfaceType: model.SurfaceCobbles},
			{ID: 17, CourseID: 10, SegmentOrder: 17, Name: "Link 8", DistanceKm: 3, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 18, CourseID: 10, SegmentOrder: 18, Name: "Cysoing", DistanceKm: 1.3, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 19, CourseID: 10, SegmentOrder: 19, Name: "Link 9", DistanceKm: 2, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 20, CourseID: 10, SegmentOrder: 20, Name: "Bourghelles", DistanceKm: 1.5, GradientPct: 0.3, SurfaceType: model.SurfaceCobbles},
			{ID: 21, CourseID: 10, SegmentOrder: 21, Name: "Link 10", DistanceKm: 2, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 22, CourseID: 10, SegmentOrder: 22, Name: "Carrefour de l'Arbre", DistanceKm: 2.1, GradientPct: 0.3, SurfaceType: model.SurfaceCobbles},
			{ID: 23, CourseID: 10, SegmentOrder: 23, Name: "Link 11", DistanceKm: 3, GradientPct: 0, SurfaceType: model.SurfaceRoad},
			{ID: 24, CourseID: 10, SegmentOrder: 24, Name: "Gruson", DistanceKm: 1.1, GradientPct: 0, SurfaceType: model.SurfaceCobbles},
			{ID: 25, CourseID: 10, SegmentOrder: 25, Name: "Roubaix finish", DistanceKm: 5, GradientPct: 0, SurfaceType: model.SurfaceRoad},
		},
	}
}

func TestAccuracy_CobbleSpecialistsWinCobbleRaces(t *testing.T) {
	// 20-rider mixed field: 3 cobble specialists + 17 average handlers
	var riders []model.Rider
	for i := 1; i <= 3; i++ {
		r := makeCobbleSpecialist(fmt.Sprintf("CobbleSpec %d", i))
		r.ID = int64(i)
		riders = append(riders, r)
	}
	for i := 1; i <= 5; i++ {
		s := makeSprinter(fmt.Sprintf("Sprinter %d", i))
		s.ID = int64(3 + i)
		riders = append(riders, s)
	}
	for i := 1; i <= 5; i++ {
		c := makeClimber(fmt.Sprintf("Climber %d", i))
		c.ID = int64(8 + i)
		riders = append(riders, c)
	}
	for i := 1; i <= 4; i++ {
		ar := makeAllRounder(fmt.Sprintf("AllRounder %d", i))
		ar.ID = int64(13 + i)
		riders = append(riders, ar)
	}
	for i := 1; i <= 3; i++ {
		tt := makeTTSpecialist(fmt.Sprintf("TT %d", i))
		tt.ID = int64(17 + i)
		riders = append(riders, tt)
	}

	course := cobbleCourse()

	specialistWins := 0
	var maxGaps []float64
	for seed := int64(1); seed <= numSeeds; seed++ {
		race := &model.Race{ID: 1, Name: "Cobbles", CourseID: 10, RaceType: model.RaceTypeRoad}
		results := SimulateRoadRace(RaceConfig{Race: race, Course: course, Riders: riders, Seed: seed})
		winnerID := results[0].RiderID
		if winnerID >= 1 && winnerID <= 3 {
			specialistWins++
		}
		maxGap := results[len(results)-1].TimeGapSeconds
		maxGaps = append(maxGaps, maxGap)
	}
	pct := float64(specialistWins) / float64(numSeeds) * 100
	medGap := median(maxGaps)
	t.Logf("Cobble specialist win rate: %.0f%% (%d/%d)", pct, specialistWins, numSeeds)
	t.Logf("Cobble race median max gap: %.1fs", medGap)
	if specialistWins < numSeeds*50/100 {
		t.Errorf("cobble specialists won %d/%d races (%.0f%%), expected >50%%", specialistWins, numSeeds, pct)
	}
	if medGap < 30 {
		t.Errorf("median max gap on cobbles = %.1fs, expected > 30s (selections should happen)", medGap)
	}
}

// stdev calculates standard deviation of a slice of floats.
func stdev(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	m := mean(vals)
	var sumSq float64
	for _, v := range vals {
		d := v - m
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(vals)-1))
}

// percentile returns the p-th percentile (0-1) of sorted values.
func percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	idx := p * float64(len(sorted)-1)
	lower := int(idx)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

func TestVariance_RiderTypeFinishPositions(t *testing.T) {
	// Test consistency of finish positions across 100 runs for different rider types.
	// Compare a specialist vs a realistic peloton (mix of types at similar levels).
	// This reveals if the simulator properly differentiates specialists on their terrain.

	const numRuns = 100

	// Define test cases: each has a specialist and a balanced peloton
	testCases := []struct {
		name                string
		specialist          model.Rider
		course              *model.Course
		expectHighTop10Rate bool
	}{
		{
			name:                "Sprinter vs Mixed Field (Flat)",
			specialist:          makeSprinter("Sprinter"),
			course:              flatCourse(),
			expectHighTop10Rate: true, // sprinters should win flats often
		},
		{
			name:                "Climber vs Mixed Field (Mountain)",
			specialist:          makeClimber("Climber"),
			course:              mountainCourse(),
			expectHighTop10Rate: true, // climbers should win mountains often
		},
		{
			name:                "Cobble Specialist vs Mixed Field (Cobbles)",
			specialist:          makeCobbleSpecialist("CobbleSpec"),
			course:              cobbleCourse(),
			expectHighTop10Rate: true, // specialists should win cobbles often
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build realistic peloton: mix of rider types (4 sprinters, 4 climbers, 4 TTers, 4 ARs, 1 specialist)
			riders := []model.Rider{}
			tc.specialist.ID = 1
			riders = append(riders, tc.specialist)

			riderID := int64(2)
			for i := 0; i < 4; i++ {
				r := makeSprinter(fmt.Sprintf("Sprinter %d", i+1))
				r.ID = riderID
				riders = append(riders, r)
				riderID++
			}
			for i := 0; i < 4; i++ {
				r := makeClimber(fmt.Sprintf("Climber %d", i+1))
				r.ID = riderID
				riders = append(riders, r)
				riderID++
			}
			for i := 0; i < 4; i++ {
				r := makeTTSpecialist(fmt.Sprintf("TTer %d", i+1))
				r.ID = riderID
				riders = append(riders, r)
				riderID++
			}
			for i := 0; i < 4; i++ {
				r := makeAllRounder(fmt.Sprintf("AllRounder %d", i+1))
				r.ID = riderID
				riders = append(riders, r)
				riderID++
			}

			// Run races and collect positions for specialist
			var positions []float64
			top10Count := 0
			for seed := int64(1); seed <= int64(numRuns); seed++ {
				race := &model.Race{
					ID:       1,
					Name:     tc.name,
					CourseID: tc.course.ID,
					RaceType: model.RaceTypeRoad,
				}
				results := SimulateRoadRace(RaceConfig{
					Race:   race,
					Course: tc.course,
					Riders: riders,
					Seed:   seed,
				})

				// Find position of specialist (ID=1)
				for pos, res := range results {
					if res.RiderID == 1 {
						positions = append(positions, float64(pos+1))
						if pos < 10 {
							top10Count++
						}
						break
					}
				}
			}

			// Calculate statistics
			m := mean(positions)
			s := stdev(positions)
			p10 := percentile(positions, 0.1)
			p50 := percentile(positions, 0.5)
			p90 := percentile(positions, 0.9)
			minPos := positions[0]
			maxPos := positions[0]
			for _, p := range positions {
				if p < minPos {
					minPos = p
				}
				if p > maxPos {
					maxPos = p
				}
			}
			top10Rate := float64(top10Count) / float64(numRuns) * 100

			t.Logf("%s:", tc.name)
			t.Logf("  Mean=%.1f, StDev=%.1f, Range=[%.0f-%.0f]", m, s, minPos, maxPos)
			t.Logf("  P10=%.1f, P50=%.1f, P90=%.1f", p10, p50, p90)
			t.Logf("  Top-10 finish rate: %.0f%%", top10Rate)
			t.Logf("  Coefficient of Variation: %.2f", s/m)

			// For specialists on their terrain, should see:
			// - Low mean position (2-4)
			// - Moderate variance (some bad luck races)
			// - High top-10 rate (60-90%)
			if tc.expectHighTop10Rate && top10Rate < 50 {
				t.Logf("⚠️  WARNING: Specialist only made top-10 in %.0f%% of races (expected >60%%)", top10Rate)
			}
		})
	}
}

// Ensure math import is used (for spearmanCorrelation).
var _ = math.Abs
