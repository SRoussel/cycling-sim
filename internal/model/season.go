package model

import "time"

// Season is one save game: the player's team and a year of racing. A season
// keeps its own copy of every rider (SeasonRider), so ageing, transfers and
// form never touch the reference riders table.
type Season struct {
	ID        int64
	Name      string
	Team      string // the player's team
	Year      int    // the year being raced
	StartYear int
	Date      time.Time // the season's current date (after the last event)
}

// SeasonRider is a rider as he stands in a season: his attributes (which
// change with age), his team and contract, and how worn he is.
type SeasonRider struct {
	Rider
	ContractEnd int     // last year of his contract
	Salary      int     // thousand euro per year
	Morale      float64 // 0-100, 60 neutral
	Load        float64 // recent racing load in race days, decaying (see season.LoadDecayDays)
	RaceDays    int     // race days this year
	Retired     bool
}

// Event kinds and categories of a season's calendar.
const (
	EventOneDay    = "oneday"
	EventStageRace = "stagerace"

	CategoryMonument  = "monument"
	CategoryClassic   = "classic"
	CategoryGrandTour = "grandtour"
	CategoryWeekLong  = "weeklong"
)

// SeasonEvent is a race on a season's calendar: a one-day race (RefID: race
// ID) or a stage race (RefID: stage race ID).
type SeasonEvent struct {
	ID       int64
	SeasonID int64
	Year     int
	Kind     string
	RefID    int64
	Name     string
	Category string
	Start    time.Time
	End      time.Time
	Done     bool
}

// Days is the number of race days of the event.
func (e SeasonEvent) Days() int {
	return int(e.End.Sub(e.Start).Hours()/24) + 1
}

// Season result kinds.
const (
	ResultRace  = "race"  // a one-day race's result
	ResultStage = "stage" // a stage of a stage race
	ResultGC    = "gc"    // a stage race's final GC
)

// SeasonResult is one rider's place in an event (a one-day race, one stage
// or a final GC) and the points it earned.
type SeasonResult struct {
	SeasonID int64
	EventID  int64
	Year     int
	RiderID  int64
	Team     string
	Kind     string
	Stage    int
	Position int
	GapS     float64
	Points   int
}
