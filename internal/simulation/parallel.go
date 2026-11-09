package simulation

import (
	"runtime"
	"sync"
)

// parallelRuns calls f(0..n-1) on all CPUs and returns the results in run
// order, so aggregates don't depend on scheduling. Each run must own its
// state (seeded RNG, copied segments); shared inputs are read only.
func parallelRuns[T any](n int, f func(run int) T) []T {
	out := make([]T, n)
	workers := runtime.GOMAXPROCS(0)
	if workers > n {
		workers = n
	}
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				out[i] = f(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		next <- i
	}
	close(next)
	wg.Wait()
	return out
}
