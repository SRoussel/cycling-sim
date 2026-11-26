package simulation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"cycling_sim/internal/model"
)

type ProgressGroup struct {
	Name       string
	GapSeconds float64
	Riders     []string // the first few names (and "+N more")
	Size       int      // riders in the group
}

type RiderTelemetryPoint struct {
	SpeedKph   float64 `json:"speedKph"`
	PowerWatts float64 `json:"powerWatts"`
	EffortPct  float64 `json:"effortPct"`
	FatiguePct float64 `json:"fatiguePct"`
	WPrimePct  float64 `json:"wPrimePct"`
	// LegsPct: the rider's threshold now as a share of his fresh one (the
	// day's durability fade, stage-race fatigue and form included).
	LegsPct float64 `json:"legsPct"`
	// FuelPct: carbohydrate left as a share of a full store (fuel.go).
	FuelPct float64 `json:"fuelPct"`
	// GroupPos: the rider's place in his group's line (1 = in the wind).
	GroupPos int     `json:"groupPos,omitempty"`
	RouteKm  float64 `json:"routeKm"`
	RoutePct float64 `json:"routePct"`
}

type ProfilePoint struct {
	Km   float64 `json:"km"`
	Elev float64 `json:"elev"`
}

type RouteProfile struct {
	TotalKm float64        `json:"totalKm"`
	Points  []ProfilePoint `json:"points"`
	Sectors []RouteSector  `json:"sectors,omitempty"`
}

type ProgressSnapshot struct {
	KmCovered      float64
	KmTotal        float64
	ElapsedTime    float64
	SegmentName    string
	Groups         []ProgressGroup
	RiderGroups    map[int64]string
	RiderTelemetry map[int64]RiderTelemetryPoint
	RouteProfile   *RouteProfile
	TeamState      *TeamState
	Events         []string
	// Race is the true state at the end of each segment (nil on the
	// interpolated frames in between): the input for decision moments.
	Race *RaceView
}

