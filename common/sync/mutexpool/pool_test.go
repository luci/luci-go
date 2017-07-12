// Copyright 2017 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
