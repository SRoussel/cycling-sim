package pcmdb

import (
	"os"
	"testing"
)

// The real mod is only on machines where it's installed.
const worldDB = "/mnt/c/Program Files (x86)/Steam/steamapps/workshop/content/2511310/3647555648/OfficialRelease.cdb"

func TestWorldDB(t *testing.T) {
	if _, err := os.Stat(worldDB); err != nil {
		t.Skip("WorldDB mod not installed")
	}
	db, err := Open(worldDB)
	if err != nil {
		t.Fatal(err)
	}
	c := db.Tables["DYN_cyclist"]
	if c == nil || c.Rows < 5000 {
		t.Fatalf("DYN_cyclist missing or small: %+v", c)
	}
	found := false
	for i := 0; i < c.Rows; i++ {
		if c.String("gene_sz_lastname", i) == "Vingegaard" && c.String("gene_sz_firstname", i) == "Jonas" {
			found = true
			if w, h := c.Int("gene_i_weight", i), c.Int("gene_i_size", i); w < 55 || w > 65 || h < 170 || h > 180 {
				t.Errorf("Vingegaard weight %d height %d", w, h)
			}
			if m := c.Int("charac_i_mountain", i); m < 80 {
				t.Errorf("Vingegaard mountain %d", m)
			}
		}
	}
	if !found {
		t.Error("Vingegaard not found")
	}
	if db.Tables["DYN_team"] == nil {
		t.Error("DYN_team missing")
	}
}
