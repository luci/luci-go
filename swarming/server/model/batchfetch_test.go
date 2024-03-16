// Copyright 2024 The LUCI Authors.
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

package model

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBatchFetcher(t *testing.T) {
	t.Parallel()

	type entity struct {
		ID    int64 `gae:"$id"`
		Value int
	}

	const minID = 10
	const maxID = 20

	ctx := memory.Use(context.Background())
	for i := minID; i <= maxID; i++ {
		err := datastore.Put(ctx, &entity{
			ID:    int64(i),
			Value: i,
		})
		if err != nil {
			panic(err)
		}
	}

	params := []struct {
		batchSize        int
		concurrencyLimit int
	}{
		{3, 4},   // normal case
		{3, 1},   // edge case
		{1, 1},   // edge case
		{1, 100}, // edge case
	}
	for _, param := range params {
		Convey(fmt.Sprintf("Works for %d %d", param.batchSize, param.concurrencyLimit), t, func() {
			f := NewBatchFetcher[int, entity](ctx, param.batchSize, param.concurrencyLimit)
			defer f.Close()

			for i := 1; i < maxID+10; i++ {
				f.Fetch(i, &entity{ID: int64(i)})
			}
			f.Wait()

			// A bunch of missing entities first.
			for i := 1; i < minID; i++ {
				_, err := f.Get(i)
				So(err, ShouldEqual, datastore.ErrNoSuchEntity)
			}
			// Then correctly fetched ones.
			for i := minID; i <= maxID; i++ {
				e, err := f.Get(i)
				So(err, ShouldBeNil)
				So(e.Value, ShouldEqual, i)
			}
			// Then a bunch more of missing ones.
			for i := maxID + 1; i < maxID+10; i++ {
				_, err := f.Get(i)
				So(err, ShouldEqual, datastore.ErrNoSuchEntity)
			}

			// This one wasn't fetched at all.
			_, err := f.Get(1000)
			So(err, ShouldEqual, ErrNoSuchFetchOperation)
		})
	}

	Convey("Fatal errors", t, func() {
		ctx, fb := featureBreaker.FilterRDS(ctx, nil)

		fatalErr := errors.New("BOOM")
		fb.BreakFeatures(fatalErr, featureBreaker.DatastoreFeatures...)

		f := NewBatchFetcher[int, entity](ctx, 3, 2)
		defer f.Close()
		for i := minID; i <= maxID; i++ {
			f.Fetch(i, &entity{ID: int64(i)})
		}
		f.Wait()

		for i := minID; i <= maxID; i++ {
			_, err := f.Get(i)
			So(err, ShouldEqual, fatalErr)
		}
	})

	Convey("Cancellation", t, func() {
		const batchSize = 3
		const expectedCalls = 5

		ctx, fb := featureBreaker.FilterRDS(ctx, nil)

		var m sync.Mutex
		var pendingCalls int
		allPendingNow := make(chan struct{})

		fb.BreakFeaturesWithCallback(
			func(ctx context.Context, _ string) error {
				// Notify the main goroutine when all RPCs are in-flight.
				m.Lock()
				pendingCalls += 1
				if pendingCalls == expectedCalls {
					close(allPendingNow)
				}
				m.Unlock()
				// Make the call stuck until the context expires.
				<-ctx.Done()
				return ctx.Err()
			},
			featureBreaker.DatastoreFeatures...,
		)

		f := NewBatchFetcher[int, entity](ctx, batchSize, expectedCalls+1)
		defer f.Close()
		for i := 1; i <= batchSize*expectedCalls; i++ {
			f.Fetch(i, &entity{ID: int64(i)})
		}

		// Wait until all expected RPCs have been launched.
		<-allPendingNow
		// This should cancel them all.
		f.Close()
		// Does nothing (and doesn't panic).
		f.Wait()

		// All operations are cancelled.
		for i := 1; i <= batchSize*expectedCalls; i++ {
			_, err := f.Get(i)
			So(err, ShouldEqual, context.Canceled)
		}
	})
}
