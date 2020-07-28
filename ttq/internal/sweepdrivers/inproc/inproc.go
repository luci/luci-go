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
)

type Options struct {
	// ScanInterval establishes upper bound on frequency of sweeps per shard.
	ScanInterval time.Duration
	// MaxConcurrentScansPerShard limits concurrent Database.FetchRemindersMeta calls
	// at MaxConcurrentScansPerShard * ttq.Options.Shards.
	MaxConcurrentScansPerShard uint
	// MaxConcurrentScansPerShard limits concurrent Database.FetchRemindersMeta calls
	// at MaxConcurrentPostProcessBatchesPerShard * ttq.Options.Shards. Since its
	// batch will have its Payload populated, this has direct effect on RAM usage.
	MaxConcurrentPostProcessBatchesPerShard uint
	// BatchPostProcessSize limits how many Reminders are worked on by
	// PostProcessBatch. Since these Reminders will have Payload populated, this
	// has direct effect on RAM usage.
	BatchPostProcessSize uint
}

// Sweeper sweeps everything within a single process.
//
// Each of the impl.Options.Options is swept separately.
type Sweeper struct {
	impl *internal.Impl
	opts Options

	// shouldContinue is used in tests only as a callback after each iteration.
	shouldContinue func(shard int, iteration int64) bool
}

// Sweep runs the sweep until the context is canceled.
func (s *Sweeper) SweepContinuously(ctx context.Context) {
	scans := s.impl.SweepAll()
	wg := sync.WaitGroup{}
	wg.Add(len(scans))
	for _, scan := range scans {
		scan := scan
		go func() {
			defer wg.Done()
			ctx := logging.SetField(ctx, "shard", scan.Shard)
			s.sweepShardContinuously(ctx, scan)
		}()
	}
	wg.Wait()
}

