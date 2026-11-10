package database

import (
	"cycling_sim/internal/model"
	"fmt"
	"time"
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
	r, err := scanRace(db.QueryRow(`
		SELECT id, name, course_id, race_type, date, temperature, wind_speed_kmh, wind_direction_deg
		FROM races WHERE id = ?`, id))
	if err != nil {
		return nil, fmt.Errorf("getting race %d: %w", id, err)
	}
	return &r, nil
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
		r, err := scanRace(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning race: %w", err)
		}
		races = append(races, r)
	}
	return races, rows.Err()
}

func scanRace(scanner rowScanner) (model.Race, error) {
	var r model.Race
	var raceType, dateStr string
	if err := scanner.Scan(
		&r.ID, &r.Name, &r.CourseID, &raceType, &dateStr, &r.Temperature,
		&r.WindSpeedKmh, &r.WindDirectionDeg,
	); err != nil {
		return model.Race{}, err
	}
	r.RaceType = model.RaceType(raceType)
	parsedDate, err := parseRaceDate(dateStr)
	if err != nil {
		return model.Race{}, err
	}
	r.Date = parsedDate
	return r, nil
}

func parseRaceDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing race date %q: %w", dateStr, err)
	}
	return parsed, nil
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
		INSERT INTO stage_race_stages (stage_race_id, race_id, stage_number, rest_day_before, prestige)
		VALUES (?, ?, ?, ?, ?)`,
		srs.StageRaceID, srs.RaceID, srs.StageNumber, boolToInt(srs.RestDayBefore), srs.Prestige)
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

// ListStageRaces returns all stage races with their stage counts.
func (db *DB) ListStageRaces() ([]model.StageRace, error) {
	rows, err := db.Query(`
		SELECT sr.id, sr.name, COUNT(srs.id)
		FROM stage_races sr
		LEFT JOIN stage_race_stages srs ON srs.stage_race_id = sr.id
		GROUP BY sr.id ORDER BY sr.id`)
	if err != nil {
		return nil, fmt.Errorf("listing stage races: %w", err)
	}
	defer rows.Close()

	var result []model.StageRace
	for rows.Next() {
		var sr model.StageRace
		var stageCount int
		if err := rows.Scan(&sr.ID, &sr.Name, &stageCount); err != nil {
			return nil, fmt.Errorf("scanning stage race: %w", err)
		}
		sr.Stages = make([]model.StageRaceStage, stageCount)
		result = append(result, sr)
	}
	return result, rows.Err()
}

// GetStageRace retrieves a stage race with its stages.
func (db *DB) GetStageRace(id int64) (*model.StageRace, error) {
	sr := &model.StageRace{}
	err := db.QueryRow("SELECT id, name FROM stage_races WHERE id = ?", id).Scan(&sr.ID, &sr.Name)
	if err != nil {
		return nil, fmt.Errorf("getting stage race %d: %w", id, err)
	}

	rows, err := db.Query(`
		SELECT id, stage_race_id, race_id, stage_number, rest_day_before, prestige
		FROM stage_race_stages WHERE stage_race_id = ? ORDER BY stage_number`, id)
	if err != nil {
		return nil, fmt.Errorf("getting stages: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s model.StageRaceStage
		var restDay int
		if err := rows.Scan(&s.ID, &s.StageRaceID, &s.RaceID, &s.StageNumber, &restDay, &s.Prestige); err != nil {
			return nil, fmt.Errorf("scanning stage: %w", err)
		}
		s.RestDayBefore = restDay != 0
		sr.Stages = append(sr.Stages, s)
	}
	return sr, rows.Err()
}

// SetStagePrestige sets a stage's prestige (in place; keeps results).
func (db *DB) SetStagePrestige(stageRaceID int64, stageNumber int, prestige float64) error {
	_, err := db.Exec(`UPDATE stage_race_stages SET prestige = ? WHERE stage_race_id = ? AND stage_number = ?`,
		prestige, stageRaceID, stageNumber)
	if err != nil {
		return fmt.Errorf("setting stage prestige: %w", err)
	}
	return nil
}
