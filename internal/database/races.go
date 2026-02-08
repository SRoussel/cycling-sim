package database

import (
	"cycling_sim/internal/model"
	"fmt"
)

// CreateRace inserts a race.
func (db *DB) CreateRace(r *model.Race) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("invalid race: %w", err)
	}
	result, err := db.Exec(`
		INSERT INTO races (name, course_id, race_type, date, temperature, wind_speed_kmh, wind_direction_deg)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.Name, r.CourseID, string(r.RaceType), r.Date.Format("2006-01-02"), r.Temperature,
		r.WindSpeedKmh, r.WindDirectionDeg)
	if err != nil {
		return fmt.Errorf("inserting race: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting race id: %w", err)
	}
	r.ID = id
	return nil
}

// GetRace retrieves a race by ID.
func (db *DB) GetRace(id int64) (*model.Race, error) {
	r := &model.Race{}
	var raceType, dateStr string
	err := db.QueryRow(`
		SELECT id, name, course_id, race_type, date, temperature, wind_speed_kmh, wind_direction_deg
		FROM races WHERE id = ?`, id).Scan(
		&r.ID, &r.Name, &r.CourseID, &raceType, &dateStr, &r.Temperature,
		&r.WindSpeedKmh, &r.WindDirectionDeg)
	if err != nil {
		return nil, fmt.Errorf("getting race %d: %w", id, err)
	}
	r.RaceType = model.RaceType(raceType)
	return r, nil
}

// ListRaces returns all races.
func (db *DB) ListRaces() ([]model.Race, error) {
	rows, err := db.Query(`
		SELECT id, name, course_id, race_type, date, temperature, wind_speed_kmh, wind_direction_deg
		FROM races ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listing races: %w", err)
	}
	defer rows.Close()

	var races []model.Race
	for rows.Next() {
		var r model.Race
		var raceType, dateStr string
		if err := rows.Scan(&r.ID, &r.Name, &r.CourseID, &raceType, &dateStr, &r.Temperature,
			&r.WindSpeedKmh, &r.WindDirectionDeg); err != nil {
			return nil, fmt.Errorf("scanning race: %w", err)
		}
		r.RaceType = model.RaceType(raceType)
		races = append(races, r)
	}
	return races, rows.Err()
}

// CreateStageRace inserts a stage race.
func (db *DB) CreateStageRace(sr *model.StageRace) error {
	if err := sr.Validate(); err != nil {
		return fmt.Errorf("invalid stage race: %w", err)
	}
	result, err := db.Exec("INSERT INTO stage_races (name) VALUES (?)", sr.Name)
	if err != nil {
		return fmt.Errorf("inserting stage race: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting stage race id: %w", err)
	}
	sr.ID = id
	return nil
}

// AddStageToRace links a race as a stage in a stage race.
func (db *DB) AddStageToRace(srs *model.StageRaceStage) error {
	result, err := db.Exec(`
		INSERT INTO stage_race_stages (stage_race_id, race_id, stage_number, rest_day_before)
		VALUES (?, ?, ?, ?)`,
		srs.StageRaceID, srs.RaceID, srs.StageNumber, boolToInt(srs.RestDayBefore))
	if err != nil {
		return fmt.Errorf("adding stage to race: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting stage link id: %w", err)
	}
	srs.ID = id
	return nil
}

// GetStageRace retrieves a stage race with its stages.
func (db *DB) GetStageRace(id int64) (*model.StageRace, error) {
	sr := &model.StageRace{}
	err := db.QueryRow("SELECT id, name FROM stage_races WHERE id = ?", id).Scan(&sr.ID, &sr.Name)
	if err != nil {
		return nil, fmt.Errorf("getting stage race %d: %w", id, err)
	}

	rows, err := db.Query(`
		SELECT id, stage_race_id, race_id, stage_number, rest_day_before
		FROM stage_race_stages WHERE stage_race_id = ? ORDER BY stage_number`, id)
	if err != nil {
		return nil, fmt.Errorf("getting stages: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s model.StageRaceStage
		var restDay int
		if err := rows.Scan(&s.ID, &s.StageRaceID, &s.RaceID, &s.StageNumber, &restDay); err != nil {
			return nil, fmt.Errorf("scanning stage: %w", err)
		}
		s.RestDayBefore = restDay != 0
		sr.Stages = append(sr.Stages, s)
	}
	return sr, rows.Err()
}
