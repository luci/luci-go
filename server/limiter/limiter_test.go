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

package limiter

import (
	"context"
	"sync"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
)

func TestMaxConcurrencyLimit(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		const limiterName = "test-limiter"
		const maxConcurrent = 5
		const allConcurrent = 12

		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		block := make(chan struct{}) // stalls all requests
		wg := &sync.WaitGroup{}      // waits until all requests are done

		t.Run("In enforcing mode", func(t *ftt.Test) {
			l, _ := New(Options{
				Name:                  limiterName,
				MaxConcurrentRequests: maxConcurrent,
			})

			accepted, rejected := makeConcurrentRequests(ctx, t, l, allConcurrent, block, wg)
			assert.Loosely(t, accepted, should.Equal(maxConcurrent))
			assert.Loosely(t, rejected, should.Equal(allConcurrent-maxConcurrent))

			// There are still maxConcurrent requests blocked. Also the rest of the
			// requests were already rejected. Verify metrics reflect all that.
			l.ReportMetrics(ctx)
			assert.Loosely(t, concurrencyCurGauge.Get(ctx, limiterName), should.Equal(maxConcurrent))
			assert.Loosely(t, concurrencyMaxGauge.Get(ctx, limiterName), should.Equal(maxConcurrent))
			assert.Loosely(t, rejectedCounter.Get(ctx, limiterName, "call", "peer", "max concurrency"), should.Equal(allConcurrent-maxConcurrent))

			// Unblock pending requests.
			close(block)
			wg.Wait()

			// Metrics show there are no concurrent requests anymore.
			l.ReportMetrics(ctx)
			assert.Loosely(t, concurrencyCurGauge.Get(ctx, limiterName), should.BeZero)
		})

		t.Run("In advisory mode", func(t *ftt.Test) {
			l, _ := New(Options{
				Name:                  limiterName,
				MaxConcurrentRequests: maxConcurrent,
				AdvisoryMode:          true,
			})

			// All requests are actually accepted.
			accepted, rejected := makeConcurrentRequests(ctx, t, l, allConcurrent, block, wg)
			assert.Loosely(t, accepted, should.Equal(allConcurrent))
			assert.Loosely(t, rejected, should.BeZero)

			// But metrics reflect that some requests should have been rejected if
			// not running in the advisory mode. Also concurrencyCurGauge reflects
			// the reality (all allConcurrent requests are executing now).
			l.ReportMetrics(ctx)
			assert.Loosely(t, concurrencyCurGauge.Get(ctx, limiterName), should.Equal(allConcurrent))
			assert.Loosely(t, concurrencyMaxGauge.Get(ctx, limiterName), should.Equal(maxConcurrent))
			assert.Loosely(t, rejectedCounter.Get(ctx, limiterName, "call", "peer", "max concurrency"), should.Equal(allConcurrent-maxConcurrent))

			// Unblock pending requests.
			close(block)
			wg.Wait()
		})
	})
}

func makeConcurrentRequests(ctx context.Context, t testing.TB, l *Limiter, count int, block chan struct{}, wg *sync.WaitGroup) (accepted, rejected int) {
	t.Helper()
	verdicts := make(chan error) // nil if accepted, non-nil if rejected

	// Note: this test tries to simulate real server environment where calls to
	// CheckRequest happen from multiple goroutines.
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			done, err := l.CheckRequest(ctx, &RequestInfo{
				CallLabel: "call",
				PeerLabel: "peer",
			})
			if err != nil {
				verdicts <- err
				return
			}
			verdicts <- nil
			<-block
			defer done()
		}()
	}

	// Collect the verdicts.
	for i := 0; i < count; i++ {
		err := <-verdicts
		if err == nil {
			accepted++
		} else {
			assert.Loosely(t, err, should.ErrLike("max concurrency limit: the server limit reached"), truth.LineContext())
			rejected++
		}
	}
	return
}
