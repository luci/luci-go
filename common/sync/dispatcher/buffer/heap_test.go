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
	"container/heap"
	"math/rand"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBatchHeap(t *testing.T) {
	Convey(`batchHeap`, t, func() {
		h := batchHeap{}

		Convey(`ordering`, func() {
			checkSorted := func() {
				for i := 1; i < len(h); i++ {
					So(h.Less(i-1, i), ShouldBeTrue)
					So(h.Less(i, i-1), ShouldBeFalse)
				}
			}

			Convey(`-currentlyLeased`, func() {
				h = append(h, &Batch{currentlyLeased: false}, &Batch{currentlyLeased: true})
				checkSorted()
			})

			Convey(`id`, func() {
				h = append(h, &Batch{id: 0}, &Batch{id: 1})
				checkSorted()
			})

			Convey(`nextSend`, func() {
				now := time.Now()
				h = append(h, &Batch{nextSend: now}, &Batch{nextSend: now.Add(time.Second)})
				checkSorted()
			})

			Convey(`(-currentlyLeased, id)`, func() {
				h = append(h,
					&Batch{currentlyLeased: false, id: 1},
					&Batch{currentlyLeased: true, id: 0},
				)
				checkSorted()
			})

			Convey(`(nextSend, id)`, func() {
				now := time.Now().UTC()
				h = append(h,
					&Batch{nextSend: now, id: 0},
					&Batch{nextSend: now.Add(time.Second), id: 1},
				)
				checkSorted()
			})

			Convey(`(-currentlyLeased, nextSend, id)`, func() {
				now := time.Now().UTC()
				h = append(h,
					&Batch{currentlyLeased: false, nextSend: now, id: 0},
					&Batch{currentlyLeased: false, nextSend: now.Add(time.Second), id: 0},
					&Batch{currentlyLeased: false, nextSend: now.Add(time.Second), id: 1},
					&Batch{currentlyLeased: false, nextSend: now.Add(2 * time.Second), id: 0},
					&Batch{currentlyLeased: true, nextSend: now, id: 0},
					&Batch{currentlyLeased: true, nextSend: now.Add(time.Second), id: 0},
					&Batch{currentlyLeased: true, nextSend: now.Add(time.Second), id: 1},
					&Batch{currentlyLeased: true, nextSend: now.Add(2 * time.Second), id: 0},
				)
				checkSorted()

				Convey(`test push-pop yields sorted`, func() {
					shuffled := make(batchHeap, len(h))
					copy(shuffled, h)
					rand.Shuffle(len(shuffled), shuffled.Swap)
					So(shuffled, ShouldNotResemble, h)

					heap.Init(&shuffled)
					sorted := make(batchHeap, 0, len(shuffled))
					for len(shuffled) > 0 {
						sorted = append(sorted, heap.Pop(&shuffled).(*Batch))
					}
					So(sorted, ShouldResemble, h)
				})
			})

			Convey(`batch leasing`, func() {
				h.PushBatch(&Batch{id: 10})
				h.PushBatch(&Batch{id: 20})

				Convey(`leasing a batch moves it to the end`, func() {
					So(h[0].id, ShouldEqual, 10)
					b10 := h.LeaseBatch()
					So(b10.id, ShouldEqual, 10)

					So(h[0].id, ShouldEqual, 20)
					b20 := h.LeaseBatch()
					So(b20.id, ShouldEqual, 20)

					So(func() { h.LeaseBatch() }, ShouldPanicLike, "no unleased batches")

					Convey(`can unlease batches in old, new`, func() {
						So(h.TryUnleaseBatch(b20), ShouldBeTrue)
						So(h[0].id, ShouldEqual, 20)

						So(h.TryUnleaseBatch(b10), ShouldBeTrue)
						So(h[0].id, ShouldEqual, 10)
					})

					Convey(`can unlease batches in new, old`, func() {
						So(h.TryUnleaseBatch(b10), ShouldBeTrue)
						So(h[0].id, ShouldEqual, 10)

						So(h.TryUnleaseBatch(b20), ShouldBeTrue)
						So(h[0].id, ShouldEqual, 10)
					})

					Convey(`cannot unlease bogus Batch`, func() {
						So(func() {
							h.TryUnleaseBatch(&Batch{currentlyLeased: true})
						}, ShouldPanicLike, "does not appear in heap")
					})

					Convey(`dropping batch OK`, func() {
						So(h.TryDropBatch(b20), ShouldBeTrue)
						So(h.TryUnleaseBatch(b10), ShouldBeTrue)
						So(h, ShouldResemble, batchHeap{&Batch{id: 10}})
					})
				})
			})

			Convey(`batch dropping`, func() {
				h.PushBatch(&Batch{id: 10})
				h.PushBatch(&Batch{id: 20})

				Convey(`will not drop batch if target is older than oldest batch`, func() {
					So(h.PopOldestBatchOlderThan(5), ShouldBeNil)
					So(heap.Pop(&h).(*Batch), ShouldResemble, &Batch{id: 10})
					So(heap.Pop(&h).(*Batch), ShouldResemble, &Batch{id: 20})
					So(h, ShouldBeEmpty)
				})

				Convey(`drops an old batch`, func() {
					So(h.PopOldestBatchOlderThan(15), ShouldResemble, &Batch{id: 10})
					So(heap.Pop(&h).(*Batch), ShouldResemble, &Batch{id: 20})
					So(h, ShouldBeEmpty)
				})

			})

		})
	})

}