type StageProgressSnapshot struct {
	StageNumber      int
	StageName        string
	StageKmCovered   float64
	StageKmTotal     float64
	StageElapsedTime float64
	OverallKmCovered float64
	OverallKmTotal   float64
	OverallElapsed   float64
	Groups           []ProgressGroup
	RiderGroups      map[int64]string
	RiderTelemetry   map[int64]RiderTelemetryPoint
	RouteProfile     *RouteProfile
	TeamState        *TeamState
	Events           []string
	Race             *RaceView
	// StageStart marks the update before a stage is raced (the moment to
	// plan it); TimeTrial says the stage is one.
	StageStart bool
	TimeTrial  bool
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// RouteSector is a cobbled sector: consecutive cobbled segments of one name.
type RouteSector struct {
	Name     string  `json:"name"`
	StartKm  float64 `json:"startKm"`
	LengthKm float64 `json:"lengthKm"`
	Stars    int     `json:"stars"`   // 1-5 (0: unknown)
	Surface  string  `json:"surface"` // cobbles or gravel
}

// routeSectors merges a course's unpaved segments into sectors (cobbles or
// gravel; a change of surface or name starts a new one).
func routeSectors(course *model.Course) []RouteSector {
	var out []RouteSector
	km := 0.0
	prevCobbled := false
	for _, seg := range course.Segments {
		cobbled := seg.SurfaceType.Unpaved()
		if cobbled {
			n := len(out)
			name := sectorName(seg.Name)
			if prevCobbled && n > 0 && out[n-1].Name == name && out[n-1].Surface == string(seg.SurfaceType) {
				out[n-1].LengthKm += seg.DistanceKm
				if seg.CobbleStars > out[n-1].Stars {
					out[n-1].Stars = seg.CobbleStars
				}
			} else {
				out = append(out, RouteSector{Name: name, StartKm: km, LengthKm: seg.DistanceKm, Stars: seg.CobbleStars, Surface: string(seg.SurfaceType)})
			}
		}
		prevCobbled = cobbled
		km += seg.DistanceKm
	}
	return out
}

// sectorName drops the " (3★)" the stage files append to sector names.
func sectorName(name string) string {
	if i := strings.LastIndex(name, " ("); i > 0 && strings.HasSuffix(name, "★)") {
		return name[:i]
	}
	return name
}

// BuildRouteProfile is a course's profile and sectors, as sent to the game.
func BuildRouteProfile(course *model.Course) *RouteProfile { return buildRouteProfile(course) }

func buildRouteProfile(course *model.Course) *RouteProfile {
	if course == nil || len(course.Segments) == 0 {
		return nil
	}
	points := make([]ProfilePoint, 0, len(course.Segments)+1)
	km := 0.0
	elev := 0.0
	points = append(points, ProfilePoint{Km: km, Elev: elev})
	low := 0.0
	for _, seg := range course.Segments {
		km += seg.DistanceKm
		elev += seg.DistanceKm * 1000.0 * (seg.GradientPct / 100.0)
		low = math.Min(low, elev)
		points = append(points, ProfilePoint{Km: km, Elev: elev})
	}
	// Heights above the course's lowest point, as the physics takes them
	// (withElevation): from the start the profile went below sea level.
	for i := range points {
		points[i].Elev -= low
	}
	return &RouteProfile{TotalKm: course.TotalDistanceKm, Points: points, Sectors: routeSectors(course)}
}

func segmentStartTimes(states []*RiderState) map[int64]float64 {
	out := make(map[int64]float64, len(states))
	for _, rs := range states {
		out[rs.Rider.ID] = rs.TotalTime
	}
	return out
}

func buildRiderTelemetry(states []*RiderState, segment *model.Segment, segmentTimes map[int64]float64, routeKm float64, routeTotalKm float64) map[int64]RiderTelemetryPoint {
	if len(states) == 0 {
		return nil
	}
	out := make(map[int64]RiderTelemetryPoint, len(states))
	routePct := 0.0
	if routeTotalKm > 0 {
		routePct = clampPercent(routeKm / routeTotalKm * 100.0)
	}
	for _, rs := range states {
		if rs.DNF {
			continue
		}
		delta := rs.TotalTime - segmentTimes[rs.Rider.ID]
		speedKph := 0.0
		if segment != nil && delta > 0 {
			speedKph = segment.DistanceKm * 3600.0 / delta
		}
		powerWatts := rs.LastSegmentPower
		effortPct := 0.0
		if rs.EffectiveFTP > 0 {
			effortPct = powerWatts / float64(rs.EffectiveFTP) * 100.0
		}
		fatiguePct := 0.0
		if rs.Rider.FTPWatts > 0 {
			fatiguePct = (1.0 - float64(rs.EffectiveFTP)/float64(rs.Rider.FTPWatts)) * 100.0
		}
		wPrimePct := clampPercent(rs.WPrime.FractionRemaining() * 100.0)
		out[rs.Rider.ID] = RiderTelemetryPoint{
			SpeedKph:   speedKph,
			PowerWatts: powerWatts,
			EffortPct:  effortPct,
			FatiguePct: clampPercent(fatiguePct),
			WPrimePct:  wPrimePct,
			LegsPct:    clampPercent(legsPct(rs)),
			FuelPct:    rs.FuelShare() * 100,
			GroupPos:   rs.GroupPosition,
			RouteKm:    routeKm,
			RoutePct:   routePct,
		}
	}
	return out
}

func leaderElapsed(states []*RiderState) float64 {
	leader := 0.0
	set := false
	for _, rs := range states {
		if rs.DNF {
			continue
		}
		if !set || rs.TotalTime < leader {
			leader = rs.TotalTime
			set = true
		}
	}
	if set {
		return leader
	}
	for _, rs := range states {
		if !set || rs.TotalTime < leader {
			leader = rs.TotalTime
			set = true
		}
	}
	if !set {
		return 0
	}
	return leader
}

// frameEveryKm: inside a long segment, a watched race emits a frame (and
// its director may stop the race) about this often.
const frameEveryKm = 2.0

func timelineStepsForSegment(distanceKm float64) int {
	if distanceKm <= 0 {
		return 1
	}
	steps := int(math.Ceil(distanceKm / 0.8))
	if steps < 1 {
		steps = 1
	}
	if steps > 12 {
		steps = 12
	}
	return steps
}

func cloneRiderGroups(in map[int64]string) map[int64]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[int64]string, len(in))
	for riderID, group := range in {
		out[riderID] = group
	}
	return out
}

