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
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"golang.org/x/time/rate"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func dummySendFn(*buffer.Batch) error { return nil }

func noDrop(dropped *buffer.Batch) { panic(fmt.Sprintf("dropping %+v", dropped)) }

func dbgIfVerbose(ctx context.Context) (context.Context, func(string, ...interface{})) {
	if testing.Verbose() {
		ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		return ctx, logging.Get(logging.SetField(ctx, "dispatcher.coordinator", true)).Infof
	}
	return ctx, func(string, ...interface{}) {}
}

func TestChannelConstruction(t *testing.T) {
	Convey(`Channel`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		Convey(`construction`, func() {

			Convey(`success`, func() {
				ch, err := NewChannel(ctx, nil, dummySendFn)
				So(err, ShouldBeNil)
				close(ch.C)
				<-ch.DrainC
			})

			Convey(`failure`, func() {
				Convey(`bad SendFn`, func() {
					_, err := NewChannel(ctx, nil, nil)
					So(err, ShouldErrLike, "send is required")
				})

				Convey(`bad Options`, func() {
					_, err := NewChannel(ctx, &Options{
						QPSLimit: rate.NewLimiter(100, 0),
					}, dummySendFn)
					So(err, ShouldErrLike, "normalizing dispatcher.Options")
				})

				Convey(`bad Options.Buffer`, func() {
					_, err := NewChannel(ctx, &Options{
						Buffer: buffer.Options{
							BatchSize: -3,
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

		ch, err := NewChannel(ctx, &Options{
			DropFn:   noDrop,
			QPSLimit: rate.NewLimiter(rate.Inf, 0),
			Buffer: buffer.Options{
				MaxLeases:    1,
				BatchSize:    1,
				FullBehavior: &buffer.BlockNewItems{MaxItems: 10},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch) (err error) {
			cvctx.So(batch.Data, ShouldHaveLength, 1)
			str := batch.Data[0].(string)
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
	Convey(`context cancellation ends channel`, t, func(cvctx C) {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		ctx, dbg := dbgIfVerbose(ctx)
		cctx, cancel := context.WithCancel(ctx)

		sentBatches := []string{}
		blockSend := make(chan struct{})

		ch, err := NewChannel(cctx, &Options{
			QPSLimit: rate.NewLimiter(rate.Inf, 0),
			DropFn:   noDrop,
			Buffer: buffer.Options{
				MaxLeases:    1,
				BatchSize:    1,
				FullBehavior: &buffer.BlockNewItems{MaxItems: 1},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch) (err error) {
			sentBatches = append(sentBatches, batch.Data[0].(string))
			<-blockSend
			return
		})
		So(err, ShouldBeNil)

		ch.C <- "hey"
		select {
		case ch.C <- "blocked":
			panic("channel should have been blocked")
		case <-time.After(time.Millisecond):
			// OK
		}

		cancel()
		ch.CloseAndDrain(ctx)
	})
}

func TestQPSLimit(t *testing.T) {
	Convey(`QPS limited send`, t, func() {
		ctx := context.Background() // uses real time!
		ctx, dbg := dbgIfVerbose(ctx)

		sentBatches := []int{}

		ch, err := NewChannel(ctx, &Options{
			QPSLimit: rate.NewLimiter(rate.Every(10*time.Millisecond), 1),
			DropFn:   noDrop,
			Buffer: buffer.Options{
				MaxLeases:    1,
				BatchSize:    1,
				FullBehavior: &buffer.BlockNewItems{MaxItems: 20},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch) (err error) {
			sentBatches = append(sentBatches, batch.Data[0].(int))
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

		ch, err := NewChannel(ctx, &Options{
			QPSLimit: rate.NewLimiter(rate.Every(10*time.Millisecond), 10),
			DropFn:   noDrop,
			Buffer: buffer.Options{
				MaxLeases:    4,
				BatchSize:    1,
				FullBehavior: &buffer.BlockNewItems{MaxItems: 20},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch) (err error) {
			lock.Lock()
			sentBatches = append(sentBatches, batch.Data[0].(int))
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

		ch, err := NewChannel(ctx, &Options{
			QPSLimit: rate.NewLimiter(rate.Inf, 0),
			DropFn: func(batch *buffer.Batch) {
				droppedBatches = append(droppedBatches, batch.Data[0].(int))
			},
			ErrorFn: func(batch *buffer.Batch, err error) (retry bool) {
				return false
			},
			Buffer: buffer.Options{
				MaxLeases:    1,
				BatchSize:    1,
				FullBehavior: &buffer.BlockNewItems{MaxItems: 20},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch) (err error) {
			itm := batch.Data[0].(int)
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
		ch, err := NewChannel(ctx, &Options{
			QPSLimit: limiter,
			Buffer: buffer.Options{
				MaxLeases:    1,
				BatchSize:    1,
				FullBehavior: &buffer.DropOldestBatch{MaxLiveItems: 1},
			},
			testingDbg: dbg,
		}, func(batch *buffer.Batch) (err error) {
			sentBatches = append(sentBatches, batch.Data[0].(int))
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
