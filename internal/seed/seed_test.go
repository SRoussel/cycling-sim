package seed

import (
	"path/filepath"
	"testing"

	"cycling_sim/internal/database"
)

func TestSeedPopulatesAndIsIdempotent(t *testing.T) {
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	res, err := Seed(db)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if res.Warnings != 0 {
		t.Errorf("seed produced %d warnings", res.Warnings)
	}
	if res.Riders < 100 || res.Courses < 20 || res.Races < 50 || res.StageRaces < 3 {
		t.Errorf("unexpectedly small seed: %+v", res)
	}

	// Every stage race must reference existing races with contiguous stage numbers.
	srs, _ := db.ListStageRaces()
	for _, s := range srs {
		full, err := db.GetStageRace(s.ID)
		if err != nil {
			t.Fatalf("get stage race %d: %v", s.ID, err)
		}
		for i, st := range full.Stages {
			if st.StageNumber != i+1 {
				t.Errorf("%s: stage %d has number %d", full.Name, i+1, st.StageNumber)
			}
			if _, err := db.GetRace(st.RaceID); err != nil {
				t.Errorf("%s stage %d: missing race %d", full.Name, st.StageNumber, st.RaceID)
			}
		}
	}

	again, err := Seed(db)
	if err != nil {
		t.Fatal(err)
	}
	if again.Riders != 0 || again.Courses != 0 {
		t.Errorf("second seed should be a no-op, got %+v", again)
	}
}

func TestClearDBFileMissingIsOK(t *testing.T) {
	if err := ClearDBFile(filepath.Join(t.TempDir(), "nope.db")); err != nil {
		t.Errorf("clearing a missing file should succeed: %v", err)
	}
	if err := ClearDBFile(""); err == nil {
		t.Error("empty path should error")
	}
}
