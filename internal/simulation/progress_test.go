package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

// Watching a race (frames inside long segments) doesn't change it, and a
// long segment is shown every few km rather than only at its end.
func TestWatchingDoesNotChangeTheRace(t *testing.T) {
	riders := makeField()
	spreadTeams(riders, 5)
	course := flatCourse()
	race := &model.Race{ID: 1, Name: "Flat", RaceType: model.RaceTypeRoad}
	plain := SimulateRoadRace(RaceConfig{Race: race, Course: course, Riders: riders, Seed: 3})
	var views []float64
	watched := SimulateRoadRaceWithProgress(RaceConfig{Race: race, Course: course, Riders: riders, Seed: 3}, func(s ProgressSnapshot) {
		if s.Race != nil {
			views = append(views, s.Race.KmRemaining)
		}
	})
	if len(plain) != len(watched) {
		t.Fatalf("results %d vs %d", len(plain), len(watched))
	}
	for i := range plain {
		if plain[i].RiderID != watched[i].RiderID || plain[i].TimeGapSeconds != watched[i].TimeGapSeconds {
			t.Fatalf("place %d differs: %+v vs %+v", i+1, plain[i], watched[i])
		}
	}
	longest := 0.0
	for _, seg := range course.Segments {
		longest = max(longest, seg.DistanceKm)
	}
	for i := 1; i < len(views); i++ {
		if d := views[i-1] - views[i]; d > 2*frameEveryKm+1e-6 && longest > 2*frameEveryKm {
			t.Errorf("no view for %.1f km (%.1f to %.1f to go)", d, views[i-1], views[i])
		}
	}
}
