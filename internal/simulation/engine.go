package simulation

import (
	"math/rand"
	"sort"
	"time"

	"cycling_sim/internal/model"
)

// RaceConfig holds configuration for running a simulation.
type RaceConfig struct {
	Race             *model.Race
	Course           *model.Course
	Riders           []model.Rider
	Seed             int64 // random seed for reproducibility (0 = random)
	IncidentsEnabled bool  // enable crashes/punctures simulation
	ActionOverrides  ActionOverrideProvider
}

// SimulateRoadRace runs a complete road race simulation and returns results.
func SimulateRoadRace(config RaceConfig) []model.RaceResult {
	rng := rngFromSeed(config.Seed)

	// Initialize rider states
	states := initStates(config.Riders, rng)

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
	tacticsParams := DefaultTacticsParams()
	raceState := NewRaceState(states, rng, config.Race != nil && isStageRaceStageName(config.Race.Name))
	raceState.Overrides = config.ActionOverrides
	coveredDist := 0.0
	for i := range segments {
		seg := &segments[i]
		SimulateSegment(states, seg, false, totalDist-coveredDist, rng, raceState, &tacticsParams, config.IncidentsEnabled)
		coveredDist += seg.DistanceKm
	}

	// Check if the last segment is flat → sprint finish among the lead group
	lastSeg := &segments[len(segments)-1]
	if lastSeg.GradientPct <= 2.0 && lastSeg.GradientPct >= -2.0 {
		// Find riders in the lead group (smallest TotalTime cluster)
		sort.Slice(states, func(i, j int) bool {
			return states[i].TotalTime < states[j].TotalTime
		})
		// Lead group: riders within 3.0 seconds of the leader (bunch finish)
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
		// DNF riders go to the end, sorted by time gaps
		if states[i].DNF && states[j].DNF {
			return states[i].TotalTime < states[j].TotalTime
		}
		if states[i].DNF {
			return false // DNF riders at the end
		}
		if states[j].DNF {
			return true
		}
		return states[i].TotalTime < states[j].TotalTime
	})

	winnerTime := states[0].TotalTime
	results := make([]model.RaceResult, 0, len(states))
	position := 1
	for _, rs := range states {
		result := model.RaceResult{
			RaceID:            config.Race.ID,
			RiderID:           rs.Rider.ID,
			Position:          position,
			FinishTimeSeconds: rs.TotalTime,
			TimeGapSeconds:    rs.TotalTime - winnerTime,
		}
		if rs.DNF {
			result.Position = 0 // 0 indicates DNF
		}
		results = append(results, result)
		if !rs.DNF {
			position++
		}
	}
	return results
}

// SimulateTT runs a time trial simulation.
func SimulateTT(config RaceConfig) []model.RaceResult {
	rng := rngFromSeed(config.Seed)
	states := initStates(config.Riders, rng)

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
					seg.GradientPct, seg.SurfaceType, rs.Rider.Handling, rs.Rider.DescendingAbility, seg.IsTechnical)
			} else {
				speed = SpeedFromPower(power, rs.Rider.TotalMassKg(), ttCdA,
					seg.GradientPct, seg.SurfaceType, rs.Rider.Handling)
			}

			segTime := SegmentTime(speed, seg.DistanceKm)
			rs.TotalTime += segTime
			rs.WPrime.UpdateWithHandling(power, rs.EffectiveFTP, segTime, rs.Rider.Handling, seg.SurfaceType == model.SurfaceCobbles)
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
	rng := rngFromSeed(seed)

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

	activeRiders := make([]model.Rider, len(riders))
	copy(activeRiders, riders)

	for _, stage := range stages {
		// Apply rest day
		if stage.RestDayBefore {
			for _, f := range fatigueMap {
				f.ApplyRestDay()
			}
		}

		// Create adjusted riders with fatigue-reduced FTP
		adjustedRiders := make([]model.Rider, len(activeRiders))
		copy(adjustedRiders, activeRiders)
		for i := range adjustedRiders {
			if f, ok := fatigueMap[adjustedRiders[i].ID]; ok {
				adjustedRiders[i].FTPWatts = f.EffectiveFTP(adjustedRiders[i].FTPWatts)
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
		activeByID := make(map[int64]model.Rider, len(adjustedRiders))
		for _, r := range adjustedRiders {
			activeByID[r.ID] = r
		}
		remaining := make([]model.Rider, 0, len(adjustedRiders))
		for _, res := range results {
			if res.Position == 0 {
				delete(gcTimes, res.RiderID)
				delete(fatigueMap, res.RiderID)
				continue
			}
			gcTimes[res.RiderID] += res.FinishTimeSeconds

			stageStress := StageStress(stage.Course, stage.Race.RaceType, res.TimeGapSeconds)
			if f, ok := fatigueMap[res.RiderID]; ok {
				f.AddStageStress(stageStress)
			}
			if rider, ok := activeByID[res.RiderID]; ok {
				remaining = append(remaining, rider)
			}
		}
		activeRiders = remaining

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

func rngFromSeed(seed int64) *rand.Rand {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return rand.New(rand.NewSource(seed))
}

func initStates(riders []model.Rider, rng *rand.Rand) []*RiderState {
	states := make([]*RiderState, len(riders))
	for i := range riders {
		rider := &riders[i]
		states[i] = &RiderState{
			Rider:        rider,
			WPrime:       NewWPrimeState(rider),
			FormFactor:   DailyFormFactor(rider, rng),
			EffectiveFTP: rider.FTPWatts,
		}
	}
	AssignTeamRoles(states)
	return states
}

// StageConfig holds the configuration for one stage in a stage race.
type StageConfig struct {
	Race          *model.Race
	Course        *model.Course
	StageNumber   int
	RestDayBefore bool
}
