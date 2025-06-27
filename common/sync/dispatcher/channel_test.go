// Copyright 2019 The LUCI Authors.
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

package dispatcher

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func dummySendFn[T any](*buffer.Batch[T]) error { return nil }

func noDrop[T any](dropped *buffer.Batch[T], flush bool) {
	if flush {
		return
	}
	panic(fmt.Sprintf("dropping %+v", dropped))
}

func dbgIfVerbose(ctx context.Context) (context.Context, func(string, ...any)) {
	if testing.Verbose() {
		ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		return ctx, logging.Get(logging.SetField(ctx, "dispatcher.coordinator", true)).Infof
	}
	return ctx, func(string, ...any) {}
}

func TestChannelConstruction(t *testing.T) {
	ftt.Run(`Channel`, t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ctx, dbg := dbgIfVerbose(ctx)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		t.Run(`construction`, func(t *ftt.Test) {

			t.Run(`success`, func(t *ftt.Test) {
				ch, err := NewChannel(ctx, &Options[string]{testingDbg: dbg}, dummySendFn)
				assert.Loosely(t, err, should.BeNil)
				ch.Close()
				<-ch.DrainC
			})

			t.Run(`failure`, func(t *ftt.Test) {
				t.Run(`bad SendFn`, func(t *ftt.Test) {
					_, err := NewChannel[string](ctx, nil, nil)
					assert.Loosely(t, err, should.ErrLike("send is required"))
				})

				t.Run(`bad Options`, func(t *ftt.Test) {
					_, err := NewChannel(ctx, &Options[string]{
						QPSLimit: rate.NewLimiter(100, 0),
					}, dummySendFn)
					assert.Loosely(t, err, should.ErrLike("normalizing dispatcher.Options"))
				})

				t.Run(`bad Options.Buffer`, func(t *ftt.Test) {
					_, err := NewChannel(ctx, &Options[string]{
						Buffer: buffer.Options{
							BatchItemsMax: -3,
						},
					}, dummySendFn)
					assert.Loosely(t, err, should.ErrLike("allocating Buffer"))
				})
			})

		})

	})

}

