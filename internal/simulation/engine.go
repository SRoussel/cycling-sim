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
	// GCGaps maps rider ID → cumulative GC deficit in seconds vs. the current leader.
	// Set by the stage race runner before each stage. Nil for standalone races.
	GCGaps map[int64]float64
	// TeamPlan maps rider ID → a draw in [0,1) made once per stage race:
	// whether a team frees a strong domestique to ride for GC (see
	// AssignCoLeaders). Nil: the hard top-10 rule.
	TeamPlan map[int64]float64
	// Shape, when set, records how the race unfolds (see RaceShape).
	Shape *RaceShape
	// Stage races: how coveted this stage is (0-1) and the stages each rider
	// has won so far (see setHunting).
	Prestige  float64
	StageWins map[int64]int
	// FormDate, when set, gives riders their seasonal form for that date
	// (see SeasonForm); zero: everyone at their best (one-off races).
	FormDate time.Time
	// RaceForm: each rider's form multiplier for the whole stage race
	// (RaceFormFactor); nil in one-day races, where daily form is the race.
	RaceForm map[int64]float64
}

// SimulateRoadRace runs a complete road race simulation and returns results.
func SimulateRoadRace(config RaceConfig) []model.RaceResult {
	return SimulateRoadRaceWithProgress(config, nil)
}

// SimulateTT runs a time trial simulation.
func SimulateTT(config RaceConfig) []model.RaceResult {
	rng := rngFromSeed(config.Seed)
	states := initStates(config.Riders, rng)
	applySeasonForm(states, config.FormDate)
	for _, rs := range states {
		if f, ok := config.RaceForm[rs.Rider.ID]; ok {
			rs.FormFactor *= f
		}
	}

	segments := PrepareSegments(config.Course.Segments, config.Race, 0)

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
				speed = segmentSpeed(power, rs.Rider.TotalMassKg(), ttCdA, seg, rs.Rider.Handling)
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
	return SimulateStageRaceWithProgress(stageRace, stages, riders, seed, nil, nil)
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
	Prestige      float64 // how coveted the stage is (0-1)
	// Shape, when set, records how the stage unfolds (see RaceShape).
	Shape *RaceShape
	// SeasonalForm: riders race with their form on the stage race's middle
	// day (read from the first stage; see SeasonForm).
	SeasonalForm bool
}
