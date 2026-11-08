package simulation

import (
	"math"
	"testing"

	"cycling_sim/internal/model"
)

func TestWithWindComponents(t *testing.T) {
	segs := []model.Segment{{HeadingDeg: 0}, {HeadingDeg: 90}, {HeadingDeg: 180}}
	out := WithWind(segs, 36, 0) // 10 m/s from the north
	if math.Abs(out[0].HeadwindMPS-10) > 1e-9 || out[0].CrosswindMPS > 1e-9 {
		t.Errorf("riding north into a north wind = headwind: %+v", out[0])
	}
	if math.Abs(out[1].CrosswindMPS-10) > 1e-9 || math.Abs(out[1].HeadwindMPS) > 1e-9 {
		t.Errorf("riding east in a north wind = crosswind: %+v", out[1])
	}
	if math.Abs(out[2].HeadwindMPS+10) > 1e-9 {
		t.Errorf("riding south = tailwind: %+v", out[2])
	}
	if segs[0].HeadwindMPS != 0 {
		t.Error("WithWind must not modify the input")
	}
}

func TestHeadwindSlowsTailwindSpeeds(t *testing.T) {
	calm := SpeedFromPower(300, 75, 0.32, 0, model.SurfaceRoad, 70)
	head := SpeedFromPowerWind(300, 75, 0.32, 0, model.SurfaceRoad, 70, 5)
	tail := SpeedFromPowerWind(300, 75, 0.32, 0, model.SurfaceRoad, 70, -5)
	if !(head < calm && calm < tail) {
		t.Errorf("want head < calm < tail, got %.2f %.2f %.2f", head, calm, tail)
	}
	if p := segmentPower(calm, 75, 0.32, &model.Segment{}, 70); math.Abs(p-300) > 0.5 {
		t.Errorf("segmentPower should invert the solver in calm air, got %.1f", p)
	}
}

func TestCrosswindRemovesGutterShelter(t *testing.T) {
	seg := &model.Segment{CrosswindMPS: 8}
	inside := crosswindDraft(0.4, 5, seg, 13)
	gutter := crosswindDraft(0.4, 30, seg, 13)
	if inside != 0.4 || gutter <= 0.4 {
		t.Errorf("echelon keeps shelter (%.2f), gutter loses it (%.2f)", inside, gutter)
	}
	if crosswindDraft(0.4, 30, &model.Segment{}, 13) != 0.4 {
		t.Error("no crosswind, no penalty")
	}
}

// A hard pace in a strong crosswind should split a flat race; in calm air the
// same race stays together.
func TestCrosswindSplitsFlatRace(t *testing.T) {
	riders := makeField()
	spreadTeams(riders, 5)
	course := flatCourse()
	for i := range course.Segments {
		course.Segments[i].HeadingDeg = 90 // all eastbound
	}
	spread := func(windKmh float64) float64 {
		total := 0.0
		for seed := int64(1); seed <= 10; seed++ {
			race := &model.Race{ID: 1, Name: "Flat", RaceType: model.RaceTypeRoad, WindSpeedKmh: windKmh, WindDirectionDeg: 0}
			res := SimulateRoadRace(RaceConfig{Race: race, Course: course, Riders: riders, Seed: seed})
			total += res[len(res)-1].TimeGapSeconds
		}
		return total / 10
	}
	calm, windy := spread(0), spread(45)
	t.Logf("avg last-rider gap: calm %.0fs, 45 km/h crosswind %.0fs", calm, windy)
	if windy <= calm {
		t.Errorf("crosswind should split the field more than calm air (%.0f vs %.0f)", windy, calm)
	}
}

func TestProtectedRidersMoveUpInCrosswind(t *testing.T) {
	build := func() []riderPower {
		var rps []riderPower
		for i := 0; i < 30; i++ {
			r := makeRider("R", 380, 70, 70, 70, 70, 70, 70, 70, 70, 70)
			r.ID = int64(i + 1)
			r.Team = []string{"A", "B", "C"}[i%3]
			rs := &RiderState{Rider: &r, TeamRole: RoleDomestique}
			if i < 3 {
				rs.TeamRole = RoleLeader // one leader per team
			}
			rps = append(rps, riderPower{state: rs})
		}
		return rps
	}
	calm := build()
	assignTeamPositions(calm, false)
	windy := build()
	assignTeamPositions(windy, true)
	if p := calm[0].state.GroupPosition; p <= echelonCapacity {
		t.Errorf("in calm air the leader should sit deep in the bunch, got position %d", p)
	}
	if p := windy[0].state.GroupPosition; p > len(windy)-3 {
		t.Errorf("in a crosswind the leader should move up, got position %d", p)
	}
}
