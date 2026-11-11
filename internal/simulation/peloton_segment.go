package simulation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"cycling_sim/internal/model"
)

// partitionByTimeGap splits riders into road clusters: riders within
// contactSeconds of the rider ahead are in contact and ride as one road group.
// Returns clusters sorted by leader time (fastest cluster first).
func partitionByTimeGap(riders []*RiderState, contact float64) [][]*RiderState {
	if len(riders) <= 1 {
		return [][]*RiderState{riders}
	}
	sorted := make([]*RiderState, len(riders))
	copy(sorted, riders)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TotalTime < sorted[j].TotalTime })

	var clusters [][]*RiderState
	cluster := []*RiderState{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		if sorted[i].TotalTime-sorted[i-1].TotalTime <= contact {
			cluster = append(cluster, sorted[i])
		} else {
			clusters = append(clusters, cluster)
			cluster = []*RiderState{sorted[i]}
		}
	}
	return append(clusters, cluster)
}

// updateBreakawayState derives the breakaway situation from the road: the
// leading road group is a breakaway when it is smaller than the main group.
// BreakawayGap is the actual time gap from the break to the main group.
func updateBreakawayState(state *RaceState, clusters [][]*RiderState) {
	if state == nil {
		return
	}
	main := 0
	for i, c := range clusters {
		if len(c) > len(clusters[main]) {
			main = i
		}
	}
	isBreak := main > 0
	if isBreak && !state.BreakawayActive {
		state.LogEvent("breakaway", breakawayComposition(clusters[0], 6))
	} else if !isBreak && state.BreakawayActive {
		state.LogEvent("breakaway", "breakaway caught")
	}
	state.HasBreakaway = isBreak
	state.BreakawayActive = isBreak
	state.BreakawayRiders = map[int64]bool{}
	for _, c := range clusters[:main] {
		for _, rs := range c {
			state.BreakawayRiders[rs.Rider.ID] = true
		}
	}
	state.BreakawayGap = 0
	state.BreakawayHasDangerous = false
	if !isBreak {
		return
	}
	state.BreakawayGap = clusters[main][0].TotalTime - clusters[0][0].TotalTime
	for _, c := range clusters[:main] {
		for _, rs := range c {
			if state.isDangerous(rs) {
				state.BreakawayHasDangerous = true
			}
		}
	}
}

