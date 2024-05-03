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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
	Convey(`Channel`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ctx, dbg := dbgIfVerbose(ctx)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		Convey(`construction`, func() {

			Convey(`success`, func() {
				ch, err := NewChannel[string](ctx, &Options[string]{testingDbg: dbg}, dummySendFn)
				So(err, ShouldBeNil)
				ch.Close()
				<-ch.DrainC
			})

			Convey(`failure`, func() {
				Convey(`bad SendFn`, func() {
					_, err := NewChannel[string](ctx, nil, nil)
					So(err, ShouldErrLike, "send is required")
				})

				Convey(`bad Options`, func() {
					_, err := NewChannel[string](ctx, &Options[string]{
						QPSLimit: rate.NewLimiter(100, 0),
					}, dummySendFn)
					So(err, ShouldErrLike, "normalizing dispatcher.Options")
				})

				Convey(`bad Options.Buffer`, func() {
					_, err := NewChannel[string](ctx, &Options[string]{
						Buffer: buffer.Options{
							BatchItemsMax: -3,
						},
					}, dummySendFn)
					So(err, ShouldErrLike, "allocating Buffer")
				})
			})

		})

	})

}

func TestSerialSenderWithoutDrops(t *testing.T) {
	Convey(`serial world-state sender without drops`, t, func(cvctx C) {
		ctx, tclock := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ctx, dbg := dbgIfVerbose(ctx)

		sentBatches := []string{}
		enableThisError := false

		ch, err := NewChannel[string](ctx, &Options[string]{
			DropFn:   noDrop[string],
			QPSLimit: rate.NewLimiter(rate.Inf, 0),
			Buffer: buffer.Options{
				MaxLeases:     1,
				BatchItemsMax: 1,
				FullBehavior:  &buffer.BlockNewItems{MaxItems: 10},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch[string]) (err error) {
			cvctx.So(batch.Data, ShouldHaveLength, 1)
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
		So(err, ShouldBeNil)
		defer ch.CloseAndDrain(ctx)

		Convey(`no errors`, func() {
			ch.C <- "Hello"
			ch.C <- "World!"
			ch.C <- "This"
			ch.C <- "is"
			ch.C <- "a"
			ch.C <- "test."
			ch.CloseAndDrain(ctx)

			So(sentBatches, ShouldResemble, []string{
				"Hello", "World!",
				"This", "is", "a", "test.",
			})
		})

		Convey(`error and retry`, func() {
			enableThisError = true

			ch.C <- "Hello"
			ch.C <- "World!"
			ch.C <- "This"
			ch.C <- "is"
			ch.C <- "a"
			ch.C <- "test."
			ch.CloseAndDrain(ctx)

			So(sentBatches, ShouldResemble, []string{
				"Hello", "World!",
				"is", "a", "test.", "This",
			})
		})

	})
}

func TestContextShutdown(t *testing.T) {
	Convey(`context cancelation ends channel`, t, func(cvctx C) {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ctx, dbg := dbgIfVerbose(ctx)
		cctx, cancel := context.WithCancel(ctx)

		sentBatches := []string{}
		droppedBatches := []string{}

		ch, err := NewChannel[any](cctx, &Options[any]{
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
		So(err, ShouldBeNil)

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

		So(sentBatches, ShouldContain, "hey")
		So(droppedBatches, ShouldContain, "buffered")
		So(droppedBatches, ShouldContain, "IGNORE ME")
	})
}

func TestQPSLimit(t *testing.T) {
	Convey(`QPS limited send`, t, func() {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)

		sentBatches := []int{}

		ch, err := NewChannel[int](ctx, &Options[int]{
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
		So(err, ShouldBeNil)

		expected := []int{}

		start := time.Now()
		for i := 0; i < 20; i++ {
			ch.C <- i
			expected = append(expected, i)
		}
		ch.CloseAndDrain(ctx)
		end := time.Now()

		So(sentBatches, ShouldResemble, expected)

		// 20 batches, minus a batch because the QPSLimiter starts with full tokens.
		minThreshold := 19 * 10 * time.Millisecond
		So(end, ShouldHappenAfter, start.Add(minThreshold))
	})
}

func TestQPSLimitParallel(t *testing.T) {
	Convey(`QPS limited send (parallel)`, t, func() {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)

		var lock sync.Mutex
		sentBatches := []int{}

		ch, err := NewChannel[int](ctx, &Options[int]{
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
		So(err, ShouldBeNil)

		start := time.Now()
		for i := 0; i < 20; i++ {
			ch.C <- i
		}
		ch.CloseAndDrain(ctx)
		end := time.Now()

		// We know it should have 20 things, but the order will be pseudo-random
		So(sentBatches, ShouldHaveLength, 20)

		// 20 batches across 4 workers, minus half a batch for sampling error.
		minThreshold := 5*10*time.Millisecond - 5*time.Millisecond

		So(end, ShouldHappenAfter, start.Add(minThreshold))
	})
}

func TestExplicitDrops(t *testing.T) {
	Convey(`explict drops with ErrorFn`, t, func() {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)

		sentBatches := []int{}
		droppedBatches := []int{}

		ch, err := NewChannel[int](ctx, &Options[int]{
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
		So(err, ShouldBeNil)

		for i := 0; i < 20; i++ {
			ch.C <- i
		}
		ch.CloseAndDrain(ctx)

		So(sentBatches, ShouldResemble, []int{1, 3, 5, 7, 9, 11, 13, 15, 17, 19})
		So(droppedBatches, ShouldResemble, []int{0, 2, 4, 6, 8, 10, 12, 14, 16, 18})
	})
}

func TestImplicitDrops(t *testing.T) {
	Convey(`implicit drops with DropOldestBatch`, t, func(cvctx C) {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)

		sentBatches := []int{}
		sendBlocker := make(chan struct{})

		limiter := rate.NewLimiter(rate.Every(100*time.Millisecond), 1)
		ch, err := NewChannel[int](ctx, &Options[int]{
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
		So(err, ShouldBeNil)
		// Grab the first token; channel can't send until it recharges.
		limiter.Reserve()

		// Stuff a bunch of crap into the channel. We have 100ms to do this until
		// the channel is able to send something. Should be plenty of time (running
		// this on my laptop takes 3-4ms with verbose logs).
		for i := 0; i < 20; i++ {
			ch.C <- i
		}
		// At this point we can start draining the channel.
		close(ch.C)
		// then unblock the sender
		close(sendBlocker)
		// Then wait for the channel to drain
		<-ch.DrainC

		// We should only have seen one batch actually sent.
		So(sentBatches, ShouldHaveLength, 1)
	})
}

func TestContextCancel(t *testing.T) {
	Convey(`can use context cancelation for termination`, t, func() {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		ch, err := NewChannel[any](ctx, &Options[any]{
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
		So(err, ShouldBeNil)

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
	Convey(`can set DrainedFn to do exactly-once termination tasks`, t, func() {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		amDrained := false

		ch, err := NewChannel[any](ctx, &Options[any]{
			DrainedFn:  func() { amDrained = true },
			testingDbg: dbg,
		}, func(batch *buffer.Batch[any]) (err error) {
			// doesn't matter :)
			return
		})
		So(err, ShouldBeNil)

		ch.Close()
		<-ch.DrainC
		So(amDrained, ShouldBeTrue)
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
	for i := 0; i < 10; i++ {
		Convey(fmt.Sprintf(`ensure that the channel can shutdown cleanly (%d)`, i), t, func() {
			ctx := context.Background() // uses real time!
			ctx, dbg := dbgIfVerbose(ctx)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			inSendFn := make(chan struct{})
			holdSendFn := make(chan struct{})

			ch, err := NewChannel[any](ctx, &Options[any]{
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
			So(err, ShouldBeNil)

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

	Convey(`Correct use of Timer.Reset`, t, func(cvctx C) {
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

		ch, err := NewChannel[int](ctx, &Options[int]{
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
		So(err, ShouldBeNil)

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
		So(sent, ShouldHaveLength, N)
		sort.Ints(sent)
		for i := 1; i <= N; i++ {
			So(sent[i-1], ShouldEqual, i)
		}
	})
}

func TestSizeBasedChannel(t *testing.T) {
	t.Parallel()

	Convey(`Size based channel`, t, func(cvctx C) {
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
			ItemSizeFunc: func(itm any) int {
				return len(itm.(string))
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

		ch, err := NewChannel[string](ctx, opts, func(batch *buffer.Batch[string]) (err error) {
			mu.Lock()
			defer mu.Unlock()
			for _, itm := range batch.Data {
				out = append(out, itm.Item)
			}
			return nil
		})
		So(err, ShouldBeNil)

		bigString := strings.Repeat("something.", 5) // 50 bytes

		mu.Lock()
		needUnlock = true

		for i := 0; i < 10; i++ {
			ch.C <- bigString
		}

		select {
		case ch.C <- "extra string":
			So(true, ShouldBeFalse) // shouldn't be able to push more
		case <-clock.After(ctx, 250*time.Millisecond):
		}

		mu.Unlock()
		needUnlock = false

		select {
		case ch.C <- "extra string": // no problem now
		case <-clock.After(ctx, 250*time.Millisecond):
			So(true, ShouldBeFalse)
		}

		// pushing a giant object in will end up going to ErrorFn
		ch.C <- strings.Repeat("something.", 50) // 500 bytes
		// pushing an empty object (without having ItemSizeFunc assign it
		// a non-zero arbitrary size) goes to ErrorFn.
		ch.C <- ""

		ch.CloseAndDrain(ctx)

		So(fails, ShouldHaveLength, 2)
		So(fails[0].Data, ShouldHaveLength, 1)
		So(fails[0].Data[0].Item, ShouldHaveLength, 500)
		So(fails[0].Data[0].Size, ShouldEqual, 500)
		So(fails[1].Data, ShouldHaveLength, 1)
		So(fails[1].Data[0].Item, ShouldHaveLength, 0)
		So(fails[1].Data[0].Size, ShouldEqual, 0)
		So(errs[0], ShouldErrLike, buffer.ErrItemTooLarge)
		So(errs[1], ShouldErrLike, buffer.ErrItemTooSmall)

		So(out, ShouldHaveLength, 11)
		So(out[len(out)-1], ShouldResemble, "extra string")
	})
}

func TestMinQPS(t *testing.T) {
	t.Parallel()
	Convey(`TestMinQPS`, t, func() {
		Convey(`send w/ minimal frequency`, func() {
			ctx := context.Background() // uses real time!
			ctx, dbg := dbgIfVerbose(ctx)

			numNilBatches := 0

			ch, err := NewChannel[int](ctx, &Options[int]{
				MinQPS: rate.Every(100 * time.Millisecond),
				DropFn: noDrop[int],
				Buffer: buffer.Options{
					MaxLeases:     1,
					BatchItemsMax: 1,
					FullBehavior:  &buffer.BlockNewItems{MaxItems: 20},
				},
				testingDbg: dbg,
			}, func(batch *buffer.Batch[int]) (err error) {
				if batch == nil {
					numNilBatches++
				}
				return
			})
			So(err, ShouldBeNil)

			for i := 0; i < 20; i++ {
				switch i {
				case 9:
					time.Sleep(2 * time.Second) // to make a gap that ch is empty.
				}
				ch.C <- i
			}
			ch.CloseAndDrain(ctx)
			So(numNilBatches, ShouldBeGreaterThan, 0)
		})

		Convey(`send w/ minimal frequency non block`, func() {
			ctx := context.Background() // uses real time!
			ctx, dbg := dbgIfVerbose(ctx)

			mu := sync.Mutex{}
			numNilBatch := 0
			minWaitDuration := 100 * time.Millisecond

			ch, err := NewChannel[int](ctx, &Options[int]{
				MinQPS: rate.Every(minWaitDuration),
				DropFn: noDrop[int],
				Buffer: buffer.Options{
					MaxLeases:     1,
					BatchItemsMax: 1,
					FullBehavior:  &buffer.BlockNewItems{MaxItems: 20},
				},
				testingDbg: dbg,
			}, func(batch *buffer.Batch[int]) (err error) {
				mu.Lock()
				if batch == nil {
					numNilBatch++
				}
				mu.Unlock()
				time.Sleep(200 * time.Millisecond)
				return
			})
			So(err, ShouldBeNil)

			for i := 0; i < 20; i++ {
				ch.C <- i
			}
			ch.CloseAndDrain(ctx)

			So(numNilBatch, ShouldEqual, 0)
		})
	})
}
