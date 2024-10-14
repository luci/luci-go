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

package clock

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// timeBase is the amount of time where short-response goroutine events "should
// happen". This isn't a great measure, since scheduling can take longer. Making
// this short will make the test run faster at the possible expense of increased
// raciness. Making this longer will increase test time, but will potentially
// reduce the change of race-related errors.
//
// This should be kept >60ms which is a fairly gratuitous RTC-based scheduler
// delay (1 hr / 2^16) that some older OSes may be subject to.
const timeBase = 60 * time.Millisecond

// veryLongTime is a time long enough that it won't feasably happen during the
// course of test execution.
const veryLongTime = 1000 * timeBase

// TestSystemClock tests the non-trivial system clock methods.
func TestSystemClock(t *testing.T) {
	t.Parallel()

	ftt.Run(`A cancelable Context`, t, func(t *ftt.Test) {
		c, cancelFunc := context.WithCancel(context.Background())
		sc := GetSystemClock()

		t.Run(`Will perform a full sleep if the Context isn't canceled.`, func(t *ftt.Test) {
			assert.Loosely(t, sc.Sleep(c, timeBase).Incomplete(), should.BeFalse)
		})

		t.Run(`Will terminate the Sleep prematurely if the Context is canceled.`, func(t *ftt.Test) {
			cancelFunc()
			assert.Loosely(t, sc.Sleep(c, veryLongTime).Incomplete(), should.BeTrue)
			assert.Loosely(t, sc.Sleep(c, veryLongTime).Err, should.Equal(context.Canceled))
		})
	})
}