// resolveCatches enforces that groups can't pass through each other on the
// road: a group that would arrive before the group ahead of it caught that
// group during the segment, and finishes with it.
func resolveCatches(groups []*Group, startTimes map[*RiderState]float64) {
	type span struct {
		g            *Group
		start, front float64
	}
	var spans []span
	for _, g := range groups {
		if len(g.Riders) == 0 {
			continue
		}
		sp := span{g: g, start: startTimes[g.Riders[0]], front: g.Riders[0].TotalTime}
		for _, rs := range g.Riders {
			sp.start = math.Min(sp.start, startTimes[rs])
			sp.front = math.Min(sp.front, rs.TotalTime)
		}
		spans = append(spans, sp)
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	floor := math.Inf(-1)
	for _, sp := range spans {
		if sp.front < floor {
			for _, rs := range sp.g.Riders {
				if rs.TotalTime < floor {
					rs.TotalTime = floor
				}
			}
			sp.front = floor
		}
		floor = sp.front
	}
}

// maxStepKm is the longest distance simulated without riders re-deciding.
// Short steps let the race react on realistic timescales (a few minutes)
// instead of once per course segment, which can be tens of km.
const maxStepKm = 2.0

// fineStepKm is the step in the decisive part of a one-day race (after the
// early phase): attacks, responses and counter-attacks play out over tens of
// seconds, which a 2 km step averages away.
const fineStepKm = 0.25

// fineContactSeconds is the contact range with fine steps. Riders in contact
// close up to the front of their group for free; with 2 km steps a 10 s
// range stood in for the effort of closing ~150 m, but with fine steps it
// would hand a dropped rider that gap back every 20 s (a synthetic mountain
// race lost its climbing order: Spearman 0.98 -> 0.40). Short steps simulate
// the closing effort, so only riders on each other's wheels merge.
const fineContactSeconds = 3.0

// fineSteps reports whether a one-day race is in its decisive part here.
func fineSteps(state *RaceState, isTT bool, remainingKm float64) bool {
	return state != nil && !isTT && !state.IsStageRace && PhaseFromKm(remainingKm) != PhaseEarly
}

// SimulateSegment advances all riders through a segment in short steps.
func SimulateSegment(riders []*RiderState, segment *model.Segment, isTT bool, distanceRemaining float64, rng *rand.Rand, state *RaceState, params *TacticsParams, incidentsEnabled ...bool) {
	n := int(math.Ceil(segment.DistanceKm / maxStepKm))
	if n < 1 {
		n = 1
	}
	step := *segment
	step.DistanceKm = segment.DistanceKm / float64(n)
	if fineSteps(state, isTT, distanceRemaining-segment.DistanceKm) {
		simulateFine(riders, segment, step.DistanceKm, isTT, distanceRemaining, rng, state, params, incidentsEnabled...)
		return
	}
	for i := 0; i < n; i++ {
		if state != nil {
			state.SegmentKmLeft = segment.DistanceKm - float64(i)*step.DistanceKm
		}
		if step.WindMPS > 0 {
			setWindComponents(&step, stepHeading(segment, i))
		}
		simulateStep(riders, &step, isTT, distanceRemaining-float64(i)*step.DistanceKm, rng, state, params, incidentsEnabled...)
	}
}

// simulateFine is SimulateSegment with fineStepKm steps once the race is in
// its decisive part. The road's meander (wind) keeps its coarse-step scale.
func simulateFine(riders []*RiderState, segment *model.Segment, coarseKm float64, isTT bool, distanceRemaining float64, rng *rand.Rand, state *RaceState, params *TacticsParams, incidentsEnabled ...bool) {
	step := *segment
	done := 0.0
	for done < segment.DistanceKm-1e-9 {
		size := maxStepKm
		if fineSteps(state, isTT, distanceRemaining-done) {
			size = fineStepKm
		}
		left := segment.DistanceKm - done
		k := math.Ceil(left/size - 1e-9)
		if k < 1 {
			k = 1
		}
		step.DistanceKm = left / k
		state.SegmentKmLeft = left
		if step.WindMPS > 0 {
			setWindComponents(&step, stepHeadingAt(segment, (done+step.DistanceKm/2)/coarseKm-0.5))
		}
		simulateStep(riders, &step, isTT, distanceRemaining-done, rng, state, params, incidentsEnabled...)
		done += step.DistanceKm
	}
}

// simulateStep processes all riders through one step of a segment.
func simulateStep(riders []*RiderState, segment *model.Segment, isTT bool, distanceRemaining float64, rng *rand.Rand, state *RaceState, params *TacticsParams, incidentsEnabled ...bool) {
	localParams := params
	if localParams == nil {
		defaultParams := DefaultTacticsParams()
		localParams = &defaultParams
	}
	if state != nil {
		state.KmRemaining = distanceRemaining
		state.Phase = PhaseFromKm(distanceRemaining)
		state.Segment = segment
		state.Climb = climbOf(segment)
		// Sprint trains only really matter on flat/rolling finishes.
		state.SprintChaseActive = state.Phase == PhaseFinal && state.Rival.SprintTeamsChasing && state.Climb < terrainDrag && segment.SurfaceType != model.SurfaceCobbles
		if state.SprintChaseActive && !state.LastSprintChase {
			state.LogEvent("sprint-chase", "sprinter teams drive the chase")
		}
		state.LastSprintChase = state.SprintChaseActive
		adjusted := ApplyEventModifiers(*localParams, state)
		localParams = &adjusted
	}

	// Apply random incidents (crashes/punctures) first, if enabled
	enableIncidents := len(incidentsEnabled) > 0 && incidentsEnabled[0]
	if enableIncidents {
		ApplyIncidents(riders, segment, rng)
	}

	// Remove DNF riders from active participation
	var activeRiders []*RiderState
	for _, r := range riders {
		if !r.DNF {
			activeRiders = append(activeRiders, r)
		}
	}

	// Partition riders into road clusters: dropped riders can't teleport back.
	contact := contactSeconds
	if fineSteps(state, isTT, distanceRemaining) {
		contact = fineContactSeconds
	}
	clusters := partitionByTimeGap(activeRiders, contact)
	updateBreakawayState(state, clusters)
	if state != nil {
		state.Clusters = clusters
	}

	// Form groups within each cluster independently
	var groups []*Group
	groupIDOffset := 0
	var ahead []*RiderState
	if state != nil {
		state.lineTime = nil // fatigue and position changed since the last step
		state.reach = 0
		state.canFollow = nil
		state.fastLines = nil
	}
	for ci, cluster := range clusters {
		if state != nil {
			state.Road, state.Chasers, state.ChaseGap = cluster, nil, 0
			if ci+1 < len(clusters) && len(cluster) > 0 && len(clusters[ci+1]) > 0 {
				state.Chasers = clusters[ci+1]
				state.ChaseGap = clusters[ci+1][0].TotalTime - cluster[0].TotalTime
			}
			state.Ahead = make(map[*RiderState]float64, len(ahead))
			if len(cluster) > 0 {
				for _, a := range ahead {
					state.Ahead[a] = cluster[0].TotalTime - a.TotalTime
				}
			}
		}
		clusterGroups := FormGroups(cluster, segment, isTT, distanceRemaining, rng, state, localParams)
		ahead = append(ahead, cluster...)
		for _, g := range clusterGroups {
			g.ID += groupIDOffset
			for _, rs := range g.Riders {
				rs.GroupID += groupIDOffset
			}
		}
		groupIDOffset += len(clusterGroups)
		groups = append(groups, clusterGroups...)
	}

	// Riders in the same group ride together: the back of the group closes up
	// to its front (gaps inside contact distance don't persist).
	if !isTT {
		for _, g := range groups {
			front := math.Inf(1)
			for _, rs := range g.Riders {
				front = math.Min(front, rs.TotalTime)
			}
			for _, rs := range g.Riders {
				rs.TotalTime = front
			}
		}
	}

	startTimes := make(map[*RiderState]float64, len(activeRiders))
	for _, rs := range activeRiders {
		startTimes[rs] = rs.TotalTime
	}

	for _, group := range groups {
		var groupSpeed float64

		// Every group travels together at its speed.
		groupSpeed = group.Speed
		if groupSpeed < minSpeedMPS {
			groupSpeed = minSpeedMPS
		}
		segTime := SegmentTime(groupSpeed, segment.DistanceKm)

		for _, rs := range group.Riders {
			rs.TotalTime += segTime

			// Power this rider actually produces at the group's speed.
			draftFactor := riderCdAFactor(rs, group, segment)
			actualPower := segmentPower(groupSpeed, rs.Rider.TotalMassKg(), rs.Rider.CdA*draftFactor, segment, rs.Rider.Handling)
			if actualPower < 0 {
				actualPower = 0
			}

			rs.WPrime.UpdateWithHandling(actualPower, rs.thresholdIntOn(segment), segTime, rs.Rider.Handling, segment.SurfaceType == model.SurfaceCobbles)
			rs.addWork(actualPower, segTime)
		}
	}
	if !isTT {
		resolveCatches(groups, startTimes)
	}
}

func breakawayComposition(riders []*RiderState, limit int) string {
	if len(riders) == 0 {
		return "breakaway formed"
	}
	if limit <= 0 {
		limit = 1
	}
	if len(riders) < limit {
		limit = len(riders)
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		parts = append(parts, riders[i].Rider.Name)
	}
	if len(riders) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(riders)-limit))
	}
	return "breakaway: " + strings.Join(parts, ", ")
}
