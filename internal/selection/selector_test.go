package selection

import (
	"testing"

	"cycling_sim/internal/model"
)

func makeRider(id int64, name, team string, climbing, sprint, tt, endurance int) model.Rider {
	return model.Rider{
		ID:                id,
		Name:              name,
		Team:              team,
		Nationality:       "TST",
		WeightKg:          70,
		HeightCm:          180,
		FTPWatts:          350,
		MaxSprintWatts:    1400,
		VO2MaxWatts:       450,
		CdA:               0.32,
		ClimbingAbility:   climbing,
		SprintAbility:     sprint,
		TTAbility:         tt,
		DescendingAbility: 70,
		Endurance:         endurance,
		Recovery:          70,
		Consistency:       70,
		TacticalAwareness: 70,
	}
}

func TestMountainProfileScoresClimbersHigher(t *testing.T) {
	profile := CourseProfile{ClimbingDemand: 0.9, SprintDemand: 0.0, TTDemand: 0.1}
	climber := makeRider(1, "Climber", "A", 95, 40, 60, 70)
	sprinter := makeRider(2, "Sprinter", "A", 40, 95, 60, 70)

	climberScore := RiderScore(&climber, profile)
	sprinterScore := RiderScore(&sprinter, profile)

	if climberScore <= sprinterScore {
		t.Errorf("mountain profile: climber score %.1f should be > sprinter score %.1f", climberScore, sprinterScore)
	}
}

func TestFlatProfileScoresSprintersHigher(t *testing.T) {
	profile := CourseProfile{ClimbingDemand: 0.0, SprintDemand: 0.8, TTDemand: 0.1, EnduranceDemand: 0.1}
	climber := makeRider(1, "Climber", "A", 95, 40, 60, 70)
	sprinter := makeRider(2, "Sprinter", "A", 40, 95, 60, 70)

	climberScore := RiderScore(&climber, profile)
	sprinterScore := RiderScore(&sprinter, profile)

	if sprinterScore <= climberScore {
		t.Errorf("flat profile: sprinter score %.1f should be > climber score %.1f", sprinterScore, climberScore)
	}
}

func TestAnalyzeCourseTT(t *testing.T) {
	course := &model.Course{Name: "TT", TotalDistanceKm: 40}
	profile := AnalyzeCourse(course, model.RaceTypeTT)

	if profile.TTDemand < 0.5 {
		t.Errorf("TT course should have high TTDemand, got %.2f", profile.TTDemand)
	}
	if profile.SprintDemand > 0.1 {
		t.Errorf("TT course should have low SprintDemand, got %.2f", profile.SprintDemand)
	}
}

func TestAnalyzeCourseMountain(t *testing.T) {
	course := &model.Course{
		Name:            "Mountain",
		TotalDistanceKm: 165,
		Segments: []model.Segment{
			{Name: "Flat", DistanceKm: 80, SurfaceType: model.SurfaceRoad},
			{Name: "Col", DistanceKm: 18, GradientPct: 6.9, SurfaceType: model.SurfaceRoad},
			{Name: "Descent", DistanceKm: 20, GradientPct: -6.0, SurfaceType: model.SurfaceRoad},
			{Name: "Valley", DistanceKm: 20, GradientPct: 0.5, SurfaceType: model.SurfaceRoad},
			{Name: "Alpe", DistanceKm: 13.8, GradientPct: 8.1, SurfaceType: model.SurfaceRoad},
		},
	}
	profile := AnalyzeCourse(course, model.RaceTypeRoad)

	if profile.ClimbingDemand < 0.3 {
		t.Errorf("mountain course should have climbing demand > 0.3, got %.2f", profile.ClimbingDemand)
	}
	if profile.SprintDemand > 0.1 {
		t.Errorf("mountain summit finish should have low sprint demand, got %.2f", profile.SprintDemand)
	}
}

