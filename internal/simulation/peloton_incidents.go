package simulation

import (
	"math/rand"

	"cycling_sim/internal/model"
)

// ApplyIncidents randomly applies crashes/punctures to riders in a segment.
// Crashes are more likely on technical terrain, punctures are rare throughout.
// Probability is very low to not overwhelm race results with random bad luck.
func ApplyIncidents(riders []*RiderState, segment *model.Segment, rng *rand.Rand) {
	// Base incident probability per segment: low overall, higher on technical terrain
	baseProbability := 0.003
	if segment.IsTechnical {
		baseProbability = 0.008
	}
	if segment.SurfaceType.Unpaved() {
		baseProbability = 0.015
	}
	if segment.GradientPct < -3.0 {
		baseProbability = 0.006
	}

	for _, rs := range riders {
		if rs.DNF {
			continue // already out of race
		}

		probability := baseProbability
		if segment.SurfaceType.Unpaved() || segment.IsTechnical || segment.GradientPct < -3.0 {
			handlingMod := 1.0 + float64(70-rs.Rider.Handling)*0.01
			if handlingMod < 0.6 {
				handlingMod = 0.6
			}
			if handlingMod > 1.6 {
				handlingMod = 1.6
			}
			probability *= handlingMod
		}

		if rng.Float64() > ScaleToDistance(probability, segment.DistanceKm) {
			continue // no incident for this rider
		}

		// Determine incident type: 70% puncture, 30% crash
		if rng.Float64() < 0.65 {
			// Puncture: 30-60 second penalty
			penalty := 30.0 + rng.Float64()*30.0
			rs.TotalPenalty += penalty
			rs.TotalTime += penalty
		} else {
			// Crash: chance it ends race (DNF) depends on handling and terrain
			dnfChance := 0.08
			if segment.SurfaceType.Unpaved() || segment.IsTechnical || segment.GradientPct < -3.0 {
				dnfChance = 0.15
			}
			if segment.SurfaceType.Unpaved() {
				dnfChance = 0.18
			}
			if rs.Rider.Handling < 65 {
				dnfChance += float64(65-rs.Rider.Handling) * 0.006
			}
			if dnfChance > 0.2 {
				dnfChance = 0.2
			}
			if rng.Float64() < dnfChance {
				rs.DNF = true // severe crash, rider out
			} else {
				// Major crash penalty: 2-5 minutes + potential injury fatigue
				penalty := 120.0 + rng.Float64()*180.0
				rs.TotalPenalty += penalty
				rs.TotalTime += penalty
				// Reduce FTP temporarily from injuries
				rs.EffectiveFTP = int(float64(rs.EffectiveFTP) * 0.90)
			}
		}
	}
}
