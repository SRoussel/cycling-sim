package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

func TestRouteSectorsMergeSameName(t *testing.T) {
	c := &model.Course{TotalDistanceKm: 10, Segments: []model.Segment{
		{Name: "Road", DistanceKm: 2, SurfaceType: model.SurfaceRoad},
		{Name: "Arenberg (5★)", DistanceKm: 1, SurfaceType: model.SurfaceCobbles, CobbleStars: 5},
		{Name: "Arenberg (5★)", DistanceKm: 1.1, SurfaceType: model.SurfaceCobbles, CobbleStars: 5},
		{Name: "Gruson (2★)", DistanceKm: 1, SurfaceType: model.SurfaceCobbles, CobbleStars: 2},
		{Name: "Road", DistanceKm: 2, SurfaceType: model.SurfaceRoad},
		{Name: "Gruson (2★)", DistanceKm: 2.9, SurfaceType: model.SurfaceCobbles, CobbleStars: 2},
	}}
	got := routeSectors(c)
	if len(got) != 3 {
		t.Fatalf("want Arenberg, Gruson, Gruson (a road gap splits sectors), got %+v", got)
	}
	if got[0].Name != "Arenberg" {
		t.Errorf("stars suffix should be dropped: %q", got[0].Name)
	}
	if got[0].StartKm != 2 || got[0].LengthKm < 2.09 || got[0].Stars != 5 {
		t.Errorf("Arenberg: %+v", got[0])
	}
}
