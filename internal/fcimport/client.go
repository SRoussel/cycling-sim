package fcimport

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://firstcycling.com"

// Client fetches pages from FirstCycling.
type Client struct {
	http *http.Client
}

// NewClient creates an FC client with sensible timeouts.
func NewClient() *Client {
	return &Client{
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchResultsPage downloads the HTML for a race results page.
// stage == 0 requests the overall/GC result (no &e= param).
func (c *Client) FetchResultsPage(fcRaceID, year, stage int) ([]byte, error) {
	url := fmt.Sprintf("%s/race.php?r=%d&y=%d", baseURL, fcRaceID, year)
	if stage > 0 {
		url += fmt.Sprintf("&e=%d", stage)
	}
	return c.get(url)
}

func (c *Client) get(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; cycling-sim-importer/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	return body, nil
}
