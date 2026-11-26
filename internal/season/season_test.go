package season

import (
	"testing"
	"time"

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

func TestYearEndAndStart(t *testing.T) {
	db := seededDB(t)
	team := someTeam(t, db)
	s, _ := New(db, team, "")
	events, _ := db.ListSeasonEvents(s.ID, s.Year)
	if _, err := EndYear(db, s); err == nil {
		t.Fatal("ended a year with races left")
	}
	for i := range events {
		events[i].Done = true
	}
	if err := db.SaveSeasonEvents(events); err != nil {
		t.Fatal(err)
	}
	before, _ := db.ListSeasonRiders(s.ID)
	if _, err := AutoRenew(db, s, 26); err != nil {
		t.Fatal(err)
	}
	if _, err := EndYear(db, s); err != nil {
		t.Fatal(err)
	}
	if s.Phase != model.PhaseOffseason || s.Year != FirstYear+1 {
		t.Fatalf("after the year's end: %s %d", s.Phase, s.Year)
	}
	after, _ := db.ListSeasonRiders(s.ID)
	age := map[int64]int{}
	for _, sr := range before {
		age[sr.ID] = sr.Age
	}
	retired, free, neo := 0, 0, 0
	for _, sr := range after {
		switch {
		case sr.Retired:
			retired++
		case age[sr.ID] == 0:
			neo++
			if sr.Age < 19 || sr.Age > 21 || sr.Team != "" {
				t.Errorf("new pro %s: %d, team %q", sr.Name, sr.Age, sr.Team)
			}
		default:
			if sr.Age != age[sr.ID]+1 {
				t.Errorf("%s aged %d -> %d", sr.Name, age[sr.ID], sr.Age)
			}
		}
		if sr.FreeAgent() {
			free++
		}
		if !sr.Retired && sr.Team != "" && sr.ContractEnd < s.Year {
			t.Errorf("%s on %s with a contract to %d", sr.Name, sr.Team, sr.ContractEnd)
		}
	}
	if retired == 0 || neo != retired || free == 0 {
		t.Fatalf("retired %d, new pros %d, free agents %d", retired, neo, free)
	}
	if _, err := StartYear(db, s); err != nil {
		t.Fatal(err)
	}
	final, _ := db.ListSeasonRiders(s.ID)
	roster := map[string]int{}
	for _, sr := range final {
		if !sr.Retired && sr.Team != "" {
			roster[sr.Team]++
		}
	}
	if roster[team] < 22 {
		t.Errorf("our roster: %d riders", roster[team])
	}
	next, _ := db.ListSeasonEvents(s.ID, s.Year)
	if len(next) != len(events) || next[0].Start.Year() != s.Year {
		t.Fatalf("next year's calendar: %d events", len(next))
	}
	objs, _ := db.ListObjectives(s.ID, s.Year)
	if len(objs) < 2 {
		t.Errorf("%d objectives", len(objs))
	}
}

func TestRaceNews(t *testing.T) {
	p := &Prepared{Season: &model.Season{ID: 1, Year: 2026, Team: "EF"}, Event: &model.SeasonEvent{Name: "Strade Bianche"}}
	names := map[int64]string{1: "Tadej Pogačar", 2: "Madis Mihkels"}
	res := []model.SeasonResult{
		{RiderID: 1, Team: "UAE", Kind: model.ResultRace, Position: 1},
		{RiderID: 2, Team: "EF", Kind: model.ResultRace, Position: 66},
	}
	news := raceNews(p, res, names, time.Time{})
	if len(news) != 2 || news[0].Text != "Tadej Pogačar (UAE) wins Strade Bianche." || news[0].Mine ||
		news[1].Text != "Our best at Strade Bianche: Madis Mihkels, 66th." || !news[1].Mine {
		t.Errorf("news %+v", news)
	}
	for n, want := range map[int]string{1: "1st", 2: "2nd", 11: "11th", 12: "12th", 21: "21st", 103: "103rd"} {
		if got := ordinal(n); got != want {
			t.Errorf("ordinal(%d) = %s, want %s", n, got, want)
		}
	}
}

// Transfers: AI teams renew their best riders at the start of the year; the
// riders left can only agree to join from 1 August, and ask more to drop to
// a weaker team.
func TestTransfers(t *testing.T) {
	db := seededDB(t)
	team := someTeam(t, db)
	s, _ := New(db, team, "")
	if s.RenewedYear != s.Year {
		t.Fatalf("a new season's AI renewals: year %d, want %d", s.RenewedYear, s.Year)
	}
	srs, _ := db.ListSeasonRiders(s.ID)
	values := Values(db, activeRiders(srs))
	ranks := TeamRanks(activeRiders(srs), values)
	// The best AI rider whose contract ends this year and wasn't renewed.
	var target *model.SeasonRider
	renewed, expiring := 0, 0
	for i := range srs {
		sr := &srs[i]
		if sr.Retired || sr.Team == "" || sr.Team == team || sr.ContractEnd > s.Year {
			continue
		}
		expiring++
		if sr.NextTeam == sr.Team {
			renewed++
			continue
		}
		if target == nil || values[sr.ID] > values[target.ID] {
			target = sr
		}
	}
	if expiring == 0 || renewed*2 < expiring {
		t.Fatalf("AI teams renewed %d of %d expiring contracts", renewed, expiring)
	}
	if target == nil {
		t.Skip("every expiring AI rider was renewed")
	}
	ok, msg, err := Offer(db, s, target.ID, 99999, 2)
	if err != nil || ok {
		t.Fatalf("signed before the transfer period: %v %q", err, msg)
	}
	s.Date = TransferOpens(s.Year)
	steps := ranks[team] - ranks[target.Team]
	ask := AskingSalaryTo(*target, values[target.ID], true, steps)
	if steps > 0 && ask <= AskingSalary(*target, values[target.ID], true) {
		t.Errorf("no premium for a step down of %d places", steps)
	}
	if ok, msg, _ := Offer(db, s, target.ID, ask-10, 2); ok {
		t.Errorf("signed below his asking salary: %q", msg)
	}
}

// The player plans a rider's races and form targets.
func TestRiderPlan(t *testing.T) {
	db := seededDB(t)
	team := someTeam(t, db)
	s, _ := New(db, team, "")
	srs, _ := db.ListSeasonRiders(s.ID)
	var rid int64
	for _, sr := range srs {
		if sr.Team == team {
			rid = sr.ID
			break
		}
	}
	_, plan, err := RiderPlan(db, s, rid)
	if err != nil || len(plan) == 0 {
		t.Fatalf("plan: %v (%d events)", err, len(plan))
	}
	// Ride and peak for one race only: the last with room for him.
	var last model.SeasonEvent
	for _, p := range plan {
		if p.Planned || p.Squad < p.Size {
			last = p.Event
		}
	}
	if last.ID == 0 {
		t.Skip("every squad is full")
	}
	if err := SetRiderPlan(db, s, rid, []int64{last.ID}, []int64{last.ID}); err != nil {
		t.Fatal(err)
	}
	_, plan, _ = RiderPlan(db, s, rid)
	for _, p := range plan {
		if p.Planned != (p.Event.ID == last.ID) || p.Peak != (p.Event.ID == last.ID) {
			t.Errorf("%s: planned %v peak %v", p.Event.Name, p.Planned, p.Peak)
		}
	}
	for _, p := range plan {
		if p.Event.ID == last.ID && p.FormPct < 99.9 {
			t.Errorf("form at his target %.1f%%, want 100%%", p.FormPct)
		}
	}
	if err := SetRiderPlan(db, s, rid, nil, []int64{1, 2, 3, 4, 5}); err == nil {
		t.Error("five peaks accepted")
	}
}
