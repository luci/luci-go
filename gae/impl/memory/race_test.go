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

package memory

import (
	"sync"
	"sync/atomic"
	"testing"

	ds "go.chromium.org/gae/service/datastore"

	"golang.org/x/net/context"
)

func TestRaceGetPut(t *testing.T) {
	t.Parallel()

	value := int32(0)
	num := int32(0)

	c := Use(context.Background())

	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := ds.RunInTransaction(c, func(c context.Context) error {
				atomic.AddInt32(&num, 1)

				obj := pmap("$key", ds.MakeKey(c, "Obj", 1))
				if err := ds.Get(c, obj); err != nil && err != ds.ErrNoSuchEntity {
					t.Fatal("error get", err)
				}
				cur := int64(0)
				if ps := obj.Slice("Value"); len(ps) > 0 {
					cur = ps[0].Value().(int64)
				}

				cur++
				obj["Value"] = prop(cur)

				return ds.Put(c, obj)
			}, &ds.TransactionOptions{Attempts: 200})

			if err != nil {
				t.Fatal("error during transaction", err)
			}

			atomic.AddInt32(&value, 1)
		}()
	}
	wg.Wait()

	obj := pmap("$key", ds.MakeKey(c, "Obj", 1))
	if ds.Get(c, obj) != nil {
		t.FailNow()
	}
	t.Logf("Ran %d inner functions", num)
	if int64(value) != obj.Slice("Value")[0].Value().(int64) {
		t.Fatalf("value wrong value %d v %d", value, obj.Slice("Value")[0].Value().(int64))
	}
}

func TestRaceNonConflictingPuts(t *testing.T) {
	t.Parallel()

	c := Use(context.Background())

	num := int32(0)

	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := ds.RunInTransaction(c, func(c context.Context) error {
				return ds.Put(c, pmap(
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
