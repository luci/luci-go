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

package clock

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSystemTimer(t *testing.T) {
	t.Parallel()

	ftt.Run(`A systemTimer instance`, t, func(t *ftt.Test) {
		ctx, cancelFunc := context.WithCancel(context.Background())
		timer := GetSystemClock().NewTimer(ctx)
		defer timer.Stop()

		t.Run(`Should start with a non-nil channel.`, func(t *ftt.Test) {
			assert.Loosely(t, timer.GetC(), should.NotBeNil)
		})

		t.Run(`When stopped, should return inactive.`, func(t *ftt.Test) {
			assert.Loosely(t, timer.Stop(), should.BeFalse)
		})

		t.Run(`Will return immediately if the Context is canceled before Reset.`, func(t *ftt.Test) {
			cancelFunc()

			timer.Reset(veryLongTime)
			assert.Loosely(t, (<-timer.GetC()).Err, should.Equal(context.Canceled))
		})

		t.Run(`Will return if the Context is canceled after Reset.`, func(t *ftt.Test) {
			timer.Reset(veryLongTime)
			cancelFunc()

			assert.Loosely(t, (<-timer.GetC()).Err, should.Equal(context.Canceled))
		})

		t.Run(`A timer will use the same channel when Reset.`, func(t *ftt.Test) {
			timerC := timer.GetC()

			// Reset our timer to something more reasonable. It should trigger the
			// timer channel, which should trigger our
			timer.Reset(timeBase)
			assert.Loosely(t, (<-timerC).Err, should.BeNil)
		})

		t.Run(`A timer will not signal if stopped.`, func(t *ftt.Test) {
			timer.Reset(timeBase)
			timer.Stop()

			// This isn't a perfect test, but it's a good boundary for flake.
			time.Sleep(3 * timeBase)

			triggered := false
			select {
			case <-timer.GetC():
				triggered = true
			default:
				break
			}
			assert.Loosely(t, triggered, should.BeFalse)
		})

		t.Run(`When reset`, func(t *ftt.Test) {
			assert.Loosely(t, timer.Reset(veryLongTime), should.BeFalse)

			t.Run(`When reset again to a short duration, should return that it was active and trigger.`, func(t *ftt.Test) {
				// Upper bound of supported platform resolution. Windows is 15ms, so
				// make sure we exceed that.
				assert.Loosely(t, timer.Reset(timeBase), should.BeTrue)
				assert.Loosely(t, (<-timer.GetC()).IsZero(), should.BeFalse)

				// Again (reschedule).
				assert.Loosely(t, timer.Reset(timeBase), should.BeFalse)
				assert.Loosely(t, (<-timer.GetC()).IsZero(), should.BeFalse)
			})

			t.Run(`When stopped, should return active and have a non-nil C.`, func(t *ftt.Test) {
				active := timer.Stop()
				assert.Loosely(t, active, should.BeTrue)
				assert.Loosely(t, timer.GetC(), should.NotBeNil)
			})

			t.Run(`When stopped, should return active.`, func(t *ftt.Test) {
				active := timer.Stop()
				assert.Loosely(t, active, should.BeTrue)
			})

			t.Run(`Should have a non-nil channel.`, func(t *ftt.Test) {
				assert.Loosely(t, timer.GetC(), should.NotBeNil)
			})
		})
	})
}
