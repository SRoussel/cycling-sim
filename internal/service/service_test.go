package service

import (
	"reflect"
	"testing"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/seed"
)

func seededDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := seed.Seed(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func findRace(t *testing.T, db *database.DB, name string) model.Race {
	t.Helper()
	races, _ := db.ListRaces()
	for _, r := range races {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("race %q not seeded", name)
	return model.Race{}
}

func TestRunRaceDeterministicWithSeed(t *testing.T) {
	db := seededDB(t)
	race := findRace(t, db, "Milan-San Remo")
	run := func() []model.RaceResult {
		cfg, err := LoadRaceConfig(db, race.ID, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Seed = 99
		return RunRace(cfg, nil)
	}
	a, b := run(), run()
	if len(a) == 0 {
		t.Fatal("no results")
	}
	if !reflect.DeepEqual(a, b) {
		t.Error("same seed produced different results")
	}
}

func TestOneDayRacesExcludesStages(t *testing.T) {
	db := seededDB(t)
	races, err := OneDayRaces(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(races) == 0 {
		t.Fatal("no one-day races")
	}
	for _, r := range races {
		if r.RaceType == model.RaceTypeStage {
			t.Errorf("%s is a stage", r.Name)
		}
	}
}

func TestStageRaceRoundTrip(t *testing.T) {
	db := seededDB(t)
	srs, _ := db.ListStageRaces()
	// Pick the shortest stage race to keep the test fast.
	var srID int64
	min := 1 << 30
	for _, s := range srs {
		if len(s.Stages) < min {
			min, srID = len(s.Stages), s.ID
		}
	}
	plan, err := PrepareStageRace(db, srID, true)
	if err != nil {
		t.Fatal(err)
	}
	out := RunStageRace(plan, 5, nil, nil)
	if len(out.Results) != len(plan.Stages) || len(out.GC) != len(plan.Stages) {
		t.Fatalf("got %d results / %d GC for %d stages", len(out.Results), len(out.GC), len(plan.Stages))
	}
	if err := SaveStageRaceOutput(db, out); err != nil {
		t.Fatal(err)
	}
	last := len(plan.Stages)
	gc, err := db.GetGCStandings(srID, last)
	if err != nil || len(gc) == 0 {
		t.Fatalf("GC not saved: %v", err)
	}
	// Reset must wipe saved standings.
	if _, err := PrepareStageRace(db, srID, true); err != nil {
		t.Fatal(err)
	}
	gc, _ = db.GetGCStandings(srID, last)
	if len(gc) != 0 {
		t.Errorf("reset left %d GC rows", len(gc))
	}
}
