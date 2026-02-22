package fcimport

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// ParseResultsPage parses the raw HTML from a FirstCycling results page and
// returns the ordered list of results.
func ParseResultsPage(body []byte) ([]RawResult, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Find all <tr> elements that contain a rider link.
	var results []RawResult
	var winnerSecs int

	walkNode(doc, func(n *html.Node) {
		if n.Type != html.ElementNode || n.Data != "tr" {
			return
		}
		r, ok := parseResultRow(n)
		if !ok {
			return
		}
		// First valid row gives the winner's absolute time.
		if len(results) == 0 && r.WinnerTimeSecs > 0 {
			winnerSecs = r.WinnerTimeSecs
		}
		if r.WinnerTimeSecs == 0 && winnerSecs > 0 {
			r.WinnerTimeSecs = winnerSecs
		}
		results = append(results, r)
	})

	return results, nil
}

// parseResultRow attempts to parse a <tr> as a results row.
// Returns (result, true) on success, or (_, false) if the row isn't a result row.
func parseResultRow(tr *html.Node) (RawResult, bool) {
	tds := childElements(tr, "td")
	if len(tds) < 4 {
		return RawResult{}, false
	}

	// td[0] = position
	posText := strings.TrimSpace(textContent(tds[0]))
	pos, err := strconv.Atoi(posText)
	if err != nil {
		// Could be "DNS", "DNF", "OTL", "DSQ" – skip non-finishers.
		return RawResult{}, false
	}

	// Find the rider <a> link in one of the early cells.
	// Typically td[1] contains <a href="rider.php?r=XXX&y=YYYY">NAME</a>
	riderTD := tds[1]
	if len(tds) >= 5 {
		// Some pages have a flag/number column; find first td with a rider.php link.
		for _, td := range tds[:4] {
			if findRiderLink(td) != nil {
				riderTD = td
				break
			}
		}
	}

	riderLink := findRiderLink(riderTD)
	if riderLink == nil {
		return RawResult{}, false
	}

	fcRiderID, rawName := extractRiderInfo(riderLink)
	if fcRiderID == 0 || rawName == "" {
		return RawResult{}, false
	}

	// Remaining cells: team, uci_pts, time (positions may shift by 1 if there's an extra col).
	// Use the last 3 cells: team, uci, time.
	teamName := ""
	uciPts := 0
	timeStr := ""

	n := len(tds)
	if n >= 3 {
		timeStr = strings.TrimSpace(textContent(tds[n-1]))
		uciPts, _ = strconv.Atoi(strings.TrimSpace(textContent(tds[n-2])))
		teamName = strings.TrimSpace(textContent(tds[n-3]))
	}

	var winnerSecs, gapSecs int
	if strings.HasPrefix(timeStr, "+") {
		gapSecs = parseGap(timeStr)
	} else {
		winnerSecs = parseAbsTime(timeStr)
	}

	return RawResult{
		Position:       pos,
		FCRiderID:      fcRiderID,
		RawName:        rawName,
		NormalizedName: normalizeRiderName(rawName),
		TeamName:       teamName,
		UCIPoints:      uciPts,
		TimeStr:        timeStr,
		WinnerTimeSecs: winnerSecs,
		GapSecs:        gapSecs,
	}, true
}

// findRiderLink returns the first <a href="rider.php?r=..."> under node n, or nil.
func findRiderLink(n *html.Node) *html.Node {
	var found *html.Node
	walkNode(n, func(node *html.Node) {
		if found != nil {
			return
		}
		if node.Type == html.ElementNode && node.Data == "a" {
			href := attrVal(node, "href")
			if strings.Contains(href, "rider.php?r=") || strings.Contains(href, "rider.php") {
				found = node
			}
		}
	})
	return found
}

