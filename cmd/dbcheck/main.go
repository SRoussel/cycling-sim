package main

import (
	"cycling_sim/internal/database"
	"fmt"
	"sort"
)

func main() {
	dbPath := database.DefaultPath()
	db, err := database.Open(dbPath)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Check Alpecin, Lidl-Trek rider counts and individual scores
	allRiders, _ := db.ListRiders()
	type riderScore struct {
		id    int64
		name  string
		score float64
	}
	for _, teamName := range []string{"Alpecin-Premier Tech", "Lidl-Trek"} {
		var members []riderScore
		for _, r := range allRiders {
			if r.Team == teamName {
				members = append(members, riderScore{r.ID, r.Name,
					float64(r.SprintAbility)*0.8 + float64(r.ClimbingAbility)*0.06 + float64(r.TTAbility)*0.1 + float64(r.Endurance)*0.73})
			}
		}
		sort.Slice(members, func(i, j int) bool { return members[i].score > members[j].score })
		fmt.Printf("\n%s (%d riders):\n", teamName, len(members))
		for i, m := range members {
			selected := ""
			if i < 7 {
				selected = " *SELECTED*"
			}
			fmt.Printf("  ID=%d %-30s score=%.1f%s\n", m.id, m.name, m.score, selected)
		}
	}
	for _, info := range []struct {
		id   int64
		name string
	}{{4, "LBL"}, {5, "Lombardia"}} {
		segs, err := db.GetSegments(info.id)
		if err != nil {
			fmt.Printf("%s: error %v\n", info.name, err)
			continue
		}
		fmt.Printf("\n=== %s (CourseID=%d, Segs=%d) ===\n", info.name, info.id, len(segs))
		for i, s := range segs {
			surface := "road"
			if s.SurfaceType.Unpaved() {
				surface = "cobble"
			}
			fmt.Printf("  [%d] %-35s  dist=%.1fkm  grad=%.1f%%  surf=%s\n",
				i+1, s.Name, s.DistanceKm, s.GradientPct, surface)
		}
	}
}