func interpolateRiderGroups(previous map[int64]string, current map[int64]string, frac float64) map[int64]string {
	if len(current) == 0 {
		return nil
	}
	if frac >= 1 || len(previous) == 0 {
		return cloneRiderGroups(current)
	}
	return cloneRiderGroups(previous)
}

func SimulateRoadRaceWithProgress(config RaceConfig, onProgress func(ProgressSnapshot)) []model.RaceResult {
	rng := rngFromSeed(config.Seed)

	states := initStates(config.Riders, rng)
	applySeasonForm(states, config.FormDate)
	for _, rs := range states {
		if f, ok := config.RaceForm[rs.Rider.ID]; ok {
			rs.FormFactor *= f
		}
	}
	segments := PrepareSegments(config.Course.Segments, config.Race, 0)
	if len(segments) == 0 {
		return nil
	}

	totalDist := 0.0
	for _, seg := range segments {
		totalDist += seg.DistanceKm
	}

	tacticsParams := DefaultTacticsParams()
	stageRace := config.Race != nil && model.IsStageRaceStageName(config.Race.Name)
	if !stageRace {
		AssignCourseLeaders(states, segments)
	}
	raceState := NewRaceState(states, rng, stageRace)
	if !stageRace {
		raceState.OpenKm = openingKm(segments, states)
	}
	raceState.markFavourites(states, segments)
	raceState.Overrides = config.ActionOverrides
	raceState.GCGaps = config.GCGaps
	AssignCoLeaders(states, config.GCGaps, config.TeamPlan, raceState.IsStageRace)
	applyLeaderChoices(states, config.ActionOverrides) // the player's race plan
	raceState.setBreakIntent(states, segments, rng)    // needs GC gaps and roles
	raceState.setHunting(states, config.Prestige, config.StageWins, rng)
	eventIndex := 0
	routeProfile := buildRouteProfile(config.Course)

	coveredDist := 0.0
	var prevGroups []ProgressGroup
	var prevRiderGroups map[int64]string
	if onProgress != nil {
		var teamState *TeamState
		if config.ActionOverrides != nil {
			teamState = config.ActionOverrides.SprintTrainState(totalDist)
		}
		onProgress(ProgressSnapshot{
			KmCovered:      0,
			KmTotal:        totalDist,
			ElapsedTime:    0,
			SegmentName:    "Start",
			Groups:         nil,
			RiderGroups:    nil,
			RiderTelemetry: nil,
			RouteProfile:   routeProfile,
			TeamState:      teamState,
			Events:         []string{"Race start"},
		})
	}
	for i := range segments {
		seg := &segments[i]
		segStartDist := coveredDist
		segStartTimes := segmentStartTimes(states)
		startElapsed := leaderElapsed(states)
		if raceState != nil {
			raceState.Course, raceState.SegIndex = segments, i
		}
		// emit sends the frames for the part of the segment ridden since
		// the last emit (segDone km into it, up to upTo km).
		segDone := 0.0
		emit := func(upTo float64) {
			events := []string{}
			var rawEvents []TacticsEvent
			if raceState != nil && eventIndex < len(raceState.Events) {
				rawEvents = append(rawEvents, raceState.Events[eventIndex:]...)
				for _, ev := range raceState.Events[eventIndex:] {
					events = append(events, fmt.Sprintf("%s: %s | km=%.1f", ev.Type, ev.Reason, ev.Km))
				}
				eventIndex = len(raceState.Events)
			}
			piece := *seg
			piece.DistanceKm = upTo - segDone
			endElapsed := leaderElapsed(states)
			groups, riderGroups := buildProgressData(states, seg)
			steps := timelineStepsForSegment(piece.DistanceKm)
			for step := 1; step <= steps; step++ {
				frac := float64(step) / float64(steps)
				km := segStartDist + segDone + piece.DistanceKm*frac
				kmRemaining := totalDist - km
				if kmRemaining < 0 {
					kmRemaining = 0
				}
				elapsed := startElapsed + (endElapsed-startElapsed)*frac
				interpGroups := interpolateProgressGroups(prevGroups, groups, frac)
				interpRiderGroups := interpolateRiderGroups(prevRiderGroups, riderGroups, frac)
				riderTelemetry := buildRiderTelemetry(states, &piece, segStartTimes, km, totalDist)
				var teamState *TeamState
				if raceState != nil && raceState.Overrides != nil {
					teamState = raceState.Overrides.SprintTrainState(kmRemaining)
				}
				frameEvents := []string{}
				var view *RaceView
				if step == steps {
					frameEvents = events
					view = buildRaceView(states, raceState, kmRemaining, totalDist, rawEvents)
				}
				onProgress(ProgressSnapshot{
					KmCovered:      km,
					KmTotal:        totalDist,
					ElapsedTime:    elapsed,
					SegmentName:    seg.Name,
					Groups:         interpGroups,
					RiderGroups:    interpRiderGroups,
					RiderTelemetry: riderTelemetry,
					RouteProfile:   nil,
					TeamState:      teamState,
					Events:         frameEvents,
					Race:           view,
				})
			}
			prevGroups = groups
			prevRiderGroups = riderGroups
			segDone, startElapsed, segStartTimes = upTo, endElapsed, segmentStartTimes(states)
		}
		// A long segment is shown (and can be decided on) every few km, not
		// only at its end. Watching only: the CLI and validation (no
		// onProgress) run without the hook.
		if onProgress != nil && raceState != nil && seg.DistanceKm > 2*frameEveryKm {
			raceState.OnStep = func(done float64) {
				if done-segDone >= frameEveryKm && seg.DistanceKm-done >= frameEveryKm/2 {
					emit(done)
				}
			}
		}
		SimulateSegment(states, seg, false, totalDist-coveredDist, rng, raceState, &tacticsParams, config.IncidentsEnabled)
		if raceState != nil {
			raceState.OnStep = nil
		}
		coveredDist += seg.DistanceKm
		config.Shape.observe(raceState, totalDist-coveredDist)
		if onProgress != nil {
			emit(seg.DistanceKm)
		}
	}

	lastSeg := &segments[len(segments)-1]
	if c := ClimbFactor(lastSeg); c <= terrainFlat && c >= -terrainFlat { // flat finish: bunch sprint
		sort.Slice(states, func(i, j int) bool {
			return states[i].TotalTime < states[j].TotalTime
		})
		// Apply SprintFinish to each cluster of riders finishing within 3 s of
		// each other. This covers both the front group (classic bunch sprint) and
		// any large peloton that finishes together behind a breakaway — without it
		// such riders would be ordered by their position in the states slice, which
		// is arbitrary and does not respect sprint ability.
		i := 0
		for i < len(states) {
			if states[i].DNF {
				i++
				continue
			}
			clusterTime := states[i].TotalTime
			j := i + 1
			for j < len(states) && !states[j].DNF && states[j].TotalTime-clusterTime < 3.0 {
				j++
			}
			if j-i > 1 {
				SprintFinish(states[i:j], rng, func(team string) bool {
					return raceState != nil && raceState.teamOrders(team).Leadout == OrderNo
				})
			}
			i = j
		}
	}

	sort.Slice(states, func(i, j int) bool {
		if states[i].DNF && states[j].DNF {
			return states[i].TotalTime < states[j].TotalTime
		}
		if states[i].DNF {
			return false
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
			result.Position = 0
		}
		results = append(results, result)
		if !rs.DNF {
			position++
		}
	}
	return results
}

func SimulateStageRaceWithProgress(stageRace *model.StageRace, stages []StageConfig, riders []model.Rider, seed int64, onProgress func(StageProgressSnapshot), overrides ActionOverrideProvider) ([][]model.RaceResult, [][]model.GCStanding) {
	rng := rngFromSeed(seed)
	// Team plans are made once, before the race (own RNG, so stage seeds
	// don't depend on them).
	planRNG := rand.New(rand.NewSource(rng.Int63()))
	plan := make(map[int64]float64, len(riders))
	for _, r := range riders {
		plan[r.ID] = planRNG.Float64()
	}
	// Race form: how good a race each rider is having, drawn once and held
	// for every stage (on top of day-to-day form). Some riders have a great
	// Giro, others a poor one.
	raceForm := make(map[int64]float64, len(riders))
	for _, r := range riders {
		raceForm[r.ID] = RaceFormFactor(&r, planRNG)
	}
	stageWins := map[int64]int{}
	// Seasonal form holds for the whole race: the form of its middle day.
	var formDate time.Time
	if n := len(stages); n > 0 && stages[0].SeasonalForm && stages[0].Race != nil && stages[n-1].Race != nil {
		first, last := stages[0].Race.Date, stages[n-1].Race.Date
		formDate = first.Add(last.Sub(first) / 2)
	}

	fatigueMap := make(map[int64]*StageFatigueState)
	for i := range riders {
		fatigueMap[riders[i].ID] = NewStageFatigueState(&riders[i])
	}

	var allResults [][]model.RaceResult
	var allStandings [][]model.GCStanding

	gcTimes := make(map[int64]float64)
	for _, r := range riders {
		gcTimes[r.ID] = 0
	}

	overallKmTotal := 0.0
	for _, stage := range stages {
		if stage.Course != nil {
			overallKmTotal += stage.Course.TotalDistanceKm
		}
	}
	overallKmCovered := 0.0
	overallElapsed := 0.0

	activeRiders := make([]model.Rider, len(riders))
	copy(activeRiders, riders)

	for stageIdx, stage := range stages {
		if stage.RestDayBefore {
			for _, f := range fatigueMap {
				f.ApplyRestDay()
			}
		}

		stageKmTotal := 0.0
		if stage.Course != nil {
			stageKmTotal = stage.Course.TotalDistanceKm
		}
		if onProgress != nil {
			stageProfile := buildRouteProfile(stage.Course)
			onProgress(StageProgressSnapshot{
				StageNumber:      stage.StageNumber,
				StageName:        stage.Race.Name,
				StageKmCovered:   0,
				StageKmTotal:     stageKmTotal,
				StageElapsedTime: 0,
				OverallKmCovered: overallKmCovered,
				OverallKmTotal:   overallKmTotal,
				OverallElapsed:   overallElapsed,
				Groups:           nil,
				RiderGroups:      nil,
				RiderTelemetry:   nil,
				RouteProfile:     stageProfile,
				TeamState:        nil,
				Events:           []string{fmt.Sprintf("Stage %d start", stage.StageNumber)},
				StageStart:       true,
				TimeTrial:        stage.Race != nil && stage.Race.RaceType == model.RaceTypeTT,
			})
		}

		adjustedRiders := make([]model.Rider, len(activeRiders))
		copy(adjustedRiders, activeRiders)
		for i := range adjustedRiders {
			if f, ok := fatigueMap[adjustedRiders[i].ID]; ok {
				adjustedRiders[i].FTPWatts = f.EffectiveFTP(adjustedRiders[i].FTPWatts)
			}
		}

		// Compute GC gaps (seconds behind leader) so tactics can adapt to race situation.
		var gcGaps map[int64]float64
		if len(gcTimes) > 0 {
			leaderTime := math.MaxFloat64
			for _, t := range gcTimes {
				if t < leaderTime {
					leaderTime = t
				}
			}
			gcGaps = make(map[int64]float64, len(gcTimes))
			for id, t := range gcTimes {
				gcGaps[id] = t - leaderTime
			}
		}

		config := RaceConfig{
			Race:             stage.Race,
			Course:           stage.Course,
			Riders:           adjustedRiders,
			Seed:             rng.Int63(),
			ActionOverrides:  overrides,
			GCGaps:           gcGaps,
			TeamPlan:         plan,
			Shape:            stage.Shape,
			Prestige:         stage.Prestige,
			StageWins:        stageWins,
			FormDate:         formDate,
			RaceForm:         raceForm,
			IncidentsEnabled: stage.Incidents,
		}

		var results []model.RaceResult
		stageElapsedForUpdate := 0.0
		if stage.Race.RaceType == model.RaceTypeTT {
			results = SimulateTT(config)
			stageElapsed := 0.0
			if len(results) > 0 {
				stageElapsed = results[0].FinishTimeSeconds
			}
			stageElapsedForUpdate = stageElapsed
			overallElapsed += stageElapsed
			if onProgress != nil {
				overallKmCovered += stageKmTotal
				onProgress(StageProgressSnapshot{
					StageNumber:      stage.StageNumber,
					StageName:        stage.Race.Name,
					StageKmCovered:   stageKmTotal,
					StageKmTotal:     stageKmTotal,
					StageElapsedTime: stageElapsed,
					OverallKmCovered: overallKmCovered,
					OverallKmTotal:   overallKmTotal,
					OverallElapsed:   overallElapsed,
					Groups:           nil,
					RiderGroups:      nil,
					RiderTelemetry:   nil,
					RouteProfile:     nil,
					TeamState:        nil,
					Events:           []string{"Stage completed"},
				})
			}
		} else {
			stageElapsed := 0.0
			results = SimulateRoadRaceWithProgress(config, func(snapshot ProgressSnapshot) {
				if onProgress == nil {
					return
				}
				stageElapsed = snapshot.ElapsedTime
				onProgress(StageProgressSnapshot{
					StageNumber:      stage.StageNumber,
					StageName:        stage.Race.Name,
					StageKmCovered:   snapshot.KmCovered,
					StageKmTotal:     snapshot.KmTotal,
					StageElapsedTime: snapshot.ElapsedTime,
					OverallKmCovered: overallKmCovered + snapshot.KmCovered,
					OverallKmTotal:   overallKmTotal,
					OverallElapsed:   overallElapsed + snapshot.ElapsedTime,
					Groups:           snapshot.Groups,
					RiderGroups:      snapshot.RiderGroups,
					RiderTelemetry:   snapshot.RiderTelemetry,
					RouteProfile:     snapshot.RouteProfile,
					TeamState:        snapshot.TeamState,
					Events:           snapshot.Events,
					Race:             snapshot.Race,
				})
			})
			stageElapsedForUpdate = stageElapsed
			overallElapsed += stageElapsed
			overallKmCovered += stageKmTotal
			if onProgress != nil {
				onProgress(StageProgressSnapshot{
					StageNumber:      stage.StageNumber,
					StageName:        stage.Race.Name,
					StageKmCovered:   stageKmTotal,
					StageKmTotal:     stageKmTotal,
					StageElapsedTime: stageElapsed,
					OverallKmCovered: overallKmCovered,
					OverallKmTotal:   overallKmTotal,
					OverallElapsed:   overallElapsed,
					Groups:           nil,
					RiderGroups:      nil,
					RiderTelemetry:   nil,
					RouteProfile:     nil,
					TeamState:        nil,
					Events:           []string{fmt.Sprintf("Stage %d complete", stage.StageNumber)},
				})
			}
		}

		if len(results) > 0 {
			stageWins[results[0].RiderID]++
		}
		if cut := ApplyTimeCut(results, stage.Course, stage.Race.RaceType); cut > 0 && onProgress != nil {
			onProgress(StageProgressSnapshot{
				StageNumber:      stage.StageNumber,
				StageName:        stage.Race.Name,
				StageKmCovered:   stageKmTotal,
				StageKmTotal:     stageKmTotal,
				StageElapsedTime: stageElapsedForUpdate,
				OverallKmCovered: overallKmCovered,
				OverallKmTotal:   overallKmTotal,
				OverallElapsed:   overallElapsed,
				Events:           []string{fmt.Sprintf("%d riders outside the time limit", cut)},
			})
		}
		allResults = append(allResults, results)

		// Carry the unadjusted riders forward: fatigue is applied afresh to
		// the base FTP each stage (carrying adjustedRiders compounded it).
		activeByID := make(map[int64]model.Rider, len(activeRiders))
		for _, r := range activeRiders {
			activeByID[r.ID] = r
		}
		remaining := make([]model.Rider, 0, len(activeRiders))
		for _, res := range results {
			if res.Position == 0 {
				delete(gcTimes, res.RiderID)
				delete(fatigueMap, res.RiderID)
				continue
			}
			gcTimes[res.RiderID] += res.FinishTimeSeconds - stageBonus(res.Position, stage.Race.RaceType)
			stageStress := StageStress(stage.Course, stage.Race.RaceType, res.TimeGapSeconds)
			if f, ok := fatigueMap[res.RiderID]; ok {
				f.AddStageStress(stageStress)
			}
			if rider, ok := activeByID[res.RiderID]; ok {
				remaining = append(remaining, rider)
			}
		}
		activeRiders = remaining

		type gcEntry struct {
			riderID int64
			time    float64
		}
		entries := make([]gcEntry, 0, len(gcTimes))
		for id, t := range gcTimes {
			entries = append(entries, gcEntry{riderID: id, time: t})
		}
		// Ties (same total time) go by this stage's place, as real
		// countback does, then rider ID: never map order.
		stagePos := make(map[int64]int, len(results))
		for _, res := range results {
			stagePos[res.RiderID] = res.Position
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].time != entries[j].time {
				return entries[i].time < entries[j].time
			}
			if pi, pj := stagePos[entries[i].riderID], stagePos[entries[j].riderID]; pi != pj {
				return pi < pj
			}
			return entries[i].riderID < entries[j].riderID
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

		if onProgress != nil {
			onProgress(StageProgressSnapshot{
				StageNumber:      stage.StageNumber,
				StageName:        stage.Race.Name,
				StageKmCovered:   stageKmTotal,
				StageKmTotal:     stageKmTotal,
				StageElapsedTime: stageElapsedForUpdate,
				OverallKmCovered: overallKmCovered,
				OverallKmTotal:   overallKmTotal,
				OverallElapsed:   overallElapsed,
				Groups:           nil,
				RiderGroups:      nil,
				RiderTelemetry:   nil,
				RouteProfile:     nil,
				TeamState:        nil,
				Events:           []string{fmt.Sprintf("GC updated after stage %d", stageIdx+1)},
			})
		}
	}

	return allResults, allStandings
}

// maxProgressGroups: the live view lists at most this many road groups.
const maxProgressGroups = 8

func buildProgressData(states []*RiderState, segment *model.Segment) ([]ProgressGroup, map[int64]string) {
	active := make([]*RiderState, 0, len(states))
	for _, rs := range states {
		if !rs.DNF {
			active = append(active, rs)
		}
	}
	if len(active) == 0 {
		return nil, nil
	}

	sort.Slice(active, func(i, j int) bool {
		return active[i].TotalTime < active[j].TotalTime
	})

	gapThreshold := progressGapThreshold(segment)
	clusters := make([][]*RiderState, 0)
	cluster := []*RiderState{active[0]}
	for i := 1; i < len(active); i++ {
		gap := active[i].TotalTime - active[i-1].TotalTime
		if gap <= gapThreshold {
			cluster = append(cluster, active[i])
		} else {
			clusters = append(clusters, cluster)
			cluster = []*RiderState{active[i]}
		}
	}
	clusters = append(clusters, cluster)

	leaderTime := active[0].TotalTime
	// The largest group of 15 or more is the peloton, wherever it is.
	peloton := -1
	for idx, group := range clusters {
		if len(group) >= 15 && (peloton < 0 || len(group) > len(clusters[peloton])) {
			peloton = idx
		}
	}
	groups := make([]ProgressGroup, 0, len(clusters))
	riderGroups := make(map[int64]string, len(active))
	for idx, group := range clusters {
		name := groupName(idx, len(group))
		if idx == peloton {
			name = "Peloton"
		} else if name == "Peloton" {
			name = fmt.Sprintf("Group %d", idx+1)
		}
		for _, rs := range group {
			riderGroups[rs.Rider.ID] = name // every rider gets his group
		}
		if len(groups) < maxProgressGroups {
			groups = append(groups, ProgressGroup{
				Name:       name,
				GapSeconds: group[0].TotalTime - leaderTime,
				Riders:     riderNames(group, groupNamesShown(len(group))),
				Size:       len(group),
			})
		}
	}
	return groups, riderGroups
}

func interpolateProgressGroups(previous []ProgressGroup, current []ProgressGroup, frac float64) []ProgressGroup {
	if len(current) == 0 {
		return nil
	}
	if frac <= 0 {
		frac = 0
	}
	if frac >= 1 {
		groups := make([]ProgressGroup, len(current))
		copy(groups, current)
		return groups
	}

	prevByName := make(map[string]ProgressGroup, len(previous))
	for _, group := range previous {
		prevByName[group.Name] = group
	}

	// Only the gap eases between updates; who is in a group is always the
	// current state (showing the previous members mislabelled groups whose
	// membership changed, e.g. a 1-rider "Lead" listing the old bunch).
	out := make([]ProgressGroup, len(current))
	for i, group := range current {
		startGap := group.GapSeconds
		if prev, ok := prevByName[group.Name]; ok {
			startGap = prev.GapSeconds
		}
		gap := startGap + (group.GapSeconds-startGap)*frac
		if gap < 0 {
			gap = 0
		}

		out[i] = ProgressGroup{
			Name:       group.Name,
			GapSeconds: gap,
			Riders:     group.Riders,
			Size:       group.Size,
		}
	}

	return out
}

func progressGapThreshold(_ *model.Segment) float64 {
	return contactSeconds // display groups match the model's road contact
}

func groupName(index int, groupSize int) string {
	switch index {
	case 0:
		return "Lead"
	case 1:
		return "Chasers"
	case 2:
		if groupSize >= 12 {
			return "Peloton"
		}
		return "Group 3"
	default:
		return fmt.Sprintf("Group %d", index+1)
	}
}

// groupNamesShown is how many names a progress group lists: everyone in a
// group the player can read at a glance, the first few of a big bunch.
func groupNamesShown(size int) int {
	if size <= 30 {
		return size
	}
	return 5
}

func riderNames(group []*RiderState, limit int) []string {
	if len(group) == 0 {
		return nil
	}
	if len(group) <= limit {
		names := make([]string, len(group))
		for i, rs := range group {
			names[i] = rs.Rider.Name
		}
		return names
	}

	names := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		names = append(names, group[i].Rider.Name)
	}
	names = append(names, fmt.Sprintf("+%d more", len(group)-limit))
	return names
}

// legsPct is a rider's current threshold as a percentage of his rated one.
func legsPct(rs *RiderState) float64 {
	if rs.Rider.FTPWatts <= 0 {
		return 100
	}
	return rs.CurrentFTP() / float64(rs.Rider.FTPWatts) * 100
}
