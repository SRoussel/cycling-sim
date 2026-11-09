package seed

import (
	"os"
	"path/filepath"
	"testing"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
)

func TestLoadAndApplyOverrides(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.csv")
	os.WriteFile(p, []byte("# comment\nName,FTP_watts,Climbing,Source\n  Some  Rider ,450,,manual\n"), 0o644)
	m, err := LoadOverrides(p)
	if err != nil {
		t.Fatal(err)
	}
	o, ok := m["some rider"]
	if !ok || o.Source != "manual" {
		t.Fatalf("override not loaded: %+v", m)
	}
	r := model.Rider{FTPWatts: 380, ClimbingAbility: 70}
	if err := o.Apply(&r); err != nil {
		t.Fatal(err)
	}
	if r.FTPWatts != 450 || r.ClimbingAbility != 70 {
		t.Errorf("apply wrong: %+v", r)
	}
}

func TestLoadOverridesRejectsUnknownColumn(t *testing.T) {
	p := filepath.Join(t.TempDir(), "o.csv")
	os.WriteFile(p, []byte("Name,Bogus\nX,1\n"), 0o644)
	if _, err := LoadOverrides(p); err == nil {
		t.Error("expected error for unknown column")
	}
}

// Every override in the shipped data file must match a rider and pass validation.
func TestShippedOverridesMatchRiders(t *testing.T) {
	m, err := LoadOverrides(findDataFile("rider_overrides.csv"))
	if err != nil {
		t.Fatal(err)
	}
	db, _ := database.OpenMemory()
	defer db.Close()
	if _, err := Seed(db); err != nil {
		t.Fatal(err)
	}
	riders, _ := db.ListRiders()
	byName := map[string]model.Rider{}
	for _, r := range riders {
		byName[normalizeName(r.Name)] = r
	}
	for key, o := range m {
		r, ok := byName[key]
		if !ok {
			t.Errorf("override %q matches no rider", o.Name)
			continue
		}
		if err := r.Validate(); err != nil {
			t.Errorf("%s invalid after override: %v", o.Name, err)
		}
	}
}

// Every shipped rating layer must load, name only real riders, and leave
// valid riders; LLM-drafted rows must carry a confidence level.
func TestShippedRatingLayers(t *testing.T) {
	riders, err := BuildRiders()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, r := range riders {
		names[normalizeName(r.Name)] = true
		if err := r.Validate(); err != nil {
			t.Errorf("%s invalid after rating layers: %v", r.Name, err)
		}
	}
	for _, layer := range RatingLayers {
		m, err := LoadOverrides(findDataFile(layer))
		if err != nil {
			t.Fatalf("%s: %v", layer, err)
		}
		for key, o := range m {
			if !names[key] {
				t.Errorf("%s: %q matches no rider", layer, o.Name)
			}
			if layer == "rider_llm.csv" && o.Confidence != "high" && o.Confidence != "medium" && o.Confidence != "low" {
				t.Errorf("%s: %q has confidence %q", layer, o.Name, o.Confidence)
			}
		}
	}
}

func TestApplyRatingsKeepsResults(t *testing.T) {
	db, _ := database.OpenMemory()
	defer db.Close()
	if _, err := Seed(db); err != nil {
		t.Fatal(err)
	}
	riders, _ := db.ListRiders()
	// Simulate a stale database: one rider's ratings drift.
	stale := riders[0]
	stale.FTPWatts += 50
	if err := db.UpdateRiderRatings([]model.Rider{stale}); err != nil {
		t.Fatal(err)
	}
	n, err := ApplyRatings(db)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(riders) {
		t.Errorf("updated %d riders, want %d", n, len(riders))
	}
	got, _ := db.GetRider(stale.ID)
	if got.FTPWatts != riders[0].FTPWatts {
		t.Errorf("FTP %d after apply, want %d", got.FTPWatts, riders[0].FTPWatts)
	}
	if after, _ := db.ListRiders(); len(after) != len(riders) {
		t.Errorf("apply changed the rider count: %d → %d", len(riders), len(after))
	}
}
