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
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCache(t *testing.T) {
	t.Parallel()

	Convey(`A testing Cache setup`, t, withTestEnv(func(te *testEnv) {
		currentValue := Value{
			Schema:      "testSchema",
			Description: "test cache entry",
			Data:        []byte("first"),
		}
		cache := makeTestCache("test")
		cache.refreshFn = func(context.Context, []byte, Value) (Value, error) { return currentValue, nil }

		// otherLocker is a memLocker intance (used by makeTestCache) that
		// identifies itself as a different client than the one running the test.
		//
		// This is used to create lock conflicts.
		otherLocker := memLocker{"other client ID"}

		Convey(`Will panic if it has neither a name or namespace configured.`, func() {
			cache.Name = ""
			cache.Namespace = ""

			So(func() {
				cache.Get(te, []byte("foo"))
			}, ShouldPanic)
		})

		Convey(`Will error on Get if no Handler is installed.`, func() {
			cache.HandlerFunc = nil

			_, err := cache.Get(te, []byte("foo"))
			So(err, ShouldErrLike, "unable to generate Handler")
		})

		Convey(`Can perform an initial Get.`, func() {
			v, err := cache.Get(te, []byte("foo"))
			So(err, ShouldBeNil)
			So(v, ShouldResemble, currentValue)
			So(cache.refreshes, ShouldEqual, 1)

			Convey(`A successive Get will load from datastore.`, func() {
				cache.reset()

				v, err := cache.Get(te, []byte("foo"))
				So(err, ShouldBeNil)
				So(v, ShouldResemble, currentValue)
				So(cache.refreshes, ShouldEqual, 0)
			})

			Convey(`After that entry has expired, a successive Get will return the stale data.`, func() {
				cache.reset()
				te.Clock.Add(expireFactor * cache.refreshInterval)

				v, err := cache.Get(te, []byte("foo"))
				So(err, ShouldBeNil)
				So(v, ShouldResemble, currentValue)
				So(cache.refreshes, ShouldEqual, 0)
			})

			Convey(`When GetMulti is broken, a successive Get will refresh.`, func() {
				cache.reset()
				te.DatastoreFB.BreakFeatures(errors.New("test error"), "GetMulti")

				v, err := cache.Get(te, []byte("foo"))
				So(err, ShouldBeNil)
				So(v, ShouldResemble, currentValue)
				So(cache.refreshes, ShouldEqual, 1)
			})
		})

		Convey(`Can perform a Get while in another entity's transaction`, func() {
			err := datastore.RunInTransaction(te, func(c context.Context) error {
				// Hit some random entity first. Since we're not cross-group, this means
				// that any invalid single-group datastore accesses from the cache will
				// error.
				r, err := datastore.Exists(c, datastore.PropertyMap{
					"$kind": datastore.MkProperty("SomeEntitySomewhere"),
					"$id":   datastore.MkProperty(1),
				})
				if err != nil {
					return err
				}
				So(r.Any(), ShouldBeFalse)

				// Perform a cache Get.
				v, err := cache.Get(c, []byte("foo"))
				if err != nil {
					return err
				}
				So(v, ShouldResemble, currentValue)
				return nil
			}, nil)
			So(err, ShouldBeNil)
			So(cache.refreshes, ShouldEqual, 1)
		})

		Convey(`The refresh function is called with the original namespace.`, func() {
			c := info.MustNamespace(te, "dog")

			cache.refreshFn = func(rc context.Context, key []byte, v Value) (Value, error) {
				So(info.GetNamespace(rc), ShouldEqual, "dog")
				return currentValue, nil
			}

			v, err := cache.Get(c, []byte("foo"))
			So(err, ShouldBeNil)
			So(v, ShouldResemble, currentValue)
			So(cache.refreshes, ShouldEqual, 1)
		})

		Convey(`When PutMulti is broken, will fail open.`, func() {
			te.DatastoreFB.BreakFeatures(errors.New("test error"), "PutMulti")

			cache.reset()
			v, err := cache.Get(te, []byte("foo"))
			So(err, ShouldBeNil)
			So(v, ShouldResemble, currentValue)
			So(cache.refreshes, ShouldEqual, 1)

			cache.reset()
			v, err = cache.Get(te, []byte("foo"))
			So(err, ShouldBeNil)
			So(v, ShouldResemble, currentValue)
			So(cache.refreshes, ShouldEqual, 1)
		})

		Convey(`When Refresh returns an error, it is propagated.`, func() {
			testErr := errors.New("test error")
			cache.refreshFn = func(context.Context, []byte, Value) (Value, error) { return Value{}, testErr }

			_, err := cache.Get(te, []byte("foo"))
			So(err, ShouldEqual, testErr)

			_, err = cache.Get(te, []byte("foo"))
			So(err, ShouldEqual, testErr)
		})

		Convey(`When the entry's lock is held`, func() {
			e := entry{
				CacheName: cache.Name,
				Key:       []byte("foo"),
			}

			// When a sleep is requested, automatically jump into the future.
			var sleepCB func()
			sleeps := 0
			te.Clock.SetTimerCallback(func(d time.Duration, t clock.Timer) {
				for _, tag := range testclock.GetTags(t) {
					if tag == "datastoreCacheLockRetry" {
						if sleepCB != nil {
							sleepCB()
						}
						sleeps++
						break
					}
				}
				te.Clock.Add(d)
			})

			Convey(`Will manually refresh with original namespace after retries.`, func() {

				cache.refreshFn = func(rc context.Context, key []byte, v Value) (Value, error) {
					So(info.GetNamespace(rc), ShouldEqual, "dog")
					return currentValue, nil
				}

				// Hold the entity's refresh lock. Our cache will not be able to acquire
				// it.
				var v Value
				err := otherLocker.TryWithLock(cache.withNamespace(te), e.lockKey(), func(c context.Context) (err error) {
					v, err = cache.Get(info.MustNamespace(te, "dog"), []byte("foo"))
					return
				})
				So(err, ShouldBeNil)
				So(v, ShouldResemble, currentValue)
				So(sleeps, ShouldEqual, initialLoadLockRetries)
				So(cache.refreshes, ShouldEqual, 1)
			})

			Convey(`Will propagate a lock-less refresh failure.`, func() {
				terr := errors.New("test error")
				cache.refreshFn = func(context.Context, []byte, Value) (Value, error) { return Value{}, terr }

				// Hold the entity's refresh lock. Our cache will not be able to acquire
				// it.
				err := otherLocker.TryWithLock(cache.withNamespace(te), e.lockKey(), func(c context.Context) (err error) {
					_, err = cache.Get(te, []byte("foo"))
					return
				})
				So(err, ShouldEqual, terr)
				So(sleeps, ShouldEqual, initialLoadLockRetries)
				So(cache.refreshes, ShouldEqual, 1)
			})

			Convey(`Will not refresh if the value is put in between sleeps.`, func() {
				sleepCB = func() {
					So(datastore.Put(cache.withNamespace(te), &entry{
						CacheName: cache.Name,
						Key:       []byte("foo"),
						Data:      []byte("ohaithere"),
					}), ShouldBeNil)
				}

				// Hold the entity's refresh lock. Our cache will not be able to acquire
				// it.
				err := otherLocker.TryWithLock(cache.withNamespace(te), e.lockKey(), func(c context.Context) (err error) {
					_, err = cache.Get(te, []byte("foo"))
					return
				})
				So(err, ShouldBeNil)
				So(sleeps, ShouldEqual, 1)
				So(cache.refreshes, ShouldEqual, 0)
			})

			Convey(`(Fail Open) Will refresh if GetMulti is broken during retries.`, func() {
				te.DatastoreFB.BreakFeatures(errors.New("test error"), "GetMulti")

				// Hold the entity's refresh lock. Our cache will not be able to acquire
				// it.
				err := otherLocker.TryWithLock(cache.withNamespace(te), e.lockKey(), func(c context.Context) (err error) {
					_, err = cache.Get(te, []byte("foo"))
					return
				})
				So(err, ShouldBeNil)
				So(sleeps, ShouldEqual, initialLoadLockRetries)
				So(cache.refreshes, ShouldEqual, 1)
			})
		})

		Convey(`When multiple entries refresh at the same time, only one call is made.`, func() {
			const agents = 32

			// The first agent will acquire a memlock on this cache item. All of the
			// other ones will enter a retry loop, attempting to get the lock. Each
			// retry round will sleep a little in between attempts.
			//
			// Our first agent will acquire the lock and refresh. We'll block it there
			// to let the other agents catch up. The other agents will fail to acquire
			// the lock and sleep, where we'll intercept them in the sleep timer
			// callback.
			//
			// After all of our agents are synchronized at the same point, we'll
			// release our first agent and let it finish. This will Put the result in
			// datastore. After the first agent finishes, we'll start time again and
			// let the other agents retry. Their next attempt will find the result
			// from the first agent in datastore and return it without refreshing
			// themselves.
			agentSleepingC := make(chan struct{})
			agentReleaseC := make(chan struct{})
			te.Clock.SetTimerCallback(func(d time.Duration, t clock.Timer) {
				for _, tag := range testclock.GetTags(t) {
					if tag == "datastoreCacheLockRetry" {
						agentSleepingC <- struct{}{}
						<-agentReleaseC
						break
					}
				}
				te.Clock.Add(d)
			})

			// Refresh will block pending refreshC. We'll release this when all of
			// the other agents have tried to sleep following failure to lock.
			refreshC := make(chan struct{})
			cache.refreshFn = func(context.Context, []byte, Value) (Value, error) {
				<-refreshC
				return currentValue, nil
			}

			// Start our refresh agents in parallel.
			values, errors := make([]Value, agents), make([]error, agents)
			doneC := make(chan int)
			for i := 0; i < agents; i++ {
				go func(idx int) {
					defer func() {
						doneC <- idx
					}()
					c := info.GetTestable(te).SetRequestID(fmt.Sprintf("agent-%d", idx))
					values[idx], errors[idx] = cache.Get(c, []byte("foo"))
				}(i)
			}

			// Wait for all but one of our agents to sleep. The non-sleeping one will
			// have gotten the lock on the entry.
			for i := 1; i < agents; i++ {
				<-agentSleepingC
			}
			// Allow our agent-with-lock to complete its Refresh.
			close(refreshC)
			// Wait for our agent-with-lock to finish.
			<-doneC
			// Release our sleeping agents.
			close(agentReleaseC)
			// Reap the remaining agents.
			for i := 1; i < agents; i++ {
				<-doneC
			}

			So(cache.refreshes, ShouldEqual, 1)
			for i := 0; i < agents; i++ {
				So(values[i], ShouldResemble, currentValue)
				So(errors[i], ShouldBeNil)
			}
		})

		Convey(`When accessing an entry past its access update threshold`, func() {
			accessedAWhileAgo := te.Clock.Now().Add(-cache.AccessUpdateInterval)
			ev := Value{
				Schema:      "test schema",
				Data:        []byte("bar"),
				Description: "test entry",
			}
			e := entry{
				CacheName:     cache.Name,
				Key:           []byte("foo"),
				LastRefreshed: te.Clock.Now(),
				LastAccessed:  accessedAWhileAgo,
			}
			e.loadValue(ev)
			So(datastore.Put(cache.withNamespace(te), &e), ShouldBeNil)

			Convey(`Will update its LastAccessed timestamp.`, func() {
				v, err := cache.Get(te, []byte("foo"))
				So(err, ShouldBeNil)
				So(v, ShouldResemble, ev)

				So(datastore.Get(cache.withNamespace(te), &e), ShouldBeNil)
				So(e.LastAccessed, ShouldResemble, te.Clock.Now())
			})

			Convey(`Will ignore accessed timestamp PutMulti failures.`, func() {
				te.DatastoreFB.BreakFeatures(errors.New("test error"), "PutMulti")

				v, err := cache.Get(te, []byte("foo"))
				So(err, ShouldBeNil)
				So(v, ShouldResemble, ev)

				So(datastore.Get(cache.withNamespace(te), &e), ShouldBeNil)
				So(e.LastAccessed, ShouldResemble, accessedAWhileAgo)
			})

			Convey(`Will ignore accessed timestamp if the entry's lock is held.`, func() {
				var v Value
				err := otherLocker.TryWithLock(cache.withNamespace(te), e.lockKey(), func(c context.Context) (err error) {
					v, err = cache.Get(te, []byte("foo"))
					return
				})
				So(err, ShouldBeNil)
				So(v, ShouldResemble, ev)

				So(datastore.Get(cache.withNamespace(te), &e), ShouldBeNil)
				So(e.LastAccessed, ShouldResemble, accessedAWhileAgo)
			})
		})
	}))
}
