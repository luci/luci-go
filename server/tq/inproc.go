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

package tq

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/sweep"
	"go.chromium.org/luci/server/tq/internal/workset"
)

// InProcSweeperOptions is configuration for the process of "sweeping" of
// transactional tasks reminders performed centrally in the current process.
type InProcSweeperOptions struct {
	// SweepShards defines how many concurrent sweeping jobs to run.
	//
	// Default is 16.
	SweepShards int

	// TasksPerScan caps maximum number of tasks that a sweep job will process.
	//
	// Defaults to 2048.
	TasksPerScan int

	// SecondaryScanShards caps the sharding of additional sweep scans to be
	// performed if the initial scan didn't cover the whole assigned partition.
	// In practice, this matters only when database is slow or there is a huge
	// backlog.
	//
	// Defaults to 16.
	SecondaryScanShards int

	// SubmitBatchSize limits a single of a single processed batch.
	//
	// When processing a batch, the sweeper loads bodies of all tasks in
	// the batch, thus this setting directly affects memory usage. There will
	// be at most SubmitBatchSize*SubmitConcurrentBatches task bodies worked-on at
	// any moment in time.
	//
	// Default is 512.
	SubmitBatchSize int

	// SubmitConcurrentBatches limits how many submit batches can be worked on
	// concurrently.
	//
	// Default is 8.
	SubmitConcurrentBatches int
}

// NewInProcSweeper creates a sweeper that performs sweeping in the current
// process whenever Sweep is called.
func NewInProcSweeper(opts InProcSweeperOptions) Sweeper {
	if opts.SweepShards == 0 {
		opts.SweepShards = 16
	}
	if opts.TasksPerScan == 0 {
		opts.TasksPerScan = 2048
	}
	if opts.SecondaryScanShards == 0 {
		opts.SecondaryScanShards = 16
	}
	if opts.SubmitBatchSize == 0 {
		opts.SubmitBatchSize = 512
	}
	if opts.SubmitConcurrentBatches == 0 {
		opts.SubmitConcurrentBatches = 8
	}
	return &inprocSweeper{opts: opts}
}

// inprocSweeper implements Sweeper interface.
type inprocSweeper struct {
	opts    InProcSweeperOptions
	running int32 // 1 of already running a sweep
}

// sweep performs as much of the sweep as possible.
//
// Logs internal errors and carries on.
func (s *inprocSweeper) sweep(ctx context.Context, sub Submitter, reminderKeySpaceBytes int) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return errors.New("a sweep is already running")
	}
	defer atomic.StoreInt32(&s.running, 0)

	// We'll sweep all known DBs and have a BatchProcessor per DB kind for
	// processing reminders in this DB.
	procs := map[string]*sweep.BatchProcessor{}
	for _, kind := range db.Kinds() {
		proc := &sweep.BatchProcessor{
			Context:           logging.SetField(ctx, "db", kind),
			DB:                db.NonTxnDB(ctx, kind),
			Submitter:         sub,
			BatchSize:         s.opts.SubmitBatchSize,
			ConcurrentBatches: s.opts.SubmitConcurrentBatches,
		}
		if err := proc.Start(); err != nil {
			for _, running := range procs {
				running.Stop()
			}
			return err
		}
		procs[kind] = proc
	}

	// Seed all future work: SweepShards scans per DB kind.
	partitions := partition.Universe(reminderKeySpaceBytes).Split(s.opts.SweepShards)
	initial := make([]workset.Item, 0, len(procs)*len(partitions))
	for _, proc := range procs {
		for _, p := range partitions {
			initial = append(initial, &sweep.ScanParams{
				DB:                  proc.DB,
				Partition:           p,
				KeySpaceBytes:       reminderKeySpaceBytes,
				TasksPerScan:        s.opts.TasksPerScan,
				SecondaryScanShards: s.opts.SecondaryScanShards,
				Level:               0,
			})
		}
	}
	work := workset.New(initial, nil)

	// Run `SweepShards` workers (even if serving multiple DBs) that do scans of
	// whatever partitions need scanning. Each will feed produced reminders into
	// a BatchProcessor which will batch-process them.
	wg := sync.WaitGroup{}
	wg.Add(s.opts.SweepShards)
	for i := 0; i < s.opts.SweepShards; i++ {
		go func() {
			defer wg.Done()
			// Pick up some random partition we haven't scanned yet. Scan it, and
			// enqueue all follow ups. Do until the queue is empty and all scan
			// workers are done.
			for {
				item, done := work.Pop(ctx)
				if item == nil {
					return // no more work or the context is done
				}
				params := item.(*sweep.ScanParams)
				var followUp []workset.Item
				for _, part := range s.scan(ctx, params, procs[params.DB.Kind()]) {
					params := *params
					params.Partition = part
					params.Level += 1 // we need to go deeper
					followUp = append(followUp, &params)
				}
				done(followUp)
			}
		}()
	}
	wg.Wait()

	// At this point all scanners are done, but BatchProcessors may still be
	// working. Drain them.
	for _, proc := range procs {
		if count := proc.Stop(); count != 0 {
			logging.Infof(proc.Context, "Successfully processed %d reminder(s)", count)
		}
	}

	return ctx.Err()
}

// scan scans a single partition.
//
// Enqueues discovered reminders for processing into a batch processor. Returns
// a list of partitions to scan next.
//
// Logs errors but otherwise ignores them.
func (s *inprocSweeper) scan(ctx context.Context, p *sweep.ScanParams, proc *sweep.BatchProcessor) []*partition.Partition {
	logging.Infof(ctx, "Sweeping (level %d): %s", p.Level, p.Partition)

	// Don't block for too long in a single scan.
	scanCtx, cancel := clock.WithTimeout(ctx, time.Minute)
	defer cancel()
	reminders, followUp := sweep.Scan(scanCtx, p)

	// Feed all reminders to a batching processor. This blocks if the processor is
	// filled to the limit already. This is what we want to avoid OOMs.
	proc.Enqueue(ctx, reminders)

	return followUp
}
