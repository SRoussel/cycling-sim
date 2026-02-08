package simulation

import (
	"math/rand"
	"sort"

	"cycling_sim/internal/model"
)

// RaceConfig holds configuration for running a simulation.
type RaceConfig struct {
	Race   *model.Race
	Course *model.Course
	Riders []model.Rider
	Seed   int64 // random seed for reproducibility (0 = random)
}

// SimulateRoadRace runs a complete road race simulation and returns results.
func SimulateRoadRace(config RaceConfig) []model.RaceResult {
	rng := rand.New(rand.NewSource(config.Seed))

	// Initialize rider states
	states := make([]*RiderState, len(config.Riders))
	for i := range config.Riders {
		rider := &config.Riders[i]
		states[i] = &RiderState{
			Rider:        rider,
			WPrime:       NewWPrimeState(rider),
			FormFactor:   DailyFormFactor(rider, rng),
			EffectiveFTP: rider.FTPWatts,
		}
	}

	segments := config.Course.Segments
	if len(segments) == 0 {
		return nil
	}

	// Calculate total remaining distance for tactical decisions
	totalDist := 0.0
	for _, seg := range segments {
		totalDist += seg.DistanceKm
	}

	// Process each segment
	coveredDist := 0.0
	for i := range segments {
		seg := &segments[i]
		SimulateSegment(states, seg, false, totalDist-coveredDist, rng)
		coveredDist += seg.DistanceKm
	}

	// Check if the last segment is flat → sprint finish among the lead group
	lastSeg := &segments[len(segments)-1]
	if lastSeg.GradientPct <= 2.0 && lastSeg.GradientPct >= -2.0 {
		// Find riders in the lead group (smallest TotalTime cluster)
		sort.Slice(states, func(i, j int) bool {
			return states[i].TotalTime < states[j].TotalTime
		})
		// Lead group: riders within 3 seconds of the leader
		leaderTime := states[0].TotalTime
		var sprintGroup []*RiderState
		for _, rs := range states {
			if rs.TotalTime-leaderTime < 3.0 {
				sprintGroup = append(sprintGroup, rs)
			}
		}
		if len(sprintGroup) > 1 {
			SprintFinish(sprintGroup, rng)
		}
	}

	// Sort by total time and create results
	sort.Slice(states, func(i, j int) bool {
		return states[i].TotalTime < states[j].TotalTime
	})

	winnerTime := states[0].TotalTime
	results := make([]model.RaceResult, len(states))
	for i, rs := range states {
		results[i] = model.RaceResult{
			RaceID:            config.Race.ID,
			RiderID:           rs.Rider.ID,
			Position:          i + 1,
			FinishTimeSeconds: rs.TotalTime,
			TimeGapSeconds:    rs.TotalTime - winnerTime,
		}
	}
	return results
}

// SimulateTT runs a time trial simulation.
func SimulateTT(config RaceConfig) []model.RaceResult {
	rng := rand.New(rand.NewSource(config.Seed))

	states := make([]*RiderState, len(config.Riders))
	for i := range config.Riders {
		rider := &config.Riders[i]
		states[i] = &RiderState{
			Rider:        rider,
			WPrime:       NewWPrimeState(rider),
			FormFactor:   DailyFormFactor(rider, rng),
			EffectiveFTP: rider.FTPWatts,
		}
	}

	segments := config.Course.Segments

	// TT pacing: TTAbility affects how evenly power is distributed
	for _, rs := range states {
		for j := range segments {
			seg := &segments[j]

			// Base power is FTP modified by TT ability and form
			basePower := float64(rs.EffectiveFTP) * rs.FormFactor
			power := EffectivePower(rs.Rider, basePower, seg, true)

			// Pacing: high TT ability = more even pacing, slight above FTP
			// Low TT ability = inconsistent, occasionally too hard
			pacingFactor := 1.0 + float64(rs.Rider.TTAbility-70)*0.001
			// Add segment-level pacing variation (worse pacers vary more)
			pacingVariance := rng.NormFloat64() * 0.01 * (1.0 - float64(rs.Rider.TTAbility)/150.0)
			power *= pacingFactor + pacingVariance

			// TT aero position: CdA is typically 15-20% better in TT
			ttCdA := rs.Rider.CdA * 0.82

			var speed float64
			if seg.GradientPct < -2.0 {
				speed = DescentSpeed(power, rs.Rider.TotalMassKg(), ttCdA,
					seg.GradientPct, seg.SurfaceType, rs.Rider.DescendingAbility, seg.IsTechnical)
			} else {
				speed = SpeedFromPower(power, rs.Rider.TotalMassKg(), ttCdA,
					seg.GradientPct, seg.SurfaceType)
			}

			segTime := SegmentTime(speed, seg.DistanceKm)
			rs.TotalTime += segTime
			rs.WPrime.Update(power, rs.EffectiveFTP, segTime)
		}
	}

	// Sort and create results
	sort.Slice(states, func(i, j int) bool {
		return states[i].TotalTime < states[j].TotalTime
	})

	winnerTime := states[0].TotalTime
	results := make([]model.RaceResult, len(states))
	for i, rs := range states {
		results[i] = model.RaceResult{
			RaceID:            config.Race.ID,
			RiderID:           rs.Rider.ID,
			Position:          i + 1,
			FinishTimeSeconds: rs.TotalTime,
			TimeGapSeconds:    rs.TotalTime - winnerTime,
		}
	}
	return results
}

