package main

import (
	"fmt"
	"math"
	"sort"

	"cycling_sim/internal/model"
	"cycling_sim/internal/simulation"
)

// Rider profile (Nov 2026): a rider's power-duration curve and his ratings,
// each against the field's distribution, for the rider panel.

// profileDurations are the curve's points (seconds).
var profileDurations = []float64{5, 10, 15, 30, 60, 120, 180, 300, 600, 1200, 1800, 3600, 7200, 4 * 3600, 6 * 3600}

// CurvePoint is the curve at one duration: the rider's power and the
// field's 10th / 50th / 90th percentiles, in W/kg and in watts, and the
// rider's percentile in the field (0-100).
type CurvePoint struct {
	Seconds float64 `json:"seconds"`
	WKg     float64 `json:"wkg"`
	Watts   float64 `json:"watts"`
	WKgP10  float64 `json:"wkgP10"`
	WKgP50  float64 `json:"wkgP50"`
	WKgP90  float64 `json:"wkgP90"`
	WP10    float64 `json:"wP10"`
	WP50    float64 `json:"wP50"`
	WP90    float64 `json:"wP90"`
	PctWKg  float64 `json:"pctWkg"`
	PctW    float64 `json:"pctW"`
}

// ProfileStat is one rating with the rider's percentile in the field.
type ProfileStat struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Pct   float64 `json:"pct"`
	About string  `json:"about"` // what it does in a race
	Unit  string  `json:"unit"`
}

// RiderProfile is everything the rider panel shows.
type RiderProfile struct {
	ID        int64         `json:"id"`
	Name      string        `json:"name"`
	Team      string        `json:"team"`
	Age       int           `json:"age"`
	Specialty string        `json:"specialty"`
	FieldSize int           `json:"fieldSize"`
	Field     string        `json:"field"` // who the percentiles compare with
	Curve     []CurvePoint  `json:"curve"`
	Physical  []ProfileStat `json:"physical"`
	Ratings   []ProfileStat `json:"ratings"`
}

type profileMetric struct {
	key, label, unit, about string
	get                     func(r *model.Rider) float64
}

var profilePhysical = []profileMetric{
	{"weight", "Weight", "kg", "Gravity on climbs; light riders are jostled more on cobbles.", func(r *model.Rider) float64 { return r.WeightKg }},
	{"height", "Height", "cm", "With weight, sets frontal area and so drag.", func(r *model.Rider) float64 { return r.HeightCm }},
	{"ftp", "Threshold", "W", "Power held for about an hour: the engine of every effort.", func(r *model.Rider) float64 { return float64(r.FTPWatts) }},
	{"wkg", "Threshold", "W/kg", "Threshold per kilo: decides long climbs.", func(r *model.Rider) float64 { return float64(r.FTPWatts) / r.WeightKg }},
	{"cda", "Drag area", "m²", "Air resistance (lower is faster): decides the flat and time trials.", func(r *model.Rider) float64 { return r.CdA }},
	{"wpkm", "Watts per drag", "W/m²", "Threshold over drag area: speed on the flat, alone.", func(r *model.Rider) float64 { return float64(r.FTPWatts) / r.CdA }},
	{"sprint", "Peak sprint", "W", "Peak power for the last 200 m (Sprint rating applied).", func(r *model.Rider) float64 { return simulation.SprintPower(r) }},
	{"wprime", "Anaerobic reserve", "kJ", "Work above threshold before he must ease (W'): body mass × punchiness. Pays for attacks, short climbs and surges.", func(r *model.Rider) float64 {
		return simulation.NewWPrimeState(r).WPrimeMax / 1000
	}},
}

var profileRatings = []profileMetric{
	{"climbing", "Climbing", "", "Power on climbs, ±0.2% per point around 70; none on the flat.", func(r *model.Rider) float64 { return float64(r.ClimbingAbility) }},
	{"tt", "Time trial", "", "Power and steadier pacing in time trials, ±0.2% per point.", func(r *model.Rider) float64 { return float64(r.TTAbility) }},
	{"sprint", "Sprint", "", "Sprint power in the finish, ±0.2% per point.", func(r *model.Rider) float64 { return float64(r.SprintAbility) }},
	{"punch", "Punch", "", "How punchy for his level: sets his anaerobic reserve per kilo and how far above threshold he can go (1.1-1.3×).", func(r *model.Rider) float64 { return float64(r.PunchRating()) }},
	{"repeat", "Repeatability", "", "How fast the reserve refills between efforts within a race.", func(r *model.Rider) float64 { return float64(r.RepeatabilityRating()) }},
	{"endurance", "Endurance", "", "How little he fades over a long, hard day (threshold and reserve).", func(r *model.Rider) float64 { return float64(r.Endurance) }},
	{"handling", "Cobbles", "", "Power on cobbles and gravel (+0.3% per point), fewer crashes on rough roads.", func(r *model.Rider) float64 { return float64(r.Handling) }},
	{"descending", "Descending", "", "How fast he dares to go downhill.", func(r *model.Rider) float64 { return float64(r.DescendingAbility) }},
	{"recovery", "Recovery", "", "Recovery between stages: less fatigue carried into the next day.", func(r *model.Rider) float64 { return float64(r.Recovery) }},
	{"consistency", "Consistency", "", "How little his form varies from day to day and race to race.", func(r *model.Rider) float64 { return float64(r.Consistency) }},
	{"tactical", "Tactical", "", "Appetite for breakaways and attacks.", func(r *model.Rider) float64 { return float64(r.TacticalAwareness) }},
}

