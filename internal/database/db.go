package database

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB connection to the SQLite database.
type DB struct {
	*sql.DB
}

// Open creates or opens a SQLite database at the given path.
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// Enable WAL mode and foreign keys
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA foreign_keys=ON"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}
	db := &DB{sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return db, nil
}

// OpenMemory creates an in-memory SQLite database (useful for testing).
func OpenMemory() (*DB, error) {
	return Open(":memory:")
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS riders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		team TEXT NOT NULL,
		nationality TEXT NOT NULL DEFAULT '',
		age INTEGER NOT NULL DEFAULT 28,
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
		handling INTEGER NOT NULL DEFAULT 70,
		endurance INTEGER NOT NULL,
		recovery INTEGER NOT NULL,
		consistency INTEGER NOT NULL,
		tactical_awareness INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS courses (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		total_distance_km REAL NOT NULL
	);

	CREATE TABLE IF NOT EXISTS segments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		course_id INTEGER NOT NULL,
		segment_order INTEGER NOT NULL,
		name TEXT NOT NULL,
		distance_km REAL NOT NULL,
		gradient_pct REAL NOT NULL,
		surface_type TEXT NOT NULL DEFAULT 'road',
		is_technical INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_segments_course ON segments(course_id);

	CREATE TABLE IF NOT EXISTS races (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		course_id INTEGER NOT NULL,
		race_type TEXT NOT NULL,
		date TEXT NOT NULL DEFAULT '',
		temperature REAL NOT NULL DEFAULT 20,
		wind_speed_kmh REAL NOT NULL DEFAULT 0,
		wind_direction_deg REAL NOT NULL DEFAULT 0,
		FOREIGN KEY (course_id) REFERENCES courses(id)
	);
	CREATE INDEX IF NOT EXISTS idx_races_course ON races(course_id);

	CREATE TABLE IF NOT EXISTS stage_races (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS stage_race_stages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		stage_race_id INTEGER NOT NULL,
		race_id INTEGER NOT NULL,
		stage_number INTEGER NOT NULL,
		rest_day_before INTEGER NOT NULL DEFAULT 0,
		prestige REAL NOT NULL DEFAULT 0,
		FOREIGN KEY (stage_race_id) REFERENCES stage_races(id) ON DELETE CASCADE,
		FOREIGN KEY (race_id) REFERENCES races(id)
	);
	CREATE INDEX IF NOT EXISTS idx_srs_stage_race ON stage_race_stages(stage_race_id);

	CREATE TABLE IF NOT EXISTS race_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		race_id INTEGER NOT NULL,
		rider_id INTEGER NOT NULL,
		position INTEGER NOT NULL,
		finish_time_seconds REAL NOT NULL,
		time_gap_seconds REAL NOT NULL DEFAULT 0,
		FOREIGN KEY (race_id) REFERENCES races(id) ON DELETE CASCADE,
		FOREIGN KEY (rider_id) REFERENCES riders(id)
	);
	CREATE INDEX IF NOT EXISTS idx_results_race ON race_results(race_id);
	CREATE INDEX IF NOT EXISTS idx_results_rider ON race_results(rider_id);

	CREATE TABLE IF NOT EXISTS gc_standings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		stage_race_id INTEGER NOT NULL,
		rider_id INTEGER NOT NULL,
		after_stage INTEGER NOT NULL,
		total_time_seconds REAL NOT NULL,
		gc_position INTEGER NOT NULL,
		FOREIGN KEY (stage_race_id) REFERENCES stage_races(id) ON DELETE CASCADE,
		FOREIGN KEY (rider_id) REFERENCES riders(id)
	);
	CREATE INDEX IF NOT EXISTS idx_gc_stage_race ON gc_standings(stage_race_id);
	CREATE INDEX IF NOT EXISTS idx_gc_rider ON gc_standings(rider_id);

	CREATE TABLE IF NOT EXISTS points_standings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		stage_race_id INTEGER NOT NULL,
		rider_id INTEGER NOT NULL,
		after_stage INTEGER NOT NULL,
		points INTEGER NOT NULL,
		points_position INTEGER NOT NULL,
		FOREIGN KEY (stage_race_id) REFERENCES stage_races(id) ON DELETE CASCADE,
		FOREIGN KEY (rider_id) REFERENCES riders(id)
	);
	CREATE INDEX IF NOT EXISTS idx_points_stage_race ON points_standings(stage_race_id);
	CREATE INDEX IF NOT EXISTS idx_points_rider ON points_standings(rider_id);

	CREATE TABLE IF NOT EXISTS mountains_standings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		stage_race_id INTEGER NOT NULL,
		rider_id INTEGER NOT NULL,
		after_stage INTEGER NOT NULL,
		points INTEGER NOT NULL,
		kom_position INTEGER NOT NULL,
		FOREIGN KEY (stage_race_id) REFERENCES stage_races(id) ON DELETE CASCADE,
		FOREIGN KEY (rider_id) REFERENCES riders(id)
	);
	CREATE INDEX IF NOT EXISTS idx_kom_stage_race ON mountains_standings(stage_race_id);
	CREATE INDEX IF NOT EXISTS idx_kom_rider ON mountains_standings(rider_id);

	CREATE TABLE IF NOT EXISTS youth_standings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		stage_race_id INTEGER NOT NULL,
		rider_id INTEGER NOT NULL,
		after_stage INTEGER NOT NULL,
		total_time_seconds REAL NOT NULL,
		youth_position INTEGER NOT NULL,
		FOREIGN KEY (stage_race_id) REFERENCES stage_races(id) ON DELETE CASCADE,
		FOREIGN KEY (rider_id) REFERENCES riders(id)
	);
	CREATE INDEX IF NOT EXISTS idx_youth_stage_race ON youth_standings(stage_race_id);
	CREATE INDEX IF NOT EXISTS idx_youth_rider ON youth_standings(rider_id);

	-- Reference data imported from FirstCycling for validation / calibration.
	CREATE TABLE IF NOT EXISTS fc_race_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		fc_race_id INTEGER NOT NULL,
		year INTEGER NOT NULL,
		stage INTEGER NOT NULL DEFAULT 0,
		position INTEGER NOT NULL,
		fc_rider_id INTEGER NOT NULL,
		rider_name TEXT NOT NULL,
		team_name TEXT NOT NULL,
		uci_points INTEGER NOT NULL DEFAULT 0,
		winner_time_secs INTEGER NOT NULL DEFAULT 0,
		gap_secs INTEGER NOT NULL DEFAULT 0,
		fetched_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_fc_results_unique
		ON fc_race_results(fc_race_id, year, stage, position);

	-- Maps FC rider IDs to internal rider IDs; NULL rider_id means unmatched.
	CREATE TABLE IF NOT EXISTS fc_rider_map (
		fc_rider_id INTEGER PRIMARY KEY,
		fc_name TEXT NOT NULL,
		rider_id INTEGER,
		FOREIGN KEY (rider_id) REFERENCES riders(id)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	if _, err := db.Exec(seasonSchema); err != nil {
		return err
	}

	// Lightweight migrations for older DB files (ignored if already applied).
	for _, stmt := range []string{
		`ALTER TABLE riders ADD COLUMN age INTEGER NOT NULL DEFAULT 28`,
		`ALTER TABLE segments ADD COLUMN heading_deg REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE segments ADD COLUMN cobble_stars INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE stage_race_stages ADD COLUMN prestige REAL NOT NULL DEFAULT 0`,
	} {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}
