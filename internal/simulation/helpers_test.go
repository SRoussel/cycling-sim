package simulation

import (
	"fmt"
	"math/rand"

	"cycling_sim/internal/model"
)

func newTestRng() *rand.Rand {
	return rand.New(rand.NewSource(12345))
}

// spreadTeams assigns riders to n teams round-robin by ID, so each team gets
// its own mix of riders (and its own sprinter/leader) as in a real field.
func spreadTeams(riders []model.Rider, n int) {
	for i := range riders {
		riders[i].Team = fmt.Sprintf("Team %d", riders[i].ID%int64(n))
	}
}
