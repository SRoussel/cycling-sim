package database

import (
	"cycling_sim/internal/model"
	"testing"
	"time"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenMemory()
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testRider() model.Rider {
	return model.Rider{
		Name: "Tadej Pogačar", Team: "UAE Team Emirates", Nationality: "SLO",
		WeightKg: 66, HeightCm: 176, FTPWatts: 410, MaxSprintWatts: 1400,
		VO2MaxWatts: 480, CdA: 0.32, ClimbingAbility: 95, SprintAbility: 75,
		TTAbility: 88, DescendingAbility: 85, Endurance: 90, Recovery: 88,
		Consistency: 92, TacticalAwareness: 90,
	}
}

func TestRiderCRUD(t *testing.T) {
	db := testDB(t)
	r := testRider()

	if err := db.CreateRider(&r); err != nil {
		t.Fatalf("create: %v", err)
	}
	if r.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	got, err := db.GetRider(r.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Tadej Pogačar" || got.FTPWatts != 410 {
		t.Errorf("unexpected rider: %+v", got)
	}

	riders, err := db.ListRiders()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(riders) != 1 {
		t.Errorf("expected 1 rider, got %d", len(riders))
	}

	if err := db.DeleteRider(r.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	riders, _ = db.ListRiders()
	if len(riders) != 0 {
		t.Error("expected 0 riders after delete")
	}
}

func TestCourseCRUD(t *testing.T) {
	db := testDB(t)
	c := model.Course{Name: "Alpe d'Huez", TotalDistanceKm: 13.8}
	if err := db.CreateCourse(&c); err != nil {
		t.Fatalf("create course: %v", err)
	}

	seg := model.Segment{
		CourseID: c.ID, SegmentOrder: 1, Name: "Main Climb",
		DistanceKm: 13.8, GradientPct: 8.1, SurfaceType: model.SurfaceRoad,
	}
	if err := db.CreateSegment(&seg); err != nil {
		t.Fatalf("create segment: %v", err)
	}

	got, err := db.GetCourse(c.ID)
	if err != nil {
		t.Fatalf("get course: %v", err)
	}
	if len(got.Segments) != 1 {
		t.Errorf("expected 1 segment, got %d", len(got.Segments))
	}
	if got.Segments[0].GradientPct != 8.1 {
		t.Errorf("unexpected gradient: %.1f", got.Segments[0].GradientPct)
	}
}

func TestRaceCRUD(t *testing.T) {
	db := testDB(t)
	c := model.Course{Name: "Test Course", TotalDistanceKm: 200}
	db.CreateCourse(&c)

	r := model.Race{
		Name: "Stage 1", CourseID: c.ID, RaceType: model.RaceTypeRoad,
		Date: time.Now(), Temperature: 25, WindSpeedKmh: 15, WindDirectionDeg: 180,
	}
	if err := db.CreateRace(&r); err != nil {
		t.Fatalf("create race: %v", err)
	}

	got, err := db.GetRace(r.ID)
	if err != nil {
		t.Fatalf("get race: %v", err)
	}
	if got.RaceType != model.RaceTypeRoad {
		t.Errorf("unexpected race type: %s", got.RaceType)
	}
}

func TestStageRaceCRUD(t *testing.T) {
	db := testDB(t)
	c := model.Course{Name: "Course", TotalDistanceKm: 150}
	db.CreateCourse(&c)

	race1 := model.Race{Name: "S1", CourseID: c.ID, RaceType: model.RaceTypeRoad}
	race2 := model.Race{Name: "S2", CourseID: c.ID, RaceType: model.RaceTypeRoad}
	db.CreateRace(&race1)
	db.CreateRace(&race2)

	sr := model.StageRace{Name: "Mini Tour"}
	if err := db.CreateStageRace(&sr); err != nil {
		t.Fatalf("create stage race: %v", err)
	}

	db.AddStageToRace(&model.StageRaceStage{StageRaceID: sr.ID, RaceID: race1.ID, StageNumber: 1})
	db.AddStageToRace(&model.StageRaceStage{StageRaceID: sr.ID, RaceID: race2.ID, StageNumber: 2, RestDayBefore: true})

	got, err := db.GetStageRace(sr.ID)
	if err != nil {
		t.Fatalf("get stage race: %v", err)
	}
	if km, err := db.StageRaceDistances(); err != nil || km[sr.ID] != 300 {
		t.Errorf("stage race distance %v (%v), want the stages' sum", km[sr.ID], err)
	}
	if len(got.Stages) != 2 {
		t.Errorf("expected 2 stages, got %d", len(got.Stages))
	}
	if !got.Stages[1].RestDayBefore {
		t.Error("expected rest day before stage 2")
	}
}

func TestResultsCRUD(t *testing.T) {
	db := testDB(t)
	c := model.Course{Name: "Course", TotalDistanceKm: 200}
	db.CreateCourse(&c)
	race := model.Race{Name: "Race", CourseID: c.ID, RaceType: model.RaceTypeRoad}
	db.CreateRace(&race)

	r1 := testRider()
	r2 := testRider()
	r2.Name = "Jonas Vingegaard"
	r2.Team = "Visma-Lease a Bike"
	db.CreateRider(&r1)
	db.CreateRider(&r2)

	results := []model.RaceResult{
		{RaceID: race.ID, RiderID: r1.ID, Position: 1, FinishTimeSeconds: 18000, TimeGapSeconds: 0},
		{RaceID: race.ID, RiderID: r2.ID, Position: 2, FinishTimeSeconds: 18045, TimeGapSeconds: 45},
	}
	if err := db.SaveRaceResults(results); err != nil {
		t.Fatalf("save results: %v", err)
	}

	got, err := db.GetRaceResults(race.ID)
	if err != nil {
		t.Fatalf("get results: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d", len(got))
	}
	if got[0].TimeGapSeconds != 0 || got[1].TimeGapSeconds != 45 {
		t.Errorf("unexpected time gaps: %.0f, %.0f", got[0].TimeGapSeconds, got[1].TimeGapSeconds)
	}
}

func TestGCStandings(t *testing.T) {
	db := testDB(t)
	c := model.Course{Name: "Course", TotalDistanceKm: 200}
	db.CreateCourse(&c)
	race := model.Race{Name: "Race", CourseID: c.ID, RaceType: model.RaceTypeRoad}
	db.CreateRace(&race)
	sr := model.StageRace{Name: "Tour"}
	db.CreateStageRace(&sr)

	r1 := testRider()
	r2 := testRider()
	r2.Name = "Remco Evenepoel"
	r2.Team = "Soudal-QuickStep"
	db.CreateRider(&r1)
	db.CreateRider(&r2)

	standings := []model.GCStanding{
		{StageRaceID: sr.ID, RiderID: r1.ID, AfterStage: 1, TotalTimeSeconds: 18000, GCPosition: 1},
		{StageRaceID: sr.ID, RiderID: r2.ID, AfterStage: 1, TotalTimeSeconds: 18060, GCPosition: 2},
	}
	if err := db.SaveGCStandings(standings); err != nil {
		t.Fatalf("save standings: %v", err)
	}

	got, err := db.GetGCStandings(sr.ID, 1)
	if err != nil {
		t.Fatalf("get standings: %v", err)
	}
	if len(got) != 2 || got[0].GCPosition != 1 {
		t.Errorf("unexpected standings: %+v", got)
	}
}
