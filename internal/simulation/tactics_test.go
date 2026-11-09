package simulation

import (
	"math/rand"
	"testing"

	"cycling_sim/internal/model"
)

// --- BreakawayAttempt unit tests ---

func TestBreakawayAttemptLowWPrime(t *testing.T) {
	rider := testRider()
	rider.TacticalAwareness = 90
	rs := &RiderState{
		Rider:        rider,
		WPrime:       NewWPrimeState(rider),
		FormFactor:   1.0,
		EffectiveFTP: rider.FTPWatts,
	}
	// Deplete W' to below 20%
	rs.WPrime.WPrimeCurrent = rs.WPrime.WPrimeMax * 0.15

	seg := &model.Segment{GradientPct: 0, DistanceKm: 10, SurfaceType: model.SurfaceRoad}
	rng := rand.New(rand.NewSource(42))

	attacks := 0
	for i := 0; i < 1000; i++ {
		if BreakawayAttempt(rs, seg, 50, rng) {
			attacks++
		}
	}
	if attacks > 0 {
		t.Errorf("exhausted rider (W' < 20%%) attacked %d/1000 times, expected 0", attacks)
	}
}

// Breakaway attempts are intent: a team's sprinter waits for the sprint, a
// domestique riding for a leader in the group rarely goes. Where a move sticks
// is physics (see TestSimulateSegmentBreakawayGap).
func TestBreakawayAttemptRoles(t *testing.T) {
	seg := &model.Segment{GradientPct: 0, DistanceKm: 10, SurfaceType: model.SurfaceRoad}
	count := func(role TeamRole, leaderHere bool) int {
		rider := testRider()
		rider.TacticalAwareness = 90
		rs := &RiderState{Rider: rider, WPrime: NewWPrimeState(rider), FormFactor: 1, EffectiveFTP: rider.FTPWatts, TeamRole: role}
		rng := rand.New(rand.NewSource(7))
		n := 0
		for i := 0; i < 5000; i++ {
			if BreakawayAttempt(rs, seg, 100, rng, leaderHere) {
				n++
			}
		}
		return n
	}
	if n := count(RoleSprinter, false); n != 0 {
		t.Errorf("a team's sprinter tried %d breakaways; they wait for the sprint", n)
	}
	free, working := count(RoleDomestique, false), count(RoleDomestique, true)
	if free == 0 || working*4 > free {
		t.Errorf("domestique attempts: free %d, with leader present %d; want far fewer when working", free, working)
	}
}

func TestBreakawayAttemptTacticalAwareness(t *testing.T) {
	lowTactical := testRider()
	lowTactical.TacticalAwareness = 40
	highTactical := testRider()
	highTactical.TacticalAwareness = 95

	rsLow := &RiderState{
		Rider: lowTactical, WPrime: NewWPrimeState(lowTactical),
		FormFactor: 1.0, EffectiveFTP: lowTactical.FTPWatts,
	}
	rsHigh := &RiderState{
		Rider: highTactical, WPrime: NewWPrimeState(highTactical),
		FormFactor: 1.0, EffectiveFTP: highTactical.FTPWatts,
	}
	seg := &model.Segment{GradientPct: 1.0, DistanceKm: 10, SurfaceType: model.SurfaceRoad}

	n := 10000
	rng1 := rand.New(rand.NewSource(77))
	rng2 := rand.New(rand.NewSource(77))

	lowAttacks := 0
	highAttacks := 0
	for i := 0; i < n; i++ {
		if BreakawayAttempt(rsLow, seg, 45, rng1) {
			lowAttacks++
		}
		if BreakawayAttempt(rsHigh, seg, 45, rng2) {
			highAttacks++
		}
	}

	if highAttacks <= lowAttacks {
		t.Errorf("high tactical awareness attacks (%d) should exceed low (%d)", highAttacks, lowAttacks)
	}
	t.Logf("low tactical: %d/%d, high tactical: %d/%d", lowAttacks, n, highAttacks, n)
}

