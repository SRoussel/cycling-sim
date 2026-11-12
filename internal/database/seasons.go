package database

import (
	"database/sql"
	"fmt"
	"time"

	"cycling_sim/internal/model"
)

const seasonSchema = `
CREATE TABLE IF NOT EXISTS seasons (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	team TEXT NOT NULL,
	year INTEGER NOT NULL,
	start_year INTEGER NOT NULL,
	date TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS season_riders (
	season_id INTEGER NOT NULL,
	rider_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	team TEXT NOT NULL,
	nationality TEXT NOT NULL DEFAULT '',
	age INTEGER NOT NULL,
	weight_kg REAL NOT NULL,
	height_cm REAL NOT NULL,
	ftp_watts INTEGER NOT NULL,
	max_sprint_watts INTEGER NOT NULL,
	vo2max_watts INTEGER NOT NULL,
	cda REAL NOT NULL,
	climbing_ability INTEGER NOT NULL,
	sprint_ability INTEGER NOT NULL,
	tt_ability INTEGER NOT NULL,
	descending_ability INTEGER NOT NULL,
	handling INTEGER NOT NULL,
	endurance INTEGER NOT NULL,
	recovery INTEGER NOT NULL,
	consistency INTEGER NOT NULL,
	tactical_awareness INTEGER NOT NULL,
	contract_end INTEGER NOT NULL,
	salary INTEGER NOT NULL,
	morale REAL NOT NULL,
	load REAL NOT NULL DEFAULT 0,
	race_days INTEGER NOT NULL DEFAULT 0,
	retired INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (season_id, rider_id),
	FOREIGN KEY (season_id) REFERENCES seasons(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS season_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	season_id INTEGER NOT NULL,
	year INTEGER NOT NULL,
	kind TEXT NOT NULL,
	ref_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	category TEXT NOT NULL,
	start_date TEXT NOT NULL,
	end_date TEXT NOT NULL,
	done INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (season_id) REFERENCES seasons(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_season_events ON season_events(season_id, year);

CREATE TABLE IF NOT EXISTS season_results (
	season_id INTEGER NOT NULL,
	event_id INTEGER NOT NULL,
	year INTEGER NOT NULL,
	rider_id INTEGER NOT NULL,
	team TEXT NOT NULL,
	kind TEXT NOT NULL,
	stage INTEGER NOT NULL DEFAULT 0,
	position INTEGER NOT NULL,
	gap_s REAL NOT NULL,
	points INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (season_id) REFERENCES seasons(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_season_results ON season_results(season_id, year);
CREATE INDEX IF NOT EXISTS idx_season_results_event ON season_results(event_id);

CREATE TABLE IF NOT EXISTS season_entries (
	season_id INTEGER NOT NULL,
	event_id INTEGER NOT NULL,
	rider_id INTEGER NOT NULL,
	PRIMARY KEY (event_id, rider_id),
	FOREIGN KEY (season_id) REFERENCES seasons(id) ON DELETE CASCADE
);
`

const dateLayout = "2006-01-02"

func parseDate(s string) time.Time {
	t, _ := time.Parse(dateLayout, s)
	return t
}

// CreateSeason inserts a season and assigns its ID.
func (db *DB) CreateSeason(s *model.Season) error {
	res, err := db.Exec(`INSERT INTO seasons (name, team, year, start_year, date) VALUES (?, ?, ?, ?, ?)`,
		s.Name, s.Team, s.Year, s.StartYear, s.Date.Format(dateLayout))
	if err != nil {
		return fmt.Errorf("inserting season: %w", err)
	}
	s.ID, err = res.LastInsertId()
	return err
}

// UpdateSeason saves a season's year, date and team.
func (db *DB) UpdateSeason(s *model.Season) error {
	_, err := db.Exec(`UPDATE seasons SET name = ?, team = ?, year = ?, date = ? WHERE id = ?`,
		s.Name, s.Team, s.Year, s.Date.Format(dateLayout), s.ID)
	return err
}

