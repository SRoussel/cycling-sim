package seed

import (
	"math"
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

func TestCourseSegmentsMatchStatedDistance(t *testing.T) {
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := Seed(db); err != nil {
		t.Fatal(err)
	}
	courses, _ := db.ListCourses()
	for _, c := range courses {
		full, err := db.GetCourse(c.ID)
		if err != nil {
			t.Fatal(err)
		}
		sum := 0.0
		for _, s := range full.Segments {
			sum += s.DistanceKm
		}
		if math.Abs(sum-full.TotalDistanceKm) > 0.2 {
			t.Errorf("%s: segments sum to %.1f km, stated %.1f km", full.Name, sum, full.TotalDistanceKm)
		}
	}
}

// New seed content (here: everything but the riders) is added to a database
// that already has riders, without re-creating them.
func TestSeedAddsMissingContent(t *testing.T) {
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	riders, err := BuildRiders()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateRider(&riders[0]); err != nil {
		t.Fatal(err)
	}
	res, err := Seed(db)
	if err != nil {
		t.Fatal(err)
	}
	if res.Riders != 0 || res.Courses == 0 || res.StageRaces == 0 {
		t.Errorf("want only courses/races added, got %+v", res)
	}
}

func TestTour2023Route(t *testing.T) {
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := Seed(db); err != nil {
		t.Fatal(err)
	}
	srs, _ := db.ListStageRaces()
	for _, s := range srs {
		if s.Name != Tour2023 {
			continue
		}
		sr, err := db.GetStageRace(s.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(sr.Stages) != 21 {
			t.Fatalf("want 21 stages, got %d", len(sr.Stages))
		}
		km, rests := 0.0, 0
		for _, st := range sr.Stages {
			r, _ := db.GetRace(st.RaceID)
			c, _ := db.GetCourse(r.CourseID)
			km += c.TotalDistanceKm
			if st.RestDayBefore {
				rests++
			}
		}
		if math.Abs(km-3405.6) > 0.1 || rests != 2 {
			t.Errorf("route: %.1f km, %d rest days; want 3405.6 km, 2", km, rests)
		}
		return
	}
	t.Fatalf("%s not seeded", Tour2023)
}
