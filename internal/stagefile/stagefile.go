// Package stagefile reads stage files (.cds, "CTStageData") from Pro Cycling
// Manager mods that the user has installed locally, and turns their route
// profile and cobbled/gravel sectors into course segments.
//
// The format is undocumented; what this package relies on was worked out from
// files and checked against the climb records each file carries, known climbs
// and sectors (Paris-Roubaix's five-star sectors):
//
//   - the stage name is the first length-prefixed string after the header;
//   - the profile is an int32 count followed by (distance, altitude) float32
//     pairs, every 20 units; distance is in 10 m units and altitude in 7.5 m
//     units (see AltitudeUnitM);
//   - sectors are records of int32 name length, UTF-8 name, int32 difficulty
//     (1-5 stars), float32 start and float32 end (profile units).
//
// Nothing is fetched or redistributed: the files stay where the user installed
// them and only derived segments are written, to a local file.
package stagefile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Point is one profile sample.
type Point struct {
	D, Alt float64 // profile units
}

// Sector is a cobbled or gravel sector.
type Sector struct {
	Name       string
	Stars      int // difficulty 1-5
	Start, End float64
}

// Stage is the parsed content of a stage file.
type Stage struct {
	Name    string
	Profile []Point
	Sectors []Sector
}

// Length is the stage length in profile units.
func (s *Stage) Length() float64 {
	if len(s.Profile) == 0 {
		return 0
	}
	return s.Profile[len(s.Profile)-1].D
}

// ParseFile parses a .cds stage file.
func ParseFile(path string) (*Stage, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	st, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return st, nil
}

// Profile units. Distance is in 10 m units and altitude in 7.5 m units: the
// climb records each file carries (start, end, length and height gain in
// metres) give 7.5 m per altitude unit in every file checked, and match
// published climbs (Koppenberg 0.5 km at 11.7%, Redoute 1.5 km at 9.4%).
const (
	DistanceUnitM = 10.0
	AltitudeUnitM = 7.5
)

var magic = []byte("CTStageData")

// Parse parses stage file content.
func Parse(b []byte) (*Stage, error) {
	at := indexOf(b, magic)
	if at < 0 {
		return nil, errors.New("not a stage file (no CTStageData header)")
	}
	st := &Stage{}
	// The name is the first "Start - Finish" string (falling back to the first
	// string) in the header.
	nameEnd := -1
	for i := at + len(magic); i < len(b)-4 && i < at+256; i++ {
		if s, end, ok := lengthPrefixed(b, i, 3, 120); ok {
			if nameEnd < 0 || strings.Contains(s, " - ") {
				st.Name, nameEnd = strings.TrimRight(s, "; "), end
			}
			if strings.Contains(s, " - ") {
				break
			}
		}
	}
	if nameEnd < 0 {
		return nil, errors.New("stage name not found")
	}
	off, n := findProfile(b, nameEnd)
	if off < 0 {
		return nil, errors.New("route profile not found")
	}
	st.Profile = make([]Point, n)
	for i := range st.Profile {
		st.Profile[i] = Point{D: f32(b, off+4+8*i), Alt: f32(b, off+8+8*i)}
	}
	st.Sectors = findSectors(b, off+4+8*n, st.Length())
	return st, nil
}

// findProfile returns the offset of the profile count and the count: the first
// int32 n followed by n (distance, altitude) pairs with distance starting at 0
// and rising steadily, all values finite.
func findProfile(b []byte, from int) (int, int) {
	for o := from; o < len(b)-12 && o < from+1024; o++ {
		n := int(int32(binary.LittleEndian.Uint32(b[o:])))
		if n < 20 || n > 200000 || o+4+8*n > len(b) {
			continue
		}
		if f32(b, o+4) != 0 {
			continue
		}
		ok := true
		prev := 0.0
		for i := 0; i < n && ok; i++ {
			d, a := f32(b, o+4+8*i), f32(b, o+8+8*i)
			ok = !math.IsNaN(d) && !math.IsNaN(a) && math.Abs(a) < 1e5 &&
				(i == 0 || (d > prev && d-prev <= 200))
			prev = d
		}
		if ok {
			return o, n
		}
	}
	return -1, 0
}

// findSectors scans for sector records after the profile.
func findSectors(b []byte, from int, length float64) []Sector {
	var out []Sector
	for i := from; i < len(b)-16; {
		name, end, ok := lengthPrefixed(b, i, 2, 80)
		if ok && end+12 <= len(b) {
			stars := int(int32(binary.LittleEndian.Uint32(b[end:])))
			s, e := f32(b, end+4), f32(b, end+8)
			if stars >= 1 && stars <= 5 && s >= 0 && e-s >= minSectorUnits && e <= length*1.01 && wordLike(name) {
				out = append(out, Sector{Name: name, Stars: stars, Start: s, End: e})
				i = end + 12
				continue
			}
		}
		i++
	}
	return out
}

// minSectorUnits: shorter "sectors" are noise (~50 m).
const minSectorUnits = 5

// wordLike reports whether a string reads like a place name: mostly letters
// and spaces, with at least three letters.
func wordLike(s string) bool {
	letters, other := 0, 0
	for _, r := range s {
		switch {
		case unicode.IsLetter(r):
			letters++
		case r == ' ' || r == '-' || r == '\'' || r == '(' || r == ')' || r == '.' || unicode.IsDigit(r):
		default:
			other++
		}
	}
	return letters >= 3 && other == 0
}

// lengthPrefixed reads an int32-length-prefixed printable string (UTF-8, or
// Windows-1252/Latin-1 as some files use) with at least one letter.
func lengthPrefixed(b []byte, i, minLen, maxLen int) (string, int, bool) {
	if i+4 > len(b) {
		return "", 0, false
	}
	n := int(int32(binary.LittleEndian.Uint32(b[i:])))
	if n < minLen || n > maxLen || i+4+n > len(b) {
		return "", 0, false
	}
	raw := b[i+4 : i+4+n]
	var s string
	if utf8.Valid(raw) {
		s = string(raw)
	} else {
		r := make([]rune, len(raw))
		for k, c := range raw {
			r[k] = rune(c) // Latin-1
		}
		s = string(r)
	}
	letter := false
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return "", 0, false
		}
		letter = letter || unicode.IsLetter(r)
	}
	return s, i + 4 + n, letter
}

func f32(b []byte, o int) float64 {
	return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[o:])))
}

func indexOf(b, sub []byte) int {
	for i := 0; i+len(sub) <= len(b); i++ {
		if string(b[i:i+len(sub)]) == string(sub) {
			return i
		}
	}
	return -1
}
