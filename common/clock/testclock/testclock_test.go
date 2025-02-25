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

package testclock

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTestClock(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing clock instance`, t, func(t *ftt.Test) {
		now := time.Date(2015, 01, 01, 00, 00, 00, 00, time.UTC)
		ctx, clk := UseTime(context.Background(), now)

		t.Run(`Returns the current time.`, func(t *ftt.Test) {
			assert.Loosely(t, clk.Now(), should.Match(now))
		})

		t.Run(`When sleeping with a time of zero, immediately awakens.`, func(t *ftt.Test) {
			clk.Sleep(ctx, 0)
			assert.Loosely(t, clk.Now(), should.Match(now))
		})

		t.Run(`Will panic if going backwards in time.`, func(t *ftt.Test) {
			assert.Loosely(t, func() { clk.Add(-1 * time.Second) }, should.Panic)
		})

		t.Run(`When sleeping for a period of time, awakens when signalled.`, func(t *ftt.Test) {
			sleepingC := make(chan struct{})
			clk.SetTimerCallback(func(_ time.Duration, _ clock.Timer) {
				close(sleepingC)
			})

			awakeC := make(chan time.Time)
			go func() {
				clk.Sleep(ctx, 2*time.Second)
				awakeC <- clk.Now()
			}()

			<-sleepingC
			clk.Set(now.Add(1 * time.Second))
			clk.Set(now.Add(2 * time.Second))
			assert.Loosely(t, <-awakeC, should.Match(now.Add(2*time.Second)))
		})

		t.Run(`Awakens after a period of time.`, func(t *ftt.Test) {
			afterC := clock.After(ctx, 2*time.Second)

			clk.Set(now.Add(1 * time.Second))
			clk.Set(now.Add(2 * time.Second))
			assert.Loosely(t, <-afterC, should.Match(clock.TimerResult{now.Add(2 * time.Second), nil}))
		})

		t.Run(`When sleeping, awakens if canceled.`, func(t *ftt.Test) {
			ctx, cancelFunc := context.WithCancel(ctx)

			clk.SetTimerCallback(func(_ time.Duration, _ clock.Timer) {
				cancelFunc()
			})

			assert.Loosely(t, clk.Sleep(ctx, time.Second).Incomplete(), should.BeTrue)
		})
	})
}
