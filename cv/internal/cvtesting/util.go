// Copyright 2020 The LUCI Authors.
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

// Package cvtesting reduces boilerplate in tests.
package cvtesting

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/config"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"

	. "github.com/smartystreets/goconvey/convey"
)

// TODO(tandrii): add fake config generation facilities.

// Test encapsulates typical setup for CV test.
//
// Typical use:
//   ct := cvtesting.Test{}
//   ctx, cancel := ct.SetUp()
//   defer cancel()
type Test struct {
	// Cfg manipulates CV config.
	Cfg config.TestController
	// GFake is a Gerrit fake. Defaults to an empty one.
	GFake *gf.Fake
	// TQ allows to run TQ tasks.
	TQ *tqtesting.Scheduler
	// Clock allows to move time forward.
	// By default, the time is moved automatically is something waits on it.
	Clock testclock.TestClock

	// MaxDuration limits how long a test can run as a fail safe.
	//
	// Defaults to 10s to most likely finish in pre/post submit tests,
	// with limited CPU resources.
	// Set to ~10ms when debugging a hung test.
	MaxDuration time.Duration
}

func (t *Test) SetUp() (ctx context.Context, deferme func()) {
	// Set defaults.
	if t.MaxDuration == time.Duration(0) {
		t.MaxDuration = 10 * time.Second
	}
	if t.GFake == nil {
		t.GFake = &gf.Fake{}
	}

	topCtx, cancel := context.WithTimeout(context.Background(), t.MaxDuration)
	deferme = func() {
		// Fail the test if the topCtx timed out.
		So(topCtx.Err(), ShouldBeNil)
		cancel()
	}

	// Use a date-time that is easy to eyeball in logs.
	utc := time.Date(2020, time.February, 2, 10, 30, 00, 0, time.UTC)
	// But set it up in a clock as a local time to expose incorrect assumptions of UTC.
	now := time.Date(2020, time.February, 2, 13, 30, 00, 0, time.FixedZone("Fake local", 3*60*60))
	So(now.Equal(utc), ShouldBeTrue)
	ctx, t.Clock = testclock.UseTime(topCtx, now)
	t.Clock.SetTimerCallback(func(dur time.Duration, _ clock.Timer) {
		// Move fake time forward whenever someone's waiting for it.
		t.Clock.Add(dur)
	})

	ctx = txndefer.FilterRDS(memory.Use(ctx))
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	if testing.Verbose() {
		ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
	}

	ctx = t.GFake.Install(ctx)
	ctx, t.TQ = tq.TestingContext(ctx, nil)
	return
}