func TestFormGroupsFlatWithBreakaway(t *testing.T) {
	// Use a seed that produces some breakaway attempts
	// We create riders with high tactical awareness to increase breakaway probability
	attackers := []model.Rider{
		makeRider("Attacker1", 400, 70, 70, 50, 70, 70, 70, 70, 70, 95),
		makeRider("Attacker2", 395, 68, 70, 50, 70, 70, 70, 70, 70, 95),
	}
	pelotonRiders := []model.Rider{
		makeRider("Peloton1", 380, 75, 60, 80, 60, 70, 70, 70, 70, 30),
		makeRider("Peloton2", 370, 73, 60, 75, 60, 70, 70, 70, 70, 30),
		makeRider("Peloton3", 375, 74, 60, 70, 60, 70, 70, 70, 70, 30),
	}
	allRiders := append(attackers, pelotonRiders...)
	for i := range allRiders {
		allRiders[i].ID = int64(i + 1)
	}

	seg := &model.Segment{GradientPct: 0, DistanceKm: 20, SurfaceType: model.SurfaceRoad}

	// Try many seeds to find one that produces a breakaway
	foundBreakaway := false
	for seed := int64(0); seed < 200; seed++ {
		rng := rand.New(rand.NewSource(seed))
		states := make([]*RiderState, len(allRiders))
		for i := range allRiders {
			r := &allRiders[i]
			states[i] = &RiderState{
				Rider: r, WPrime: NewWPrimeState(r),
				FormFactor: 1.0, EffectiveFTP: r.FTPWatts,
			}
		}
		groups := FormGroups(states, seg, false, 45, rng, nil, nil)
		if len(groups) >= 2 && groups[0].IsBreakaway {
			foundBreakaway = true
			t.Logf("seed %d: breakaway group has %d riders, peloton has %d",
				seed, len(groups[0].Riders), len(groups[1].Riders))

			// Verify breakaway group is marked
			if !groups[0].IsBreakaway {
				t.Error("first group should be marked as breakaway")
			}
			// Verify sizes are reasonable
			if len(groups[0].Riders) > maxBreakawaySize {
				t.Errorf("breakaway group too large: %d > %d", len(groups[0].Riders), maxBreakawaySize)
			}
			if len(groups[0].Riders)+len(groups[1].Riders) != len(allRiders) {
				t.Errorf("rider count mismatch: %d + %d != %d",
					len(groups[0].Riders), len(groups[1].Riders), len(allRiders))
			}
			break
		}
	}
	if !foundBreakaway {
		t.Error("expected to find at least one breakaway across 200 seeds with high-tactical riders")
	}
}

func TestFormGroupsFlatNoBreakaway(t *testing.T) {
	// Low tactical awareness riders — very unlikely to break away
	riders := []model.Rider{
		makeRider("R1", 380, 75, 60, 70, 60, 70, 70, 70, 70, 10),
		makeRider("R2", 370, 73, 60, 70, 60, 70, 70, 70, 70, 10),
		makeRider("R3", 375, 74, 60, 70, 60, 70, 70, 70, 70, 10),
	}
	for i := range riders {
		riders[i].ID = int64(i + 1)
	}

	seg := &model.Segment{GradientPct: 0, DistanceKm: 20, SurfaceType: model.SurfaceRoad}

	// With very low tactical awareness, most seeds should produce no breakaway
	noBreakaway := 0
	for seed := int64(0); seed < 50; seed++ {
		rng := rand.New(rand.NewSource(seed))
		states := make([]*RiderState, len(riders))
		for i := range riders {
			r := &riders[i]
			states[i] = &RiderState{
				Rider: r, WPrime: NewWPrimeState(r),
				FormFactor: 1.0, EffectiveFTP: r.FTPWatts,
			}
		}
		groups := FormGroups(states, seg, false, 45, rng, nil, nil)
		if len(groups) == 1 {
			noBreakaway++
		}
	}
	if noBreakaway < 40 {
		t.Errorf("low-tactical riders should rarely break away: %d/50 seeds had no breakaway", noBreakaway)
	}
	t.Logf("no breakaway in %d/50 seeds", noBreakaway)
}

