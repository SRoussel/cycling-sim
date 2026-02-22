// Package fcimport fetches and stores race results from FirstCycling.com.
package fcimport

// RawResult is one result row as parsed directly from a FirstCycling page,
// before any name-matching or database insertion.
type RawResult struct {
	Position       int
	FCRiderID      int
	RawName        string // FC format: "LASTNAME Firstname", e.g. "POGACAR Tadej"
	NormalizedName string // Derived: "Firstname Lastname", e.g. "Tadej Pogacar"
	TeamName       string
	UCIPoints      int
	TimeStr        string // raw time string from the page
	WinnerTimeSecs int    // absolute finish time in seconds (0 if unknown)
	GapSecs        int    // gap to winner in seconds; 0 for winner
}

// FetchRequest describes what to fetch from FirstCycling.
type FetchRequest struct {
	FCRaceID int
	Year     int
	Stage    int // 0 = GC / overall / one-day race result
}

// FetchResult bundles the parsed results with metadata.
type FetchResult struct {
	Req     FetchRequest
	Results []RawResult
}