func (s *Sweeper) sweepShardContinuously(ctx context.Context, firstScan internal.ScanItem) {
	iteration := int64(0)
	next := clock.Now(ctx)
	for {
		iteration++
		ictx := logging.SetField(ctx, "iteration", iteration)
		logging.Debugf(ictx, "started")
		err := s.sweepShard(ictx, firstScan)
		switch errOriginal := errors.Unwrap(err); {
		case errOriginal == context.Canceled || errOriginal == context.DeadlineExceeded:
			logging.Infof(ictx, "interrupted; terminating sweeping")
			return
		case err != nil:
			errors.Log(ictx, err)
		default:
			logging.Debugf(ictx, "done.")
		}
		if s.shouldContinue != nil && !s.shouldContinue(firstScan.Shard, iteration) {
			logging.Debugf(ictx, "early exit in testing")
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

func (s *Sweeper) sweepShard(ctx context.Context, firstScan internal.ScanItem) error {
	// Set 10 minute deadline to assist in debugging seemingly stuck sweeping.
	// In practice, 10 minute timeout shouldn't happen unless there is a huge
	// backlog, unavailable external service (e.g. database), or a bug in code.
	ctx, cancel := clock.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	var errs errors.MultiError
	var mutex sync.Mutex

	errorFn := func(_ *buffer.Batch, err error) bool {
		mutex.Lock()
		defer mutex.Unlock()
		errs = append(errs, err)
		return false // don't retry. Applies to both scanning and postProcessing.
	}

	// dispatcher.Channel is a good fit because it already supports backpressure
	// AND batching with a native Go chan interface for producers.
	// This is important to keep RAM usage in check. It's also nice to have
	// built-in limit on concurrent batche & respect for context cancelation.
	postProcessChannel, err := dispatcher.NewChannel(
		ctx,
		&dispatcher.Options{
			ErrorFn: errorFn,
			Buffer: buffer.Options{
				MaxLeases: int(s.opts.MaxConcurrentPostProcessBatchesPerShard),
				BatchSize: int(s.opts.BatchPostProcessSize),
				// Max waiting time to fill the batch.
				BatchDuration: 10 * time.Millisecond,
				FullBehavior: &buffer.BlockNewItems{
					// If all postProcessing workers are busy, block scanners.
					MaxItems: int(s.opts.MaxConcurrentPostProcessBatchesPerShard) * int(s.opts.BatchPostProcessSize),
				},
			},
			DropFn: func(b *buffer.Batch, flush bool) {
				if b == nil && flush {
					// FYI from dispatcher.Channel that all draining is done.
					return
				}
				logging.Warningf(ctx, "dropping %d Reminders", len(b.Data))
			},
		},
		// SendFn
		func(data *buffer.Batch) error {
			batch := make([]*internal.Reminder, len(data.Data))
			for i, d := range data.Data {
				batch[i] = d.(*internal.Reminder)
			}
			return s.impl.PostProcessBatch(ctx, batch, nil)
		},
	)
	if err != nil {
		return errors.Annotate(err, "invalid static configuration").Err()
	}

	var scanChannel dispatcher.Channel // referenced in SendFn function below.
	// NOTE: scanChannel.CloseAndDrain can't be used to wait for all scan tasks
	// to complete, because they will panic when sending additional scanItems into
	// closed scanChannel.C. So, keep track of all outstanding scan work on our
	// own via scansWaitGroup.
	scansWaitGroup := sync.WaitGroup{}
	metaBatchWgDoneCalled := true

	scanChannel, err = dispatcher.NewChannel(
		ctx,
		&dispatcher.Options{
			ErrorFn: errorFn,
			Buffer: buffer.Options{
				MaxLeases: int(s.opts.MaxConcurrentScansPerShard),
				BatchSize: 1,
				FullBehavior: &buffer.DropOldestBatch{
					// Because one scan work item may result in several follow up scan
					// work items, limiting channel size will lead to deadlock.
					// Instead, instruct the channel to drop oldest scan items to avoid
					// infinite RAM usage in case of very large backlog & very slow
					// database.
					// In normal situations, there should be at most a dozen Scans per
					// shard.
					MaxLiveItems: 1024,
				},
			},
			DropFn: func(b *buffer.Batch, flush bool) {
				if b == nil && flush {
					// FYI from dispatcher.Channel that all draining is done.
					return
				}
				if b.Meta != &metaBatchWgDoneCalled {
					scansWaitGroup.Done()
					logging.Warningf(ctx, "dropped %v without processing", b.Data[0].(internal.ScanItem))
				}
			},
		},
		// SendFn
		func(data *buffer.Batch) error {
			w := data.Data[0].(internal.ScanItem)
			defer func() {
				scansWaitGroup.Done()
				data.Meta = &metaBatchWgDoneCalled
			}()

			moreScans, scanResult, err := s.impl.Scan(ctx, w)
			if err != nil {
				return errors.Annotate(err, "failed to scan %v", w).Err()
			}
			for _, r := range scanResult.Reminders {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case postProcessChannel.C <- r:
				}
			}
			for _, more := range moreScans {
				// Increment before we succeed adding `more` to scanChannel to avoid
				// wait group counter ever going to 0 prematurely.
				scansWaitGroup.Add(1)
				select {
				case <-ctx.Done():
					scansWaitGroup.Add(-1) // failed to add to channel, undo the increment.
					return ctx.Err()
				case scanChannel.C <- more:
				}
			}
			return nil
		},
	)
	if err != nil {
		return errors.Annotate(err, "invalid static configuration").Err()
	}

	// Seed the pipeline.
	scansWaitGroup.Add(1)
	scanChannel.C <- firstScan
	scansWaitGroup.Wait()
	logging.Debugf(ctx, "all scans enqueued, draining channels")
	scanChannel.CloseAndDrain(ctx)
	postProcessChannel.CloseAndDrain(ctx)
	return errors.Flatten(errs)
}
