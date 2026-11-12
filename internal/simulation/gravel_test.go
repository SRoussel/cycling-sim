package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

func TestGravelDraftBetweenRoadAndCobbles(t *testing.T) {
	for _, pos := range []int{2, 5, 20} {
		road, gravel, cobbles := draftFor(pos, model.SurfaceRoad), draftFor(pos, model.SurfaceGravel), draftFor(pos, model.SurfaceCobbles)
		if !(road < gravel && gravel < cobbles) {
			t.Errorf("position %d: CdA multipliers road %.2f < gravel %.2f < cobbles %.2f expected", pos, road, gravel, cobbles)
		}
	}
	if draftFor(30, model.SurfaceGravel) != draftFor(lineShelterPosition, model.SurfaceGravel) {
		t.Errorf("gravel is single file: no deeper shelter than the %dth wheel", lineShelterPosition)
	}
}

func TestGravelRollsALittleBetterThanPave(t *testing.T) {
	for stars := 1; stars <= 3; stars++ {
		g := forceRolling(75, 0, model.SurfaceGravel, 70, gravelRoughness(stars))
		c := forceRolling(75, 0, model.SurfaceCobbles, 70, cobbleRoughness(stars))
		r := forceRolling(75, 0, model.SurfaceRoad, 70, 1)
		if !(r < g && g < c) {
			t.Errorf("%d stars: road %.1f N < gravel %.1f N < cobbles %.1f N expected", stars, r, g, c)
		}
	}
}
