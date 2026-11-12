package main

import (
	"fmt"
	"os"
	"sort"
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
  cycling-sim season board <id>             objectives, board confidence, budget
  cycling-sim season roster <id>            your riders
  cycling-sim season market <id>            riders you can sign (contracts ending, free agents)
  cycling-sim season offer <id> <rider> <salary-k> <years>   offer a contract
  cycling-sim season end-year <id>          close the year, open the transfer window
  cycling-sim season start-year <id>        close the window, start the next year
  cycling-sim season years <id> <n> [--seed N]  simulate n whole years (races, year end, window)
  cycling-sim season news <id>              latest news
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
	case "board":
		s := get(1)
		printBoard(db, s)
	case "roster":
		s := get(1)
		printRoster(db, s)
	case "market":
		s := get(1)
		printMarket(db, s)
	case "offer":
		s := get(1)
		rid := idArg(args, 2, seasonUsage)
		salary := idArg(args, 3, seasonUsage)
		years := idArg(args, 4, seasonUsage)
		ok, reply, err := season.Offer(db, s, rid, int(salary), int(years))
		if err != nil {
			fatalf("%v\n", err)
		}
		fmt.Println(reply)
		if !ok {
			os.Exit(2)
		}
	case "end-year":
		s := get(1)
		news, err := season.EndYear(db, s)
		if err != nil {
			fatalf("%v\n", err)
		}
		printNews(news)
	case "start-year":
		s := get(1)
		news, err := season.StartYear(db, s)
		if err != nil {
			fatalf("%v\n", err)
		}
		printNews(news)
	case "years":
		s := get(1)
		n := int(idArg(args, 2, seasonUsage))
		for y := 0; y < n; y++ {
			if s.Phase == model.PhaseOffseason {
				if _, err := season.StartYear(db, s); err != nil {
					fatalf("%v\n", err)
				}
			}
			for i := 0; ; i++ {
				e, err := season.NextEvent(db, s)
				if err != nil {
					fatalf("%v\n", err)
				}
				if e == nil {
					break
				}
				p, err := season.Prepare(db, s, e.ID, nil)
				if err != nil {
					fatalf("%v\n", err)
				}
				seed := seedVal
				if seed != 0 {
					seed += int64(y*100 + i)
				}
				if err := season.Simulate(db, p, seed); err != nil {
					fatalf("%v\n", err)
				}
			}
			year := s.Year
			if _, err := season.AutoRenew(db, s, 26); err != nil {
				fatalf("%v\n", err)
			}
			printStandingsTop(db, s, year)
			printBoard(db, s)
			news, err := season.EndYear(db, s)
			if err != nil {
				fatalf("%v\n", err)
			}
			for _, item := range news {
				if item.Mine {
					fmt.Println("  " + item.Text)
				}
			}
		}
	case "news":
		s := get(1)
		items, _ := db.ListNews(s.ID, 40)
		printNews(items)
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

func printNews(items []model.NewsItem) {
	for _, n := range items {
		mark := "  "
		if n.Mine {
			mark = "* "
		}
		fmt.Printf("%s%s  %s\n", mark, n.Date.Format("2 Jan 2006"), n.Text)
	}
}

func printStandingsTop(db *database.DB, s *model.Season, year int) {
	names := riderNames(db, s)
	riders, teams, _ := season.Standings(db, s, year)
	fmt.Printf("== %d ==\n", year)
	for i := 0; i < 5 && i < len(riders); i++ {
		fmt.Printf("  %d. %-26s %-28s %5d pts %2d wins\n", i+1, names[riders[i].RiderID], riders[i].Team, riders[i].Points, riders[i].Wins)
	}
	for i, t := range teams {
		if t.Team == s.Team {
			fmt.Printf("  %s: %d. in the team ranking, %d pts, %d wins\n", s.Team, i+1, t.Points, t.Wins)
		}
	}
}

func printBoard(db *database.DB, s *model.Season) {
	ps, err := season.ObjectivesProgress(db, s)
	if err != nil {
		fatalf("%v\n", err)
	}
	srs, _ := db.ListSeasonRiders(s.ID)
	next, n := season.NextWageBill(srs, s.Team, s.Year)
	fmt.Printf("%s %d (%s): budget %s, wages %s; next year %s committed for %d riders\n", s.Team, s.Year, s.Phase,
		season.Money(s.Budget), season.Money(season.WageBill(srs, s.Team)), season.Money(next), n)
	for _, p := range ps {
		state := "   "
		switch {
		case p.Met:
			state = "✓  "
		case p.Decided:
			state = "✗  "
		}
		fmt.Printf("  %s%-60s now %d\n", state, p.Text, p.Current)
	}
	fmt.Printf("  Board confidence %.0f%%\n", season.BoardConfidence(ps))
}

func printRoster(db *database.DB, s *model.Season) {
	srs, _ := db.ListSeasonRiders(s.ID)
	riders, _, _ := season.Standings(db, s, s.Year)
	pts := map[int64]int{}
	for _, r := range riders {
		pts[r.RiderID] = r.Points
	}
	fmt.Printf("%-5s %-26s %3s %5s %5s %8s %4s %6s %5s %4s %5s\n", "id", "rider", "age", "W", "W/kg", "salary", "to", "morale", "load", "days", "pts")
	for _, sr := range srs {
		if sr.Team != s.Team || sr.Retired {
			continue
		}
		fmt.Printf("%-5d %-26s %3d %5d %5.2f %8s %4d %6.0f %5.1f %4d %5d\n", sr.ID, sr.Name, sr.Age, sr.FTPWatts,
			float64(sr.FTPWatts)/sr.WeightKg, season.Money(sr.Salary), sr.ContractEnd, sr.Morale, sr.Load, sr.RaceDays, pts[sr.ID])
	}
}

func printMarket(db *database.DB, s *model.Season) {
	srs, _ := db.ListSeasonRiders(s.ID)
	var active []model.Rider
	for _, sr := range srs {
		if !sr.Retired && sr.Team != "" {
			active = append(active, sr.Rider)
		}
	}
	values := season.Values(db, active)
	type row struct {
		sr  model.SeasonRider
		v   float64
		ask int
	}
	var rows []row
	for _, sr := range srs {
		if sr.Retired || sr.NextTeam != "" || sr.Team == s.Team {
			continue
		}
		free := sr.Team == "" && s.Phase == model.PhaseOffseason
		if !free && !(sr.Team != "" && sr.ContractEnd <= s.Year && s.Phase == model.PhaseRacing) {
			continue
		}
		v, ok := values[sr.ID]
		if !ok {
			v = season.Values(db, append(append([]model.Rider{}, active...), sr.Rider))[sr.ID]
		}
		rows = append(rows, row{sr, v, season.AskingSalary(sr, v, sr.Team != "")})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].v > rows[j].v })
	for i, r := range rows {
		if i >= 30 {
			break
		}
		team := r.sr.Team
		if team == "" {
			team = "free agent"
		}
		fmt.Printf("%-5d %-26s %3d %-28s %5.2f W/kg  value %5.2f  asks %s\n", r.sr.ID, r.sr.Name, r.sr.Age, team,
			float64(r.sr.FTPWatts)/r.sr.WeightKg, r.v, season.Money(r.ask))
	}
}