// extractRiderInfo returns (fcRiderID, rawName) from a rider <a> element.
func extractRiderInfo(a *html.Node) (int, string) {
	href := attrVal(a, "href")
	name := strings.TrimSpace(textContent(a))

	// href is like "rider.php?r=2275&y=2024" or "/rider.php?r=2275"
	// Strip leading slash if present.
	href = strings.TrimPrefix(href, "/")
	queryStr := href
	if idx := strings.Index(href, "?"); idx >= 0 {
		queryStr = href[idx+1:]
	}
	params, err := url.ParseQuery(queryStr)
	if err != nil {
		return 0, ""
	}
	rStr := params.Get("r")
	if rStr == "" {
		return 0, ""
	}
	id, err := strconv.Atoi(rStr)
	if err != nil {
		return 0, ""
	}
	return id, name
}

// normalizeRiderName converts FC "LASTNAME Firstname" format to "Firstname Lastname"
// with title-casing applied to each part.
//
// Examples:
//
//	"BARDET Romain"       → "Romain Bardet"
//	"VAN AERT Wout"       → "Wout Van Aert"
//	"VAN DEN BROEK Frank" → "Frank Van Den Broek"
func normalizeRiderName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	words := strings.Fields(raw)

	// Identify the boundary: FC puts ALL-CAPS surname words first, then the given
	// name(s) which contain at least one lowercase letter.
	splitIdx := len(words) // default: all surname
	for i, w := range words {
		if !isAllUpper(w) {
			splitIdx = i
			break
		}
	}

	surnameParts := words[:splitIdx]
	givenParts := words[splitIdx:]

	// Rebuild in "Given Surname" order with title casing.
	parts := make([]string, 0, len(words))
	for _, p := range givenParts {
		parts = append(parts, titleCase(p))
	}
	for _, p := range surnameParts {
		parts = append(parts, titleCase(p))
	}
	return strings.Join(parts, " ")
}

// isAllUpper returns true if every letter in s is uppercase (ignoring non-letters).
func isAllUpper(s string) bool {
	hasLetter := false
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
		} else if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return hasLetter
}

// titleCase upper-cases the first rune and lower-cases the rest.
// Preserves special characters (é, ü, etc.) by working at rune level.
func titleCase(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	result := make([]rune, len(runes))
	for i, r := range runes {
		if i == 0 {
			result[i] = toUpperRune(r)
		} else {
			result[i] = toLowerRune(r)
		}
	}
	return string(result)
}

func toUpperRune(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

func toLowerRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + 32
	}
	return r
}

// parseAbsTime converts "H:MM:SS" or "HH:MM:SS" to total seconds.
func parseAbsTime(s string) int {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return 0
	}
	total := 0
	for _, p := range parts {
		n, _ := strconv.Atoi(strings.TrimSpace(p))
		total = total*60 + n
	}
	return total
}

// parseGap converts "+ HH:MM:SS", "+ M:SS", "+ SS" to gap seconds.
func parseGap(s string) int {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "+")
	s = strings.TrimSpace(s)
	if s == "00" || s == "0" {
		return 0
	}
	// Could be "MM:SS", "H:MM:SS", or just "SS"
	parts := strings.Split(s, ":")
	total := 0
	for _, p := range parts {
		n, _ := strconv.Atoi(strings.TrimSpace(p))
		total = total*60 + n
	}
	return total
}

// --- HTML traversal helpers ---

// walkNode calls f on every node in the subtree rooted at n.
func walkNode(n *html.Node, f func(*html.Node)) {
	f(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkNode(c, f)
	}
}

// childElements returns direct child elements of n with the given tag name.
func childElements(n *html.Node, tag string) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			out = append(out, c)
		}
	}
	return out
}

// textContent returns all text node content under n, concatenated.
func textContent(n *html.Node) string {
	var sb strings.Builder
	walkNode(n, func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
	})
	return sb.String()
}

// attrVal returns the value of an attribute of element n.
func attrVal(n *html.Node, attr string) string {
	for _, a := range n.Attr {
		if a.Key == attr {
			return a.Val
		}
	}
	return ""
}
