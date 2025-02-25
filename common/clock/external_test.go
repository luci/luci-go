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

// testClock is a Clock implementation used for testing.
type testClock struct {
	nowCallback      func() time.Time
	sleepCallback    func() TimerResult
	newTimerCallback func() Timer
}

func (tc *testClock) Now() time.Time {
	return tc.nowCallback()
}

func (tc *testClock) Sleep(context.Context, time.Duration) TimerResult {
	return tc.sleepCallback()
}

func (tc *testClock) NewTimer(context.Context) Timer {
	return tc.newTimerCallback()
}

func TestExternal(t *testing.T) {
	t.Parallel()

	now := time.Date(2015, 01, 01, 0, 0, 0, 0, time.UTC)
	ftt.Run(`A Context with a testClock installed`, t, func(t *ftt.Test) {
		tc := &testClock{}
		c := Set(context.Background(), tc)

		t.Run(`Now() will use the testClock's Now().`, func(t *ftt.Test) {
			used := false
			tc.nowCallback = func() time.Time {
				used = true
				return now
			}

			assert.Loosely(t, Now(c), should.Match(now))
			assert.Loosely(t, used, should.BeTrue)
		})

		t.Run(`Sleep() will use testClock's Sleep().`, func(t *ftt.Test) {
			used := false
			tc.sleepCallback = func() TimerResult {
				used = true
				return TimerResult{}
			}

			Sleep(c, time.Second)
			assert.Loosely(t, used, should.BeTrue)
		})

		t.Run(`NewTimer() will use testClock's NewTimer().`, func(t *ftt.Test) {
			used := false
			tc.newTimerCallback = func() Timer {
				used = true
				return nil
			}

			NewTimer(c)
			assert.Loosely(t, used, should.BeTrue)
		})
	})

	ftt.Run(`An Context with no clock installed`, t, func(t *ftt.Test) {
		c := context.Background()

		t.Run(`Will return a SystemClock instance.`, func(t *ftt.Test) {
			assert.Loosely(t, Get(c), should.HaveType[systemClock])
		})
	})
}
