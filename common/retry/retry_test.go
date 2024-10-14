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

package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// testIterator is an Iterator implementation used for testing.
type testIterator struct {
	total int
	count int
}

func (i *testIterator) Next(_ context.Context, _ error) time.Duration {
	defer func() { i.count++ }()
	if i.count >= i.total {
		return Stop
	}
	return time.Second
}

func TestRetry(t *testing.T) {
	t.Parallel()

	// Generic test failure.
	failure := errors.New("retry: test error")

	ftt.Run(`A testing function`, t, func(t *ftt.Test) {
		ctx, c := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))

		// Every time we sleep, update time by one second and count.
		sleeps := 0
		c.SetTimerCallback(func(time.Duration, clock.Timer) {
			c.Add(1 * time.Second)
			sleeps++
		})

		t.Run(`A test Iterator with three retries`, func(t *ftt.Test) {
			g := func() Iterator {
				return &testIterator{total: 3}
			}

			t.Run(`Executes a successful function once.`, func(t *ftt.Test) {
				var count, callbacks int
				err := Retry(ctx, g, func() error {
					count++
					return nil
				}, func(error, time.Duration) {
					callbacks++
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(1))
				assert.Loosely(t, callbacks, should.BeZero)
				assert.Loosely(t, sleeps, should.BeZero)
			})

			t.Run(`Executes a failing function three times.`, func(t *ftt.Test) {
				var count, callbacks int
				err := Retry(ctx, g, func() error {
					count++
					return failure
				}, func(error, time.Duration) {
					callbacks++
				})
				assert.Loosely(t, err, should.Equal(failure))
				assert.Loosely(t, count, should.Equal(4))
				assert.Loosely(t, callbacks, should.Equal(3))
				assert.Loosely(t, sleeps, should.Equal(3))
			})

			t.Run(`Executes a function that fails once, then succeeds once.`, func(t *ftt.Test) {
				var count, callbacks int
				err := Retry(ctx, g, func() error {
					defer func() { count++ }()
					if count == 0 {
						return errors.New("retry: test error")
					}
					return nil
				}, func(error, time.Duration) {
					callbacks++
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(2))
				assert.Loosely(t, callbacks, should.Equal(1))
				assert.Loosely(t, sleeps, should.Equal(1))
			})

			t.Run(`Does not retry if context is done.`, func(t *ftt.Test) {
				ctx, cancel := context.WithCancel(ctx)
				var count, callbacks int
				err := Retry(ctx, g, func() error {
					cancel()
					count++
					return failure
				}, func(error, time.Duration) {
					callbacks++
				})
				assert.Loosely(t, err, should.Equal(failure))
				assert.Loosely(t, count, should.Equal(1))
				assert.Loosely(t, callbacks, should.BeZero)
				assert.Loosely(t, sleeps, should.BeZero)
			})
		})

		t.Run(`Does not retry if callback is not set.`, func(t *ftt.Test) {
			assert.Loosely(t, Retry(ctx, nil, func() error {
				return failure
			}, nil), should.Equal(failure))
			assert.Loosely(t, sleeps, should.BeZero)
		})
	})
}
