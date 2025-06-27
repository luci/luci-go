// Copyright 2015 The LUCI Authors.
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

package txnBuf

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
)

type Counter struct {
	ID int64 `gae:"$id"`

	Value int64
}

func TestRace(t *testing.T) {
	t.Parallel()

	c := FilterRDS(memory.Use(context.Background()))

	wg := sync.WaitGroup{}
	for i := range 100 {
		id := int64(i + 1)

		wg.Add(1)
		go func() {
			defer wg.Done()
			err := ds.RunInTransaction(c, func(c context.Context) error {
				for range 100 {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						ctr := &Counter{ID: id}
						if err := ds.Get(c, ctr); err != nil && err != ds.ErrNoSuchEntity {
							panic(fmt.Sprintf("bad Get: %s", err))
						}
						ctr.Value++
						return ds.Put(c, ctr)
					}, nil)
					if err != nil {
						panic(fmt.Sprintf("bad inner RIT: %s", err))
					}
				}

				return nil
			}, nil)
			if err != nil {
				panic(fmt.Sprintf("bad outer RIT: %s", err))
			}
		}()
	}
	wg.Wait()
}