func TestSerialSenderWithoutDrops(t *testing.T) {
	ftt.Run(`serial world-state sender without drops`, t, func(cvctx *ftt.Test) {
		ctx, tclock := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ctx, dbg := dbgIfVerbose(ctx)

		sentBatches := []string{}
		enableThisError := false

		ch, err := NewChannel(ctx, &Options[string]{
			DropFn:   noDrop[string],
			QPSLimit: rate.NewLimiter(rate.Inf, 0),
			Buffer: buffer.Options{
				MaxLeases:     1,
				BatchItemsMax: 1,
				FullBehavior:  &buffer.BlockNewItems{MaxItems: 10},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch[string]) (err error) {
			assert.Loosely(cvctx, batch.Data, should.HaveLength(1))
			str := batch.Data[0].Item
			if enableThisError && str == "This" {
				enableThisError = false
				return errors.New("narp", transient.Tag)
			}
			sentBatches = append(sentBatches, str)
			if str == "test." {
				defaultRetryAmount := buffer.Defaults.Retry().Next(ctx, nil)
				tclock.Set(tclock.Now().Add(defaultRetryAmount))
			}
			return nil
		})
		assert.Loosely(cvctx, err, should.BeNil)
		defer ch.CloseAndDrain(ctx)

		cvctx.Run(`no errors`, func(cvctx *ftt.Test) {
			ch.C <- "Hello"
			ch.C <- "World!"
			ch.C <- "This"
			ch.C <- "is"
			ch.C <- "a"
			ch.C <- "test."
			ch.CloseAndDrain(ctx)

			assert.Loosely(cvctx, sentBatches, should.Match([]string{
				"Hello", "World!",
				"This", "is", "a", "test.",
			}))
		})

		cvctx.Run(`error and retry`, func(cvctx *ftt.Test) {
			enableThisError = true

			ch.C <- "Hello"
			ch.C <- "World!"
			ch.C <- "This"
			ch.C <- "is"
			ch.C <- "a"
			ch.C <- "test."
			ch.CloseAndDrain(ctx)

			assert.Loosely(cvctx, sentBatches, should.Match([]string{
				"Hello", "World!",
				"is", "a", "test.", "This",
			}))
		})

	})
}

func TestContextShutdown(t *testing.T) {
	ftt.Run(`context cancelation ends channel`, t, func(cvctx *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ctx, dbg := dbgIfVerbose(ctx)
		cctx, cancel := context.WithCancel(ctx)

		sentBatches := []string{}
		droppedBatches := []string{}

		ch, err := NewChannel(cctx, &Options[any]{
			QPSLimit: rate.NewLimiter(rate.Inf, 0),
			DropFn: func(dropped *buffer.Batch[any], flush bool) {
				if flush {
					return
				}
				droppedBatches = append(droppedBatches, dropped.Data[0].Item.(string))
			},
			Buffer: buffer.Options{
				MaxLeases:     1,
				BatchItemsMax: 1,
				FullBehavior:  &buffer.BlockNewItems{MaxItems: 2},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch[any]) (err error) {
			sentBatches = append(sentBatches, batch.Data[0].Item.(string))
			<-cctx.Done()
			return
		})
		assert.Loosely(cvctx, err, should.BeNil)

		ch.C <- "hey"
		ch.C <- "buffered"
		select {
		case ch.C <- "blocked":
			panic("channel should have been blocked")
		case <-time.After(time.Millisecond):
			// OK
		}

		cancel()
		ch.C <- "IGNORE ME" // canceled channel can be written to, but is dropped

		ch.CloseAndDrain(ctx)

		assert.Loosely(cvctx, sentBatches, should.Contain("hey"))
		assert.Loosely(cvctx, droppedBatches, should.Contain("buffered"))
		assert.Loosely(cvctx, droppedBatches, should.Contain("IGNORE ME"))
	})
}

func TestQPSLimit(t *testing.T) {
	ftt.Run(`QPS limited send`, t, func(t *ftt.Test) {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)

		sentBatches := []int{}

		ch, err := NewChannel(ctx, &Options[int]{
			QPSLimit: rate.NewLimiter(rate.Every(10*time.Millisecond), 1),
			DropFn:   noDrop[int],
			Buffer: buffer.Options{
				MaxLeases:     1,
				BatchItemsMax: 1,
				FullBehavior:  &buffer.BlockNewItems{MaxItems: 20},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch[int]) (err error) {
			sentBatches = append(sentBatches, batch.Data[0].Item)
			return
		})
		assert.Loosely(t, err, should.BeNil)

		expected := []int{}

		start := time.Now()
		for i := range 20 {
			ch.C <- i
			expected = append(expected, i)
		}
		ch.CloseAndDrain(ctx)
		end := time.Now()

		assert.Loosely(t, sentBatches, should.Match(expected))

		// 20 batches, minus a batch because the QPSLimiter starts with full tokens.
		minThreshold := 19 * 10 * time.Millisecond
		assert.Loosely(t, end, should.HappenAfter(start.Add(minThreshold)))
	})
}

func TestQPSLimitParallel(t *testing.T) {
	ftt.Run(`QPS limited send (parallel)`, t, func(t *ftt.Test) {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)

		var lock sync.Mutex
		sentBatches := []int{}

		ch, err := NewChannel(ctx, &Options[int]{
			QPSLimit: rate.NewLimiter(rate.Every(10*time.Millisecond), 10),
			DropFn:   noDrop[int],
			Buffer: buffer.Options{
				MaxLeases:     4,
				BatchItemsMax: 1,
				FullBehavior:  &buffer.BlockNewItems{MaxItems: 20},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch[int]) (err error) {
			lock.Lock()
			sentBatches = append(sentBatches, batch.Data[0].Item)
			lock.Unlock()
			return
		})
		assert.Loosely(t, err, should.BeNil)

		start := time.Now()
		for i := range 20 {
			ch.C <- i
		}
		ch.CloseAndDrain(ctx)
		end := time.Now()

		// We know it should have 20 things, but the order will be pseudo-random
		assert.Loosely(t, sentBatches, should.HaveLength(20))

		// 20 batches across 4 workers, minus half a batch for sampling error.
		minThreshold := 5*10*time.Millisecond - 5*time.Millisecond

		assert.Loosely(t, end, should.HappenAfter(start.Add(minThreshold)))
	})
}

func TestExplicitDrops(t *testing.T) {
	ftt.Run(`explict drops with ErrorFn`, t, func(t *ftt.Test) {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)

		sentBatches := []int{}
		droppedBatches := []int{}

		ch, err := NewChannel(ctx, &Options[int]{
			QPSLimit: rate.NewLimiter(rate.Inf, 0),
			DropFn: func(batch *buffer.Batch[int], flush bool) {
				if flush {
					return
				}
				droppedBatches = append(droppedBatches, batch.Data[0].Item)
			},
			ErrorFn: func(batch *buffer.Batch[int], err error) (retry bool) {
				return false
			},
			Buffer: buffer.Options{
				MaxLeases:     1,
				BatchItemsMax: 1,
				FullBehavior:  &buffer.BlockNewItems{MaxItems: 20},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch[int]) (err error) {
			itm := batch.Data[0].Item
			if itm%2 == 0 {
				err = errors.New("number is even")
			} else {
				sentBatches = append(sentBatches, itm)
			}
			return
		})
		assert.Loosely(t, err, should.BeNil)

		for i := range 20 {
			ch.C <- i
		}
		ch.CloseAndDrain(ctx)

		assert.Loosely(t, sentBatches, should.Match([]int{1, 3, 5, 7, 9, 11, 13, 15, 17, 19}))
		assert.Loosely(t, droppedBatches, should.Match([]int{0, 2, 4, 6, 8, 10, 12, 14, 16, 18}))
	})
}

func TestImplicitDrops(t *testing.T) {
	ftt.Run(`implicit drops with DropOldestBatch`, t, func(cvctx *ftt.Test) {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)

		sentBatches := []int{}
		sendBlocker := make(chan struct{})

		limiter := rate.NewLimiter(rate.Every(100*time.Millisecond), 1)
		ch, err := NewChannel(ctx, &Options[int]{
			QPSLimit: limiter,
			Buffer: buffer.Options{
				MaxLeases:     1,
				BatchItemsMax: 1,
				FullBehavior:  &buffer.DropOldestBatch{MaxLiveItems: 1},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch[int]) (err error) {
			sentBatches = append(sentBatches, batch.Data[0].Item)
			<-sendBlocker
			return
		})
		assert.Loosely(cvctx, err, should.BeNil)
		// Grab the first token; channel can't send until it recharges.
		limiter.Reserve()

		// Stuff a bunch of crap into the channel. We have 100ms to do this until
		// the channel is able to send something. Should be plenty of time (running
		// this on my laptop takes 3-4ms with verbose logs).
		for i := range 20 {
			ch.C <- i
		}
		// At this point we can start draining the channel.
		close(ch.C)
		// then unblock the sender
		close(sendBlocker)
		// Then wait for the channel to drain
		<-ch.DrainC

		// We should only have seen one batch actually sent.
		assert.Loosely(cvctx, sentBatches, should.HaveLength(1))
	})
}

func TestContextCancel(t *testing.T) {
	ftt.Run(`can use context cancelation for termination`, t, func(t *ftt.Test) {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		ch, err := NewChannel(ctx, &Options[any]{
			QPSLimit: rate.NewLimiter(rate.Inf, 0),
			Buffer: buffer.Options{
				MaxLeases:     1,
				BatchItemsMax: 1,
				FullBehavior:  &buffer.BlockNewItems{MaxItems: 20},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch[any]) (err error) {
			// doesn't matter :)
			return
		})
		assert.Loosely(t, err, should.BeNil)

		writerDone := make(chan struct{})
		go func() {
			defer close(writerDone)
			i := 0
			for {
				select {
				case ch.C <- i:
				case <-ctx.Done():
					return
				}
				i++
			}
		}()
		cancel()

		<-writerDone

		close(ch.C) // still responsible for closing C
		<-ch.DrainC // everything shuts down now
	})
}

func TestDrainedFn(t *testing.T) {
	ftt.Run(`can set DrainedFn to do exactly-once termination tasks`, t, func(t *ftt.Test) {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		amDrained := false

		ch, err := NewChannel(ctx, &Options[any]{
			DrainedFn:  func() { amDrained = true },
			testingDbg: dbg,
		}, func(batch *buffer.Batch[any]) (err error) {
			// doesn't matter :)
			return
		})
		assert.Loosely(t, err, should.BeNil)

		ch.Close()
		<-ch.DrainC
		assert.Loosely(t, amDrained, should.BeTrue)
	})
}

func TestCloseDeadlockRegression(t *testing.T) {
	// This is a regression test for crbug.com/1006623
	//
	// A single run of the test, even with the broken code, doesn't reliably
	// reproduce it. However, running the test ~10 times seems to be VERY likely
	// to catch the deadlock at least once. We could make the test 100% likely to
	// catch the race, but it would involve adding extra synchronization channels
	// to the production code, which makes us nervous :).
	//
	// This code should never hang if the coordinator code is correct.
	for i := range 10 {
		ftt.Run(fmt.Sprintf(`ensure that the channel can shutdown cleanly (%d)`, i), t, func(t *ftt.Test) {
			ctx := context.Background() // uses real time!
			ctx, dbg := dbgIfVerbose(ctx)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			inSendFn := make(chan struct{})
			holdSendFn := make(chan struct{})

			ch, err := NewChannel(ctx, &Options[any]{
				testingDbg: dbg,
				Buffer: buffer.Options{
					MaxLeases:     1,
					BatchItemsMax: 1,
					FullBehavior: &buffer.DropOldestBatch{
						MaxLiveItems: 1,
					},
				},
				QPSLimit: rate.NewLimiter(rate.Inf, 1),
			}, func(batch *buffer.Batch[any]) (err error) {
				inSendFn <- struct{}{}
				<-holdSendFn
				return
			})
			assert.Loosely(t, err, should.BeNil)

			ch.C <- nil
			// Now ensure we're in the send function
			<-inSendFn

			ch.C <- nil // this will go into UnleasedItemCount

			// While still in the send function, cancel the context and close the
			// channel.
			cancel()
			ch.Close()

			// Now unblock the send function
			close(holdSendFn)

			// We should drain properly
			<-ch.DrainC
		})
	}
}

func TestCorrectTimerUsage(t *testing.T) {
	t.Parallel()

	ftt.Run(`Correct use of Timer.Reset`, t, func(cvctx *ftt.Test) {
		ctx, tclock := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ctx, dbg := dbgIfVerbose(ctx)
		tclock.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			switch {
			case testclock.HasTags(t, "coordinator") || testclock.HasTags(t, "test-itself"):
				logging.Debugf(ctx, "unblocking %s", testclock.GetTags(t))
				tclock.Add(d)
			}
		})

		mu := sync.Mutex{}
		sent := []int{}

		ch, err := NewChannel(ctx, &Options[int]{
			DropFn: noDrop[int],
			Buffer: buffer.Options{
				MaxLeases:     10,
				BatchItemsMax: 3,
				BatchAgeMax:   time.Second,
				FullBehavior:  &buffer.BlockNewItems{MaxItems: 15},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch[int]) (err error) {
			// Add randomish delays.
			timer := clock.NewTimer(clock.Tag(ctx, "test-itself"))
			timer.Reset(time.Millisecond)
			<-timer.GetC()

			mu.Lock()
			for i := range batch.Data {
				sent = append(sent, batch.Data[i].Item)
			}
			mu.Unlock()
			return nil
		})
		assert.Loosely(cvctx, err, should.BeNil)

		const N = 100
		for i := 1; i <= N; i++ {
			ch.C <- i
		}
		// Must not hang when tried with
		//     go test -race -run TestCorrectTimerUsage -failfast -count 1000 -timeout 20s
		//
		// NOTE: there may be failure not due to a deadlock, but due to garbage
		// collection taking too long, after lots of iterations. You can either
		// examine the stack traces or bump the timeout and observe if it increases
		// the number of iterations before failure.
		ch.CloseAndDrain(ctx)
		assert.Loosely(cvctx, sent, should.HaveLength(N))
		sort.Ints(sent)
		for i := 1; i <= N; i++ {
			assert.Loosely(cvctx, sent[i-1], should.Equal(i))
		}
	})
}

func TestSizeBasedChannel(t *testing.T) {
	t.Parallel()

	ftt.Run(`Size based channel`, t, func(cvctx *ftt.Test) {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		var mu sync.Mutex
		var needUnlock bool
		defer func() {
			if needUnlock {
				mu.Unlock()
			}
		}()
		var out []string
		var fails []*buffer.Batch[string]
		var errs []error

		opts := &Options[string]{
			testingDbg: dbg,
			ItemSizeFunc: func(itm string) int {
				return len(itm)
			},
			ErrorFn: func(failedBatch *buffer.Batch[string], err error) (retry bool) {
				fails = append(fails, failedBatch)
				errs = append(errs, err)
				return false
			},
			Buffer: buffer.Options{
				MaxLeases:     1,
				BatchItemsMax: -1,
				BatchSizeMax:  100,
				FullBehavior:  &buffer.BlockNewItems{},
			},
			QPSLimit: rate.NewLimiter(rate.Inf, 1),
		}

		ch, err := NewChannel(ctx, opts, func(batch *buffer.Batch[string]) (err error) {
			mu.Lock()
			defer mu.Unlock()
			for _, itm := range batch.Data {
				out = append(out, itm.Item)
			}
			return nil
		})
		assert.Loosely(cvctx, err, should.BeNil)

		bigString := strings.Repeat("something.", 5) // 50 bytes

		mu.Lock()
		needUnlock = true

		for range 10 {
			ch.C <- bigString
		}

		select {
		case ch.C <- "extra string":
			assert.Loosely(cvctx, true, should.BeFalse) // shouldn't be able to push more
		case <-clock.After(ctx, 250*time.Millisecond):
		}

		mu.Unlock()
		needUnlock = false

		select {
		case ch.C <- "extra string": // no problem now
		case <-clock.After(ctx, 250*time.Millisecond):
			assert.Loosely(cvctx, true, should.BeFalse)
		}

		// pushing a giant object in will end up going to ErrorFn
		ch.C <- strings.Repeat("something.", 50) // 500 bytes
		// pushing an empty object (without having ItemSizeFunc assign it
		// a non-zero arbitrary size) goes to ErrorFn.
		ch.C <- ""

		ch.CloseAndDrain(ctx)

		assert.Loosely(cvctx, fails, should.HaveLength(2))
		assert.Loosely(cvctx, fails[0].Data, should.HaveLength(1))
		assert.Loosely(cvctx, fails[0].Data[0].Item, should.HaveLength(500))
		assert.Loosely(cvctx, fails[0].Data[0].Size, should.Equal(500))
		assert.Loosely(cvctx, fails[1].Data, should.HaveLength(1))
		assert.Loosely(cvctx, fails[1].Data[0].Item, should.HaveLength(0))
		assert.Loosely(cvctx, fails[1].Data[0].Size, should.BeZero)
		assert.Loosely(cvctx, errs[0], should.ErrLike(buffer.ErrItemTooLarge))
		assert.Loosely(cvctx, errs[1], should.ErrLike(buffer.ErrItemTooSmall))

		assert.Loosely(cvctx, out, should.HaveLength(11))
		assert.Loosely(cvctx, out[len(out)-1], should.Match("extra string"))
	})
}

func TestMinQPS(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestMinQPS`, t, func(t *ftt.Test) {
		t.Run(`send w/ minimal frequency`, func(t *ftt.Test) {
			ctx := context.Background() // uses real time!
			ctx, dbg := dbgIfVerbose(ctx)

			numSyntheticBatches := 0

			ch, err := NewChannel(ctx, &Options[int]{
				MinQPS: rate.Every(100 * time.Millisecond),
				DropFn: noDrop[int],
				Buffer: buffer.Options{
					MaxLeases:     1,
					BatchItemsMax: 1,
					FullBehavior:  &buffer.BlockNewItems{MaxItems: 20},
				},
				testingDbg: dbg,
			}, func(batch *buffer.Batch[int]) (err error) {
				if batch.Data[0].Synthetic {
					numSyntheticBatches++
				}
				return
			})
			assert.Loosely(t, err, should.BeNil)

			for i := range 20 {
				switch i {
				case 9:
					time.Sleep(2 * time.Second) // to make a gap that ch is empty.
				}
				ch.C <- i
			}
			ch.CloseAndDrain(ctx)
			assert.Loosely(t, numSyntheticBatches, should.BeGreaterThan(0))
		})

		t.Run(`send w/ minimal frequency non block`, func(t *ftt.Test) {
			ctx := context.Background() // uses real time!
			ctx, dbg := dbgIfVerbose(ctx)

			mu := sync.Mutex{}
			numSyntheticBatches := 0

			ch, err := NewChannel(ctx, &Options[int]{
				MinQPS: rate.Every(100 * time.Millisecond),
				DropFn: noDrop[int],
				Buffer: buffer.Options{
					MaxLeases:     1,
					BatchItemsMax: 1,
					FullBehavior:  &buffer.BlockNewItems{MaxItems: 20},
				},
				testingDbg: dbg,
			}, func(batch *buffer.Batch[int]) (err error) {
				mu.Lock()
				if batch.Data[0].Synthetic {
					numSyntheticBatches++
				}
				mu.Unlock()
				time.Sleep(200 * time.Millisecond)
				return
			})
			assert.Loosely(t, err, should.BeNil)

			for i := range 20 {
				ch.C <- i
			}
			ch.CloseAndDrain(ctx)

			assert.Loosely(t, numSyntheticBatches, should.BeZero)
		})

		t.Run(`regression: ensure clean shutdown`, func(t *ftt.Test) {
			// In go.dev/issue/69276 we saw that the following could occur:
			//   * A Channel configured with MinQPS was getting MinQPS pings
			//   * Program closed resultCh after sending the final message
			//   * Program panic'd due to send of MinQPS batch

			ctx := context.Background() // uses real time!
			ctx, dbg := dbgIfVerbose(ctx)

			for range 100 {
				ch, err := NewChannel(ctx, &Options[int]{
					MinQPS: rate.Every(time.Nanosecond),
					DropFn: noDrop[int],
					Buffer: buffer.Options{
						MaxLeases: 1,
					},
					testingDbg: dbg,
				}, func(data *buffer.Batch[int]) error {
					time.Sleep(time.Duration(rand.N(50)) * time.Nanosecond)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				time.Sleep(5 * time.Millisecond)
				ch.CloseAndDrain(ctx)
			}
		})
	})
}

// TestProcess tests writing to a channel from one goroutine and reading it from
// another.
func TestProcess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ch := make(chan int)
	go func() {
		for i := 1000; i < 2000; i++ {
			ch <- i
			t.Logf("send: %d\n", i)
		}
		close(ch)
	}()

	err := Process(ctx, ch, nil, func(buf *buffer.Batch[int]) error {
		for i, item := range buf.Data {
			t.Logf("receive: %d (%d of %d in batch)\n", item.Item, i, len(buf.Data))
		}
		return nil
	})

	if err != nil {
		t.Error(err)
	}
}

// TestNilSafety tests that calling Close or CloseDrain safely does nothing on
// a nil channel.
//
// If this test "fails", then an exception will be thrown, so we don't have an
// explicit test.
func TestNilSafety(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var zero Channel[bool]
	zero.Close()
	zero.CloseAndDrain(ctx)

	var nilPtr *Channel[bool]
	nilPtr.Close()
	nilPtr.CloseAndDrain(ctx)
}
