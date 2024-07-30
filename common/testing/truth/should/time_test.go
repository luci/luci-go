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

package should

import (
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
)

func TestHappenBefore(t *testing.T) {
	t.Parallel()

	now := testclock.TestRecentTimeLocal

	t.Run("simple", shouldPass(HappenBefore(now)(now.Add(-time.Second))))

	t.Run("simple equal", shouldFail(HappenBefore(now)(now), "equal"))
	t.Run("simple false", shouldFail(HappenBefore(now.Add(-time.Second))(now), "Diff"))
}

func TestHappenOnOrBefore(t *testing.T) {
	t.Parallel()

	now := testclock.TestRecentTimeLocal

	t.Run("simple", shouldPass(HappenOnOrBefore(now)(now.Add(-time.Second))))
	t.Run("simple equal", shouldPass(HappenOnOrBefore(now)(now)))

	t.Run("simple false", shouldFail(HappenOnOrBefore(now)(now.Add(time.Second)), "Diff"))
}

func TestHappenAfter(t *testing.T) {
	t.Parallel()

	now := testclock.TestRecentTimeLocal

	t.Run("simple", shouldPass(HappenAfter(now)(now.Add(time.Second))))

	t.Run("simple equal", shouldFail(HappenAfter(now)(now), "equal"))
	t.Run("simple false", shouldFail(HappenAfter(now)(now.Add(-time.Second))))
}

func TestHappenOnOrAfter(t *testing.T) {
	t.Parallel()

	now := testclock.TestRecentTimeLocal

	t.Run("simple", shouldPass(HappenOnOrAfter(now)(now.Add(time.Second))))
	t.Run("simple equal", shouldPass(HappenOnOrAfter(now)(now)))

	t.Run("simple false", shouldFail(HappenOnOrAfter(now)(now.Add(-time.Second)), "Diff"))
}

func TestHappenOnOrBetween(t *testing.T) {
	now := testclock.TestRecentTimeLocal

	t.Run("simple", shouldPass(HappenOnOrBetween(now.Add(-time.Second), now.Add(time.Second))(now)))
	t.Run("simple equal", shouldPass(HappenOnOrBetween(now, now)(now)))

	later := now.Add(time.Second)
	t.Run("simple false", shouldFail(HappenOnOrBetween(now, later)(now.Add(-time.Second)), fmt.Sprintf("[%s, %s]", now, later)))

	t.Run("panic with bad bounds", func(t *testing.T) {
		mustPanicLike(t, "should.HappenOnOrBetween", func() {
			HappenOnOrBetween(now.Add(time.Second), now)
		})
	})
}

func TestHappenWithin(t *testing.T) {
	now := testclock.TestRecentTimeLocal

	t.Run("simple past", shouldPass(HappenWithin(time.Second, now.Add(-time.Second))(now)))
	t.Run("simple future", shouldPass(HappenWithin(time.Second, now)(now)))
	t.Run("simple future(2)", shouldPass(HappenWithin(time.Second, now)(now.Add(time.Second))))
	t.Run("simple equal", shouldPass(HappenWithin(0, now)(now)))

	t.Run("simple false", shouldFail(HappenWithin(time.Second, now)(now.Add(-2*time.Second)), "± 1s"))

	t.Run("panic with bad bounds", func(t *testing.T) {
		mustPanicLike(t, "should.HappenWithin", func() {
			HappenWithin(-time.Second, now)
		})
	})
}
