package model

import "testing"

func validRider() Rider {
	return Rider{
		Name:              "Tadej Pogačar",
		Team:              "UAE Team Emirates",
		Nationality:       "SLO",
		WeightKg:          66,
		HeightCm:          176,
		FTPWatts:          410,
		MaxSprintWatts:    1400,
		VO2MaxWatts:       480,
		CdA:               0.32,
		ClimbingAbility:   95,
		SprintAbility:     75,
		TTAbility:         88,
		DescendingAbility: 85,
		Endurance:         90,
		Recovery:          88,
		Consistency:       92,
		TacticalAwareness: 90,
	}
}

func TestRiderValidation(t *testing.T) {
	r := validRider()
	if err := r.Validate(); err != nil {
		t.Fatalf("valid rider should pass validation: %v", err)
	}

	tests := []struct {
		name   string
		modify func(*Rider)
	}{
		{"empty name", func(r *Rider) { r.Name = "" }},
		{"empty team", func(r *Rider) { r.Team = "" }},
		{"weight too low", func(r *Rider) { r.WeightKg = 30 }},
		{"weight too high", func(r *Rider) { r.WeightKg = 120 }},
		{"height too low", func(r *Rider) { r.HeightCm = 100 }},
		{"FTP too low", func(r *Rider) { r.FTPWatts = 100 }},
		{"FTP too high", func(r *Rider) { r.FTPWatts = 600 }},
		{"sprint watts too low", func(r *Rider) { r.MaxSprintWatts = 500 }},
		{"VO2max too low", func(r *Rider) { r.VO2MaxWatts = 200 }},
		{"CdA too low", func(r *Rider) { r.CdA = 0.1 }},
		{"climbing ability zero", func(r *Rider) { r.ClimbingAbility = 0 }},
		{"sprint ability too high", func(r *Rider) { r.SprintAbility = 101 }},
		{"consistency zero", func(r *Rider) { r.Consistency = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validRider()
			tt.modify(&r)
			if err := r.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestRiderTotalMass(t *testing.T) {
	r := validRider()
	expected := 66.0 + 8.0
	if got := r.TotalMassKg(); got != expected {
		t.Errorf("TotalMassKg() = %.1f, want %.1f", got, expected)
	}
}

func TestCourseValidation(t *testing.T) {
	c := Course{Name: "Alpe d'Huez", TotalDistanceKm: 13.8}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid course should pass: %v", err)
	}

	c.Name = ""
	if err := c.Validate(); err == nil {
		t.Error("empty name should fail")
	}

	c.Name = "Test"
	c.TotalDistanceKm = 0
	if err := c.Validate(); err == nil {
		t.Error("zero distance should fail")
	}
}

func TestSegmentValidation(t *testing.T) {
	s := Segment{
		Name:        "Climb",
		DistanceKm:  5.0,
		GradientPct: 8.0,
		SurfaceType: SurfaceRoad,
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("valid segment should pass: %v", err)
	}

	s.GradientPct = 35
	if err := s.Validate(); err == nil {
		t.Error("extreme gradient should fail")
	}

	s.GradientPct = 5
	s.SurfaceType = SurfaceGravel
	if err := s.Validate(); err != nil {
		t.Errorf("gravel is a surface: %v", err)
	}
	s.SurfaceType = "ice"
	if err := s.Validate(); err == nil {
		t.Error("invalid surface type should fail")
	}
}

func TestRaceValidation(t *testing.T) {
	r := Race{Name: "Stage 1", CourseID: 1, RaceType: RaceTypeRoad}
	if err := r.Validate(); err != nil {
		t.Fatalf("valid race should pass: %v", err)
	}

	r.RaceType = "criterium"
	if err := r.Validate(); err == nil {
		t.Error("invalid race type should fail")
	}
}

func TestStageRaceValidation(t *testing.T) {
	sr := StageRace{Name: "Tour de France"}
	if err := sr.Validate(); err != nil {
		t.Fatalf("valid stage race should pass: %v", err)
	}
	sr.Name = ""
	if err := sr.Validate(); err == nil {
		t.Error("empty name should fail")
	}
}
