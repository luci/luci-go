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
	"sync"
	"testing"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"golang.org/x/net/context"
)

type Counter struct {
	ID int64 `gae:"$id"`

	Value int64
}

func TestRace(t *testing.T) {
	t.Parallel()

	c := FilterRDS(memory.Use(context.Background()))

	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		id := int64(i + 1)

		wg.Add(1)
		go func() {
			defer wg.Done()
			err := ds.RunInTransaction(c, func(c context.Context) error {
				for i := 0; i < 100; i++ {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						ctr := &Counter{ID: id}
						if err := ds.Get(c, ctr); err != nil && err != ds.ErrNoSuchEntity {
							t.Fatal("bad Get", err)
						}
						ctr.Value++
						return ds.Put(c, ctr)
					}, nil)
					if err != nil {
						t.Fatal("bad inner RIT", err)
					}
				}

				return nil
			}, nil)
			if err != nil {
				t.Fatal("bad outer RIT", err)
			}
		}()
	}
	wg.Wait()
}
