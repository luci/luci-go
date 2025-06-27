// Copyright 2018 The LUCI Authors.
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

package dsmapperlite

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

type intEnt struct {
	ID int64 `gae:"$id"`
}

func TestMap(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())

	const count = 1000
	for i := range count {
		if err := datastore.Put(ctx, &intEnt{ID: int64(i + 1)}); err != nil {
			t.Fatalf("Storing entity: %s", err)
		}
	}
	datastore.GetTestable(ctx).CatchupIndexes()

	t.Run("visits all", func(t *testing.T) {
		m := sync.Mutex{}
		seen := map[int64]int{}

		err := Map(ctx, datastore.NewQuery("intEnt"), 10, 256, func(ctx context.Context, _ int, e *intEnt) error {
			m.Lock()
			seen[e.ID] += 1
			m.Unlock()
			return nil
		})
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}

		if len(seen) != count {
			t.Fatalf("Expected to visit %d entities, but visited %d", count, len(seen))
		}
		for key, visits := range seen {
			if visits != 1 {
				t.Fatalf("Entity %d was visited more than once: %d", key, visits)
			}
		}
	})

	t.Run("aborts on errors", func(t *testing.T) {
		var visits atomic.Int64
		err := Map(ctx, datastore.NewQuery("intEnt"), 10, 256, func(ctx context.Context, _ int, e *intEnt) error {
			if visits.Add(1) == 100 {
				return errors.New("boom")
			}
			return nil
		})
		if err == nil {
			t.Fatalf("Unexpectedly succeeded")
		}
	})
}