// GetSeason loads one season.
func (db *DB) GetSeason(id int64) (*model.Season, error) {
	var s model.Season
	var date string
	err := db.QueryRow(`SELECT id, name, team, year, start_year, date FROM seasons WHERE id = ?`, id).
		Scan(&s.ID, &s.Name, &s.Team, &s.Year, &s.StartYear, &date)
	if err != nil {
		return nil, fmt.Errorf("getting season %d: %w", id, err)
	}
	s.Date = parseDate(date)
	return &s, nil
}

// ListSeasons returns every season, newest first.
func (db *DB) ListSeasons() ([]model.Season, error) {
	rows, err := db.Query(`SELECT id, name, team, year, start_year, date FROM seasons ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Season
	for rows.Next() {
		var s model.Season
		var date string
		if err := rows.Scan(&s.ID, &s.Name, &s.Team, &s.Year, &s.StartYear, &date); err != nil {
			return nil, err
		}
		s.Date = parseDate(date)
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteSeason removes a season and everything in it.
func (db *DB) DeleteSeason(id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM season_results WHERE season_id = ?`,
		`DELETE FROM season_entries WHERE season_id = ?`,
		`DELETE FROM season_events WHERE season_id = ?`,
		`DELETE FROM season_riders WHERE season_id = ?`,
		`DELETE FROM seasons WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SaveSeasonRiders inserts or replaces riders of a season, in one transaction.
func (db *DB) SaveSeasonRiders(seasonID int64, riders []model.SeasonRider) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO season_riders (season_id, rider_id, name, team, nationality, age,
		weight_kg, height_cm, ftp_watts, max_sprint_watts, vo2max_watts, cda, climbing_ability, sprint_ability,
		tt_ability, descending_ability, handling, endurance, recovery, consistency, tactical_awareness,
		contract_end, salary, morale, load, race_days, retired)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, sr := range riders {
		r := sr.Rider
		if _, err := stmt.Exec(seasonID, r.ID, r.Name, r.Team, r.Nationality, r.Age, r.WeightKg, r.HeightCm,
			r.FTPWatts, r.MaxSprintWatts, r.VO2MaxWatts, r.CdA, r.ClimbingAbility, r.SprintAbility, r.TTAbility,
			r.DescendingAbility, r.Handling, r.Endurance, r.Recovery, r.Consistency, r.TacticalAwareness,
			sr.ContractEnd, sr.Salary, sr.Morale, sr.Load, sr.RaceDays, boolInt(sr.Retired)); err != nil {
			return fmt.Errorf("saving season rider %d: %w", r.ID, err)
		}
	}
	return tx.Commit()
}

// ListSeasonRiders returns a season's riders (retired ones too), by ID.
func (db *DB) ListSeasonRiders(seasonID int64) ([]model.SeasonRider, error) {
	rows, err := db.Query(`SELECT rider_id, name, team, nationality, age, weight_kg, height_cm, ftp_watts,
		max_sprint_watts, vo2max_watts, cda, climbing_ability, sprint_ability, tt_ability, descending_ability,
		handling, endurance, recovery, consistency, tactical_awareness, contract_end, salary, morale, load,
		race_days, retired FROM season_riders WHERE season_id = ? ORDER BY rider_id`, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SeasonRider
	for rows.Next() {
		var sr model.SeasonRider
		var retired int
		r := &sr.Rider
		if err := rows.Scan(&r.ID, &r.Name, &r.Team, &r.Nationality, &r.Age, &r.WeightKg, &r.HeightCm,
			&r.FTPWatts, &r.MaxSprintWatts, &r.VO2MaxWatts, &r.CdA, &r.ClimbingAbility, &r.SprintAbility,
			&r.TTAbility, &r.DescendingAbility, &r.Handling, &r.Endurance, &r.Recovery, &r.Consistency,
			&r.TacticalAwareness, &sr.ContractEnd, &sr.Salary, &sr.Morale, &sr.Load, &sr.RaceDays, &retired); err != nil {
			return nil, err
		}
		sr.Retired = retired != 0
		out = append(out, sr)
	}
	return out, rows.Err()
}

// SaveSeasonEvents inserts new events (ID 0) or updates existing ones.
func (db *DB) SaveSeasonEvents(events []model.SeasonEvent) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := range events {
		e := &events[i]
		if e.ID == 0 {
			res, err := tx.Exec(`INSERT INTO season_events (season_id, year, kind, ref_id, name, category, start_date, end_date, done)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, e.SeasonID, e.Year, e.Kind, e.RefID, e.Name, e.Category,
				e.Start.Format(dateLayout), e.End.Format(dateLayout), boolInt(e.Done))
			if err != nil {
				return err
			}
			if e.ID, err = res.LastInsertId(); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(`UPDATE season_events SET done = ? WHERE id = ?`, boolInt(e.Done), e.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListSeasonEvents returns a season's calendar for a year, in date order.
func (db *DB) ListSeasonEvents(seasonID int64, year int) ([]model.SeasonEvent, error) {
	rows, err := db.Query(`SELECT id, season_id, year, kind, ref_id, name, category, start_date, end_date, done
		FROM season_events WHERE season_id = ? AND year = ? ORDER BY start_date, id`, seasonID, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SeasonEvent
	for rows.Next() {
		var e model.SeasonEvent
		var start, end string
		var done int
		if err := rows.Scan(&e.ID, &e.SeasonID, &e.Year, &e.Kind, &e.RefID, &e.Name, &e.Category, &start, &end, &done); err != nil {
			return nil, err
		}
		e.Start, e.End, e.Done = parseDate(start), parseDate(end), done != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// SaveSeasonResults replaces an event's results with the given ones.
func (db *DB) SaveSeasonResults(eventID int64, results []model.SeasonResult) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM season_results WHERE event_id = ?`, eventID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO season_results (season_id, event_id, year, rider_id, team, kind, stage, position, gap_s, points)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range results {
		if _, err := stmt.Exec(r.SeasonID, eventID, r.Year, r.RiderID, r.Team, r.Kind, r.Stage, r.Position, r.GapS, r.Points); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListSeasonResults returns a season's results for a year (eventID 0: all
// events), ordered by event, kind, stage and position.
func (db *DB) ListSeasonResults(seasonID int64, year int, eventID int64) ([]model.SeasonResult, error) {
	var rows *sql.Rows
	var err error
	q := `SELECT season_id, event_id, year, rider_id, team, kind, stage, position, gap_s, points FROM season_results WHERE season_id = ? AND year = ?`
	if eventID != 0 {
		rows, err = db.Query(q+` AND event_id = ? ORDER BY kind, stage, position`, seasonID, year, eventID)
	} else {
		rows, err = db.Query(q+` ORDER BY event_id, kind, stage, position`, seasonID, year)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SeasonResult
	for rows.Next() {
		var r model.SeasonResult
		if err := rows.Scan(&r.SeasonID, &r.EventID, &r.Year, &r.RiderID, &r.Team, &r.Kind, &r.Stage, &r.Position, &r.GapS, &r.Points); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SaveEventEntries replaces an event's planned starters.
func (db *DB) SaveEventEntries(seasonID, eventID int64, riderIDs []int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM season_entries WHERE event_id = ?`, eventID); err != nil {
		return err
	}
	for _, id := range riderIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO season_entries (season_id, event_id, rider_id) VALUES (?, ?, ?)`, seasonID, eventID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListSeasonEntries returns every planned start of a season: event ID ->
// rider IDs.
func (db *DB) ListSeasonEntries(seasonID int64) (map[int64][]int64, error) {
	rows, err := db.Query(`SELECT event_id, rider_id FROM season_entries WHERE season_id = ? ORDER BY event_id, rider_id`, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]int64{}
	for rows.Next() {
		var e, r int64
		if err := rows.Scan(&e, &r); err != nil {
			return nil, err
		}
		out[e] = append(out[e], r)
	}
	return out, rows.Err()
}
