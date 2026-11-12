package selection

import (
	"sort"

	"cycling_sim/internal/model"
)

// SelectionConfig controls how many teams and riders per team to select.
type SelectionConfig struct {
	NumTeams      int
	RidersPerTeam int
}

type teamScore struct {
	team teamGroup
	avg  float64
}

// DefaultConfig returns sensible defaults based on race type.
func DefaultConfig(raceType model.RaceType, isStageRace bool, numStages int) SelectionConfig {
	if raceType == model.RaceTypeTT && !isStageRace {
		return SelectionConfig{NumTeams: 20, RidersPerTeam: 2}
	}
	if isStageRace && numStages >= 15 {
		return SelectionConfig{NumTeams: 22, RidersPerTeam: 8}
	}
	return SelectionConfig{NumTeams: 22, RidersPerTeam: 7}
}

// RiderScore computes a weighted score for a rider against a course profile.
// Higher is better. Range is roughly 1-100.
func RiderScore(rider *model.Rider, profile CourseProfile) float64 {
	score := float64(rider.ClimbingAbility)*profile.ClimbingDemand +
		float64(rider.SprintAbility)*profile.SprintDemand +
		float64(rider.TTAbility)*profile.TTDemand +
		float64(rider.DescendingAbility)*profile.TechnicalDemand +
		float64(rider.Handling)*profile.CobblesDemand +
		float64(rider.Endurance)*profile.EnduranceDemand

	// Normalize by total demand weight so score stays in ~1-100 range
	totalWeight := profile.ClimbingDemand + profile.SprintDemand +
		profile.TTDemand + profile.TechnicalDemand + profile.CobblesDemand +
		profile.EnduranceDemand
	if totalWeight > 0 {
		score /= totalWeight
	}
	return score
}

// SelectStartList picks a realistic start list for a single race.
func SelectStartList(allRiders []model.Rider, course *model.Course, race *model.Race, config SelectionConfig) []model.Rider {
	profile := AnalyzeCourse(course, race.RaceType)

	// Group riders by team
	teams := groupByTeam(allRiders)

	// Exclude teams with fewer than 3 riders
	var eligibleTeams []teamGroup
	for _, t := range teams {
		if len(t.riders) >= 3 {
			eligibleTeams = append(eligibleTeams, t)
		}
	}

	// Score each team by average rider score
	scored := scoreTeams(eligibleTeams, profile)

	// Sort teams by average score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].avg > scored[j].avg
	})

	// Take top NumTeams teams
	numTeams := config.NumTeams
	if numTeams > len(scored) {
		numTeams = len(scored)
	}

	// From each team, take top RidersPerTeam by score
	var selected []model.Rider
	for _, ts := range scored[:numTeams] {
		riders := ts.team.riders

		// For sprint-profile races, guarantee the best sprinter on each team
		// is included even if their composite score ranks outside the top N.
		guaranteedID := int64(-1)
		var guaranteedRider model.Rider
		if profile.SprintDemand >= 0.5 && config.RidersPerTeam > 1 {
			bestSprint := -1
			for i := range riders {
				if riders[i].SprintAbility > bestSprint {
					bestSprint = riders[i].SprintAbility
					guaranteedID = riders[i].ID
					guaranteedRider = riders[i]
				}
			}
		}

		// Score and sort riders within team
		sort.Slice(riders, func(i, j int) bool {
			return RiderScore(&riders[i], profile) > RiderScore(&riders[j], profile)
		})
		count := config.RidersPerTeam
		if count > len(riders) {
			count = len(riders)
		}
		teamSelected := riders[:count]

		// If the guaranteed sprinter is not already in the selection, swap out last
		if guaranteedID >= 0 {
			found := false
			for _, r := range teamSelected {
				if r.ID == guaranteedID {
					found = true
					break
				}
			}
			if !found {
				teamSelected = append(riders[:count-1:count-1], guaranteedRider)
			}
		}

		selected = append(selected, teamSelected...)
	}

	return selected
}

// StageInfo holds a course and race type for stage race selection analysis.
type StageInfo struct {
	Course   *model.Course
	RaceType model.RaceType
}

// SelectStageRaceStartList picks a start list for a multi-stage race,
// ensuring team diversity (sprinters, TT specialists, climbers, domestiques).
func SelectStageRaceStartList(allRiders []model.Rider, stages []StageInfo, config SelectionConfig) []model.Rider {
	// Analyze all stages to determine race character
	profiles := make([]CourseProfile, len(stages))
	flatStages := 0
	mountainStages := 0
	ttStages := 0
	for i, s := range stages {
		profiles[i] = AnalyzeCourse(s.Course, s.RaceType)
		if s.RaceType == model.RaceTypeTT {
			ttStages++
		} else if profiles[i].ClimbingDemand > 0.3 {
			mountainStages++
		} else {
			flatStages++
		}
	}

	// Build an overall profile for team ranking
	overallProfile := averageProfile(profiles)

	// Group riders by team, exclude small teams
	teams := groupByTeam(allRiders)
	var eligibleTeams []teamGroup
	for _, t := range teams {
		if len(t.riders) >= 3 {
			eligibleTeams = append(eligibleTeams, t)
		}
	}

	// Rank teams by average overall score
	scored := scoreTeams(eligibleTeams, overallProfile)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].avg > scored[j].avg
	})

	numTeams := config.NumTeams
	if numTeams > len(scored) {
		numTeams = len(scored)
	}

	// Select riders within each team using role-based allocation
	rpt := config.RidersPerTeam
	var selected []model.Rider
	for _, ts := range scored[:numTeams] {
		riders := ts.team.riders
		if len(riders) <= rpt {
			selected = append(selected, riders...)
			continue
		}
		picked := selectByRole(riders, rpt, flatStages, mountainStages, ttStages, overallProfile)
		selected = append(selected, picked...)
	}

	return selected
}

