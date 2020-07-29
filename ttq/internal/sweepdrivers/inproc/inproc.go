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

// Package inproc implements SweepDriver in a single process.
//
// Suitable to run in a *single* pod on GKE if you don't care about fast
// recovery of backlog of lots of tasks.
package inproc

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/ttq/internal"
	"go.chromium.org/luci/ttq/internal/reminder"
)

type Options struct {
	// ScanInterval establishes upper bound on frequency of sweeps per shard.
	ScanInterval time.Duration
	// MaxConcurrentScansPerShard limits concurrent Database.FetchRemindersMeta calls
	// at MaxConcurrentScansPerShard * ttq.Options.Shards.
	MaxConcurrentScansPerShard int
	// MaxScansBacklog limits accumulation of follow up scans by not doing them in
	// the current sweep iteration. Acts primarily as a failsafe against wasting
	// RAM when things go wrong.
	MaxScansBacklog int
	// MaxConcurrentScansPerShard limits concurrent Database.FetchRemindersMeta calls
	// at MaxConcurrentPostProcessBatchesPerShard * ttq.Options.Shards. Since its
	// batch will have its Payload populated, this has direct effect on RAM usage.
	MaxConcurrentPostProcessBatchesPerShard int
	// BatchPostProcessSize limits how many Reminders are worked on by
	// PostProcessBatch. Since these Reminders will have Payload populated, this
	// has direct effect on RAM usage.
	BatchPostProcessSize int
}

// Sweeper sweeps everything within a single process.
//
// Each of the impl.Options.Shards is swept separately.
type Sweeper struct {
	Impl *internal.Impl
	Opts *Options

	// shouldContinue is used in tests only as a callback after each iteration.
	shouldContinue func(shard int, iteration int64) bool
}

// Sweep runs the sweep until the context is canceled.
// Panics if options are not valid.
func (s *Sweeper) SweepContinuously(ctx context.Context) {
	scans := s.Impl.SweepAll()
	wg := sync.WaitGroup{}
	wg.Add(len(scans))
	for _, scan := range scans {
		// Init shard here to catch errors early.
		ctx, shard, err := s.initShard(ctx, scan)
		if err != nil {
			panic(err)
		}
		go func() {
			defer wg.Done()
			defer shard.close(ctx)
			shard.sweepContinuously(ctx)
		}()
	}
	wg.Wait()
}

// shard encapsulates pipeline with two stages:
//
//   Scan -----> PostProcess
//   ^  |
//   |  |
//   \--/
//
// Note that Scan stage may produce follow up scans.
//
// The pipeline is set up in initShard().
//
// Before and after each sweep() iteration, the pipeline is empty.
// To achieve this, scanItemsWG and ppItemsWG WaitGroups count
// every added item to the respective Scan and PostProcess stages.
// Sweep iteration thus feeds firstScan to the Scan stage and then waits
// first for all scans and then for all postProcess items to processed.
//
// Finally, in close() it suffices to close channels and wait on their
// worker goroutines to terminate.
type shard struct {
	impl      *internal.Impl
	opts      *Options
	firstScan internal.ScanItem

	// iterCtx is dependency injection of per-iteration context to functions
	// called from goroutines created in initShard before any iterations start.
	iterCtx context.Context

	scanChan      chan internal.ScanItem
	scanItemsWG   sync.WaitGroup
	scanWorkersWG sync.WaitGroup

	ppChan    dispatcher.Channel
	ppItemsWG sync.WaitGroup

	errs     errors.MultiError
	errMutex sync.Mutex

	// shouldContinue is used in tests only as a callback after each iteration.
	shouldContinue func(shard int, iteration int64) bool
}

func (s *Sweeper) initShard(ctx context.Context, firstScan internal.ScanItem) (context.Context, *shard, error) {
	ctx = logging.SetField(ctx, "shard", firstScan.Shard)
	shard := &shard{
		opts:      s.Opts,
		impl:      s.Impl,
		firstScan: firstScan,

		shouldContinue: s.shouldContinue,
	}
	// dispatcher.Channel is a good fit because it already supports backpressure
	// AND batching with a native Go chan interface for producers.
	// This is important to keep RAM usage in check. It's also nice to have
	// built-in limit on concurrent batche & respect for context cancelation.
	var err error
	shard.ppChan, err = dispatcher.NewChannel(
		ctx,
		&dispatcher.Options{
			Buffer: buffer.Options{
				MaxLeases: s.Opts.MaxConcurrentPostProcessBatchesPerShard,
				BatchSize: s.Opts.BatchPostProcessSize,
				// Max waiting time to fill the batch.
				BatchDuration: 10 * time.Millisecond,
				FullBehavior: &buffer.BlockNewItems{
					// If all postProcessing workers are busy, block scanners.
					MaxItems: s.Opts.MaxConcurrentPostProcessBatchesPerShard * s.Opts.BatchPostProcessSize,
				},
			},
		},
		shard.sendPostProcessBatch,
	)
	if err != nil {
		return nil, nil, errors.Annotate(err, "invalid static configuration").Err()
	}

	shard.scanChan = make(chan internal.ScanItem, s.Opts.MaxScansBacklog)
	shard.scanWorkersWG.Add(s.Opts.MaxConcurrentScansPerShard)
	for i := 0; i < s.Opts.MaxConcurrentScansPerShard; i++ {
		go func() {
			defer shard.scanWorkersWG.Done()
			for w := range shard.scanChan {
				shard.doScan(w)
			}
		}()
	}
	return ctx, shard, nil
}

