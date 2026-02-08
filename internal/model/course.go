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
