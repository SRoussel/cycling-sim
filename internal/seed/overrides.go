package seed

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"cycling_sim/internal/model"
)

// RiderOverride holds optional replacements for a rider's attributes.
// A nil field means "keep the base value from riders.csv".
type RiderOverride struct {
	Name   string
	Fields map[string]string
	Source string
}

// overrideColumns maps CSV header names to setters on model.Rider.
var overrideColumns = map[string]func(r *model.Rider, v string) error{
	"Weight_kg":    floatSetter(func(r *model.Rider, f float64) { r.WeightKg = f }),
	"FTP_watts":    intSetter(func(r *model.Rider, i int) { r.FTPWatts = i }),
	"Sprint_watts": intSetter(func(r *model.Rider, i int) { r.MaxSprintWatts = i }),
	"VO2max_watts": intSetter(func(r *model.Rider, i int) { r.VO2MaxWatts = i }),
	"CdA":          floatSetter(func(r *model.Rider, f float64) { r.CdA = f }),
	"Climbing":     intSetter(func(r *model.Rider, i int) { r.ClimbingAbility = i }),
	"Sprint":       intSetter(func(r *model.Rider, i int) { r.SprintAbility = i }),
	"TT":           intSetter(func(r *model.Rider, i int) { r.TTAbility = i }),
	"Descending":   intSetter(func(r *model.Rider, i int) { r.DescendingAbility = i }),
	"Endurance":    intSetter(func(r *model.Rider, i int) { r.Endurance = i }),
	"Recovery":     intSetter(func(r *model.Rider, i int) { r.Recovery = i }),
	"Consistency":  intSetter(func(r *model.Rider, i int) { r.Consistency = i }),
	"Tactical":     intSetter(func(r *model.Rider, i int) { r.TacticalAwareness = i }),
	"Handling":     intSetter(func(r *model.Rider, i int) { r.Handling = i }),
	// FTP_scale multiplies the threshold set by earlier layers (used by the
	// fitted layer, which stores corrections rather than absolute values).
	"FTP_scale": floatSetter(func(r *model.Rider, f float64) { r.FTPWatts = int(math.Round(float64(r.FTPWatts) * f)) }),
}

// OverrideColumns returns the supported override column names in a stable order.
func OverrideColumns() []string {
	return []string{"Weight_kg", "FTP_watts", "Sprint_watts", "VO2max_watts", "CdA",
		"Climbing", "Sprint", "TT", "Descending", "Endurance", "Recovery", "Consistency", "Tactical", "Handling"}
}

func intSetter(set func(*model.Rider, int)) func(*model.Rider, string) error {
	return func(r *model.Rider, v string) error {
		i, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		set(r, i)
		return nil
	}
}

func floatSetter(set func(*model.Rider, float64)) func(*model.Rider, string) error {
	return func(r *model.Rider, v string) error {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return err
		}
		set(r, f)
		return nil
	}
}

// LoadOverrides reads an overrides CSV. Lines starting with '#' are comments.
// Required column: Name. Optional: any of OverrideColumns() and Source.
// A missing file is not an error.
func LoadOverrides(path string) (map[string]RiderOverride, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	header := rows[0]
	nameCol := -1
	for i, h := range header {
		header[i] = strings.TrimSpace(h)
		if header[i] == "Name" {
			nameCol = i
		} else if header[i] != "Source" && overrideColumns[header[i]] == nil {
			return nil, fmt.Errorf("%s: unknown column %q", path, header[i])
		}
	}
	if nameCol < 0 {
		return nil, fmt.Errorf("%s: missing Name column", path)
	}
	out := make(map[string]RiderOverride)
	for _, row := range rows[1:] {
		if nameCol >= len(row) || strings.TrimSpace(row[nameCol]) == "" {
			continue
		}
		o := RiderOverride{Name: strings.TrimSpace(row[nameCol]), Fields: map[string]string{}}
		for i, v := range row {
			v = strings.TrimSpace(v)
			if i == nameCol || v == "" || i >= len(header) {
				continue
			}
			if header[i] == "Source" {
				o.Source = v
				continue
			}
			o.Fields[header[i]] = v
		}
		out[normalizeName(o.Name)] = o
	}
	return out, nil
}

// Apply replaces the rider's fields with the override values.
func (o RiderOverride) Apply(r *model.Rider) error {
	for col, v := range o.Fields {
		if err := overrideColumns[col](r, v); err != nil {
			return fmt.Errorf("override %s.%s=%q: %w", o.Name, col, v, err)
		}
	}
	return nil
}

func normalizeName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
