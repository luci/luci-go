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
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func init() {
	registry.RegisterCmpOption(cmp.FilterPath(
		func(p cmp.Path) bool {
			return p.Last().Type().Kind() == reflect.Func
		}, cmp.Transformer("func.pointer", func(f any) uintptr {
			return reflect.ValueOf(f).Pointer()
		}),
	))
}

func TestBatchHeap(t *testing.T) {
	ftt.Run(`batchHeap`, t, func(t *ftt.Test) {
		h := batchHeap[any]{}

		t.Run(`ordering`, func(t *ftt.Test) {
			checkSorted := func() {
				for i := 1; i < h.Len(); i++ {
					assert.Loosely(t, h.Less(i-1, i), should.BeTrue)
					assert.Loosely(t, h.Less(i, i-1), should.BeFalse)
				}
			}

			t.Run(`id`, func(t *ftt.Test) {
				h.data = append(h.data,
					&Batch[any]{id: 0},
					&Batch[any]{id: 1})
				checkSorted()
			})

			t.Run(`nextSend`, func(t *ftt.Test) {
				now := time.Now()
				h.data = append(h.data,
					&Batch[any]{nextSend: now},
					&Batch[any]{nextSend: now.Add(time.Second)})
				checkSorted()
			})

			t.Run(`(nextSend, id)`, func(t *ftt.Test) {
				now := time.Now().UTC()
				h.data = append(h.data,
					&Batch[any]{nextSend: now, id: 0},
					&Batch[any]{nextSend: now.Add(time.Second), id: 0},
					&Batch[any]{nextSend: now.Add(time.Second), id: 1},
					&Batch[any]{nextSend: now.Add(2 * time.Second), id: 0},
				)
				checkSorted()
			})

			t.Run(`test push-pop yields sorted`, func(t *ftt.Test) {
				now := time.Now().UTC()
				for i := uint64(0); i < 20; i++ {
					h.data = append(h.data, &Batch[any]{nextSend: now, id: i})
				}
				shuffled := batchHeap[any]{data: make([]*Batch[any], h.Len())}
				copy(shuffled.data, h.data)
				rand.Shuffle(shuffled.Len(), shuffled.Swap)
				assert.Loosely(t, shuffled, should.NotResemble(h))

				heap.Init(&shuffled)
				sorted := make([]*Batch[any], 0, shuffled.Len())
				for shuffled.Len() > 0 {
					sorted = append(sorted, heap.Pop(&shuffled).(*Batch[any]))
				}
				assert.Loosely(t, sorted, should.Resemble(h.data))
			})

			t.Run(`batch dropping`, func(t *ftt.Test) {
				h.PushBatch(&Batch[any]{id: 10})
				h.PushBatch(&Batch[any]{id: 20})

				t.Run(`drops an old batch`, func(t *ftt.Test) {
					oldest, idx := h.Oldest()
					assert.Loosely(t, oldest.id, should.Equal(uint64(10)))
					assert.Loosely(t, idx, should.BeZero)
					h.RemoveAt(idx)
					assert.Loosely(t, h.PopBatch(), should.Resemble(&Batch[any]{id: 20}))
					assert.Loosely(t, h.data, should.BeEmpty)
				})

			})

		})
	})

}
