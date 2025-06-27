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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPool(t *testing.T) {
	t.Parallel()

	ftt.Run(`A mutex pool`, t, func(t *ftt.Test) {
		var pool P

		t.Run(`Can handle multiple concurrent Mutex accesses, and will clean up when finished.`, func(t *ftt.Test) {
			const total = 1000
			var value int
			var wg sync.WaitGroup

			wg.Add(total)
			for range total {
				go func() {
					pool.WithMutex("foo", func() {
						value++
					})
					wg.Done()
				}()
			}

			wg.Wait()
			assert.Loosely(t, value, should.Equal(total))
			assert.Loosely(t, pool.mutexes, should.HaveLength(0))
		})

		t.Run(`Can handle multiple keys.`, func(t *ftt.Test) {
			const total = 100
			var recLock func(int)
			recLock = func(v int) {
				if v < total {
					pool.WithMutex(v, func() {
						assert.Loosely(t, pool.mutexes, should.HaveLength(v+1))
						recLock(v + 1)
					})
				}
			}
			recLock(0)
			assert.Loosely(t, pool.mutexes, should.HaveLength(0))
		})
	})
}
