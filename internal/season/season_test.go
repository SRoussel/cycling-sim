package season

import (
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

func someTeam(t *testing.T, db *database.DB) string {
	riders, _ := db.ListRiders()
	count := map[string]int{}
	for _, r := range riders {
		count[r.Team]++
	}
	best := ""
	for team, n := range count {
		if n > count[best] || n == count[best] && team < best {
			best = team
		}
	}
	return best
}

func TestNewSeasonCalendar(t *testing.T) {
	db := seededDB(t)
	s, err := New(db, someTeam(t, db), "")
	if err != nil {
		t.Fatal(err)
	}
	events, _ := db.ListSeasonEvents(s.ID, s.Year)
	if len(events) < 10 {
		t.Fatalf("calendar has %d events", len(events))
	}
	gts := 0
	for i, e := range events {
		if referenceEdition(e.Name) {
			t.Errorf("reference edition %s on the calendar", e.Name)
		}
		if e.Start.Year() != s.Year || e.End.Before(e.Start) {
			t.Errorf("%s dated %v-%v", e.Name, e.Start, e.End)
		}
		if i > 0 && e.Start.Before(events[i-1].Start) {
			t.Errorf("calendar out of order at %s", e.Name)
		}
		if e.Category == model.CategoryGrandTour {
			gts++
			if e.Days() < 21 {
				t.Errorf("%s lasts %d days", e.Name, e.Days())
			}
		}
	}
	if gts != 3 {
		t.Errorf("%d Grand Tours", gts)
	}
	srs, _ := db.ListSeasonRiders(s.ID)
	riders, _ := db.ListRiders()
	if len(srs) != len(riders) {
		t.Fatalf("%d season riders for %d riders", len(srs), len(riders))
	}
	for _, sr := range srs {
		if sr.ContractEnd < s.Year || sr.Salary <= 0 {
			t.Fatalf("%s: contract to %d at %dk", sr.Name, sr.ContractEnd, sr.Salary)
		}
	}
}

func TestRaceAndRecordOneDay(t *testing.T) {
	db := seededDB(t)
	team := someTeam(t, db)
	s, _ := New(db, team, "")
	next, _ := NextEvent(db, s)
	if next == nil || next.Kind != model.EventOneDay {
		t.Skipf("first event %v is not a one-day race", next)
	}
	auto, err := AutoPicks(db, s, next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(auto) != SquadSize(next) {
		t.Fatalf("auto squad %d riders", len(auto))
	}
	picks := auto[:3]
	p, err := Prepare(db, s, next.ID, picks)
	if err != nil {
		t.Fatal(err)
	}
	mine := 0
	in := map[int64]bool{}
	for _, r := range p.Riders {
		if in[r.ID] {
			t.Fatalf("rider %d twice in the field", r.ID)
		}
		in[r.ID] = true
		if r.Team == team {
			mine++
		}
	}
	for _, id := range picks {
		if !in[id] {
			t.Errorf("picked rider %d not in the field", id)
		}
	}
	if mine != SquadSize(next) {
		t.Errorf("our squad has %d riders", mine)
	}
	if err := Simulate(db, p, 7); err != nil {
		t.Fatal(err)
	}
	after, _ := NextEvent(db, s)
	if after != nil && after.ID == next.ID {
		t.Fatal("event not marked raced")
	}
	riders, teams, _ := Standings(db, s, s.Year)
	if len(riders) == 0 || len(teams) == 0 || riders[0].Points != Points(next.Category, model.ResultRace, 1) {
		t.Fatalf("standings after one race: %+v", riders[:min(3, len(riders))])
	}
	srs, _ := db.ListSeasonRiders(s.ID)
	for _, sr := range srs {
		if in[sr.ID] && (sr.Load != 1 || sr.RaceDays != 1) {
			t.Fatalf("%s rode: load %.2f, %d days", sr.Name, sr.Load, sr.RaceDays)
		}
		if !in[sr.ID] && sr.Load != 0 {
			t.Fatalf("%s didn't ride but carries %.2f", sr.Name, sr.Load)
		}
	}
	if !s.Date.Equal(next.End) {
		t.Errorf("season date %v, race %v", s.Date, next.End)
	}
}

func TestLoadFactor(t *testing.T) {
	if LoadFactor(0) != 1 || LoadFactor(loadFree) != 1 {
		t.Error("a fresh rider loses nothing")
	}
	if f := LoadFactor(21); f >= 0.95 || f < loadFloor {
		t.Errorf("straight after a Grand Tour: %.3f", f)
	}
}