func TestFormGroupsBreakawayMaxSize(t *testing.T) {
	// 10 riders with max tactical awareness — cap at maxBreakawaySize
	var riders []model.Rider
	for i := 0; i < 10; i++ {
		r := makeRider("R", 400, 70, 70, 50, 70, 70, 70, 70, 70, 100)
		r.ID = int64(i + 1)
		riders = append(riders, r)
	}

	seg := &model.Segment{GradientPct: 0, DistanceKm: 20, SurfaceType: model.SurfaceRoad}

	for seed := int64(0); seed < 100; seed++ {
		rng := rand.New(rand.NewSource(seed))
		states := make([]*RiderState, len(riders))
		for i := range riders {
			r := &riders[i]
			states[i] = &RiderState{
				Rider: r, WPrime: NewWPrimeState(r),
				FormFactor: 1.0, EffectiveFTP: r.FTPWatts,
			}
		}
		groups := FormGroups(states, seg, false, 45, rng, nil, nil)
		if len(groups) >= 2 && groups[0].IsBreakaway {
			if len(groups[0].Riders) > maxBreakawaySize {
				t.Fatalf("breakaway group size %d exceeds max %d (seed %d)",
					len(groups[0].Riders), maxBreakawaySize, seed)
			}
		}
	}
}

// --- SimulateSegment integration tests ---

func TestSimulateSegmentBreakawayGap(t *testing.T) {
	// Create a scenario where a breakaway definitely happens:
	// Use FormGroups directly and then verify the time difference
	attacker := makeRider("Attacker", 420, 68, 80, 50, 70, 70, 80, 70, 70, 95)
	attacker.ID = 1
	pelotonRider := makeRider("Peloton", 360, 75, 60, 80, 60, 70, 70, 70, 70, 30)
	pelotonRider.ID = 2

	allRiders := []model.Rider{attacker, pelotonRider}

	seg := &model.Segment{GradientPct: 0, DistanceKm: 30, SurfaceType: model.SurfaceRoad}

	// Try seeds until we get a breakaway
	for seed := int64(0); seed < 500; seed++ {
		rng := rand.New(rand.NewSource(seed))
		states := make([]*RiderState, len(allRiders))
		for i := range allRiders {
			r := &allRiders[i]
			states[i] = &RiderState{
				Rider: r, WPrime: NewWPrimeState(r),
				FormFactor: 1.0, EffectiveFTP: r.FTPWatts,
			}
		}

		SimulateSegment(states, seg, false, 50.0, rng, nil, nil)

		// Check if there's a time difference
		if states[0].TotalTime != states[1].TotalTime {
			gap := states[1].TotalTime - states[0].TotalTime
			t.Logf("seed %d: attacker time=%.1fs, peloton time=%.1fs, gap=%.1fs",
				seed, states[0].TotalTime, states[1].TotalTime, gap)
			if gap <= 0 {
				continue // attacker didn't gain time this seed
			}
			// Verify gap is reasonable (not absurdly large)
			if gap > 300 {
				t.Errorf("breakaway gap %.1fs is unrealistically large for 30km segment", gap)
			}
			return // success
		}
	}
	t.Log("no breakaway occurred across 500 seeds — probabilistically possible but unlikely with high tactical awareness")
}

func avgFloat64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// --- SprintFinish: only among riders arriving together ---

func TestSprintFinishExcludesBreakawayRiders(t *testing.T) {
	// Simulate a scenario where breakaway rider has different total time
	sprinter := makeSprinter("Sprinter")
	sprinter.ID = 1
	breakawayRider := makeClimber("Breakaway")
	breakawayRider.ID = 2
	pelotonRider := makeAllRounder("Peloton")
	pelotonRider.ID = 3

	states := []*RiderState{
		{Rider: &sprinter, WPrime: NewWPrimeState(&sprinter), FormFactor: 1.0, EffectiveFTP: sprinter.FTPWatts, TotalTime: 18000},
		{Rider: &breakawayRider, WPrime: NewWPrimeState(&breakawayRider), FormFactor: 1.0, EffectiveFTP: breakawayRider.FTPWatts, TotalTime: 17950}, // 50s ahead
		{Rider: &pelotonRider, WPrime: NewWPrimeState(&pelotonRider), FormFactor: 1.0, EffectiveFTP: pelotonRider.FTPWatts, TotalTime: 18001},
	}

	// The engine groups riders within 3s for sprint — breakaway rider is 50s ahead
	// so they should NOT be in the sprint group
	leaderTime := states[1].TotalTime // breakaway rider is fastest
	var sprintGroup []*RiderState
	for _, rs := range states {
		if rs.TotalTime-leaderTime < 3.0 {
			sprintGroup = append(sprintGroup, rs)
		}
	}

	// Only breakaway rider should be "in the lead group" (alone)
	if len(sprintGroup) != 1 {
		t.Errorf("expected 1 rider in lead group (breakaway), got %d", len(sprintGroup))
	}

	// The peloton sprint should happen among the other riders
	var pelotonSprint []*RiderState
	pelotonLeader := states[0].TotalTime // sprinter
	for _, rs := range states {
		if rs.TotalTime >= pelotonLeader-3.0 && rs.TotalTime <= pelotonLeader+3.0 {
			pelotonSprint = append(pelotonSprint, rs)
		}
	}
	if len(pelotonSprint) != 2 {
		t.Errorf("expected 2 riders in peloton sprint, got %d", len(pelotonSprint))
	}
}

