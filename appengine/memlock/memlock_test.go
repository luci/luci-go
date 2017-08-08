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

package memlock

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/impl/memory"
	mc "go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type getBlockerFilterContext struct {
	sync.Mutex

	dropAll bool
}

type getBlockerFilter struct {
	mc.RawInterface
	gbfc *getBlockerFilterContext
}

func (f *getBlockerFilter) GetMulti(keys []string, cb mc.RawItemCB) error {
	f.gbfc.Lock()
	defer f.gbfc.Unlock()
	if f.gbfc.dropAll {
		for _, key := range keys {
			cb(f.NewItem(key), nil)
		}
		return nil
	}
	return f.RawInterface.GetMulti(keys, cb)
}

func TestSimple(t *testing.T) {
	// TODO(riannucci): Mock time.After so that we don't have to delay for real.

	const key = memlockKeyPrefix + "testkey"

	Convey("basic locking", t, func() {
		start := time.Date(1986, time.October, 26, 1, 20, 00, 00, time.UTC)
		ctx, clk := testclock.UseTime(context.Background(), start)
		blocker := make(chan struct{})
		clk.SetTimerCallback(func(time.Duration, clock.Timer) {
			clk.Add(delay)
			select {
			case blocker <- struct{}{}:
			default:
			}
		})

		waitFalse := func(ctx context.Context) {
		loop:
			for {
				select {
				case <-blocker:
					continue
				case <-ctx.Done():
					break loop
				}
			}
		}

		ctx, fb := featureBreaker.FilterMC(memory.Use(ctx), nil)

		Convey("fails to acquire when memcache is down", func() {
			fb.BreakFeatures(nil, "AddMulti")
			err := TryWithLock(ctx, "testkey", "id", func(context.Context) error {
				// should never reach here
				So(false, ShouldBeTrue)
				return nil
			})
			So(err, ShouldEqual, ErrFailedToLock)
		})

		Convey("returns the inner error", func() {
			toRet := fmt.Errorf("sup")
			err := TryWithLock(ctx, "testkey", "id", func(context.Context) error {
				return toRet
			})
			So(err, ShouldEqual, toRet)
		})

		Convey("returns the error", func() {
			toRet := fmt.Errorf("sup")
			err := TryWithLock(ctx, "testkey", "id", func(context.Context) error {
				return toRet
			})
			So(err, ShouldEqual, toRet)
		})

		Convey("can acquire when empty", func() {
			err := TryWithLock(ctx, "testkey", "id", func(ctx context.Context) error {
				isDone := func() bool {
					select {
					case <-ctx.Done():
						return true
					default:
						return false
					}
				}

				So(isDone(), ShouldBeFalse)

				Convey("waiting for a while keeps refreshing the lock", func() {
					// simulate waiting for 64*delay time, and ensuring that checkLoop
					// runs that many times.
					for i := 0; i < 64; i++ {
						<-blocker
						clk.Add(delay)
					}
					So(isDone(), ShouldBeFalse)
				})

				Convey("but sometimes we might lose it", func() {
					Convey("because it was evicted", func() {
						mc.Delete(ctx, key)
						clk.Add(memcacheLockTime)
						waitFalse(ctx)
					})

					Convey("or because of service issues", func() {
						fb.BreakFeatures(nil, "CompareAndSwapMulti")
						waitFalse(ctx)
					})
				})
				return nil
			})
			So(err, ShouldBeNil)
		})

		Convey("can lose it when it gets stolen", func() {
			gbfc := getBlockerFilterContext{}
			ctx = mc.AddRawFilters(ctx, func(_ context.Context, raw mc.RawInterface) mc.RawInterface {
				return &getBlockerFilter{
					RawInterface: raw,
					gbfc:         &gbfc,
				}
			})
			err := TryWithLock(ctx, "testkey", "id", func(ctx context.Context) error {
				// simulate waiting for 64*delay time, and ensuring that checkLoop
				// runs that many times.
				for i := 0; i < 64; i++ {
					<-blocker
					clk.Add(delay)
				}
				gbfc.Lock()
				mc.Set(ctx, mc.NewItem(ctx, key).SetValue([]byte("wat")))
				gbfc.Unlock()
				waitFalse(ctx)
				return nil
			})
			So(err, ShouldBeNil)
		})

		Convey("can lose it when it gets preemptively released", func() {
			gbfc := getBlockerFilterContext{}
			ctx = mc.AddRawFilters(ctx, func(_ context.Context, raw mc.RawInterface) mc.RawInterface {
				return &getBlockerFilter{
					RawInterface: raw,
					gbfc:         &gbfc,
				}
			})
			ctx = context.WithValue(ctx, testStopCBKey, func() {
				gbfc.Lock()
				defer gbfc.Unlock()

				gbfc.dropAll = true
			})
			err := TryWithLock(ctx, "testkey", "id", func(ctx context.Context) error {
				// simulate waiting for 64*delay time, and ensuring that checkLoop
				// runs that many times.
				for i := 0; i < 64; i++ {
					<-blocker
					clk.Add(delay)
				}
				return nil
			})
			So(err, ShouldBeNil)
		})

		Convey("an empty context id is an error", func() {
			So(TryWithLock(ctx, "testkey", "", nil), ShouldEqual, ErrEmptyClientID)
		})
	})
}
