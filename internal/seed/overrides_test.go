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
