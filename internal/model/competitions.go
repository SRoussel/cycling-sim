package model

// PointsStanding represents cumulative points classification standings after a stage.
type PointsStanding struct {
	ID          int64
	StageRaceID int64
	RiderID     int64
	AfterStage  int
	Points      int
	Position    int
}

// MountainsStanding represents cumulative KOM (mountains) points standings after a stage.
type MountainsStanding struct {
	ID          int64
	StageRaceID int64
	RiderID     int64
	AfterStage  int
	Points      int
	Position    int
}

// YouthStanding represents the best young rider (U25) classification after a stage.
type YouthStanding struct {
	ID               int64
	StageRaceID      int64
	RiderID          int64
	AfterStage       int
	TotalTimeSeconds float64
	Position         int
}
