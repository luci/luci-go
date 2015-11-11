// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package txnBuf

import (
	"sync"
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"golang.org/x/net/context"
)

type Counter struct {
	ID int64 `gae:"$id"`

	Value int64
}

func TestRace(t *testing.T) {
	t.Parallel()

	c := FilterRDS(memory.Use(context.Background()))
	ds := datastore.Get(c)

	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		id := int64(i + 1)

		wg.Add(1)
		go func() {
			defer wg.Done()
			err := ds.RunInTransaction(func(c context.Context) error {
				ds := datastore.Get(c)

				for i := 0; i < 100; i++ {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := datastore.Get(c)

						ctr := &Counter{ID: id}
						if err := ds.Get(ctr); err != nil && err != datastore.ErrNoSuchEntity {
							t.Fatal("bad Get", err)
						}
						ctr.Value++
						return ds.Put(ctr)
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