func TestAnalyzeCourseFlat(t *testing.T) {
	course := &model.Course{
		Name:            "Sprint",
		TotalDistanceKm: 200,
		Segments: []model.Segment{
			{Name: "Flat", DistanceKm: 160, SurfaceType: model.SurfaceRoad},
			{Name: "Rolling", DistanceKm: 30, GradientPct: 0.5, SurfaceType: model.SurfaceRoad},
			{Name: "Finish", DistanceKm: 10, SurfaceType: model.SurfaceRoad},
		},
	}
	profile := AnalyzeCourse(course, model.RaceTypeRoad)

	if profile.SprintDemand < 0.5 {
		t.Errorf("flat course with flat finish should have sprint demand > 0.5, got %.2f", profile.SprintDemand)
	}
}

func TestSelectStartListCounts(t *testing.T) {
	// 3 teams of 10 riders each
	var riders []model.Rider
	id := int64(1)
	for _, team := range []string{"Team A", "Team B", "Team C"} {
		for i := 0; i < 10; i++ {
			riders = append(riders, makeRider(id, "Rider", team, 70, 70, 70, 70))
			id++
		}
	}

	course := &model.Course{
		Name:            "Flat",
		TotalDistanceKm: 200,
		Segments: []model.Segment{
			{Name: "Flat", DistanceKm: 200, SurfaceType: model.SurfaceRoad},
		},
	}
	race := &model.Race{RaceType: model.RaceTypeRoad}
	config := SelectionConfig{NumTeams: 2, RidersPerTeam: 7}

	selected := SelectStartList(riders, course, race, config)

	if len(selected) != 14 {
		t.Errorf("expected 14 riders (2 teams × 7), got %d", len(selected))
	}

	// Verify exactly 2 teams
	teamSet := make(map[string]int)
	for _, r := range selected {
		teamSet[r.Team]++
	}
	if len(teamSet) != 2 {
		t.Errorf("expected 2 teams, got %d", len(teamSet))
	}
	for team, count := range teamSet {
		if count != 7 {
			t.Errorf("team %s should have 7 riders, got %d", team, count)
		}
	}
}

