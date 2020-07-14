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

// Package incproc implements SweepDriver in a single process.
//
// Suitable to run in a *single* pord on GKE if you don't care about fast
// recovery of backlog of lots of tasks.
package incproc

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/ttq/internal"
	"golang.org/x/time/rate"
)

type Options struct {
	// ScanInterval establishes upper bound on frequency of complete sweeps.
	// Default: 1 minute.
	ScanInterval time.Duration
	// MaxConcurrentScansPerShard limits concurrent Database.FetchRemindersMeta calls
	// at MaxConcurrentScansPerShard * ttq.Options.Shards.
	// Default: 10
	MaxConcurrentScansPerShard uint
	// BatchPostProcessSize limits how many Reminders are worked on by
	// PostProcessBatch. Since these Reminders will have Payload populated, this
	// has direct effect on RAM usage.
	// Default: 50
	BatchPostProcessSize uint
	// MaxConcurrentScansPerShard limits concurrent Database.FetchRemindersMeta calls
	// at MaxConcurrentPostProcessBatchesPerShard * ttq.Options.Shards. Since its
	// batch will have its Payload populated, this has direct effect on RAM usage.
	// Default: 10
	MaxConcurrentPostProcessBatchesPerShard uint
	// IndividualPostProcessLimiter controls primarily impact on Cloud Tasks
	// during large backlogs of stale Reminders.
	// Default: 5k QPS with 1k burst (assumes average latency of post processing
	// is <0.2s).
	IndividualPostProcessLimiter *rate.Limiter
}

func NewSweeper(impl *internal.Impl, opts *Options) *Sweeper {
	if opts != nil {
		return &Sweeper{impl: impl, opts: *opts}
	}
	return &Sweeper{impl: impl, opts: Options{
		ScanInterval:                            time.Minute,
		MaxConcurrentScansPerShard:              10,
		MaxConcurrentPostProcessBatchesPerShard: 10,
		BatchPostProcessSize:                    50,
		IndividualPostProcessLimiter:            rate.NewLimiter(5000, 1000),
	}}
}

// Sweeper sweeps everything within a single process.
type Sweeper struct {
	impl *internal.Impl
	opts Options
}

// Sweep runs the sweep until the context is canceled.
func (s *Sweeper) SweepContinuously(ctx context.Context) error {
	next := clock.Now(ctx)
	for {
		switch err := s.sweep(ctx); {
		case err == context.Canceled || err == context.DeadlineExceeded:
			logging.Infof(ctx, "single sweep interrupted, terminating sweeping")
			return err
		case err != nil:
			logging.Errorf(ctx, "single sweep failed: %s", err)
			errors.Log(ctx, err)
		}
		next = next.Add(s.opts.ScanInterval)
		delay := clock.Until(ctx, next)
		logging.Debugf(ctx, "sleeping %s until next sweep", delay)
		if err := clock.Sleep(ctx, delay).Err; err != nil {
			logging.Infof(ctx, "sleep interrupted, terminating sweeping")
			return err
		}
	}
}

func (s *Sweeper) sweep(ctx context.Context) error {
	// Set 10 minute deadline to assist in debugging seemingly stuck sweeping.
	// In practice, 10 minute timeout shouldn't happen unless there is a huge
	// backlog, unavailable external service (e.g. database), or a bug in code.
	ctx, cancel := clock.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	scans := s.impl.SweepAll()
	return parallel.WorkPool(int(s.impl.Options.Shards), func(workChan chan<- func() error) {
		for _, scan := range scans {
			scan := scan
			workChan <- func() error { return s.oneShard(ctx, scan) }
		}
	})
}

func (s *Sweeper) oneShard(ctx context.Context, firstScan internal.ScanItem) error {
	var errs errors.MultiError
	var mutex sync.Mutex

	errorFn := func(failedBatch *buffer.Batch, err error) bool {
		mutex.Lock()
		errs = append(errs, err)
		mutex.Unlock()
		return false // don't retry. Applies to both scaning and postProcessing.
	}

	postProcessChannel, err := dispatcher.NewChannel(
		ctx,
		&dispatcher.Options{
			ErrorFn: errorFn,
			Buffer: buffer.Options{
				MaxLeases: int(s.opts.MaxConcurrentPostProcessBatchesPerShard),
				BatchSize: int(s.opts.BatchPostProcessSize),
				FullBehavior: &buffer.BlockNewItems{
					// If all postProcessing workers are busy, block scanners.
					MaxItems: int(s.opts.MaxConcurrentPostProcessBatchesPerShard) * int(s.opts.BatchPostProcessSize),
				},
			},
		},
		// SendFn
		func(data *buffer.Batch) error {
			batch := make([]*internal.Reminder, len(data.Data))
			for i, d := range data.Data {
				batch[i] = d.(*internal.Reminder)
			}
			return s.impl.PostProcessBatch(ctx, batch, s.opts.IndividualPostProcessLimiter)
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
	scanChannel, err = dispatcher.NewChannel(
		ctx,
		&dispatcher.Options{
			ErrorFn: errorFn,
			Buffer: buffer.Options{
				MaxLeases: int(s.opts.MaxConcurrentScansPerShard),
				BatchSize: 1,
				FullBehavior: &buffer.DropOldestBatch{
					// Because one scan work item may result in several follow scan work
					// items, limiting channel size will lead to deadlock.
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
				scansWaitGroup.Done()
				logging.Warningf(ctx, "dropping %v", b.Data[0].(internal.ScanItem))
			},
		},
		// SendFn
		func(data *buffer.Batch) error {
			w := data.Data[0].(internal.ScanItem)
			moreScans, scanResult, err := s.impl.Scan(ctx, w)
			if err != nil {
				// scansWaitGroup.Done will be called in DropFn.
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
				select {
				case <-ctx.Done():
					return ctx.Err()
				case scanChannel.C <- more:
					scansWaitGroup.Add(1)
				}
			}
			// Doing so at the end guarantees that scansWaitGroup's counter won't be 0
			// until all follow up scans are done (or dropped in DropFn).
			scansWaitGroup.Done()
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
	scanChannel.CloseAndDrain(ctx)
	postProcessChannel.CloseAndDrain(ctx)
	return errs
}
