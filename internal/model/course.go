package model

import "fmt"

// SurfaceType represents road surface conditions.
type SurfaceType string

const (
	SurfaceRoad    SurfaceType = "road"
	SurfaceCobbles SurfaceType = "cobbles"
)

// Course represents a race course composed of segments.
type Course struct {
	ID              int64
	Name            string
	TotalDistanceKm float64
	Segments        []Segment
}

// Validate checks course fields.
func (c *Course) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("course name is required")
	}
	if c.TotalDistanceKm <= 0 {
		return fmt.Errorf("total distance must be positive, got %.1f", c.TotalDistanceKm)
	}
	return nil
}

// Segment represents a section of a course with uniform gradient and surface.
type Segment struct {
	ID           int64
	CourseID     int64
	SegmentOrder int
	Name         string
	DistanceKm   float64
	GradientPct  float64
	SurfaceType  SurfaceType
	IsTechnical  bool
	HeadingDeg   float64 // direction of travel, 0=N, 90=E (compass degrees)

	// Wind relative to this segment, set by the simulation for a given race
	// (not stored). Headwind is negative for a tailwind.
	HeadwindMPS  float64
	CrosswindMPS float64
	WindMPS      float64 // race wind speed
	WindFromDeg  float64 // direction the race wind blows from

	// Terrain, set by the simulation when a course is prepared (not stored).
	ElevationM float64 // altitude at the segment's midpoint
	AirDensity float64 // kg/m³ at that altitude; 0 means sea level
	Climb      float64 // cached climb factor (share of power lifting weight)
	ClimbSet   bool    // Climb is valid for the current wind
}

// Validate checks segment fields.
func (s *Segment) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("segment name is required")
	}
	if s.DistanceKm <= 0 {
		return fmt.Errorf("segment distance must be positive, got %.1f", s.DistanceKm)
	}
	if s.GradientPct < -25 || s.GradientPct > 30 {
		return fmt.Errorf("gradient must be between -25%% and 30%%, got %.1f%%", s.GradientPct)
	}
	if s.SurfaceType != SurfaceRoad && s.SurfaceType != SurfaceCobbles {
		return fmt.Errorf("surface type must be 'road' or 'cobbles', got %q", s.SurfaceType)
	}
	return nil
}
