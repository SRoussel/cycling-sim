package fcimport

import (
	"fmt"
	"io"
)

// Importer fetches and processes FirstCycling results.
type Importer struct {
	client *Client
	log    io.Writer
}

// NewImporter creates an Importer that writes progress logs to log.
func NewImporter(log io.Writer) *Importer {
	return &Importer{
		client: NewClient(),
		log:    log,
	}
}

// Fetch downloads and parses the results for a single race/stage request.
func (imp *Importer) Fetch(req FetchRequest) (*FetchResult, error) {
	if imp.log != nil {
		if req.Stage > 0 {
			fmt.Fprintf(imp.log, "Fetching FC race %d year %d stage %d...\n",
				req.FCRaceID, req.Year, req.Stage)
		} else {
			fmt.Fprintf(imp.log, "Fetching FC race %d year %d (overall)...\n",
				req.FCRaceID, req.Year)
		}
	}

	body, err := imp.client.FetchResultsPage(req.FCRaceID, req.Year, req.Stage)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	results, err := ParseResultsPage(body)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	if imp.log != nil {
		fmt.Fprintf(imp.log, "  → %d results parsed\n", len(results))
	}

	return &FetchResult{Req: req, Results: results}, nil
}

// FetchStages fetches individual stage results for stageFrom..stageTo (inclusive).
func (imp *Importer) FetchStages(fcRaceID, year, stageFrom, stageTo int) ([]*FetchResult, error) {
	var out []*FetchResult
	for s := stageFrom; s <= stageTo; s++ {
		r, err := imp.Fetch(FetchRequest{FCRaceID: fcRaceID, Year: year, Stage: s})
		if err != nil {
			return out, fmt.Errorf("stage %d: %w", s, err)
		}
		out = append(out, r)
	}
	return out, nil
}
