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

// manualClock is a partial Clock implementation that allows us to release
// blocking calls.
type manualClock struct {
	Clock

	now             time.Time
	timeoutCallback func(time.Duration) bool
	testFinishedC   chan struct{}
}

func (mc *manualClock) Now() time.Time {
	return mc.now
}

func (mc *manualClock) NewTimer(ctx context.Context) Timer {
	return &manualTimer{
		ctx:     ctx,
		mc:      mc,
		resultC: make(chan TimerResult),
	}
}

type manualTimer struct {
	Timer

	ctx     context.Context
	mc      *manualClock
	resultC chan TimerResult

	running bool
	stopC   chan struct{}
}

func (mt *manualTimer) GetC() <-chan TimerResult { return mt.resultC }

func (mt *manualTimer) Reset(d time.Duration) bool {
	running := mt.Stop()
	mt.stopC, mt.running = make(chan struct{}), true

	go func() {
		ar := TimerResult{}
		defer func() {
			mt.resultC <- ar
		}()

		// If we are instructed to immediately timeout, do so.
		if cb := mt.mc.timeoutCallback; cb != nil && cb(d) {
			return
		}

		select {
		case <-mt.ctx.Done():
			ar.Err = mt.ctx.Err()
		case <-mt.mc.testFinishedC:
			break
		}
	}()
	return running
}

func (mt *manualTimer) Stop() bool {
	if !mt.running {
		return false
	}

	mt.running = false
	close(mt.stopC)
	return true
}

func wait(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestClockContext(t *testing.T) {
	t.Parallel()

	ftt.Run(`A manual testing clock`, t, func(t *ftt.Test) {
		mc := manualClock{
			now:           time.Date(2016, 1, 1, 0, 0, 0, 0, time.Local),
			testFinishedC: make(chan struct{}),
		}
		defer close(mc.testFinishedC)

		t.Run(`A context with a deadline wrapping a cancellable parent`, func(t *ftt.Test) {
			t.Run(`Successfully reports its deadline.`, func(t *ftt.Test) {
				ctx, cancel := WithTimeout(Set(context.Background(), &mc), 10*time.Millisecond)
				defer cancel()

				deadline, ok := ctx.Deadline()
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, deadline.After(mc.now), should.BeTrue)
			})

			t.Run(`Will successfully time out.`, func(t *ftt.Test) {
				mc.timeoutCallback = func(time.Duration) bool {
					return true
				}

				cctx, cancel := context.WithCancel(Set(context.Background(), &mc))
				defer cancel()
				ctx, cancel := WithTimeout(cctx, 10*time.Millisecond)
				defer cancel()
				assert.Loosely(t, wait(ctx).Error(), should.Equal(context.DeadlineExceeded.Error()))
			})

			t.Run(`Will successfully cancel with its cancel func.`, func(t *ftt.Test) {
				cctx, cancel := context.WithCancel(Set(context.Background(), &mc))
				defer cancel()
				ctx, cf := WithTimeout(cctx, 10*time.Millisecond)
				go cf()
				assert.Loosely(t, wait(ctx), should.Equal(context.Canceled))
			})

			t.Run(`Will successfully cancel if the parent is canceled.`, func(t *ftt.Test) {
				cctx, pcf := context.WithCancel(Set(context.Background(), &mc))
				ctx, cancel := WithTimeout(cctx, 10*time.Millisecond)
				defer cancel()
				go pcf()
				assert.Loosely(t, wait(ctx), should.Equal(context.Canceled))
			})
		})

		t.Run(`A context with a deadline wrapping a parent with a shorter deadline`, func(t *ftt.Test) {
			cctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			ctx, cf := WithTimeout(cctx, 1*time.Hour)
			defer cf()

			t.Run(`Will successfully time out.`, func(t *ftt.Test) {
				mc.timeoutCallback = func(d time.Duration) bool {
					return d == 10*time.Millisecond
				}

				assert.Loosely(t, wait(ctx).Error(), should.Equal(context.DeadlineExceeded.Error()))
			})

			t.Run(`Will successfully cancel with its cancel func.`, func(t *ftt.Test) {
				go cf()
				assert.Loosely(t, wait(ctx), should.Equal(context.Canceled))
			})
		})

		t.Run(`A context with a deadline in the past`, func(t *ftt.Test) {
			ctx, _ := WithDeadline(context.Background(), mc.now.Add(-time.Second))

			t.Run(`Will time out immediately.`, func(t *ftt.Test) {
				assert.Loosely(t, wait(ctx).Error(), should.Equal(context.DeadlineExceeded.Error()))
			})
		})
	})
}
