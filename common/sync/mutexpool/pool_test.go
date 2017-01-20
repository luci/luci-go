// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package mutexpool

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPool(t *testing.T) {
	t.Parallel()

	Convey(`A mutex pool`, t, func() {
		var pool P

		Convey(`Can handle multiple concurrent Mutex accesses, and will clean up when finished.`, func() {
			const total = 1000
			var value int
			var wg sync.WaitGroup

			wg.Add(total)
			for i := 0; i < total; i++ {
				go func() {
					pool.WithMutex("foo", func() {
						value++
					})
					wg.Done()
				}()
			}

			wg.Wait()
			So(value, ShouldEqual, total)
			So(pool.mutexes, ShouldHaveLength, 0)
		})

		Convey(`Can handle multiple keys.`, func() {
			const total = 100
			var recLock func(int)
			recLock = func(v int) {
				if v < total {
					pool.WithMutex(v, func() {
						So(pool.mutexes, ShouldHaveLength, v+1)
						recLock(v + 1)
					})
				}
			}
			recLock(0)
			So(pool.mutexes, ShouldHaveLength, 0)
		})
	})
}
