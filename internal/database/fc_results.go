package database

import (
	"cycling_sim/internal/fcimport"
	"fmt"
)

// FCResult represents a stored FirstCycling reference result.
type FCResult struct {
	ID             int64
	FCRaceID       int
	Year           int
	Stage          int
	Position       int
	FCRiderID      int
	RiderName      string
	TeamName       string
	UCIPoints      int
	WinnerTimeSecs int
	GapSecs        int
	FetchedAt      string
	// Filled by joined queries when a match is known:
	InternalRiderID int64 // 0 if unmatched
}

// UpsertFCResults stores a batch of parsed results, replacing any existing rows
// for the same (fc_race_id, year, stage, position).
func (db *DB) UpsertFCResults(req fcimport.FetchRequest, results []fcimport.RawResult) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO fc_race_results
			(fc_race_id, year, stage, position, fc_rider_id, rider_name, team_name,
			 uci_points, winner_time_secs, gap_secs)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(fc_race_id, year, stage, position)
		DO UPDATE SET
			fc_rider_id      = excluded.fc_rider_id,
			rider_name       = excluded.rider_name,
			team_name        = excluded.team_name,
			uci_points       = excluded.uci_points,
			winner_time_secs = excluded.winner_time_secs,
			gap_secs         = excluded.gap_secs,
			fetched_at       = datetime('now')`)
	if err != nil {
		return fmt.Errorf("prepare upsert: %w", err)
	}
	defer stmt.Close()

	for _, r := range results {
		_, err := stmt.Exec(
			req.FCRaceID, req.Year, req.Stage,
			r.Position, r.FCRiderID, r.NormalizedName, r.TeamName,
			r.UCIPoints, r.WinnerTimeSecs, r.GapSecs,
		)
		if err != nil {
			return fmt.Errorf("upsert row pos=%d: %w", r.Position, err)
		}
	}

	// Upsert fc_rider_map entries (only new FC rider IDs; don't overwrite existing matches).
	stmtMap, err := tx.Prepare(`
		INSERT INTO fc_rider_map (fc_rider_id, fc_name)
		VALUES (?, ?)
		ON CONFLICT(fc_rider_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare rider map: %w", err)
	}
	defer stmtMap.Close()

	for _, r := range results {
		if _, err := stmtMap.Exec(r.FCRiderID, r.NormalizedName); err != nil {
			return fmt.Errorf("upsert rider map fc_rider_id=%d: %w", r.FCRiderID, err)
		}
	}

	return tx.Commit()
}

// ListFCResults returns stored reference results for a given race/year/stage.
// stage == 0 returns the overall/GC results.
func (db *DB) ListFCResults(fcRaceID, year, stage int) ([]FCResult, error) {
	rows, err := db.Query(`
		SELECT f.id, f.fc_race_id, f.year, f.stage, f.position,
		       f.fc_rider_id, f.rider_name, f.team_name, f.uci_points,
		       f.winner_time_secs, f.gap_secs, f.fetched_at,
		       COALESCE(m.rider_id, 0)
		FROM fc_race_results f
		LEFT JOIN fc_rider_map m ON m.fc_rider_id = f.fc_rider_id
		WHERE f.fc_race_id = ? AND f.year = ? AND f.stage = ?
		ORDER BY f.position`,
		fcRaceID, year, stage)
	if err != nil {
		return nil, fmt.Errorf("list fc results: %w", err)
	}
	defer rows.Close()

	var out []FCResult
	for rows.Next() {
		var r FCResult
		if err := rows.Scan(&r.ID, &r.FCRaceID, &r.Year, &r.Stage, &r.Position,
			&r.FCRiderID, &r.RiderName, &r.TeamName, &r.UCIPoints,
			&r.WinnerTimeSecs, &r.GapSecs, &r.FetchedAt, &r.InternalRiderID); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListFCFetched returns a summary of all (fc_race_id, year, stage) combos stored.
func (db *DB) ListFCFetched() ([]FCFetchedSummary, error) {
	rows, err := db.Query(`
		SELECT fc_race_id, year, stage, COUNT(*) as result_count,
		       MIN(fetched_at) as fetched_at
		FROM fc_race_results
		GROUP BY fc_race_id, year, stage
		ORDER BY fc_race_id, year, stage`)
	if err != nil {
		return nil, fmt.Errorf("list fc fetched: %w", err)
	}
	defer rows.Close()

	var out []FCFetchedSummary
	for rows.Next() {
		var s FCFetchedSummary
		if err := rows.Scan(&s.FCRaceID, &s.Year, &s.Stage, &s.ResultCount, &s.FetchedAt); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// FCFetchedSummary is one row from ListFCFetched.
type FCFetchedSummary struct {
	FCRaceID    int
	Year        int
	Stage       int
	ResultCount int
	FetchedAt   string
}

// ListRiderNamesAndIDs returns all (id, name) pairs for building a name lookup map.
func (db *DB) ListRiderNamesAndIDs() ([]fcimport.RiderNameID, error) {
	rows, err := db.Query(`SELECT id, name FROM riders ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list rider names: %w", err)
	}
	defer rows.Close()

	var out []fcimport.RiderNameID
	for rows.Next() {
		var p fcimport.RiderNameID
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, fmt.Errorf("scan rider name: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ApplyFCRiderMatch sets the internal rider_id for a given FC rider ID.
// Pass riderID == 0 to clear an existing match.
func (db *DB) ApplyFCRiderMatch(fcRiderID int, riderID int64) error {
	var err error
	if riderID == 0 {
		_, err = db.Exec(`UPDATE fc_rider_map SET rider_id = NULL WHERE fc_rider_id = ?`, fcRiderID)
	} else {
		_, err = db.Exec(`UPDATE fc_rider_map SET rider_id = ? WHERE fc_rider_id = ?`, riderID, fcRiderID)
	}
	if err != nil {
		return fmt.Errorf("apply fc rider match: %w", err)
	}
	return nil
}

// AutoMatchFCRiders attempts to match all unmatched FC rider names against internal
// riders by normalised name. Returns the number of new matches made.
func (db *DB) AutoMatchFCRiders() (int, error) {
	// Get all unmatched FC riders.
	rows, err := db.Query(`SELECT fc_rider_id, fc_name FROM fc_rider_map WHERE rider_id IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("query unmatched: %w", err)
	}
	type pending struct {
		fcID int
		name string
	}
	var unmatched []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.fcID, &p.name); err != nil {
			rows.Close()
			return 0, err
		}
		unmatched = append(unmatched, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	// Build name map from internal riders.
	pairs, err := db.ListRiderNamesAndIDs()
	if err != nil {
		return 0, err
	}
	nameMap := fcimport.BuildNameMap(pairs)

	matched := 0
	for _, p := range unmatched {
		id, ok := fcimport.MatchRiderByName(p.name, nameMap)
		if !ok {
			continue
		}
		if err := db.ApplyFCRiderMatch(p.fcID, id); err != nil {
			return matched, err
		}
		matched++
	}
	return matched, nil
}
