package simulation

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"testing"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/selection"
)

func TestMSRSprintAttribs(t *testing.T) {
	dbPath := os.Getenv("CYCLING_SIM_DB")
	if dbPath == "" {
		dbPath = "cycling_sim.db"
	}
	db, err := database.Open(dbPath)
	if err != nil {
		t.Skip("no DB:", err)
	}
	defer db.Close()
	riders, _ := db.ListRiders()
	ids := map[int64]bool{462: true, 485: true, 481: true, 466: true, 17: true, 14: true, 269: true, 489: true, 452: true, 429: true, 184: true}
	for _, r := range riders {
		if ids[r.ID] {
			sp := SprintPower(&r)
			t.Logf("id=%d %-30s sprint=%d maxSprint=%d sprintPower=%.0f ftp=%.0f weight=%.1fkg", r.ID, r.Name, r.SprintAbility, r.MaxSprintWatts, sp, float64(r.FTPWatts), r.WeightKg)
		}
	}
}

func TestSprintDebug(t *testing.T) {
	philipsen := &model.Rider{
		ID: 17, Name: "Philipsen",
		WeightKg: 76, HeightCm: 181, FTPWatts: 385, MaxSprintWatts: 1850,
		SprintAbility: 97, ClimbingAbility: 45, TTAbility: 60,
		DescendingAbility: 70, Endurance: 72, Handling: 70, CdA: 0.34,
	}
	martinez := &model.Rider{
		ID: 44, Name: "Martinez",
		WeightKg: 69, HeightCm: 178, FTPWatts: 385, MaxSprintWatts: 1700,
		SprintAbility: 84, ClimbingAbility: 82, TTAbility: 75,
		DescendingAbility: 70, Endurance: 85, Handling: 78, CdA: 0.31,
	}
	fmt.Printf("Philipsen sprint power: %.0fW\n", SprintPower(philipsen))
	fmt.Printf("Martinez sprint power: %.0fW\n", SprintPower(martinez))
	pState := &RiderState{Rider: philipsen, EffectiveFTP: 385, FormFactor: 1.0}
	mState := &RiderState{Rider: martinez, EffectiveFTP: 385, FormFactor: 1.0}
	pState.WPrime = NewWPrimeState(philipsen)
	mState.WPrime = NewWPrimeState(martinez)
	pWins, mWins := 0, 0
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 100; i++ {
		pS := *pState
		mS := *mState
		pS.TotalTime = 0
		mS.TotalTime = 0
		SprintFinish([]*RiderState{&pS, &mS}, rng)
		if pS.TotalTime < mS.TotalTime {
			pWins++
		} else {
			mWins++
		}
	}
	fmt.Printf("Philipsen 1v1: %d/100 wins\n", pWins)
}

func TestSprintGroupDebug(t *testing.T) {
	dbPath := os.Getenv("CYCLING_SIM_DB")
	if dbPath == "" {
		dbPath = "cycling_sim.db"
	}
	db, err := database.Open(dbPath)
	if err != nil {
		t.Skip("no DB:", err)
	}
	defer db.Close()

	allRiders, _ := db.ListRiders()
	races, _ := db.ListRaces()
	var msrRace *model.Race
	for i, r := range races {
		if r.Name == "Milan-San Remo" {
			msrRace = &races[i]
			break
		}
	}
	if msrRace == nil {
		t.Skip("Milan-San Remo not found")
	}
	course, _ := db.GetCourse(msrRace.CourseID)
	riders := selection.SelectStartList(allRiders, course, msrRace, selection.DefaultConfig(msrRace.RaceType, false, 0))

	rng := rngFromSeed(1)
	states := initStates(riders, rng)
	totalDist := 0.0
	for _, seg := range course.Segments {
		totalDist += seg.DistanceKm
	}
	tacticsParams := DefaultTacticsParams()
	raceState := NewRaceState(states, rng, false, false)
	coveredDist := 0.0
	for i := range course.Segments {
		seg := &course.Segments[i]
		SimulateSegment(states, seg, false, totalDist-coveredDist, rng, raceState, &tacticsParams)
		coveredDist += seg.DistanceKm
		sort.Slice(states, func(i, j int) bool { return states[i].TotalTime < states[j].TotalTime })
		lead := states[0].TotalTime
		// Count distinct groups (riders within 10s of each other)
		groups := 0
		if len(states) > 0 {
			groups = 1
		}
		for j := 1; j < len(states); j++ {
			if states[j].TotalTime-states[j-1].TotalTime > 10.0 {
				groups++
			}
		}
		for _, rs := range states {
			if rs.Rider.ID == 17 {
				t.Logf("After %-30s Philipsen TotalTime=%.1fs gap=%.1fs groups=%d", seg.Name, rs.TotalTime, rs.TotalTime-lead, groups)
			}
		}
	}
	sort.Slice(states, func(i, j int) bool { return states[i].TotalTime < states[j].TotalTime })
	leaderTime := states[0].TotalTime
	sprintSize := 0
	for _, rs := range states {
		if rs.TotalTime-leaderTime < 3.0 {
			sprintSize++
		}
	}
	t.Logf("Sprint group size: %d, leader time: %.1fs", sprintSize, leaderTime)
	for i := 0; i < 10 && i < len(states); i++ {
		rs := states[i]
		t.Logf("  p%d id=%d %-25s gap=%.2fs sprint=%d maxSprint=%d",
			i+1, rs.Rider.ID, rs.Rider.Name, rs.TotalTime-leaderTime,
			rs.Rider.SprintAbility, rs.Rider.MaxSprintWatts)
	}
	// Find Philipsen specifically
	for i, rs := range states {
		if rs.Rider.ID == 17 {
			t.Logf("Philipsen (id=17): pos=%d gap=%.2fs sprint=%d maxSprint=%d",
				i+1, rs.TotalTime-leaderTime, rs.Rider.SprintAbility, rs.Rider.MaxSprintWatts)
		}
	}
}
