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
				for i := uint64(0); i < 10; i++ {
					h.PushBatch(&Batch{
						id: uint64(10 - i),
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
				expectedIDs := make([]uint64, 20)

				for i := 0; i < 10; i++ {
					id := rand.Uint64()

					// Each batch is pushed in with a DECREASING time; we expect the heap
					// to reorder these so they will be popped with INCREASING time.
					nextSend := now.Add(-time.Duration(i+1) * time.Second)

					// We push in two batches with the same timestamp, differing only by
					// ID.
					expectedIDs[19-(i*2)] = id + 1
					expectedIDs[19-(i*2)-1] = id

					h.PushBatch(&Batch{nextSend: nextSend, id: id})
					h.PushBatch(&Batch{nextSend: nextSend, id: id + 1})
				}

				ids := []uint64{}
				for i := 0; len(h) > 0; i++ {
					batch := h.PopBatch()
					ids = append(ids, batch.id)
				}
				So(ids, ShouldResemble, expectedIDs)
			})

		})

		Convey(`batch dropping`, func() {
			h.PushBatch(&Batch{id: 10})
			h.PushBatch(&Batch{id: 20})

			Convey(`will not drop batch if target is older than oldest batch`, func() {
				So(h.DropOneBatchOlderThan(5), ShouldBeNil)
				So(h.PopBatch(), ShouldResemble, &Batch{id: 10})
				So(h.PopBatch(), ShouldResemble, &Batch{id: 20})
				So(h, ShouldBeEmpty)
			})

			Convey(`drops an old batch`, func() {
				So(h.DropOneBatchOlderThan(15), ShouldResemble, &Batch{id: 10})
				So(h.PopBatch(), ShouldResemble, &Batch{id: 20})
				So(h, ShouldBeEmpty)
			})
		})

	})
}