func (s *shard) close(ctx context.Context) {
	close(s.scanChan)
	s.scanWorkersWG.Wait()
	s.ppChan.CloseAndDrain(ctx)
}

func (s *shard) sweepContinuously(ctx context.Context) {
	iteration := int64(0)
	next := clock.Now(ctx)
	for {
		iteration++
		s.sweep(ctx, iteration)

		if s.shouldContinue != nil && !s.shouldContinue(s.firstScan.Shard, iteration) {
			logging.Debugf(ctx, "early exit in testing")
			return
		}
		next = next.Add(s.opts.ScanInterval)
		now := clock.Now(ctx)
		delay := next.Sub(now)
		if delay <= 0 {
			// If prior iteration took >ScanInterval, do current iteration
			// immediately, but postpone future iterations.
			next = now
		} else {
			logging.Debugf(ctx, "sleeping %s until next sweep", delay)
			if err := clock.Sleep(clock.Tag(ctx, "shard-loop"), delay).Err; err != nil {
				logging.Infof(ctx, "sleep interrupted, terminating sweeping")
				return
			}
		}
	}
}

func (s *shard) sweep(ctx context.Context, iteration int64) {
	// Set 10 minute deadline to assist in debugging seemingly stuck sweeping.
	// In practice, 10 minute timeout shouldn't happen unless there is a huge
	// backlog, unavailable external service (e.g. database), or a bug in code.
	var cancel func()
	s.iterCtx, cancel = clock.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	s.iterCtx = logging.SetField(s.iterCtx, "iteration", iteration)

	logging.Debugf(s.iterCtx, "started")

	s.scanItemsWG.Add(1)
	s.scanChan <- s.firstScan
	s.scanItemsWG.Wait()
	s.ppItemsWG.Wait()

	if len(s.errs) == 0 {
		logging.Debugf(s.iterCtx, "done.")
		return
	}
	errors.Log(s.iterCtx, errors.Flatten(s.errs))
	s.errs = s.errs[:0]
	if s.iterCtx.Err() != nil && ctx.Err() == nil {
		logging.Errorf(s.iterCtx, "took too long")
	}
}

func (s *shard) addError(err error) {
	s.errMutex.Lock()
	defer s.errMutex.Unlock()
	s.errs = append(s.errs, err)
}

func (s *shard) doScan(w internal.ScanItem) {
	defer s.scanItemsWG.Done()

	moreScans, scanResult, err := s.impl.Scan(s.iterCtx, w)
	if err != nil {
		s.addError(errors.Annotate(err, "failed to scan %v", w).Err())
		return
	}
	for _, r := range scanResult.Reminders {
		s.ppItemsWG.Add(1)
		select {
		case <-s.iterCtx.Done():
			s.ppItemsWG.Add(-1) // wasn't actually added.
			s.addError(errors.Annotate(s.iterCtx.Err(), "failed to enqueue Reminder").Err())
			return
		case s.ppChan.C <- r:
		}
	}
	for _, more := range moreScans {
		s.scanItemsWG.Add(1)
		select {
		case s.scanChan <- more:
		default:
			s.scanItemsWG.Add(-1) // wasn't actually added.
			s.addError(errors.New("backlog of scans is full"))
			return // backlog of scan items is too big already.
		}
	}
}

func (s *shard) sendPostProcessBatch(data *buffer.Batch) error {
	defer s.ppItemsWG.Add(-len(data.Data))

	batch := make([]*reminder.Reminder, len(data.Data))
	for i, d := range data.Data {
		batch[i] = d.(*reminder.Reminder)
	}
	if err := s.impl.PostProcessBatch(s.iterCtx, batch); err != nil {
		s.addError(err)
	}
	return nil
}
