package simulation

import (
	"fmt"
	"os"
	"testing"

	"cycling_sim/internal/database"
	"cycling_sim/internal/selection"
)

func TestDiagnosticGaps(t *testing.T) {
	dbPath := os.Getenv("CYCLING_SIM_DB")
	if dbPath == "" {
		dbPath = "cycling_sim.db"
	}
	db, err := database.Open(dbPath)
	if err != nil {
		t.Skip("no DB:", err)
	}
	defer db.Close()

	races, _ := db.ListRaces()
	idMap := map[int64]string{28: "vdP", 452: "Pogačar", 429: "van Aert", 190: "Pedersen", 60: "Benoot"}

	for _, race := range races {
		if race.Name != "Paris-Roubaix" {
			continue
		}
		course, _ := db.GetCourse(race.CourseID)
		allRiders, _ := db.ListRiders()
		sel := selection.SelectStartList(allRiders, course, &race, selection.DefaultConfig(race.RaceType, false, 0))
		t.Logf("Selected %d riders for Paris-Roubaix", len(sel))

		for seed := int64(1); seed <= 5; seed++ {
			cfg := RaceConfig{Race: &race, Course: course, Riders: sel, Seed: seed}
			results := SimulateRoadRace(cfg)
			wName := fmt.Sprintf("#%d", results[0].RiderID)
			if n, ok := idMap[results[0].RiderID]; ok {
				wName = n
			}
			t.Logf("Seed %d winner=%s (gap to 2nd: %.0fs)", seed, wName, results[1].TimeGapSeconds)
			for _, r := range results {
				if name, ok := idMap[r.RiderID]; ok {
					t.Logf("  %s: pos=%d gap=%.0fs", name, r.Position, r.TimeGapSeconds)
				}
			}
		}
	}
}
