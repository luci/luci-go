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

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorkSet(t *testing.T) {
	t.Parallel()

	Convey("Smoke test", t, func() {
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
				So(processed[i], ShouldBeFalse) // no dups
				processed[i] = true
			}
			So(processed, ShouldHaveLength, 4*N) // finished everything
		}

		Convey("Single worker", func() {
			worker(context.Background())
			checkResult()
		})

		Convey("Many workers", func() {
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

	Convey("Many waiters", t, func() {
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
		So(item.(int), ShouldEqual, 0)
		close(startConsumers)
		done([]Item{1, 2, 3, 4, 5, 6, 7, 8})

		wg.Wait()

		close(results)
		processed := map[int]bool{}
		for i := range results {
			So(processed[i], ShouldBeFalse) // no dups
			processed[i] = true
		}
		So(len(processed), ShouldEqual, 8)
	})

	Convey("Context cancellation", t, func() {
		ws := New([]Item{0}, nil)

		item, done := ws.Pop(context.Background())
		So(item.(int), ShouldEqual, 0)

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
		So(<-got, ShouldBeNil) // unblocked and finished with nil

		// Make sure the workset is still functional.
		go func() {
			item, done := ws.Pop(context.Background())
			done(nil)
			got <- item
		}()

		done([]Item{111})
		So((<-got).(int), ShouldEqual, 111)
	})
}
