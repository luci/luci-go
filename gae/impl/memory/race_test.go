// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/luci/gae/service/datastore"
	"golang.org/x/net/context"
)

func TestRaceGetPut(t *testing.T) {
	t.Parallel()

	value := int32(0)
	num := int32(0)

	ds := datastore.Get(Use(context.Background()))

	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := ds.RunInTransaction(func(c context.Context) error {
				atomic.AddInt32(&num, 1)

				ds := datastore.Get(c)

				obj := pmap("$key", ds.MakeKey("Obj", 1))
				if err := ds.Get(obj); err != nil && err != datastore.ErrNoSuchEntity {
					t.Fatal("error get", err)
				}
				cur := int64(0)
				if ps := obj.Slice("Value"); len(ps) > 0 {
					cur = ps[0].Value().(int64)
				}

				cur++
				obj["Value"] = prop(cur)

				return ds.Put(obj)
			}, &datastore.TransactionOptions{Attempts: 200})

			if err != nil {
				t.Fatal("error during transaction", err)
			}

			atomic.AddInt32(&value, 1)
		}()
	}
	wg.Wait()

	obj := pmap("$key", ds.MakeKey("Obj", 1))
	if ds.Get(obj) != nil {
		t.FailNow()
	}
	t.Logf("Ran %d inner functions", num)
	if int64(value) != obj.Slice("Value")[0].Value().(int64) {
		t.Fatalf("value wrong value %d v %d", value, obj.Slice("Value")[0].Value().(int64))
	}
}

func TestRaceNonConflictingPuts(t *testing.T) {
	t.Parallel()

	ds := datastore.Get(Use(context.Background()))

	num := int32(0)

	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := ds.RunInTransaction(func(c context.Context) error {
				ds := datastore.Get(c)
				return ds.Put(pmap(
					"$kind", "Thing", Next,
					"Value", 100))
			}, nil)
			if err != nil {
				t.Fatal("error during transaction", err)
			}
			atomic.AddInt32(&num, 1)
		}()
	}
	wg.Wait()

	if num != 100 {
		t.Fatal("expected 100 runs, got", num)
	}
}
