package importer

import (
	"os"
	"path/filepath"
	"testing"

	"cycling_sim/internal/model"
)

func TestNameKeyMatchesAcrossFormats(t *testing.T) {
	want := NameKey("Tadej Pogačar")
	for _, n := range []string{"POGACAR Tadej", "Pogačar Tadej", "tadej pogacar", "  Tadej   POGAČAR "} {
		if got := NameKey(n); got != want {
			t.Errorf("NameKey(%q) = %q, want %q", n, got, want)
		}
	}
	if NameKey("Søren Wærenskjold") != NameKey("WAERENSKJOLD Soren") {
		t.Error("ø/æ should fold to o/ae")
	}
}

func TestParseFirstCyclingPage(t *testing.T) {
	res, err := ParseFile("testdata/fc_sample.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("want 3 finishers (DNF skipped), got %d: %+v", len(res), res)
	}
	if res[0].Name != "POGACAR Tadej" || res[1].GapSecs != 99 || res[2].Name != "VAN DER POEL Mathieu" {
		t.Errorf("unexpected rows: %+v", res)
	}
}

func TestParsePCSPage(t *testing.T) {
	res, err := ParseFile("testdata/pcs_sample.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("want 3 rows, got %d", len(res))
	}
	if NameKey(res[0].Name) != NameKey("Tadej Pogačar") || res[1].GapSecs != 196 {
		t.Errorf("unexpected rows: %+v", res)
	}
	if NameKey(res[2].Name) != NameKey("Giulio Ciccone") {
		t.Errorf("empty link text should fall back to the URL slug, got %q", res[2].Name)
	}
}

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]int{"+ 1:23": 83, "6:13:48": 22428, "s.t.": 0, "12": 12, "UAE": -1, "": -1} {
		if got := parseDuration(in); got != want {
			t.Errorf("parseDuration(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestDeriveRanksConsistentRidersHigher(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "mountain"), 0o755)
	// A wins twice; B is 2nd twice; C only once (below minResults).
	for i, body := range []string{"1,Rider A\n2,Rider B\n3,Rider C\n", "1,Rider A\n2,Rider B\n"} {
		os.WriteFile(filepath.Join(dir, "mountain", []string{"r1.csv", "r2.csv"}[i]), []byte(body), 0o644)
	}
	lib, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	riders := []model.Rider{{Name: "Rider A", WeightKg: 60}, {Name: "Rider B", WeightKg: 70}, {Name: "Rider C", WeightKg: 65}}
	ratings := Derive(lib, riders)
	if len(ratings) != 2 {
		t.Fatalf("want ratings for A and B only, got %+v", ratings)
	}
	a, b := ratings[0], ratings[1]
	if a.Fields["Climbing"] <= b.Fields["Climbing"] {
		t.Errorf("A should out-rate B: %v vs %v", a.Fields, b.Fields)
	}
	if a.Fields["FTP_watts"] != 414 {
		t.Errorf("top climber should get 6.9 W/kg × 60 kg, got %d", a.Fields["FTP_watts"])
	}
	out := filepath.Join(dir, "derived.csv")
	if err := WriteDerived(out, ratings, "test"); err != nil {
		t.Fatal(err)
	}
}
