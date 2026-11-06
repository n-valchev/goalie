package files

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// Parallel calls work once for each index from 0 to count. Each call must write only its own result.
func Parallel(count int, work func(index int)) {
	var next atomic.Int64
	var workers sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), count) {
		workers.Go(func() {
			for {
				index := int(next.Add(1)) - 1
				if index >= count {
					return
				}
				work(index)
			}
		})
	}
	workers.Wait()
}
