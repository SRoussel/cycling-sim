package database

import (
	"os"
	"path/filepath"
)

// DefaultPath is where the game keeps its database: CYCLING_SIM_DB if set,
// else the user's app-data folder (Windows %AppData%\CyclingSim, macOS
// ~/Library/Application Support/CyclingSim, Linux ~/.config/CyclingSim), as
// a shipped game must (not next to the executable). Falls back to the
// working directory if there's no such folder.
func DefaultPath() string {
	if p := os.Getenv("CYCLING_SIM_DB"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "cycling_sim.db"
	}
	dir = filepath.Join(dir, "CyclingSim")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "cycling_sim.db"
	}
	return filepath.Join(dir, "cycling_sim.db")
}
