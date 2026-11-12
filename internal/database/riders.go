package database

import (
	"cycling_sim/internal/model"
	"fmt"
)

// CreateRider inserts a new rider and returns it with the assigned ID.
func (db *DB) CreateRider(r *model.Rider) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("invalid rider: %w", err)
	}
	result, err := db.Exec(`
		INSERT INTO riders (name, team, nationality, age, weight_kg, height_cm, ftp_watts,
			max_sprint_watts, vo2max_watts, cda, climbing_ability, sprint_ability,
			tt_ability, descending_ability, handling, endurance, recovery, consistency, tactical_awareness)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Name, r.Team, r.Nationality, r.Age, r.WeightKg, r.HeightCm, r.FTPWatts,
		r.MaxSprintWatts, r.VO2MaxWatts, r.CdA, r.ClimbingAbility, r.SprintAbility,
		r.TTAbility, r.DescendingAbility, r.Handling, r.Endurance, r.Recovery, r.Consistency, r.TacticalAwareness,
	)
	if err != nil {
		return fmt.Errorf("inserting rider: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting rider id: %w", err)
	}
	r.ID = id
	return nil
}

// GetRider retrieves a rider by ID.
func (db *DB) GetRider(id int64) (*model.Rider, error) {
	r, err := scanRider(db.QueryRow(`
		SELECT id, name, team, nationality, age, weight_kg, height_cm, ftp_watts,
			max_sprint_watts, vo2max_watts, cda, climbing_ability, sprint_ability,
			tt_ability, descending_ability, handling, endurance, recovery, consistency, tactical_awareness
		FROM riders WHERE id = ?`, id))
	if err != nil {
		return nil, fmt.Errorf("getting rider %d: %w", id, err)
	}
	return &r, nil
}

// ListRiders returns all riders.
func (db *DB) ListRiders() ([]model.Rider, error) {
	rows, err := db.Query(`
		SELECT id, name, team, nationality, age, weight_kg, height_cm, ftp_watts,
			max_sprint_watts, vo2max_watts, cda, climbing_ability, sprint_ability,
			tt_ability, descending_ability, handling, endurance, recovery, consistency, tactical_awareness
		FROM riders ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listing riders: %w", err)
	}
	defer rows.Close()

	var riders []model.Rider
	for rows.Next() {
		r, err := scanRider(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning rider: %w", err)
		}
		riders = append(riders, r)
	}
	return riders, rows.Err()
}

func scanRider(scanner rowScanner) (model.Rider, error) {
	var r model.Rider
	if err := scanner.Scan(
		&r.ID, &r.Name, &r.Team, &r.Nationality, &r.Age, &r.WeightKg, &r.HeightCm, &r.FTPWatts,
		&r.MaxSprintWatts, &r.VO2MaxWatts, &r.CdA, &r.ClimbingAbility, &r.SprintAbility,
		&r.TTAbility, &r.DescendingAbility, &r.Handling, &r.Endurance, &r.Recovery, &r.Consistency, &r.TacticalAwareness,
	); err != nil {
		return model.Rider{}, err
	}
	return r, nil
}

// DeleteRider removes a rider by ID.
func (db *DB) DeleteRider(id int64) error {
	result, err := db.Exec("DELETE FROM riders WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting rider %d: %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("rider %d not found", id)
	}
	return nil
}

// SetRiderFTPs updates riders' threshold power in one transaction.
func (db *DB) SetRiderFTPs(ftp map[int64]int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for id, w := range ftp {
		if _, err := tx.Exec("UPDATE riders SET ftp_watts = ? WHERE id = ?", w, id); err != nil {
			tx.Rollback()
			return fmt.Errorf("updating rider %d: %w", id, err)
		}
	}
	return tx.Commit()
}

// UpdateRiderRatings updates riders' physical attributes and ratings (not
// name, team or nationality) by ID, in one transaction.
func (db *DB) UpdateRiderRatings(riders []model.Rider) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, r := range riders {
		if err := r.Validate(); err != nil {
			tx.Rollback()
			return fmt.Errorf("invalid rider %s: %w", r.Name, err)
		}
		// Age is a rating layer's too (WorldDB gives real ages); 0 keeps it.
		_, err := tx.Exec(`UPDATE riders SET weight_kg = ?, height_cm = ?, ftp_watts = ?,
			max_sprint_watts = ?, vo2max_watts = ?, cda = ?, climbing_ability = ?, sprint_ability = ?,
			tt_ability = ?, descending_ability = ?, handling = ?, endurance = ?, recovery = ?,
			consistency = ?, tactical_awareness = ?, age = CASE WHEN ? > 0 THEN ? ELSE age END WHERE id = ?`,
			r.WeightKg, r.HeightCm, r.FTPWatts, r.MaxSprintWatts, r.VO2MaxWatts, r.CdA,
			r.ClimbingAbility, r.SprintAbility, r.TTAbility, r.DescendingAbility, r.Handling,
			r.Endurance, r.Recovery, r.Consistency, r.TacticalAwareness, r.Age, r.Age, r.ID)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("updating rider %d: %w", r.ID, err)
		}
	}
	return tx.Commit()
}
