package simulation

import "math/rand"

func newTestRng() *rand.Rand {
	return rand.New(rand.NewSource(12345))
}
