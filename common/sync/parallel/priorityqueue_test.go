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

package parallel

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPriorityQueue(t *testing.T) {
	// blocks all workers, so there is no capacity to do more work.
	// This prevents new work from starting.
	blockAllWorkers := func(workers int, work chan<- PriorityWork) func() {
		release := make(chan struct{})
		var starting sync.WaitGroup
		starting.Add(workers)
		for i := 0; i < workers; i++ {
			work <- PriorityWork{
				F: func() error {
					starting.Done()
					<-release
					return nil
				},
			}
		}

		starting.Wait()
		// all workers are blocked. There is no capacity to schedule more work.
		return func() { close(release) }
	}

	Convey(`PriorityQueue schedules in the priority order`, t, func() {
		var order []int
		var orderLock sync.Mutex
		err := PriorityQueue(1, func(work chan<- PriorityWork) bool {
			startScheduling := blockAllWorkers(1, work)

			// add work in the reverse order
			for i := 0; i < 5; i++ {
				i := i
				work <- PriorityWork{
					Priority: i,
					F: func() error {
						orderLock.Lock()
						order = append(order, i)
						orderLock.Unlock()
						return nil
					},
				}
			}

			startScheduling()
			return true
		})
		So(err, ShouldBeNil)
		So(order, ShouldResemble, []int{4, 3, 2, 1, 0})
	})

	Convey(`PriorityQueue stops scheduling new work when gen returns false`, t, func() {
		highPriorityExecuted := int32(0)
		const workers = 10
		err := PriorityQueue(workers, func(work chan<- PriorityWork) bool {
			startScheduling := blockAllWorkers(workers, work)

			var highPriorityStarting sync.WaitGroup
			releaseHighPriorityWorkers := make(chan struct{})
			for i := 0; i < workers; i++ {
				highPriorityStarting.Add(1)
				work <- PriorityWork{
					Priority: 100, // high priority work
					F: func() error {
						atomic.AddInt32(&highPriorityExecuted, 1)
						highPriorityStarting.Done()
						<-releaseHighPriorityWorkers
						time.Sleep(time.Nanosecond) // see hack below
						return nil
					},
				}
				work <- PriorityWork{
					Priority: 0, // low priority work
					F: func() error {
						// this work should not start
						return errors.New("low priority work started")
					},
				}
			}

			startScheduling()

			// Wait till high priority group starts.
			highPriorityStarting.Wait()

			// HACK: we need to stop scheduling (by returning false), and THEN
			// release high priority workers. Strictly speaking, doing it
			// without a race is not possible with this API, so we are cheating
			// here.
			go close(releaseHighPriorityWorkers)
			return false

		})
		So(err, ShouldBeNil)
		So(highPriorityExecuted, ShouldEqual, workers)
	})
}
