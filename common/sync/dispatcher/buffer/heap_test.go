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

			Convey(`id`, func() {
				h = append(h, &Batch{id: 0}, &Batch{id: 1})
				checkSorted()
			})

			Convey(`nextSend`, func() {
				now := time.Now()
				h = append(h, &Batch{nextSend: now}, &Batch{nextSend: now.Add(time.Second)})
				checkSorted()
			})

			Convey(`(nextSend, id)`, func() {
				now := time.Now().UTC()
				h = append(h,
					&Batch{nextSend: now, id: 0},
					&Batch{nextSend: now.Add(time.Second), id: 0},
					&Batch{nextSend: now.Add(time.Second), id: 1},
					&Batch{nextSend: now.Add(2 * time.Second), id: 0},
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

			Convey(`batch dropping`, func() {
				h.PushBatch(&Batch{id: 10})
				h.PushBatch(&Batch{id: 20})

				Convey(`drops an old batch`, func() {
					oldest, idx := h.Oldest()
					So(oldest.id, ShouldEqual, 10)
					So(idx, ShouldEqual, 0)
					h.RemoveAt(idx)
					So(h.PopBatch(), ShouldResemble, &Batch{id: 20})
					So(h, ShouldBeEmpty)
				})

			})

		})
	})

}
