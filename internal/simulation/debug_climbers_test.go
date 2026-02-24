package simulation

import (
	"fmt"
	"testing"

	"cycling_sim/internal/model"
)

func TestDebugClimberMountain(t *testing.T) {
	riders := makeField() // 5 sprinters(1-5), 5 climbers(6-10), 5 TT(11-15), 5 allrounders(16-20)
	course := mountainCourse()

	for seed := int64(1); seed <= 10; seed++ {
		race := &model.Race{ID: 1, Name: "Mountain", CourseID: 2, RaceType: model.RaceTypeRoad}
		results := SimulateRoadRace(RaceConfig{Race: race, Course: course, Riders: riders, Seed: seed})
		winner := results[0]
		rtype := "?"
		switch {
		case winner.RiderID >= 1 && winner.RiderID <= 5:
			rtype = "SPRINTER"
		case winner.RiderID >= 6 && winner.RiderID <= 10:
			rtype = "CLIMBER"
		case winner.RiderID >= 11 && winner.RiderID <= 15:
			rtype = "TT"
		case winner.RiderID >= 16 && winner.RiderID <= 20:
			rtype = "ALLROUNDER"
		}
		line := fmt.Sprintf("Seed=%d winner=rid%d (%s) gap=%.1fs | ", seed, winner.RiderID, rtype, results[len(results)-1].TimeGapSeconds)
		for i, r := range results[:5] {
			if r.RiderID >= 1 && r.RiderID <= 5 {
				line += fmt.Sprintf("P%d=SPR%d(%.1f) ", i+1, r.RiderID, r.TimeGapSeconds)
			} else if r.RiderID >= 6 && r.RiderID <= 10 {
				line += fmt.Sprintf("P%d=CLM%d(%.1f) ", i+1, r.RiderID-5, r.TimeGapSeconds)
			} else if r.RiderID >= 11 && r.RiderID <= 15 {
				line += fmt.Sprintf("P%d=TT%d(%.1f) ", i+1, r.RiderID-10, r.TimeGapSeconds)
			} else {
				line += fmt.Sprintf("P%d=AR%d(%.1f) ", i+1, r.RiderID-15, r.TimeGapSeconds)
			}
		}
		// find positions of climbers and allrounders
		clmPositions := ""
		arPositions := ""
		for _, r := range results {
			if r.RiderID >= 6 && r.RiderID <= 10 {
				clmPositions += fmt.Sprintf("C%d:P%d(+%.1f) ", r.RiderID-5, r.Position, r.TimeGapSeconds)
			}
			if r.RiderID >= 16 && r.RiderID <= 20 {
				arPositions += fmt.Sprintf("A%d:P%d(+%.1f) ", r.RiderID-15, r.Position, r.TimeGapSeconds)
			}
		}
		t.Log(line)
		t.Logf("  CLM: %s", clmPositions)
		t.Logf("  AR:  %s", arPositions)
	}
}
