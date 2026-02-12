package selection

import "cycling_sim/internal/model"

// CourseProfile describes the demands of a course as normalized weights.
type CourseProfile struct {
	ClimbingDemand  float64
	SprintDemand    float64
	TTDemand        float64
	TechnicalDemand float64
	EnduranceDemand float64
}

// AnalyzeCourse computes a CourseProfile from course segments and race type.
func AnalyzeCourse(course *model.Course, raceType model.RaceType) CourseProfile {
	if raceType == model.RaceTypeTT {
		return CourseProfile{
			ClimbingDemand:  0.1,
			SprintDemand:    0.0,
			TTDemand:        0.7,
			TechnicalDemand: 0.0,
			EnduranceDemand: 0.2,
		}
	}

	totalKm := 0.0
	climbKm := 0.0
	cobblesKm := 0.0
	for _, seg := range course.Segments {
		totalKm += seg.DistanceKm
		if seg.GradientPct > 4.0 {
			climbKm += seg.DistanceKm
		}
		if seg.SurfaceType == model.SurfaceCobbles {
			cobblesKm += seg.DistanceKm
		}
	}

	if totalKm == 0 {
		return CourseProfile{}
	}

	climbProp := climbKm / totalKm
	cobbleProp := cobblesKm / totalKm

	// Climbing demand from proportion of steep km
	climbDemand := climbProp * 3.0 // scale up so 33% climbing → ~1.0
	if climbDemand > 1.0 {
		climbDemand = 1.0
	}

	// Sprint demand: high when course is flat with a flat finish
	sprintDemand := 0.0
	if climbProp < 0.05 && cobbleProp < 0.02 {
		// Check if last segment is flat
		if len(course.Segments) > 0 {
			last := course.Segments[len(course.Segments)-1]
			if last.GradientPct <= 2.0 && last.GradientPct >= -2.0 {
				sprintDemand = 0.8
			}
		}
	}

	// Technical demand from cobbles
	techDemand := cobbleProp * 10.0 // scale up so 10% cobbles → 1.0
	if techDemand > 1.0 {
		techDemand = 1.0
	}

	// Endurance demand from total distance (200km = 0.5, 300km = 0.75)
	enduranceDemand := totalKm / 400.0
	if enduranceDemand > 1.0 {
		enduranceDemand = 1.0
	}

	return CourseProfile{
		ClimbingDemand:  climbDemand,
		SprintDemand:    sprintDemand,
		TTDemand:        0.1, // small TT component for road races (pacing)
		TechnicalDemand: techDemand,
		EnduranceDemand: enduranceDemand,
	}
}
