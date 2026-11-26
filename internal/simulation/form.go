package simulation

import (
	"math"
	"time"

	"cycling_sim/internal/model"
)

// Seasonal form. Ratings describe a rider at their best; riders reach it
// only around the races they target and are a few percent below it
// elsewhere (a Grand Tour climber is not in July shape at Liège in April).
// Form is a race option (RaceConfig.FormDate): off for one-off races, where
// everyone starts at their best, on for real editions in validation and for
// a season.

// offPeakLoss is the threshold lost far from any target (pros' power varies
// by a few percent over a season).
const offPeakLoss = 0.04

// peakWidthDays: form falls off around a target like a Gaussian with this
// width (about 2 weeks either side are close to peak).
const peakWidthDays = 28.0

// Targets, as the day of the year a peak is centred on.
const (
	peakSprintSpring = 87  // late March: Milan-San Remo, the flat spring classics
	peakCobbles      = 95  // early April: Flanders, Roubaix
	peakArdennes     = 111 // late April: Amstel, Flèche, Liège
	peakGiro         = 140 // late May
	peakTour         = 196 // mid July
	peakVuelta       = 248 // early September
	peakAutumn       = 283 // October: Lombardia
)

// Specialties that decide a rider's targets, read from their profile.
type specialty int

const (
	specAllRounder specialty = iota
	specClimber              // Grand Tour climbers
	specPuncheur             // hilly classics
	specCobbles              // cobbled classics
	specSprinter
)

// specialtyOf classifies a rider by physical profile: climbing W/kg, weight,
// handling and sprint.
func specialtyOf(r *model.Rider) specialty {
	w := climbingWKg(r)
	switch {
	case w >= 6.6:
		return specClimber
	case r.WeightKg >= 70 && r.Handling >= 85:
		return specCobbles
	case r.SprintAbility >= 85 && w < 6.0:
		return specSprinter
	case w >= 6.0:
		return specPuncheur
	}
	return specAllRounder
}

// peaksOf returns the days of the year a rider peaks on. Grand Tour
// climbers peak broadly over the summer (both the Giro and the Tour: an
// off-form climber wouldn't start either) and again in the autumn; classics
// riders in their spring block and at the Tour. All-rounders, mostly
// domestiques, have no form swing: a team selects them for a race in form
// (giving them one random block left three-quarters of a Grand Tour's
// domestiques off form, so sprint teams lost control of the race).
func peaksOf(r *model.Rider) []float64 {
	switch specialtyOf(r) {
	case specClimber:
		return []float64{peakGiro, peakTour, peakVuelta, peakAutumn}
	case specPuncheur:
		return []float64{peakArdennes, peakTour}
	case specCobbles:
		return []float64{peakSprintSpring, peakCobbles, peakTour}
	case specSprinter:
		return []float64{peakSprintSpring, peakTour}
	}
	return nil // all-rounders, mostly domestiques: in form wherever they're selected
}

// SeasonForm is a rider's threshold multiplier on a date: 1 at a target,
// down to 1-offPeakLoss far from all of them. A zero date means no form.
func SeasonForm(r *model.Rider, date time.Time) float64 {
	if date.IsZero() {
		return 1
	}
	peaks := peaksOf(r)
	if chosen := r.Peaks(); len(chosen) > 0 { // targets the player chose
		peaks = chosen
	}
	if len(peaks) == 0 {
		return 1
	}
	day := float64(date.YearDay())
	best := 0.0
	for _, p := range peaks {
		d := math.Abs(day - p)
		d = math.Min(d, 365-d) // the season wraps
		best = math.Max(best, math.Exp(-(d/peakWidthDays)*(d/peakWidthDays)))
	}
	return 1 - offPeakLoss*(1-best)
}

// applySeasonForm scales each rider's form for the race date (no-op when
// form is off).
func applySeasonForm(states []*RiderState, date time.Time) {
	if date.IsZero() {
		return
	}
	for _, rs := range states {
		rs.FormFactor *= SeasonForm(rs.Rider, date)
	}
}

// SpecialtyName labels a rider's specialty as the season model reads it
// (which races he peaks for): "Climber", "Puncheur", "Cobbles", "Sprinter"
// or "All-rounder".
func SpecialtyName(r *model.Rider) string {
	switch specialtyOf(r) {
	case specClimber:
		return "Climber"
	case specPuncheur:
		return "Puncheur"
	case specCobbles:
		return "Cobbles"
	case specSprinter:
		return "Sprinter"
	}
	return "All-rounder"
}
