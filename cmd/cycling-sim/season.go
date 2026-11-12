package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/season"
)

const seasonUsage = `Usage:
  cycling-sim season new <team> [name]      start a season with a team
  cycling-sim season list                   list seasons
  cycling-sim season show <id>              calendar and standings
  cycling-sim season next <id> [--seed N]   simulate the next race
  cycling-sim season sim <id> [n] [--seed N]  simulate the next n races (default: the rest of the year)
  cycling-sim season results <id> <event>   an event's results
  cycling-sim season delete <id>            delete a season`

func cmdSeason(db *database.DB, args []string, seedVal int64) {
	if len(args) == 0 {
		fatalf("%s\n", seasonUsage)
	}
	get := func(i int) *model.Season {
		id := idArg(args, i, seasonUsage)
		s, err := db.GetSeason(id)
		if err != nil {
			fatalf("%v\n", err)
		}
		return s
	}
	switch args[0] {
	case "new":
		if len(args) < 2 {
			fatalf("%s\n", seasonUsage)
		}
		s, err := season.New(db, args[1], strings.Join(args[2:], " "))
		if err != nil {
			fatalf("%v\n", err)
		}
		fmt.Printf("Season %d: %s, %s %d\n", s.ID, s.Name, s.Team, s.Year)
	case "list":
		ss, _ := db.ListSeasons()
		for _, s := range ss {
			fmt.Printf("%3d  %-30s %-28s %d  %s\n", s.ID, s.Name, s.Team, s.Year, s.Date.Format("2 Jan"))
		}
	case "show":
		s := get(1)
		printSeason(db, s)
	case "next", "sim":
		s := get(1)
		n := 1
		if args[0] == "sim" {
			n = 1000
			if len(args) > 2 {
				if v, err := strconv.Atoi(args[2]); err == nil {
					n = v
				}
			}
		}
		names := riderNames(db, s)
		for i := 0; i < n; i++ {
			e, err := season.NextEvent(db, s)
			if err != nil {
				fatalf("%v\n", err)
			}
			if e == nil {
				fmt.Println("The year is over.")
				break
			}
			p, err := season.Prepare(db, s, e.ID, nil)
			if err != nil {
				fatalf("%v\n", err)
			}
			seed := seedVal
			if seed != 0 {
				seed += int64(i)
			}
			if err := season.Simulate(db, p, seed); err != nil {
				fatalf("%v\n", err)
			}
			fmt.Printf("%-6s %-28s %s\n", e.Start.Format("2 Jan"), e.Name, podium(db, s, e, names))
		}
	case "results":
		s := get(1)
		eid := idArg(args, 2, seasonUsage)
		e, err := season.Event(db, s, eid)
		if err != nil {
			fatalf("%v\n", err)
		}
		names := riderNames(db, s)
		rs, _ := db.ListSeasonResults(s.ID, s.Year, eid)
		for _, r := range rs {
			if r.Position > 10 {
				continue
			}
			label := r.Kind
			if r.Kind == model.ResultStage {
				label = fmt.Sprintf("stage %d", r.Stage)
			}
			fmt.Printf("%-9s %2d. %-26s %-28s +%4.0fs %4d pts\n", label, r.Position, names[r.RiderID], r.Team, r.GapS, r.Points)
		}
		_ = e
	case "delete":
		s := get(1)
		if err := db.DeleteSeason(s.ID); err != nil {
			fatalf("%v\n", err)
		}
		fmt.Println("Deleted.")
	default:
		fmt.Fprintln(os.Stderr, seasonUsage)
		os.Exit(1)
	}
}

func riderNames(db *database.DB, s *model.Season) map[int64]string {
	srs, _ := db.ListSeasonRiders(s.ID)
	m := make(map[int64]string, len(srs))
	for _, r := range srs {
		m[r.ID] = r.Name
	}
	return m
}

// podium: a one-day race's top three, or a stage race's GC top three.
func podium(db *database.DB, s *model.Season, e *model.SeasonEvent, names map[int64]string) string {
	rs, _ := db.ListSeasonResults(s.ID, s.Year, e.ID)
	var top []string
	for _, r := range rs {
		if (r.Kind == model.ResultRace || r.Kind == model.ResultGC) && r.Position <= 3 {
			mark := ""
			if r.Team == s.Team {
				mark = " *"
			}
			top = append(top, fmt.Sprintf("%d. %s%s", r.Position, names[r.RiderID], mark))
		}
	}
	return strings.Join(top, "  ")
}

func printSeason(db *database.DB, s *model.Season) {
	fmt.Printf("%s: %s, %d (now %s)\n\n", s.Name, s.Team, s.Year, s.Date.Format("2 Jan"))
	events, _ := db.ListSeasonEvents(s.ID, s.Year)
	names := riderNames(db, s)
	for _, e := range events {
		state := "   "
		if e.Done {
			state = "✓  "
		}
		line := ""
		if e.Done {
			line = podium(db, s, &e, names)
		}
		fmt.Printf("%s%3d %-6s %-28s %-9s %s\n", state, e.ID, e.Start.Format("2 Jan"), e.Name, e.Category, line)
	}
	riders, teams, _ := season.Standings(db, s, s.Year)
	fmt.Println("\nRiders")
	for i, r := range riders {
		if i >= 15 {
			break
		}
		fmt.Printf("%3d. %-26s %-28s %5d pts %2d wins\n", i+1, names[r.RiderID], r.Team, r.Points, r.Wins)
	}
	fmt.Println("\nTeams")
	for i, t := range teams {
		mark := ""
		if t.Team == s.Team {
			mark = "  <- you"
		}
		fmt.Printf("%3d. %-28s %5d pts %2d wins%s\n", i+1, t.Team, t.Points, t.Wins, mark)
	}
}