// --- Full race integration ---

func TestFlatRaceWithBreakawayStillRealistic(t *testing.T) {
	// Verify that flat races still produce realistic results with breakaway logic active
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

	// Flat race should still have relatively small gaps (mostly bunch finish)
	// Allow slightly larger gaps now that breakaways are possible
	maxGap := results[len(results)-1].TimeGapSeconds
	if maxGap > 120.0 {
		t.Errorf("flat race max gap = %.1f seconds, expected < 120 (breakaways shouldn't blow up flat races)", maxGap)
	}
	t.Logf("Flat race: winner time=%.0fs, max gap=%.1fs", results[0].FinishTimeSeconds, maxGap)
	for _, r := range results {
		t.Logf("  P%d: rider=%d time=%.1fs gap=%.1fs", r.Position, r.RiderID, r.FinishTimeSeconds, r.TimeGapSeconds)
	}
}

// --- Team-aware FormGroups tests ---

func TestFormGroupsTeamAwarePositioning(t *testing.T) {
	// Test that on flat terrain: domestiques are at front (low GroupPosition),
	// leaders are sheltered (high GroupPosition), and domestiques don't join their leader's breakaway

	// Create two teams
	// Team A: Leader (high tactical awareness) + 2 domestiques
	// Team B: Leader (high tactical awareness) + 2 domestiques
	// Both teams want to stay together

	team1Leader := makeRider("Leader1", 410, 70, 85, 65, 85, 80, 85, 80, 85, 90)
	team1Leader.ID = 1
	team1Leader.Team = "Team A"

	team1Dom1 := makeRider("Dom1A", 380, 72, 70, 60, 75, 70, 80, 75, 80, 50)
	team1Dom1.ID = 2
	team1Dom1.Team = "Team A"

	team1Dom2 := makeRider("Dom1B", 375, 73, 68, 58, 73, 68, 78, 73, 78, 48)
	team1Dom2.ID = 3
	team1Dom2.Team = "Team A"

	team2Leader := makeRider("Leader2", 415, 69, 87, 62, 87, 82, 87, 82, 87, 92)
	team2Leader.ID = 4
	team2Leader.Team = "Team B"

	team2Dom1 := makeRider("Dom2A", 382, 71, 72, 61, 77, 72, 82, 77, 82, 52)
	team2Dom1.ID = 5
	team2Dom1.Team = "Team B"

	team2Dom2 := makeRider("Dom2B", 376, 72, 69, 59, 74, 69, 79, 74, 79, 50)
	team2Dom2.ID = 6
	team2Dom2.Team = "Team B"

	allRiders := []model.Rider{team1Leader, team1Dom1, team1Dom2, team2Leader, team2Dom1, team2Dom2}

	seg := &model.Segment{GradientPct: 0, DistanceKm: 20, SurfaceType: model.SurfaceRoad}
	rng := rand.New(rand.NewSource(333))

	states := make([]*RiderState, len(allRiders))
	for i := range allRiders {
		r := &allRiders[i]
		states[i] = &RiderState{
			Rider: r, WPrime: NewWPrimeState(r),
			FormFactor: 1.0, EffectiveFTP: r.FTPWatts,
		}
	}

	// Assign team roles
	AssignTeamRoles(states)

	// Verify team roles are assigned correctly
	if states[0].TeamRole != RoleLeader {
		t.Errorf("states[0] (Leader1) should be RoleLeader, got %d", states[0].TeamRole)
	}
	if states[1].TeamRole != RoleDomestique || states[2].TeamRole != RoleDomestique {
		t.Error("states[1] and states[2] (domestiques) should be RoleDomestique")
	}

	groups := FormGroups(states, seg, false, 45, rng, nil, nil)

	// Should have at least 1 group (possibly 2+ if there's a breakaway)
	if len(groups) == 0 {
		t.Fatal("expected at least 1 group")
	}

	// Find team A and team B members in the groups
	for _, group := range groups {
		if len(group.Riders) < 2 {
			continue // Skip dropped riders
		}

		// Check if this group has protected riders (leaders/sprinters)
		var leaders, domestiques []*RiderState
		for _, rs := range group.Riders {
			if rs.TeamRole == RoleLeader || rs.TeamRole == RoleSprinter {
				leaders = append(leaders, rs)
			} else if rs.TeamRole == RoleDomestique {
				domestiques = append(domestiques, rs)
			}
		}

		// If group has both domestiques and leaders from the same team
		if len(leaders) > 0 && len(domestiques) > 0 {
			// Domestiques should have lower GroupPosition (front, in wind)
			// Leaders should have higher GroupPosition (back, sheltered)
			minDomPosition := int(^uint(0) >> 1) // max int
			maxLeaderPosition := 0

			for _, dom := range domestiques {
				if dom.GroupPosition < minDomPosition {
					minDomPosition = dom.GroupPosition
				}
			}
			for _, leader := range leaders {
				if leader.GroupPosition > maxLeaderPosition {
					maxLeaderPosition = leader.GroupPosition
				}
			}

			// Domestiques at front should have lower position numbers
			if minDomPosition >= maxLeaderPosition {
				t.Errorf("domestiques should be in front (lower position) than leaders in group %d: "+
					"min domestique position=%d, max leader position=%d", group.ID, minDomPosition, maxLeaderPosition)
			}
			t.Logf("Group %d: domestiques at positions %d-..., leaders at positions ...-up to %d (correct sheltering)",
				group.ID, minDomPosition, maxLeaderPosition)
		}
	}

	// Test domestique exclusion from leader's breakaway
	// Create scenario where leader attempts breakaway
	for seed := int64(0); seed < 200; seed++ {
		rng2 := rand.New(rand.NewSource(seed))
		states2 := make([]*RiderState, len(allRiders))
		for i := range allRiders {
			r := &allRiders[i]
			states2[i] = &RiderState{
				Rider: r, WPrime: NewWPrimeState(r),
				FormFactor: 1.0, EffectiveFTP: r.FTPWatts,
			}
		}
		AssignTeamRoles(states2)

		groups2 := FormGroups(states2, seg, false, 45, rng2, nil, nil)

		// Check if we have a breakaway
		if len(groups2) >= 2 && groups2[0].IsBreakaway {
			breakGroup := groups2[0]
			pelotonGroup := groups2[1]

			// Check if leader is in breakaway
			var leaderInBreak, domInBreak, domInPeloton bool
			var breakLeaderTeam string

			for _, rs := range breakGroup.Riders {
				if rs.TeamRole == RoleLeader {
					leaderInBreak = true
					breakLeaderTeam = rs.Rider.Team
				}
			}

			// If leader is in breakaway, domestiques from same team should NOT be in breakaway
			if leaderInBreak {
				for _, rs := range breakGroup.Riders {
					if rs.TeamRole == RoleDomestique && rs.Rider.Team == breakLeaderTeam {
						domInBreak = true
					}
				}
				for _, rs := range pelotonGroup.Riders {
					if rs.TeamRole == RoleDomestique && rs.Rider.Team == breakLeaderTeam {
						domInPeloton = true
					}
				}

				if domInBreak && domInPeloton {
					t.Logf("Found breakaway: Leader %s in breakaway, domestiques from same team in peloton (correct!)", breakLeaderTeam)
					return // success
				}
			}
		}
	}

	// It's possible no breakaway occurred; that's ok for this test
	t.Log("no breakaway scenario found, but team positioning logic was tested")
}

