package database

import (
	"cycling_sim/internal/model"
	"fmt"
)

// SaveRaceResults saves a batch of race results.
func (db *DB) SaveRaceResults(results []model.RaceResult) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO race_results (race_id, rider_id, position, finish_time_seconds, time_gap_seconds)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for i := range results {
		r := &results[i]
		res, err := stmt.Exec(r.RaceID, r.RiderID, r.Position, r.FinishTimeSeconds, r.TimeGapSeconds)
		if err != nil {
			return fmt.Errorf("inserting result for rider %d: %w", r.RiderID, err)
		}
		id, _ := res.LastInsertId()
		r.ID = id
	}
	return tx.Commit()
}

// GetRaceResults returns results for a race, ordered by position.
func (db *DB) GetRaceResults(raceID int64) ([]model.RaceResult, error) {
	rows, err := db.Query(`
		SELECT id, race_id, rider_id, position, finish_time_seconds, time_gap_seconds
		FROM race_results WHERE race_id = ? ORDER BY position`, raceID)
	if err != nil {
		return nil, fmt.Errorf("getting results for race %d: %w", raceID, err)
	}
	defer rows.Close()

	var results []model.RaceResult
	for rows.Next() {
		var r model.RaceResult
		if err := rows.Scan(&r.ID, &r.RaceID, &r.RiderID, &r.Position, &r.FinishTimeSeconds, &r.TimeGapSeconds); err != nil {
			return nil, fmt.Errorf("scanning result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// SaveGCStandings saves GC standings after a stage.
func (db *DB) SaveGCStandings(standings []model.GCStanding) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO gc_standings (stage_race_id, rider_id, after_stage, total_time_seconds, gc_position)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for i := range standings {
		s := &standings[i]
		res, err := stmt.Exec(s.StageRaceID, s.RiderID, s.AfterStage, s.TotalTimeSeconds, s.GCPosition)
		if err != nil {
			return fmt.Errorf("inserting GC standing: %w", err)
		}
		id, _ := res.LastInsertId()
		s.ID = id
	}
	return tx.Commit()
}

// GetGCStandings returns GC standings for a stage race after a given stage.
func (db *DB) GetGCStandings(stageRaceID int64, afterStage int) ([]model.GCStanding, error) {
	rows, err := db.Query(`
		SELECT id, stage_race_id, rider_id, after_stage, total_time_seconds, gc_position
		FROM gc_standings WHERE stage_race_id = ? AND after_stage = ? ORDER BY gc_position`,
		stageRaceID, afterStage)
	if err != nil {
		return nil, fmt.Errorf("getting GC standings: %w", err)
	}
	defer rows.Close()

	var standings []model.GCStanding
	for rows.Next() {
		var s model.GCStanding
		if err := rows.Scan(&s.ID, &s.StageRaceID, &s.RiderID, &s.AfterStage, &s.TotalTimeSeconds, &s.GCPosition); err != nil {
			return nil, fmt.Errorf("scanning GC standing: %w", err)
		}
		standings = append(standings, s)
	}
	return standings, rows.Err()
}
