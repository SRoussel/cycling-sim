package simulation

import (
	"hash/fnv"
	"math"
	"sort"

	"cycling_sim/internal/model"
)

type stageProfile int

const (
	profileFlat stageProfile = iota
	profileHilly
	profileMountain
	profileTT
)

func classifyStageProfile(course *model.Course, raceType model.RaceType) stageProfile {
	if raceType == model.RaceTypeTT {
		return profileTT
	}
	if course == nil {
		return profileHilly
	}

	hasCobbles := false
	totalClimbM := 0.0
	for _, seg := range course.Segments {
		if seg.SurfaceType.Unpaved() {
			hasCobbles = true
		}
		if seg.GradientPct > 3 {
			totalClimbM += seg.DistanceKm * 1000.0 * seg.GradientPct / 100.0
		}
	}

	// Reuse the same thresholds the UI uses.
	if totalClimbM > 3000 {
		return profileMountain
	}
	if totalClimbM > 1000 {
		return profileHilly
	}
	if hasCobbles {
		return profileHilly
	}
	return profileFlat
}

func pointsScale(p stageProfile) []int {
	switch p {
	case profileFlat:
		// Make sprinters matter: flat stages are the backbone of the points jersey.
		return []int{80, 60, 50, 44, 40, 36, 32, 28, 24, 20, 18, 16, 14, 12, 10}
	case profileHilly:
		return []int{50, 40, 34, 30, 26, 22, 20, 18, 16, 14, 12, 10, 8, 6, 4}
	case profileMountain:
		// Keep some points on mountain stages, but not enough to let GC riders win the jersey by default.
		return []int{25, 20, 16, 14, 12, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	case profileTT:
		return []int{10, 8, 6, 5, 4, 3, 2, 1}
	default:
		return []int{30, 25, 22, 19, 17, 15, 13, 11, 9, 7}
	}
}

func deterministicNoise01(parts ...uint64) float64 {
	h := fnv.New64a()
	var b [8]byte
	for _, p := range parts {
		b[0] = byte(p)
		b[1] = byte(p >> 8)
		b[2] = byte(p >> 16)
		b[3] = byte(p >> 24)
		b[4] = byte(p >> 32)
		b[5] = byte(p >> 40)
		b[6] = byte(p >> 48)
		b[7] = byte(p >> 56)
		_, _ = h.Write(b[:])
	}
	// 0..1
	return float64(h.Sum64()%1000000) / 1000000.0
}

func deterministicNoiseSigned(parts ...uint64) float64 {
	// ~[-1, 1]
	return (deterministicNoise01(parts...) - 0.5) * 2.0
}

type climbCategory int

const (
	climbNone climbCategory = iota
	climbCat3
	climbCat2
	climbCat1
	climbHC
)

func categorizeClimb(seg model.Segment) climbCategory {
	if seg.GradientPct <= 4.0 {
		return climbNone
	}
	if seg.DistanceKm < 0.8 {
		return climbNone
	}
	// Approx elevation gain (meters)
	gainM := seg.DistanceKm * 1000.0 * seg.GradientPct / 100.0
	score := gainM
	// Bonus for steepness.
	score *= 1.0 + math.Max(0, (seg.GradientPct-6.0))*0.05
	if score >= 900 {
		return climbHC
	}
	if score >= 650 {
		return climbCat1
	}
	if score >= 350 {
		return climbCat2
	}
	if score >= 180 {
		return climbCat3
	}
	return climbNone
}

func komPointsForCategory(cat climbCategory) []int {
	switch cat {
	case climbHC:
		return []int{15, 12, 10, 8, 6, 4, 3, 2, 1}
	case climbCat1:
		return []int{10, 8, 6, 4, 2, 1}
	case climbCat2:
		return []int{6, 4, 2, 1}
	case climbCat3:
		return []int{2, 1}
	default:
		return nil
	}
}

func intermediateSprintScale(p stageProfile) []int {
	// Smaller than stage finish, but meaningful for sprinters.
	switch p {
	case profileFlat:
		return []int{30, 25, 22, 19, 17, 15, 13, 11, 9, 7, 6, 5, 4, 3, 2}
	case profileHilly:
		return []int{20, 17, 15, 13, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	default:
		return nil
	}
}

// DefaultAgeFromName assigns a deterministic pseudo-age when no real age data exists.
// Range is 22–36; youth eligibility is <= 25.
func DefaultAgeFromName(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return 22 + int(h.Sum32()%15)
}

// rankEntry holds a rider's position in a points-based ranking.
type rankEntry struct {
	riderID int64
	pts     int
	gcTime  float64
}

// buildPointsRanked returns active riders sorted by points (desc), breaking ties by GC time (asc).
func buildPointsRanked(pointsMap map[int64]int, gcTimes map[int64]float64, active map[int64]bool) []rankEntry {
	entries := make([]rankEntry, 0, len(pointsMap))
	for id, pts := range pointsMap {
		if !active[id] {
			continue
		}
		entries = append(entries, rankEntry{riderID: id, pts: pts, gcTime: gcTimes[id]})
	}
	sort.Slice(entries, func(a, b int) bool {
		if entries[a].pts != entries[b].pts {
			return entries[a].pts > entries[b].pts
		}
		return entries[a].gcTime < entries[b].gcTime
	})
	return entries
}

// ComputeStageRaceCompetitions builds cumulative standings after each stage.
// This is intentionally simple: points/KOM are based on stage finish positions.
func ComputeStageRaceCompetitions(stageRaceID int64, stages []StageConfig, riders []model.Rider, allResults [][]model.RaceResult) ([][]model.PointsStanding, [][]model.MountainsStanding, [][]model.YouthStanding) {
	riderByID := make(map[int64]model.Rider, len(riders))
	for _, r := range riders {
		if r.Age == 0 {
			r.Age = DefaultAgeFromName(r.Name)
		}
		riderByID[r.ID] = r
	}

	pointsByRider := make(map[int64]int)
	komByRider := make(map[int64]int)
	gcTimeByRider := make(map[int64]float64)
	active := make(map[int64]bool)
	for _, r := range riders {
		gcTimeByRider[r.ID] = 0
		active[r.ID] = true
	}

	pointsStandingsByStage := make([][]model.PointsStanding, 0, len(stages))
	komStandingsByStage := make([][]model.MountainsStanding, 0, len(stages))
	youthStandingsByStage := make([][]model.YouthStanding, 0, len(stages))

	for i, stage := range stages {
		if i >= len(allResults) {
			break
		}
		results := allResults[i]
		profile := classifyStageProfile(stage.Course, stage.Race.RaceType)
		pScale := pointsScale(profile)

		// Update GC times and points for finishers.
		posIdx := 0
		finishers := make([]int64, 0, len(results))
		for _, res := range results {
			if res.Position == 0 {
				active[res.RiderID] = false
				delete(gcTimeByRider, res.RiderID)
				continue
			}
			if !active[res.RiderID] {
				continue
			}
			finishers = append(finishers, res.RiderID)
			gcTimeByRider[res.RiderID] += res.FinishTimeSeconds

			if posIdx < len(pScale) {
				pointsByRider[res.RiderID] += pScale[posIdx]
			}
			posIdx++

		}

		// Intermediate sprint points (flat/hilly only), awarded to best sprinters among finishers.
		if profile == profileFlat || profile == profileHilly {
			sScale := intermediateSprintScale(profile)
			if len(sScale) > 0 && len(finishers) > 0 {
				type entry struct {
					riderID int64
					score   float64
				}
				entries := make([]entry, 0, len(finishers))
				for _, id := range finishers {
					r := riderByID[id]
					noise := deterministicNoiseSigned(uint64(stageRaceID), uint64(stage.StageNumber), uint64(id), 1)
					s := float64(r.SprintAbility) + noise*3.0
					entries = append(entries, entry{riderID: id, score: s})
				}
				sort.Slice(entries, func(a, b int) bool { return entries[a].score > entries[b].score })
				limit := len(sScale)
				if len(entries) < limit {
					limit = len(entries)
				}
				for idx := 0; idx < limit; idx++ {
					pointsByRider[entries[idx].riderID] += sScale[idx]
				}
			}
		}

		// KOM points for each categorized climb in the stage (proxy for multiple climbs).
		if stage.Course != nil && len(finishers) > 0 {
			for _, seg := range stage.Course.Segments {
				cat := categorizeClimb(seg)
				kPts := komPointsForCategory(cat)
				if len(kPts) == 0 {
					continue
				}
				type entry struct {
					riderID int64
					score   float64
				}
				entries := make([]entry, 0, len(finishers))
				for _, id := range finishers {
					r := riderByID[id]
					noise := deterministicNoiseSigned(uint64(stageRaceID), uint64(stage.StageNumber), uint64(seg.ID), uint64(id), 2)
					s := float64(r.ClimbingAbility) + noise*2.0
					entries = append(entries, entry{riderID: id, score: s})
				}
				sort.Slice(entries, func(a, b int) bool { return entries[a].score > entries[b].score })
				limit := len(kPts)
				if len(entries) < limit {
					limit = len(entries)
				}
				for idx := 0; idx < limit; idx++ {
					komByRider[entries[idx].riderID] += kPts[idx]
				}
			}
		}

		afterStage := stage.StageNumber

		// Build points standings.
		{
			ranked := buildPointsRanked(pointsByRider, gcTimeByRider, active)
			standings := make([]model.PointsStanding, len(ranked))
			for idx, e := range ranked {
				standings[idx] = model.PointsStanding{
					StageRaceID: stageRaceID,
					RiderID:     e.riderID,
					AfterStage:  afterStage,
					Points:      e.pts,
					Position:    idx + 1,
				}
			}
			pointsStandingsByStage = append(pointsStandingsByStage, standings)
		}

		// Build KOM standings.
		{
			ranked := buildPointsRanked(komByRider, gcTimeByRider, active)
			standings := make([]model.MountainsStanding, len(ranked))
			for idx, e := range ranked {
				standings[idx] = model.MountainsStanding{
					StageRaceID: stageRaceID,
					RiderID:     e.riderID,
					AfterStage:  afterStage,
					Points:      e.pts,
					Position:    idx + 1,
				}
			}
			komStandingsByStage = append(komStandingsByStage, standings)
		}

		// Build youth standings (U25) based on GC time.
		{
			type entry struct {
				riderID int64
				time    float64
			}
			entries := make([]entry, 0)
			for id, t := range gcTimeByRider {
				r := riderByID[id]
				if !active[id] {
					continue
				}
				if r.Age <= 25 {
					entries = append(entries, entry{riderID: id, time: t})
				}
			}
			sort.Slice(entries, func(a, b int) bool { return entries[a].time < entries[b].time })
			standings := make([]model.YouthStanding, len(entries))
			for idx, e := range entries {
				standings[idx] = model.YouthStanding{
					StageRaceID:      stageRaceID,
					RiderID:          e.riderID,
					AfterStage:       afterStage,
					TotalTimeSeconds: e.time,
					Position:         idx + 1,
				}
			}
			youthStandingsByStage = append(youthStandingsByStage, standings)
		}
	}

	return pointsStandingsByStage, komStandingsByStage, youthStandingsByStage
}
