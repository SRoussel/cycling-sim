package database

import (
	"cycling_sim/internal/model"
	"fmt"
	"math"
)

// CreateCourse inserts a course and returns it with the assigned ID.
func (db *DB) CreateCourse(c *model.Course) error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("invalid course: %w", err)
	}
	result, err := db.Exec(`INSERT INTO courses (name, total_distance_km) VALUES (?, ?)`,
		c.Name, c.TotalDistanceKm)
	if err != nil {
		return fmt.Errorf("inserting course: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting course id: %w", err)
	}
	c.ID = id
	return nil
}

// GetCourse retrieves a course by ID, including its segments.
func (db *DB) GetCourse(id int64) (*model.Course, error) {
	c := &model.Course{}
	err := db.QueryRow("SELECT id, name, total_distance_km FROM courses WHERE id = ?", id).
		Scan(&c.ID, &c.Name, &c.TotalDistanceKm)
	if err != nil {
		return nil, fmt.Errorf("getting course %d: %w", id, err)
	}
	segments, err := db.GetSegments(id)
	if err != nil {
		return nil, err
	}
	c.Segments = segments
	return c, nil
}

// ListCourses returns all courses (without segments).
func (db *DB) ListCourses() ([]model.Course, error) {
	rows, err := db.Query("SELECT id, name, total_distance_km FROM courses ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("listing courses: %w", err)
	}
	defer rows.Close()

	var courses []model.Course
	for rows.Next() {
		var c model.Course
		if err := rows.Scan(&c.ID, &c.Name, &c.TotalDistanceKm); err != nil {
			return nil, fmt.Errorf("scanning course: %w", err)
		}
		courses = append(courses, c)
	}
	return courses, rows.Err()
}

// CreateSegment inserts a segment for a course.
func (db *DB) CreateSegment(s *model.Segment) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("invalid segment: %w", err)
	}
	result, err := db.Exec(`
		INSERT INTO segments (course_id, segment_order, name, distance_km, gradient_pct, surface_type, is_technical, heading_deg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.CourseID, s.SegmentOrder, s.Name, s.DistanceKm, s.GradientPct, string(s.SurfaceType), boolToInt(s.IsTechnical), s.HeadingDeg)
	if err != nil {
		return fmt.Errorf("inserting segment: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting segment id: %w", err)
	}
	s.ID = id
	return nil
}

// GetSegments returns all segments for a course, ordered.
func (db *DB) GetSegments(courseID int64) ([]model.Segment, error) {
	rows, err := db.Query(`
		SELECT id, course_id, segment_order, name, distance_km, gradient_pct, surface_type, is_technical, heading_deg
		FROM segments WHERE course_id = ? ORDER BY segment_order`, courseID)
	if err != nil {
		return nil, fmt.Errorf("getting segments for course %d: %w", courseID, err)
	}
	defer rows.Close()

	var segments []model.Segment
	for rows.Next() {
		var s model.Segment
		var surface string
		var tech int
		if err := rows.Scan(&s.ID, &s.CourseID, &s.SegmentOrder, &s.Name, &s.DistanceKm, &s.GradientPct, &surface, &tech, &s.HeadingDeg); err != nil {
			return nil, fmt.Errorf("scanning segment: %w", err)
		}
		s.SurfaceType = model.SurfaceType(surface)
		s.IsTechnical = tech != 0
		segments = append(segments, s)
	}
	return segments, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ReplaceSegments replaces a course's segments (and updates its distance to
// their sum) in one transaction. Nothing else references segment rows.
func (db *DB) ReplaceSegments(courseID int64, segs []model.Segment) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM segments WHERE course_id = ?", courseID); err != nil {
		tx.Rollback()
		return err
	}
	total := 0.0
	for i, s := range segs {
		s.CourseID = courseID
		s.SegmentOrder = i + 1
		if err := s.Validate(); err != nil {
			tx.Rollback()
			return fmt.Errorf("segment %d: %w", i+1, err)
		}
		if _, err := tx.Exec(`INSERT INTO segments (course_id, segment_order, name, distance_km, gradient_pct, surface_type, is_technical, heading_deg)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, s.CourseID, s.SegmentOrder, s.Name, s.DistanceKm, s.GradientPct,
			string(s.SurfaceType), boolToInt(s.IsTechnical), s.HeadingDeg); err != nil {
			tx.Rollback()
			return err
		}
		total += s.DistanceKm
	}
	if _, err := tx.Exec("UPDATE courses SET total_distance_km = ? WHERE id = ?", math.Round(total*10)/10, courseID); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