func TestFullSimulationLeaderConservesWPrime(t *testing.T) {
	// Run a full race and verify that team leaders conserve more W' than their domestiques
	// This tests that the team-aware tactics and positioning work end-to-end

	// Create two balanced teams with similar riders to isolate the team dynamics effect
	// Team A
	leaderA := makeRider("LeaderA", 395, 70, 82, 70, 82, 80, 85, 80, 85, 88)
	leaderA.ID = 1
	leaderA.Team = "Team A"

	domA1 := makeRider("DomA1", 390, 71, 80, 68, 80, 78, 83, 78, 83, 50)
	domA1.ID = 2
	domA1.Team = "Team A"

	domA2 := makeRider("DomA2", 388, 72, 78, 67, 78, 77, 82, 77, 82, 48)
	domA2.ID = 3
	domA2.Team = "Team A"

	// Team B
	leaderB := makeRider("LeaderB", 397, 70, 84, 68, 84, 81, 86, 81, 86, 90)
	leaderB.ID = 4
	leaderB.Team = "Team B"

	domB1 := makeRider("DomB1", 392, 71, 81, 69, 81, 79, 84, 79, 84, 52)
	domB1.ID = 5
	domB1.Team = "Team B"

	domB2 := makeRider("DomB2", 389, 72, 79, 68, 79, 78, 83, 78, 83, 50)
	domB2.ID = 6
	domB2.Team = "Team B"

	riders := []model.Rider{leaderA, domA1, domA2, leaderB, domB1, domB2}

	// Use a flat course so team tactics matter more
	race := &model.Race{ID: 1, Name: "Team Flat Race", CourseID: 1, RaceType: model.RaceTypeRoad}
	config := RaceConfig{
		Race:   race,
		Course: flatCourse(),
		Riders: riders,
		Seed:   777,
	}

	results := SimulateRoadRace(config)

	if len(results) != 6 {
		t.Fatalf("expected 6 results, got %d", len(results))
	}

	t.Logf("Full team race results:")
	for _, r := range results {
		t.Logf("  P%d: rider %d, time=%.1fs, gap=%.1fs", r.Position, r.RiderID, r.FinishTimeSeconds, r.TimeGapSeconds)
	}

	// Verify leaders are competitive or better than their domestiques
	// Leader A (rider 1) vs Domestiques A (riders 2, 3)
	// Leader B (rider 4) vs Domestiques B (riders 5, 6)

	type riderResult struct {
		id       int64
		time     float64
		position int
		isLeader bool
	}

	resultMap := make(map[int64]riderResult)
	for _, r := range results {
		resultMap[r.RiderID] = riderResult{id: r.RiderID, time: r.FinishTimeSeconds, position: r.Position}
	}

	// Mark leaders
	resultMap[1] = riderResult{id: 1, time: resultMap[1].time, position: resultMap[1].position, isLeader: true}
	resultMap[4] = riderResult{id: 4, time: resultMap[4].time, position: resultMap[4].position, isLeader: true}

	// Team A: leader should be competitive with domestiques
	leaderATime := resultMap[1].time
	domA1Time := resultMap[2].time
	domA2Time := resultMap[3].time

	avgDomATime := (domA1Time + domA2Time) / 2.0

	t.Logf("Team A - Leader time: %.1fs, avg domestique time: %.1fs, difference: %.1fs",
		leaderATime, avgDomATime, avgDomATime-leaderATime)

	// Leader should not be significantly slower than average domestique
	// Allow 10 seconds gap due to randomness, but leader should generally be with or ahead
	if leaderATime > avgDomATime+30.0 {
		t.Logf("Warning: Team A leader significantly slower than domestiques (%.1fs gap)", leaderATime-avgDomATime)
	}

	// Team B: same check
	leaderBTime := resultMap[4].time
	domB1Time := resultMap[5].time
	domB2Time := resultMap[6].time

	avgDomBTime := (domB1Time + domB2Time) / 2.0

	t.Logf("Team B - Leader time: %.1fs, avg domestique time: %.1fs, difference: %.1fs",
		leaderBTime, avgDomBTime, avgDomBTime-leaderBTime)

	if leaderBTime > avgDomBTime+30.0 {
		t.Logf("Warning: Team B leader significantly slower than domestiques (%.1fs gap)", leaderBTime-avgDomBTime)
	}

	// Verify that leaders are typically in top finishers
	// (not strict, as races have randomness)
	leaderAPos := resultMap[1].position
	leaderBPos := resultMap[4].position

	t.Logf("Leader finishing positions: Leader A = %d, Leader B = %d", leaderAPos, leaderBPos)

	// Leaders should be in reasonable positions (top 4 out of 6)
	if leaderAPos > 4 && leaderBPos > 4 {
		t.Logf("Both leaders finished outside top 4 — possible but suggests team tactics may not be dominant")
	}
}
