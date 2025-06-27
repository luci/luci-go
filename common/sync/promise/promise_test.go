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

package promise

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

func TestPromise(t *testing.T) {
	t.Parallel()

	ftt.Run(`An instrumented Promise instance`, t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))

		type operation struct {
			d any
			e error
		}

		opC := make(chan operation)
		p := New(ctx, func(context.Context) (any, error) {
			op := <-opC
			return op.d, op.e
		})

		t.Run(`Will timeout with no data.`, func(t *ftt.Test) {
			// Wait until our Promise starts its timer. Then signal it.
			readyC := make(chan struct{})
			tc.SetTimerCallback(func(_ time.Duration, _ clock.Timer) {
				close(readyC)
			})
			go func() {
				<-readyC
				tc.Add(1 * time.Second)
			}()

			ctx, cancel := clock.WithTimeout(ctx, 1*time.Second)
			defer cancel()
			data, err := p.Get(ctx)
			assert.Loosely(t, data, should.BeNil)
			assert.Loosely(t, err.Error(), should.Equal(context.DeadlineExceeded.Error()))
		})

		t.Run(`With data already added`, func(t *ftt.Test) {
			e := errors.New("promise: fake test error")
			opC <- operation{"DATA", e}

			data, err := p.Get(ctx)
			assert.Loosely(t, data, should.Equal("DATA"))
			assert.Loosely(t, err, should.Equal(e))

			t.Run(`Will return data instead of timing out.`, func(t *ftt.Test) {
				ctx, cancelFunc := context.WithCancel(ctx)
				cancelFunc()

				data, err = p.Get(ctx)
				assert.Loosely(t, data, should.Equal("DATA"))
				assert.Loosely(t, err, should.Equal(e))
			})
		})
	})
}

func TestDeferredPromise(t *testing.T) {
	t.Parallel()

	ftt.Run(`A deferred Promise instance`, t, func(t *ftt.Test) {
		c := context.Background()

		t.Run(`Will defer running until Get is called, and will panic the Get goroutine.`, func(t *ftt.Test) {
			// Since our Get will cause the generator to be run in this goroutine,
			// calling Get with a generator that panics should cause a panic.
			p := NewDeferred(func(context.Context) (any, error) { panic("test panic") })
			assert.Loosely(t, func() { p.Get(c) }, should.Panic)
		})

		t.Run(`Can output data.`, func(t *ftt.Test) {
			p := NewDeferred(func(context.Context) (any, error) {
				return "hello", nil
			})
			v, err := p.Get(c)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, v, should.Equal("hello"))
		})
	})
}

func TestPromiseSmoke(t *testing.T) {
	t.Parallel()

	ftt.Run(`A Promise instance with multiple consumers will block.`, t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		dataC := make(chan any)
		p := New(ctx, func(context.Context) (any, error) {
			return <-dataC, nil
		})

		finishedC := make(chan string)
		for range uint(100) {
			go func() {
				data, _ := p.Get(ctx)
				finishedC <- data.(string)
			}()
			go func() {
				ctx, cancel := clock.WithTimeout(ctx, 1*time.Hour)
				defer cancel()
				data, _ := p.Get(ctx)
				finishedC <- data.(string)
			}()
		}

		dataC <- "DATA"
		for range uint(200) {
			assert.Loosely(t, <-finishedC, should.Equal("DATA"))
		}
	})
}
