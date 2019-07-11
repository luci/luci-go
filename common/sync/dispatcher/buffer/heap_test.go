// Copyright 2019 The LUCI Authors.
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

package buffer

import (
	"math/rand"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	//. "go.chromium.org/luci/common/testing/assertions"
)

func TestBatchHeap(t *testing.T) {
	Convey(`batchHeap`, t, func() {
		h := batchHeap{}

		Convey(`usage`, func() {

			Convey(`sorts batches by ID`, func() {
				// pseudo-randomly enumerate set of 1..10.
				for i := uint64(0); i < 10; i++ {
					h.PushBatch(&Batch{
						id: 1 + uint64(i*17%10),
					})
				}

				ids := []uint64{}
				for len(h) > 0 {
					ids = append(ids, h.PopBatch().id)
				}
				So(ids, ShouldResemble, []uint64{
					1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
				})
			})

			Convey(`sorts batches by time, then ID`, func() {
				now := time.Now()

				for i := 0; i < 10; i++ {
					id := rand.Uint64()

					// Each batch is pushed in with a DECREASING time; we expect the heap
					// to reorder these so they will be popped with INCREASING time.
					nextSend := now.Add(-time.Duration(i+1) * time.Second)

					h.PushBatch(&Batch{nextSend: nextSend, id: id})
					h.PushBatch(&Batch{nextSend: nextSend, id: id + 1})
				}

				prevBatch := h.PopBatch()
				for len(h) > 0 {
					batch := h.PopBatch()
					if batch.nextSend == prevBatch.nextSend {
						So(prevBatch.id, ShouldEqual, batch.id-1)
					} else {
						So(prevBatch.nextSend, ShouldHappenBefore, batch.nextSend)
					}
					prevBatch = batch
				}
			})

		})

		Convey(`batch dropping`, func() {
			h.PushBatch(&Batch{id: 10})
			h.PushBatch(&Batch{id: 20})

			Convey(`will not drop batch if target is older than oldest batch`, func() {
				So(h.PopOldestBatchOlderThan(5), ShouldBeNil)
				So(h.PopBatch(), ShouldResemble, &Batch{id: 10})
				So(h.PopBatch(), ShouldResemble, &Batch{id: 20})
				So(h, ShouldBeEmpty)
			})

			Convey(`drops an old batch`, func() {
				So(h.PopOldestBatchOlderThan(15), ShouldResemble, &Batch{id: 10})
				So(h.PopBatch(), ShouldResemble, &Batch{id: 20})
				So(h, ShouldBeEmpty)
			})
		})

	})
}
