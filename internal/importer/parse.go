// Package importer reads race results that the user saved locally (browser
// "Save page" from FirstCycling or ProCyclingStats, or a simple CSV) and
// derives rider ratings from them. It never fetches anything over the network:
// both sites disallow automated access.
package importer

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

// Result is one finisher in a saved results page.
type Result struct {
	Position int
	Name     string // as written on the page, e.g. "POGACAR Tadej"
	GapSecs  int    // -1 when unknown
}

// ParseFile parses a saved results page (.html/.htm) or CSV (.csv).
func ParseFile(path string) ([]Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return parseCSV(data)
	case ".html", ".htm":
		return parseHTML(data)
	}
	return nil, fmt.Errorf("%s: unsupported file type (want .html, .htm or .csv)", path)
}

// parseCSV reads "position,rider[,gap]" rows. A header row is optional.
func parseCSV(data []byte) ([]Result, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.Comment = '#'
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []Result
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		pos, err := strconv.Atoi(strings.TrimSpace(row[0]))
		if err != nil || pos <= 0 {
			continue // header or DNF row
		}
		res := Result{Position: pos, Name: strings.TrimSpace(row[1]), GapSecs: -1}
		if len(row) > 2 {
			res.GapSecs = parseDuration(row[2])
		}
		out = append(out, res)
	}
	return out, nil
}

// parseHTML finds table rows whose first cell is a finishing position and
// which contain a rider link (FirstCycling "rider.php?r=", PCS "rider/slug").
func parseHTML(data []byte) ([]Result, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var out []Result
	seen := map[int]bool{}
	walk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode || n.Data != "tr" {
			return
		}
		cells := children(n, "td")
		if len(cells) < 2 {
			return
		}
		pos, err := strconv.Atoi(strings.TrimSpace(text(cells[0])))
		if err != nil || pos <= 0 || seen[pos] {
			return
		}
		name := ""
		for _, c := range cells {
			if name = riderName(c); name != "" {
				break
			}
		}
		if name == "" {
			return
		}
		gap := -1
		for i := len(cells) - 1; i > 0; i-- {
			if g := parseDuration(text(cells[i])); g >= 0 {
				gap = g
				break
			}
		}
		seen[pos] = true
		out = append(out, Result{Position: pos, Name: name, GapSecs: gap})
	})
	return out, nil
}

// riderName returns the rider's name from a rider link under n, or "".
func riderName(n *html.Node) string {
	var name string
	walk(n, func(a *html.Node) {
		if name != "" || a.Type != html.ElementNode || a.Data != "a" {
			return
		}
		for _, at := range a.Attr {
			if at.Key != "href" {
				continue
			}
			switch {
			case strings.Contains(at.Val, "rider.php"): // FirstCycling: link text is the name
				name = strings.Join(strings.Fields(text(a)), " ")
			case strings.Contains(at.Val, "rider/"): // PCS: slug like rider/tadej-pogacar
				slug := at.Val[strings.LastIndex(at.Val, "rider/")+len("rider/"):]
				slug = strings.SplitN(strings.SplitN(slug, "/", 2)[0], "?", 2)[0]
				if t := strings.Join(strings.Fields(text(a)), " "); t != "" {
					name = t
				} else {
					name = strings.ReplaceAll(slug, "-", " ")
				}
			}
		}
	})
	return name
}

var durationRe = regexp.MustCompile(`^\+?\s*(\d+:)?(\d+:)?\d+$`)

// parseDuration parses "h:mm:ss", "m:ss", "ss" or "+ 1:23" into seconds,
// returning -1 if the text is not a duration. "s.t." / ",," mean same time (0).
func parseDuration(s string) int {
	s = strings.TrimSpace(strings.ReplaceAll(s, " ", " "))
	if s == "s.t." || s == ",," || s == "st" {
		return 0
	}
	s = strings.TrimSpace(strings.TrimPrefix(s, "+"))
	if !durationRe.MatchString(s) {
		return -1
	}
	total := 0
	for _, p := range strings.Split(s, ":") {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return -1
		}
		total = total*60 + v
	}
	return total
}

// NameKey turns any rider name into an order- and accent-insensitive key, so
// "POGACAR Tadej", "Pogačar Tadej", "tadej pogacar" and "Tadej Pogačar" match.
func NameKey(name string) string {
	folded := norm.NFD.String(name)
	var b strings.Builder
	for _, r := range folded {
		switch {
		case unicode.Is(unicode.Mn, r): // drop combining accents
		case r == 'ø' || r == 'Ø':
			b.WriteRune('o')
		case r == 'đ' || r == 'Đ':
			b.WriteRune('d')
		case r == 'ł' || r == 'Ł':
			b.WriteRune('l')
		case r == 'æ' || r == 'Æ':
			b.WriteString("ae")
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(' ')
		}
	}
	tokens := strings.Fields(b.String())
	sort.Strings(tokens)
	return strings.Join(tokens, " ")
}

func walk(n *html.Node, fn func(*html.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}

func children(n *html.Node, tag string) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			out = append(out, c)
		}
	}
	return out
}

func text(n *html.Node) string {
	var b strings.Builder
	walk(n, func(t *html.Node) {
		if t.Type == html.TextNode {
			b.WriteString(t.Data)
			b.WriteByte(' ')
		}
	})
	return strings.TrimSpace(b.String())
}