// SimulateStageRace runs a multi-stage race and returns per-stage results and GC standings.
func SimulateStageRace(stageRace *model.StageRace, stages []StageConfig, riders []model.Rider, seed int64) ([][]model.RaceResult, [][]model.GCStanding) {
	rng := rand.New(rand.NewSource(seed))

	// Initialize cumulative fatigue per rider
	fatigueMap := make(map[int64]*StageFatigueState)
	for i := range riders {
		fatigueMap[riders[i].ID] = NewStageFatigueState(&riders[i])
	}

	var allResults [][]model.RaceResult
	var allStandings [][]model.GCStanding

	// Track cumulative time per rider for GC
	gcTimes := make(map[int64]float64)
	for _, r := range riders {
		gcTimes[r.ID] = 0
	}

	for _, stage := range stages {
		// Apply rest day
		if stage.RestDayBefore {
			for _, f := range fatigueMap {
				f.ApplyRestDay()
			}
		}

		// Create adjusted riders with fatigue-reduced FTP
		adjustedRiders := make([]model.Rider, len(riders))
		copy(adjustedRiders, riders)
		for i := range adjustedRiders {
			if f, ok := fatigueMap[adjustedRiders[i].ID]; ok {
				adjustedRiders[i].FTPWatts = f.EffectiveFTP(riders[i].FTPWatts)
			}
		}

		config := RaceConfig{
			Race:   stage.Race,
			Course: stage.Course,
			Riders: adjustedRiders,
			Seed:   rng.Int63(),
		}

		var results []model.RaceResult
		if stage.Race.RaceType == model.RaceTypeTT {
			results = SimulateTT(config)
		} else {
			results = SimulateRoadRace(config)
		}
		allResults = append(allResults, results)

		// Update fatigue and GC times
		for _, res := range results {
			gcTimes[res.RiderID] += res.FinishTimeSeconds

			// Stage stress based on finish time relative to winner
			stageStress := 1.0 + res.TimeGapSeconds/1000.0
			if f, ok := fatigueMap[res.RiderID]; ok {
				f.AddStageStress(stageStress)
			}
		}

		// Calculate GC standings
		type gcEntry struct {
			riderID int64
			time    float64
		}
		var entries []gcEntry
		for id, t := range gcTimes {
			entries = append(entries, gcEntry{id, t})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].time < entries[j].time
		})

		standings := make([]model.GCStanding, len(entries))
		for i, e := range entries {
			standings[i] = model.GCStanding{
				StageRaceID:      stageRace.ID,
				RiderID:          e.riderID,
				AfterStage:       stage.StageNumber,
				TotalTimeSeconds: e.time,
				GCPosition:       i + 1,
			}
		}
		allStandings = append(allStandings, standings)
	}

	return allResults, allStandings
}

// StageConfig holds the configuration for one stage in a stage race.
type StageConfig struct {
	Race          *model.Race
	Course        *model.Course
	StageNumber   int
	RestDayBefore bool
}