// selectByRole picks riders from a single team to fill role-based slots.
func selectByRole(riders []model.Rider, slots int, flatStages, mountainStages, ttStages int, overallProfile CourseProfile) []model.Rider {
	totalStages := flatStages + mountainStages + ttStages
	if totalStages == 0 {
		totalStages = 1
	}

	// Reserve at most 1 sprinter slot if there are flat stages
	sprintSlots := 0
	if flatStages > 0 {
		sprintSlots = 1
	}

	// Reserve 1 TT slot if there are TT stages
	ttSlots := 0
	if ttStages > 0 {
		ttSlots = 1
	}

	// Reserve 1 domestique (endurance) slot
	domestiqueSlots := 1

	reserved := sprintSlots + ttSlots + domestiqueSlots
	if reserved > slots-1 {
		// Always keep at least 1 slot for best overall/GC rider
		reserved = slots - 1
		// Reduce in priority: domestique first, then TT, then sprint
		if reserved < sprintSlots+ttSlots+domestiqueSlots {
			domestiqueSlots = 0
		}
		if reserved < sprintSlots+ttSlots {
			ttSlots = 0
		}
	}
	gcSlots := slots - sprintSlots - ttSlots - domestiqueSlots

	used := make(map[int64]bool)
	var picked []model.Rider

	// Pick best sprinter
	if sprintSlots > 0 {
		best := bestBy(riders, used, func(r *model.Rider) float64 {
			return float64(r.SprintAbility)
		})
		if best != nil {
			picked = append(picked, *best)
			used[best.ID] = true
		}
	}

	// Pick best TT specialist
	if ttSlots > 0 {
		best := bestBy(riders, used, func(r *model.Rider) float64 {
			return float64(r.TTAbility)
		})
		if best != nil {
			picked = append(picked, *best)
			used[best.ID] = true
		}
	}

	// Pick best domestique (endurance)
	if domestiqueSlots > 0 {
		best := bestBy(riders, used, func(r *model.Rider) float64 {
			return float64(r.Endurance)
		})
		if best != nil {
			picked = append(picked, *best)
			used[best.ID] = true
		}
	}

	// Fill remaining with best overall/climbing riders
	type riderScore struct {
		rider model.Rider
		score float64
	}
	var remaining []riderScore
	for i := range riders {
		if !used[riders[i].ID] {
			remaining = append(remaining, riderScore{
				rider: riders[i],
				score: RiderScore(&riders[i], overallProfile),
			})
		}
	}
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].score > remaining[j].score
	})

	for i := 0; i < gcSlots && i < len(remaining); i++ {
		picked = append(picked, remaining[i].rider)
	}

	return picked
}

// bestBy returns the rider with highest scorer among unused riders.
func bestBy(riders []model.Rider, used map[int64]bool, scorer func(*model.Rider) float64) *model.Rider {
	var best *model.Rider
	bestScore := -1.0
	for i := range riders {
		if used[riders[i].ID] {
			continue
		}
		s := scorer(&riders[i])
		if s > bestScore {
			bestScore = s
			best = &riders[i]
		}
	}
	return best
}

// teamGroup holds riders belonging to one team.
type teamGroup struct {
	name   string
	riders []model.Rider
}

func groupByTeam(riders []model.Rider) []teamGroup {
	m := make(map[string][]model.Rider)
	for _, r := range riders {
		m[r.Team] = append(m[r.Team], r)
	}
	var groups []teamGroup
	for name, rs := range m {
		groups = append(groups, teamGroup{name: name, riders: rs})
	}
	// Sort by team name for deterministic ordering
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].name < groups[j].name
	})
	return groups
}

func scoreTeams(teams []teamGroup, profile CourseProfile) []teamScore {
	var scored []teamScore
	for _, t := range teams {
		total := 0.0
		for i := range t.riders {
			total += RiderScore(&t.riders[i], profile)
		}
		scored = append(scored, teamScore{team: t, avg: total / float64(len(t.riders))})
	}
	return scored
}

// AverageProfile is the mean demand of several courses (a stage race's).
func AverageProfile(profiles []CourseProfile) CourseProfile { return averageProfile(profiles) }

func averageProfile(profiles []CourseProfile) CourseProfile {
	if len(profiles) == 0 {
		return CourseProfile{}
	}
	var avg CourseProfile
	for _, p := range profiles {
		avg.ClimbingDemand += p.ClimbingDemand
		avg.SprintDemand += p.SprintDemand
		avg.TTDemand += p.TTDemand
		avg.TechnicalDemand += p.TechnicalDemand
		avg.EnduranceDemand += p.EnduranceDemand
	}
	n := float64(len(profiles))
	avg.ClimbingDemand /= n
	avg.SprintDemand /= n
	avg.TTDemand /= n
	avg.TechnicalDemand /= n
	avg.EnduranceDemand /= n
	return avg
}
