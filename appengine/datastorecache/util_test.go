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
	"flag"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/server/router"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"

	"golang.org/x/net/context"
)

var testConsoleLogger = flag.Bool("test.logconsole", false, "Output using a console logger.")

type testEnv struct {
	context.Context

	Middleware router.MiddlewareChain
	Router     *router.Router
	Server     *httptest.Server

	DatastoreFB featureBreaker.FeatureBreaker
	Clock       testclock.TestClock
}

func withTestEnv(fn func(te *testEnv)) func() {
	return func() {
		te := testEnv{
			Context: context.Background(),
		}
		te.Context = memory.Use(te.Context)
		te.Context, te.DatastoreFB = featureBreaker.FilterRDS(te.Context, nil)
		if *testConsoleLogger {
			te.Context = gologger.StdConfig.Use(te.Context)
			te.Context = logging.SetLevel(te.Context, logging.Debug)
		}
		te.Context, te.Clock = testclock.UseTime(te.Context, datastore.RoundTime(testclock.TestTimeUTC))

		// Install our middleware chain into our Router.
		te.Middleware = router.NewMiddlewareChain(func(ctx *router.Context, next router.Handler) {
			ctx.Context = te
			next(ctx)
		})
		te.Router = router.New()
		te.Server = httptest.NewServer(te.Router)
		defer te.Server.Close()

		fn(&te)
	}
}

// testCache is an impementation of Handler that returns an error.
type testCache struct {
	Cache

	failOpen             bool
	accessUpdateInterval time.Duration
	refreshInterval      time.Duration
	parallel             int

	refreshes int32
	refreshFn func(c context.Context, key []byte, current Value) (Value, error)
}

func makeTestCache(name string) *testCache {
	tc := testCache{
		Cache: Cache{
			Name:                 name,
			AccessUpdateInterval: time.Minute,
			PruneFactor:          1, // Prune 2*AccessUpdateInterval
		},

		refreshInterval: time.Minute,
	}
	tc.HandlerFunc = func(context.Context) Handler { return &tc }
	return &tc
}

func (tc *testCache) FailOpen() bool                       { return tc.failOpen }
func (tc *testCache) RefreshInterval([]byte) time.Duration { return tc.refreshInterval }
func (tc *testCache) Parallel() int                        { return tc.parallel }

func (tc *testCache) Refresh(c context.Context, key []byte, current Value) (Value, error) {
	atomic.AddInt32(&tc.refreshes, 1)

	if tc.refreshFn == nil {
		return Value{}, errors.New("no refresh function installed")
	}

	value, err := tc.refreshFn(c, key, current)
	if err != nil {
		return Value{}, err
	}
	return value, nil
}

func (tc *testCache) Locker(c context.Context) Locker { return MemLocker(c) }

func (tc *testCache) reset() {
	atomic.StoreInt32(&tc.refreshes, 0)
}