func TestSmallTeamsExcluded(t *testing.T) {
	var riders []model.Rider
	// 1 large team, 1 small team (2 riders)
	for i := 0; i < 8; i++ {
		riders = append(riders, makeRider(int64(i+1), "Big", "Big Team", 70, 70, 70, 70))
	}
	riders = append(riders, makeRider(100, "Small1", "Small Team", 90, 90, 90, 90))
	riders = append(riders, makeRider(101, "Small2", "Small Team", 90, 90, 90, 90))

	course := &model.Course{
		Name:            "Flat",
		TotalDistanceKm: 200,
		Segments:        []model.Segment{{Name: "Flat", DistanceKm: 200, SurfaceType: model.SurfaceRoad}},
	}
	race := &model.Race{RaceType: model.RaceTypeRoad}
	config := SelectionConfig{NumTeams: 22, RidersPerTeam: 7}

	selected := SelectStartList(riders, course, race, config)

	for _, r := range selected {
		if r.Team == "Small Team" {
			t.Error("small team (< 3 riders) should be excluded")
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	tt := DefaultConfig(model.RaceTypeTT, false, 0)
	if tt.NumTeams != 20 || tt.RidersPerTeam != 2 {
		t.Errorf("TT config: expected 20×2, got %d×%d", tt.NumTeams, tt.RidersPerTeam)
	}

	gt := DefaultConfig(model.RaceTypeRoad, true, 21)
	if gt.NumTeams != 22 || gt.RidersPerTeam != 8 {
		t.Errorf("grand tour config: expected 22×8, got %d×%d", gt.NumTeams, gt.RidersPerTeam)
	}

	oneDay := DefaultConfig(model.RaceTypeRoad, false, 0)
	if oneDay.NumTeams != 22 || oneDay.RidersPerTeam != 7 {
		t.Errorf("one-day config: expected 22×7, got %d×%d", oneDay.NumTeams, oneDay.RidersPerTeam)
	}
}

func TestStageRaceIncludesDiverseRoles(t *testing.T) {
	// Build a team with clear role specialists
	var riders []model.Rider
	id := int64(1)
	for _, team := range []string{"Team A", "Team B", "Team C"} {
		// 1 sprinter
		riders = append(riders, makeRider(id, "Sprinter-"+team, team, 40, 95, 50, 65))
		id++
		// 1 TT specialist
		riders = append(riders, makeRider(id, "TTer-"+team, team, 50, 40, 95, 65))
		id++
		// 1 endurance domestique
		riders = append(riders, makeRider(id, "Domestique-"+team, team, 60, 50, 50, 95))
		id++
		// 5 climbers/GC
		for j := 0; j < 5; j++ {
			riders = append(riders, makeRider(id, "Climber-"+team, team, 85, 45, 60, 70))
			id++
		}
	}

	stages := []StageInfo{
		{
			Course: &model.Course{
				Name: "Flat", TotalDistanceKm: 200,
				Segments: []model.Segment{
					{Name: "Flat", DistanceKm: 190, SurfaceType: model.SurfaceRoad},
					{Name: "Finish", DistanceKm: 10, SurfaceType: model.SurfaceRoad},
				},
			},
			RaceType: model.RaceTypeRoad,
		},
		{
			Course: &model.Course{
				Name: "Mountain", TotalDistanceKm: 165,
				Segments: []model.Segment{
					{Name: "Valley", DistanceKm: 80, SurfaceType: model.SurfaceRoad},
					{Name: "Col", DistanceKm: 18, GradientPct: 6.9, SurfaceType: model.SurfaceRoad},
					{Name: "Descent", DistanceKm: 20, GradientPct: -6.0, SurfaceType: model.SurfaceRoad},
					{Name: "Alpe", DistanceKm: 13.8, GradientPct: 8.1, SurfaceType: model.SurfaceRoad},
				},
			},
			RaceType: model.RaceTypeRoad,
		},
		{
			Course:   &model.Course{Name: "TT", TotalDistanceKm: 40},
			RaceType: model.RaceTypeTT,
		},
	}

	config := SelectionConfig{NumTeams: 3, RidersPerTeam: 7}
	selected := SelectStageRaceStartList(riders, stages, config)

	if len(selected) != 21 {
		t.Errorf("expected 21 riders (3×7), got %d", len(selected))
	}

	// Check that each team has at least one sprinter and one TT specialist
	teamSprinters := make(map[string]int)
	teamTTers := make(map[string]int)
	for _, r := range selected {
		if r.SprintAbility >= 90 {
			teamSprinters[r.Team]++
		}
		if r.TTAbility >= 90 {
			teamTTers[r.Team]++
		}
	}

	for _, team := range []string{"Team A", "Team B", "Team C"} {
		if teamSprinters[team] < 1 {
			t.Errorf("team %s should have at least 1 sprinter in stage race", team)
		}
		if teamTTers[team] < 1 {
			t.Errorf("team %s should have at least 1 TT specialist in stage race", team)
		}
	}
}

// TestSprintGuaranteeIncludesBestSprinter verifies that in flatfinish sprint races
// the top sprinter per team is always selected even if their composite score ranks outside top N.
func TestSprintGuaranteeIncludesBestSprinter(t *testing.T) {
	// Build a team where the pure sprinter has low endurance/climbing (low composite score)
	var riders []model.Rider
	// 8 all-rounders with solid endurance (score will be high composite)
	for i := 0; i < 8; i++ {
		riders = append(riders, makeRider(int64(i+1), "AllRounder", "Team A", 70, 78, 70, 85))
	}
	// 1 pure sprinter with Sprint=97 but low endurance → low composite
	pureSprinter := makeRider(9, "PureSprinter", "Team A", 40, 97, 55, 50)
	riders = append(riders, pureSprinter)

	// Flat sprint-finish course (like MSR)
	course := &model.Course{
		Name:            "MSR-like",
		TotalDistanceKm: 290,
		Segments: []model.Segment{
			{Name: "Flat Run-in", DistanceKm: 285, SurfaceType: model.SurfaceRoad, GradientPct: 0},
			{Name: "Final Sprint", DistanceKm: 5, SurfaceType: model.SurfaceRoad, GradientPct: 0},
		},
	}
	race := &model.Race{RaceType: model.RaceTypeRoad}
	config := SelectionConfig{NumTeams: 1, RidersPerTeam: 7}

	selected := SelectStartList(riders, course, race, config)

	found := false
	for _, r := range selected {
		if r.ID == pureSprinter.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("pure sprinter with highest sprint ability should be guaranteed selected in sprint-profile race")
	}
}
