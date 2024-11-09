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

package workset

import (
	"context"
	"sync"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestWorkSet(t *testing.T) {
	t.Parallel()

	ftt.Run("Smoke test", t, func(t *ftt.Test) {
		const N = 100

		items := []Item{}
		for i := 0; i < N; i++ {
			items = append(items, i)
		}
		ws := New(items, nil)

		results := make(chan int, 1000)

		// Worker takes a number K and
		//   If K < N: produces two numbers 2*K+N and 2*K+N+1.
		//   If N <= K < 2*N: produces a number K+2*N.
		// As a result, when everything is done we should have processed
		// a continuous range [0, 4*N).
		worker := func(ctx context.Context) {
			for {
				item, done := ws.Pop(ctx)
				if item == nil {
					return
				}
				val := item.(int)
				switch {
				case val < N:
					done([]Item{val*2 + N, val*2 + N + 1})
				case val < 2*N:
					done([]Item{val + 2*N})
				default:
					done(nil) // no new work
				}
				results <- val
			}
		}

		checkResult := func() {
			close(results)
			processed := map[int]bool{}
			for i := range results {
				assert.Loosely(t, processed[i], should.BeFalse) // no dups
				processed[i] = true
			}
			assert.Loosely(t, processed, should.HaveLength(4*N)) // finished everything
		}

		t.Run("Single worker", func(t *ftt.Test) {
			worker(context.Background())
			checkResult()
		})

		t.Run("Many workers", func(t *ftt.Test) {
			wg := sync.WaitGroup{}
			wg.Add(5)
			for i := 0; i < 5; i++ {
				go func() {
					defer wg.Done()
					worker(context.Background())
				}()
			}
			wg.Wait()
			checkResult()
		})
	})

	ftt.Run("Many waiters", t, func(t *ftt.Test) {
		results := make(chan int, 1000)

		ws := New([]Item{0}, nil)

		startConsumers := make(chan struct{})

		// Consumer goroutines.
		wg := sync.WaitGroup{}
		wg.Add(5)
		for i := 0; i < 5; i++ {
			go func() {
				defer wg.Done()
				<-startConsumers
				for {
					item, done := ws.Pop(context.Background())
					if item == nil {
						return
					}
					results <- item.(int)
					done(nil)
				}
			}()
		}

		// The producer.
		item, done := ws.Pop(context.Background())
		assert.Loosely(t, item.(int), should.BeZero)
		close(startConsumers)
		done([]Item{1, 2, 3, 4, 5, 6, 7, 8})

		wg.Wait()

		close(results)
		processed := map[int]bool{}
		for i := range results {
			assert.Loosely(t, processed[i], should.BeFalse) // no dups
			processed[i] = true
		}
		assert.Loosely(t, len(processed), should.Equal(8))
	})

	ftt.Run("Context cancellation", t, func(t *ftt.Test) {
		ws := New([]Item{0}, nil)

		item, done := ws.Pop(context.Background())
		assert.Loosely(t, item.(int), should.BeZero)

		got := make(chan Item, 1)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			item, done := ws.Pop(ctx)
			if done != nil {
				panic("should be nil")
			}
			got <- item
		}()

		// The gorouting is stuck waiting until there are new items in the set (i.e.
		// `done` is called) or the context is canceled.
		cancel()
		assert.Loosely(t, <-got, should.BeNil) // unblocked and finished with nil

		// Make sure the workset is still functional.
		go func() {
			item, done := ws.Pop(context.Background())
			done(nil)
			got <- item
		}()

		done([]Item{111})
		assert.Loosely(t, (<-got).(int), should.Equal(111))
	})
}
