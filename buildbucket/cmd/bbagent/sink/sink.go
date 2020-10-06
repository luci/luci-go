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

// Package sink implements a sink that asynchronousely sends the build
// state to an external source.
package sink

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"golang.org/x/time/rate"
)

// OutFn is called to send out the `build` to an external source.
type OutFn func(build *bbpb.Build) error

// BuildSink asynchronousely sinks the `build` to an external source
// implemented in `OutFn` at the rate of up to 1 build/s (depending on
// the speed of `OutFn`). It MAY drop intermediate builds received in
// between and only sends the latest one via `OutFn`. The build will be
// retried in exponential backoff strategy with up to 30s delay if the
// error returned by `OutFn` contains a transient Tag. This `BuildSink`
// will stop serving any new build update if `OutFn` reports a fatal
// error.
type BuildSink struct {
	ctx       context.Context
	outC      dispatcher.Channel
	outClosed int32

	closeOnce sync.Once
	closeC    chan struct{}

	fatalC           chan error
	termErrMonitorC  chan struct{}
	termErrMonitorWg sync.WaitGroup
}

// Sink asynchronously sinks the provided build. No-op if this `BuildSink` has
// encountered a fatal error or `Close()` has been called or context has been
// cancelled, whichever comes first.
func (bs *BuildSink) Sink(build *bbpb.Build) {
	select {
	case <-bs.ctx.Done():
	default:
		if atomic.LoadInt32(&(bs.outClosed)) == 0 {
			// bs.outClosed will be set to 1 when getting fatal error or
			// `Close()` is called
			bs.outC.C <- build
		}
	}
}

// Close drains all remaining unsent builds and stops monitorring the error.
// It will block on those operation unless context is cancelled. It is only
// to call `Close()` multiple times. But the subsequent call will block until
// the first call returns.
func (bs *BuildSink) Close() {
	bs.closeOnce.Do(func() {
		atomic.StoreInt32(&(bs.outClosed), 1)
		bs.outC.CloseAndDrain(bs.ctx)
		close(bs.termErrMonitorC)
		bs.termErrMonitorWg.Wait()
		close(bs.closeC)
	})
	<-bs.closeC
}

// Fatal returns a channel that will receive the first fatal error reported
// by `OutFn` if that happens. The channel will be closed when `BuildSink`
// is fully closed or context is cancelled.
func (bs *BuildSink) Fatal() <-chan error {
	return bs.fatalC
}

// NewBuildSink creates a new `BuildSink` instance.
//
// Args:
//   * `ctx` is used for cancellation and logging. Cancelling the context
//      will aggressively drain unsent builds and stops monitoring error
//      and serving new builds.
//   * `outFn` is called when `BuildSink` is ready to send the build to
//      the external source.
//   * `fatalPred` is used to decide whether the error returned by `outFn`
//      is a fatal error.
func NewBuildSink(ctx context.Context, outFn OutFn, fatalPred func(error) bool) (*BuildSink, error) {
	// having some buffer in case error monitor can't read from the channel
	// quickly.
	errC := make(chan error, 10)
	termErrMonitorC := make(chan struct{})
	errorFn := func(_ *buffer.Batch, err error) (retry bool) {
		select {
		case errC <- err:
		case <-termErrMonitorC:
		case <-ctx.Done():
			return false
		}
		return transient.Tag.In(err)
	}
	sendFn := func(b *buffer.Batch) error {
		return outFn(b.Data[0].(*bbpb.Build))
	}
	outC, err := dispatcher.NewChannel(ctx, channelOpts(ctx, errorFn), sendFn)
	if err != nil {
		return nil, err
	}

	bs := &BuildSink{
		ctx:             ctx,
		outC:            outC,
		closeC:          make(chan struct{}),
		termErrMonitorC: termErrMonitorC,
	}
	bs.monitorError(errC, fatalPred)
	return bs, nil
}

func (bs *BuildSink) monitorError(errC chan error, fatalPred func(error) bool) {
	// Buffer size is 1 as only the first fatal error will be written and we
	// don't want error monitor to block on writing since it will in turn block
	// reading from the errC.
	bs.fatalC = make(chan error, 1)
	bs.termErrMonitorWg.Add(1)
	go func() {
		defer bs.termErrMonitorWg.Done()
		defer close(bs.fatalC)
		first := true
		for {
			select {
			case err := <-errC:
				if first && fatalPred(err) {
					first = false
					atomic.StoreInt32(&(bs.outClosed), 1)
					bs.fatalC <- err
					logging.WithError(err).Errorf(bs.ctx, "BuildSink encounter fatal error; close and drain the underlying dispatcher.Channel now.")
					go bs.outC.CloseAndDrain(bs.ctx)
				}
			case <-bs.termErrMonitorC:
				return
			case <-bs.ctx.Done():
				return
			}
		}
	}()
}

// options for the dispatcher.Channel
func channelOpts(ctx context.Context, errorFn dispatcher.ErrorFn) *dispatcher.Options {
	return &dispatcher.Options{
		QPSLimit: rate.NewLimiter(1, 1),
		Buffer: buffer.Options{
			BatchSize:    1,
			MaxLeases:    1,
			FullBehavior: &buffer.DropOldestBatch{MaxLiveItems: 1},
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					Limited: retry.Limited{
						Delay:    200 * time.Millisecond, // initial delay
						Retries:  -1,
						MaxTotal: 5 * time.Minute,
					},
					Multiplier: 1.2,
					MaxDelay:   30 * time.Second,
				}
			},
		},
		DropFn:  dispatcher.DropFnSummarized(ctx, rate.NewLimiter(.1, 1)),
		ErrorFn: errorFn,
	}
}
