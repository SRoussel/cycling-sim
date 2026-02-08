package model

// RaceResult stores the finish result for a rider in a race.
type RaceResult struct {
	ID                int64
	RaceID            int64
	RiderID           int64
	Position          int
	FinishTimeSeconds float64
	TimeGapSeconds    float64
}

// GCStanding stores the general classification standing after a stage.
type GCStanding struct {
	ID               int64
	StageRaceID      int64
	RiderID          int64
	AfterStage       int
	TotalTimeSeconds float64
	GCPosition       int
}
