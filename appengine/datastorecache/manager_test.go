// Copyright 2016 The LUCI Authors.
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

package datastorecache

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func testManagerImpl(t *testing.T, mgrTemplate *manager) {
	t.Parallel()

	Convey(`A testing manager setup`, t, withTestEnv(func(te *testEnv) {
		// Build our Manager, using template parameters.
		cache := makeTestCache("test-cache")
		mgr := cache.manager()
		if mgrTemplate != nil {
			mgr.queryBatchSize = mgrTemplate.queryBatchSize
		}

		const cronPath = "/cron/manager"
		mgr.installCronRoute(cronPath, te.Router, te.Middleware)

		runCron := func() int {
			datastore.GetTestable(te).CatchupIndexes()
			resp, err := http.Get(te.Server.URL + cronPath)
			if err != nil {
				panic(fmt.Errorf("failed to GET: %s", err))
			}
			return resp.StatusCode
		}

		statsForShard := func(id int) *managerShardStats {
			st := managerShardStats{
				Shard: id + 1,
			}
			So(datastore.Get(cache.withNamespace(te), &st), ShouldBeNil)
			return &st
		}

		// Useful times.
		var (
			now                = te.Clock.Now()
			accessedRecently   = now.Add(-time.Second)
			accessedNeedsPrune = now.Add(-cache.pruneInterval())

			refreshUnnecessary = now.Add(-time.Second)
			refreshNeeded      = now.Add(-cache.refreshInterval)
		)

		Convey(`Can run on an empty data set.`, func() {
			So(runCron(), ShouldEqual, http.StatusOK)
			So(statsForShard(0), ShouldResemble, &managerShardStats{
				Shard:             1,
				LastSuccessfulRun: now,
			})
		})

		Convey(`Returns error code for cache update errors.`, func() {
			So(datastore.Put(cache.withNamespace(te), &entry{
				CacheName:     cache.Name,
				Key:           []byte("foo"),
				LastAccessed:  accessedRecently,
				LastRefreshed: refreshNeeded,
			}), ShouldBeNil)
			datastore.GetTestable(te).CatchupIndexes()

			// No refresh function installed, so this will fail.
			So(runCron(), ShouldEqual, http.StatusInternalServerError)
		})

		Convey(`With cache entries installed`, func() {
			var entries []*entry
			for i, ce := range []struct {
				key           string
				lastAccessed  time.Time
				lastRefreshed time.Time
			}{
				{"idle", accessedRecently, refreshUnnecessary},
				{"refreshMe", accessedRecently, refreshNeeded},
				{"pruneMe", accessedNeedsPrune, refreshUnnecessary},
				{"pruneMeInsteeadOfRefresh", accessedNeedsPrune, refreshNeeded},
			} {
				entries = append(entries, &entry{
					CacheName:     cache.Name,
					Key:           []byte(ce.key),
					LastAccessed:  ce.lastAccessed,
					LastRefreshed: ce.lastRefreshed,
					Data:          []byte(ce.key),
					Schema:        "test",
					Description:   fmt.Sprintf("test entry #%d", i),
				})
			}
			So(datastore.Put(cache.withNamespace(te), entries), ShouldBeNil)

			Convey(`Will refresh/prune the entries.`, func() {
				cache.refreshFn = func(context.Context, []byte, Value) (Value, error) {
					return Value{
						Schema:      "new schema",
						Description: "new hotness",
						Data:        []byte("REFRESHED"),
					}, nil
				}

				So(runCron(), ShouldEqual, http.StatusOK)
				So(statsForShard(0), ShouldResemble, &managerShardStats{
					Shard:             1,
					LastSuccessfulRun: now,
					LastEntryCount:    4,
				})
				So(cache.refreshes, ShouldEqual, 1)

				// All "refreshMe" should have their data updated.
				for _, e := range entries {
					if !bytes.Equal(e.Key, []byte("refreshMe")) {
						continue
					}
					So(datastore.Get(cache.withNamespace(te), e), ShouldBeNil)
					So(e.Data, ShouldResemble, []byte("REFRESHED"))
				}

				Convey(`A second run will not refresh any entries.`, func() {
					cache.refreshFn = nil
					So(runCron(), ShouldEqual, http.StatusOK)
					So(statsForShard(0), ShouldResemble, &managerShardStats{
						Shard:             1,
						LastSuccessfulRun: now,
						LastEntryCount:    2, // Pruned half of them.
					})
				})
			})

			Convey(`Will not prune any entries if AccessUpdateInterval is 0.`, func() {
				cache.refreshFn = func(c context.Context, key []byte, v Value) (Value, error) { return v, nil }
				cache.AccessUpdateInterval = 0

				So(runCron(), ShouldEqual, http.StatusOK)
				So(statsForShard(0), ShouldResemble, &managerShardStats{
					Shard:             1,
					LastSuccessfulRun: now,
					LastEntryCount:    4,
				})

				So(runCron(), ShouldEqual, http.StatusOK)
				So(statsForShard(0), ShouldResemble, &managerShardStats{
					Shard:             1,
					LastSuccessfulRun: now,
					LastEntryCount:    4, // None pruned.
				})
			})

			Convey(`Will not prune any entries if PruneFactor is 0.`, func() {
				cache.refreshFn = func(c context.Context, key []byte, v Value) (Value, error) { return v, nil }
				cache.PruneFactor = 0

				So(runCron(), ShouldEqual, http.StatusOK)
				So(statsForShard(0), ShouldResemble, &managerShardStats{
					Shard:             1,
					LastSuccessfulRun: now,
					LastEntryCount:    4,
				})

				So(runCron(), ShouldEqual, http.StatusOK)
				So(statsForShard(0), ShouldResemble, &managerShardStats{
					Shard:             1,
					LastSuccessfulRun: now,
					LastEntryCount:    4, // None pruned.
				})
			})

			Convey(`Will error if datastore PutMulti is broken.`, func() {
				te.DatastoreFB.BreakFeatures(errors.New("test error"), "PutMulti")

				cache.refreshFn = func(context.Context, []byte, Value) (Value, error) { return Value{}, nil }

				So(runCron(), ShouldEqual, http.StatusInternalServerError)
				So(cache.refreshes, ShouldEqual, 1)
			})

			Convey(`Will error if datastore DeleteMulti is broken.`, func() {
				te.DatastoreFB.BreakFeatures(errors.New("test error"), "DeleteMulti")

				cache.refreshFn = func(context.Context, []byte, Value) (Value, error) { return Value{}, nil }

				So(runCron(), ShouldEqual, http.StatusInternalServerError)
				So(cache.refreshes, ShouldEqual, 1)
			})

			Convey(`Will not refresh entries if there is an error.`, func() {
				// (The error is that we are refreshing, but there is no refresh
				// handler installed).
				So(runCron(), ShouldEqual, http.StatusInternalServerError)
				So(statsForShard(0), ShouldResemble, &managerShardStats{
					Shard:             1,
					LastSuccessfulRun: time.Time{},
					LastEntryCount:    4,
				})
			})

			Convey(`Will delete cache entries, if requested.`, func() {
				cache.refreshFn = func(context.Context, []byte, Value) (Value, error) { return Value{}, ErrDeleteCacheEntry }

				So(runCron(), ShouldEqual, http.StatusOK)
				So(statsForShard(0), ShouldResemble, &managerShardStats{
					Shard:             1,
					LastSuccessfulRun: now,
					LastEntryCount:    4,
				})
				// One of the four should have been refreshed, and requeted deletion.
				So(cache.refreshes, ShouldEqual, 1)

				// Do a second run. Nothing should be refreshed, since all refresh
				// candidates were deleted.
				cache.reset()
				So(runCron(), ShouldEqual, http.StatusOK)
				So(statsForShard(0), ShouldResemble, &managerShardStats{
					Shard:             1,
					LastSuccessfulRun: now,
					LastEntryCount:    1, // Only "idle" remains.
				})
				// One of the four should have been refreshed, and requeted
				// deletion.
				So(cache.refreshes, ShouldEqual, 0)
			})
		})

		Convey(`With cache entries, but no Handler function.`, func() {
			cache.HandlerFunc = nil

			var entries []*entry
			for i, la := range []time.Time{
				accessedRecently,
				accessedNeedsPrune,
			} {
				entries = append(entries, &entry{
					CacheName:     cache.Name,
					Key:           []byte(strconv.Itoa(i)),
					LastAccessed:  la,
					LastRefreshed: refreshNeeded,
					Data:          nil,
					Schema:        "test",
					Description:   fmt.Sprintf("test entry #%d", i),
				})
			}
			So(datastore.Put(cache.withNamespace(te), entries), ShouldBeNil)

			So(runCron(), ShouldEqual, http.StatusOK)
			So(statsForShard(0), ShouldResemble, &managerShardStats{
				Shard:             1,
				LastSuccessfulRun: te.Clock.Now(),
				LastEntryCount:    2,
			})

			// All are expired.
			te.Clock.Add(cache.pruneInterval())
			So(runCron(), ShouldEqual, http.StatusOK)
			So(statsForShard(0), ShouldResemble, &managerShardStats{
				Shard:             1,
				LastSuccessfulRun: te.Clock.Now(),
				LastEntryCount:    1,
			})

			// All have now been pruned.
			So(runCron(), ShouldEqual, http.StatusOK)
			So(statsForShard(0), ShouldResemble, &managerShardStats{
				Shard:             1,
				LastSuccessfulRun: te.Clock.Now(),
				LastEntryCount:    0,
			})
		})

		Convey(`Will return an error if Run is broken during Handler query.`, func() {
			te.DatastoreFB.BreakFeatures(errors.New("test error"), "Run")
			So(runCron(), ShouldEqual, http.StatusInternalServerError)
		})

		Convey(`Will return an error if Run is broken during entry refresh.`, func() {
			// We will Put "queryBatchSize+1" expired entries, then execute our
			// handler. The first round will refresh, at which point our refresh
			// handler will break Run. The next round will encounter the broken Run
			// and
			entries := make([]*entry, mgr.queryBatchSize+1)
			for i := range entries {
				entries[i] = &entry{
					CacheName:    cache.Name,
					Key:          []byte(fmt.Sprintf("entry-%d", i)),
					LastAccessed: accessedRecently,
				}
			}
			So(datastore.Put(cache.withNamespace(te), entries), ShouldBeNil)

			cache.refreshFn = func(context.Context, []byte, Value) (Value, error) {
				te.DatastoreFB.BreakFeatures(errors.New("test error"), "Run")
				return Value{}, nil
			}

			So(runCron(), ShouldEqual, http.StatusInternalServerError)
		})
	}))
}

// TestManager runs the manager tests against the production manager instance.
// This exercises the production settings.
func TestManager(t *testing.T) {
	testManagerImpl(t, nil)
}

// TestManagerConstrained runs the manager tests against a resource-constrained
// manager instance. This exercises limited (but value) manager settings.
func TestManagerConstrained(t *testing.T) {
	testManagerImpl(t, &manager{
		queryBatchSize: 1,
	})
}
