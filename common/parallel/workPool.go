// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package parallel

import (
	"sync"
)

// WorkPool creates a fixed-size pool of worker goroutines. A supplied generator
// method creates task functions and passes them through to the work pool.
// Available workers will consume tasks from the pool and execute them until the
// generator is finished.
//
// WorkPool blocks until all the generaetor completes and all workers have
// finished their tasks.
func WorkPool(workers int, gen func(chan<- func())) {
	if workers <= 0 {
		return
	}

	// Spawn worker goroutines.
	wg := sync.WaitGroup{}
	wg.Add(workers)
	workC := make(chan func(), workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()

			for work := range workC {
				work()
			}
		}()
	}

	gen(workC)
	close(workC)
	wg.Wait()
}