// GetRiderProfile describes a rider against the field: the database's
// riders, or with seasonID > 0 that season's active riders (their own
// copies, aged and developed).
func (a *App) GetRiderProfile(riderID, seasonID int64) (RiderProfile, error) {
	var field []model.Rider
	label := "all riders"
	if seasonID > 0 {
		srs, err := a.db.ListSeasonRiders(seasonID)
		if err != nil {
			return RiderProfile{}, err
		}
		var self *model.Rider
		for i := range srs {
			if srs[i].ID == riderID {
				self = &srs[i].Rider
			}
			if !srs[i].Retired && srs[i].Team != "" {
				field = append(field, srs[i].Rider)
			}
		}
		if self != nil && !containsRider(field, riderID) {
			field = append(field, *self)
		}
		label = "this season's riders"
	} else {
		riders, err := a.db.ListRiders()
		if err != nil {
			return RiderProfile{}, err
		}
		field = riders
	}
	var rider *model.Rider
	for i := range field {
		if field[i].ID == riderID {
			rider = &field[i]
		}
	}
	if rider == nil {
		return RiderProfile{}, fmt.Errorf("rider %d not found", riderID)
	}
	out := RiderProfile{ID: rider.ID, Name: rider.Name, Team: rider.Team, Age: rider.Age,
		Specialty: simulation.SpecialtyName(rider), FieldSize: len(field), Field: label}

	// The curve, for everyone (cheap: a few hundred riders × 15 points).
	curves := make([][]float64, len(field))
	self := 0
	for i := range field {
		curves[i] = simulation.PowerCurve(&field[i], profileDurations)
		if field[i].ID == riderID {
			self = i
		}
	}
	for j, t := range profileDurations {
		watts := make([]float64, len(field))
		wkg := make([]float64, len(field))
		for i := range field {
			watts[i] = curves[i][j]
			wkg[i] = curves[i][j] / field[i].WeightKg
		}
		p := CurvePoint{Seconds: t, Watts: watts[self], WKg: wkg[self],
			PctW: percentileOf(watts, watts[self]), PctWKg: percentileOf(wkg, wkg[self])}
		sort.Float64s(watts)
		sort.Float64s(wkg)
		p.WP10, p.WP50, p.WP90 = quantile(watts, 0.1), quantile(watts, 0.5), quantile(watts, 0.9)
		p.WKgP10, p.WKgP50, p.WKgP90 = quantile(wkg, 0.1), quantile(wkg, 0.5), quantile(wkg, 0.9)
		out.Curve = append(out.Curve, p)
	}
	stats := func(ms []profileMetric) []ProfileStat {
		var res []ProfileStat
		for _, m := range ms {
			vals := make([]float64, len(field))
			for i := range field {
				vals[i] = m.get(&field[i])
			}
			v := m.get(rider)
			pct := percentileOf(vals, v)
			if m.key == "cda" {
				pct = 100 - pct // lower drag ranks higher
			}
			res = append(res, ProfileStat{Key: m.key, Label: m.label, Value: v, Pct: pct, About: m.about, Unit: m.unit})
		}
		return res
	}
	out.Physical = stats(profilePhysical)
	out.Ratings = stats(profileRatings)
	return out, nil
}

func containsRider(rs []model.Rider, id int64) bool {
	for _, r := range rs {
		if r.ID == id {
			return true
		}
	}
	return false
}

// percentileOf is the share of vals below v (ties count half), 0-100.
func percentileOf(vals []float64, v float64) float64 {
	below, equal := 0, 0
	for _, x := range vals {
		switch {
		case x < v:
			below++
		case x == v:
			equal++
		}
	}
	return 100 * (float64(below) + float64(equal)/2) / float64(len(vals))
}

// quantile of sorted values (linear interpolation).
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	x := q * float64(len(sorted)-1)
	i := int(math.Floor(x))
	if i+1 >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	return sorted[i] + (sorted[i+1]-sorted[i])*(x-float64(i))
}

// ProfileRiderRef names a rider the panel can compare with.
type ProfileRiderRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Team string `json:"team"`
}

// GetProfileRiders lists the riders a profile compares with (the same field
// as GetRiderProfile), by name.
func (a *App) GetProfileRiders(seasonID int64) ([]ProfileRiderRef, error) {
	out := []ProfileRiderRef{}
	if seasonID > 0 {
		srs, err := a.db.ListSeasonRiders(seasonID)
		if err != nil {
			return nil, err
		}
		for _, sr := range srs {
			if !sr.Retired && sr.Team != "" {
				out = append(out, ProfileRiderRef{sr.ID, sr.Name, sr.Team})
			}
		}
	} else {
		riders, err := a.db.ListRiders()
		if err != nil {
			return nil, err
		}
		for _, r := range riders {
			out = append(out, ProfileRiderRef{r.ID, r.Name, r.Team})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
