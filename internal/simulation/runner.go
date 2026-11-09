package simulation

import (
	"fmt"
	"math"
	"sort"

	"cycling_sim/internal/model"
)

type ProgressGroup struct {
	Name       string
	GapSeconds float64
	Riders     []string
}

type RiderTelemetryPoint struct {
	SpeedKph   float64 `json:"speedKph"`
	PowerWatts float64 `json:"powerWatts"`
	EffortPct  float64 `json:"effortPct"`
	FatiguePct float64 `json:"fatiguePct"`
	WPrimePct  float64 `json:"wPrimePct"`
	RouteKm    float64 `json:"routeKm"`
	RoutePct   float64 `json:"routePct"`
}

type ProfilePoint struct {
	Km   float64 `json:"km"`
	Elev float64 `json:"elev"`
}

type RouteProfile struct {
	TotalKm float64        `json:"totalKm"`
	Points  []ProfilePoint `json:"points"`
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

func buildRouteProfile(course *model.Course) *RouteProfile {
	if course == nil || len(course.Segments) == 0 {
		return nil
	}
	points := make([]ProfilePoint, 0, len(course.Segments)+1)
	km := 0.0
	elev := 0.0
	points = append(points, ProfilePoint{Km: km, Elev: elev})
	for _, seg := range course.Segments {
		km += seg.DistanceKm
		elev += seg.DistanceKm * 1000.0 * (seg.GradientPct / 100.0)
		points = append(points, ProfilePoint{Km: km, Elev: elev})
	}
	return &RouteProfile{TotalKm: course.TotalDistanceKm, Points: points}
}

// courseHasCobbles returns true when the course has at least 15km of cobbled sectors
// (Paris-Roubaix style, not just rough bergs as in Tour of Flanders).
func courseHasCobbles(course *model.Course) bool {
	if course == nil {
		return false
	}
	cobbleKm := 0.0
	for _, seg := range course.Segments {
		if seg.SurfaceType == model.SurfaceCobbles {
			cobbleKm += seg.DistanceKm
		}
	}
	return cobbleKm >= 15.0
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
	segments := PrepareSegments(config.Course.Segments, config.Race, 0)
	if len(segments) == 0 {
		return nil
	}

	totalDist := 0.0
	for _, seg := range segments {
		totalDist += seg.DistanceKm
	}

	tacticsParams := DefaultTacticsParams()
	hasCobbles := courseHasCobbles(config.Course)
	raceState := NewRaceState(states, rng, config.Race != nil && model.IsStageRaceStageName(config.Race.Name), hasCobbles)
	raceState.Overrides = config.ActionOverrides
	raceState.GCGaps = config.GCGaps
	AssignCoLeaders(states, config.GCGaps, raceState.IsStageRace)
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
		SimulateSegment(states, seg, false, totalDist-coveredDist, rng, raceState, &tacticsParams, config.IncidentsEnabled)
		coveredDist += seg.DistanceKm
		endElapsed := leaderElapsed(states)
		if onProgress != nil {
			events := []string{}
			if raceState != nil && eventIndex < len(raceState.Events) {
				for _, ev := range raceState.Events[eventIndex:] {
					events = append(events, fmt.Sprintf("%s: %s", ev.Type, ev.Reason))
				}
				eventIndex = len(raceState.Events)
			}
			if len(events) == 0 {
				events = append(events, fmt.Sprintf("Segment complete: %s", seg.Name))
			}
			groups, riderGroups := buildProgressData(states, seg)
			steps := timelineStepsForSegment(seg.DistanceKm)
			for step := 1; step <= steps; step++ {
				frac := float64(step) / float64(steps)
				km := segStartDist + seg.DistanceKm*frac
				kmRemaining := totalDist - km
				if kmRemaining < 0 {
					kmRemaining = 0
				}
				elapsed := startElapsed + (endElapsed-startElapsed)*frac
				interpGroups := interpolateProgressGroups(prevGroups, groups, frac)
				interpRiderGroups := interpolateRiderGroups(prevRiderGroups, riderGroups, frac)
				riderTelemetry := buildRiderTelemetry(states, seg, segStartTimes, km, totalDist)
				var teamState *TeamState
				if raceState != nil && raceState.Overrides != nil {
					teamState = raceState.Overrides.SprintTrainState(kmRemaining)
				}
				frameEvents := []string{}
				if step == steps {
					frameEvents = events
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
				})
			}
			prevGroups = groups
			prevRiderGroups = riderGroups
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
				SprintFinish(states[i:j], rng)
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
			Race:            stage.Race,
			Course:          stage.Course,
			Riders:          adjustedRiders,
			Seed:            rng.Int63(),
			ActionOverrides: overrides,
			GCGaps:          gcGaps,
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

		type gcEntry struct {
			riderID int64
			time    float64
		}
		entries := make([]gcEntry, 0, len(gcTimes))
		for id, t := range gcTimes {
			entries = append(entries, gcEntry{riderID: id, time: t})
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
	groups := make([]ProgressGroup, 0, len(clusters))
	riderGroups := make(map[int64]string, len(active))
	for idx, group := range clusters {
		name := groupName(idx, len(group))
		for _, rs := range group {
			riderGroups[rs.Rider.ID] = name
		}
		gap := group[0].TotalTime - leaderTime
		groups = append(groups, ProgressGroup{
			Name:       name,
			GapSeconds: gap,
			Riders:     riderNames(group, 5),
		})
		if len(groups) >= 4 {
			break
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

	out := make([]ProgressGroup, len(current))
	for i, group := range current {
		startGap := group.GapSeconds
		displayRiders := group.Riders
		if prev, ok := prevByName[group.Name]; ok {
			startGap = prev.GapSeconds
			if frac < 1.0 && len(prev.Riders) > 0 {
				displayRiders = prev.Riders
			}
		} else if i < len(previous) {
			startGap = previous[i].GapSeconds
			if frac < 1.0 && len(previous[i].Riders) > 0 {
				displayRiders = previous[i].Riders
			}
		}

		gap := startGap + (group.GapSeconds-startGap)*frac
		if gap < 0 {
			gap = 0
		}

		out[i] = ProgressGroup{
			Name:       group.Name,
			GapSeconds: gap,
			Riders:     displayRiders,
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
