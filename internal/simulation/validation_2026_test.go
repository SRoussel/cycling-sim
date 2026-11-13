package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

func TestNameResolver(t *testing.T) {
	riders := []model.Rider{{ID: 1, Name: "Mikkel Bjerg"}, {ID: 2, Name: "Albert Withen Philipsen"}, {ID: 3, Name: "Tadej Pogačar"}, {ID: 4, Name: "Jasper Philipsen"}}
	resolve := nameResolver(riders)
	for name, want := range map[string]int64{"Mikkel Norsgaard Bjerg": 1, "Albert Philipsen": 2, "Tadej Pogacar": 3, "jasper philipsen": 4, "Philipsen": 0} {
		r, ok := resolve(name)
		if (want == 0) == ok || ok && r.ID != want {
			t.Errorf("%s -> %d %v, want %d", name, r.ID, ok, want)
		}
	}
}
