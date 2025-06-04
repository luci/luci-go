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

package sweep

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/server/tq/internal"
	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/lessor"
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/tqpb"
)

// Distributed implements distributed sweeping.
//
// Requires its EnqueueSweepTask callback to be configured in a way that
// enqueued tasks eventually result in ExecSweepTask call (perhaps in a
// different process).
type Distributed struct {
	// EnqueueSweepTask submits the task for execution somewhere in the fleet.
	EnqueueSweepTask func(ctx context.Context, task *tqpb.SweepTask) error
	// Submitter is used to submit Cloud Tasks requests.
	Submitter internal.Submitter
}

// ExecSweepTask executes a previously enqueued sweep task.
//
// Note: we never want to retry failed ExecSweepTask. These tasks fork. If we
// retry on transient errors that are not really transient we may accidentally
// blow up with exponential number of tasks. Better just to wait for the next
// fresh sweep. For that reason the implementation is careful not to return
// errors marked with transient.Tag.
func (d *Distributed) ExecSweepTask(ctx context.Context, task *tqpb.SweepTask) error {
	// The corresponding DB must be registered in the process, otherwise we won't
	// know how to enumerate reminders.
	db := db.NonTxnDB(ctx, task.Db)
	if db == nil {
		return errors.Fmt("no TQ db kind %q registered in the process", task.Db)
	}

	// Similarly a lessor is needed for coordination.
	lessor, err := lessor.Get(ctx, task.LessorId)
	if err != nil {
		return errors.Fmt("can't initialize lessor %q: %w", task.LessorId, err)
	}

	part, err := partition.FromString(task.Partition)
	if err != nil {
		return errors.Fmt("bad task payload: %w", err)
	}

	// Ensure there is time to process reminders produced by the scan.
	scanTimeout := time.Minute
	if d, ok := ctx.Deadline(); ok {
		scanTimeout = d.Sub(clock.Now(ctx)) / 5
	}
	scanCtx, cancel := clock.WithTimeout(ctx, scanTimeout)
	defer cancel()

	// Discover stale reminders and a list of partitions we need to additionally
	// scan. Use the configuration passed with the task.
	reminders, followUp := Scan(scanCtx, &ScanParams{
		DB:                  db,
		Partition:           part,
		KeySpaceBytes:       int(task.KeySpaceBytes),
		TasksPerScan:        int(task.TasksPerScan),
		SecondaryScanShards: int(task.SecondaryScanShards),
		Level:               int(task.Level),
	})

	wg := sync.WaitGroup{}
	wg.Add(2)
	lerr := errors.NewLazyMultiError(2)
	go func() {
		defer wg.Done()
		lerr.Assign(0, d.enqueueFollowUp(ctx, task, followUp))
	}()
	go func() {
		defer wg.Done()
		count, err := d.processReminders(ctx, lessor, task.LeaseSectionId, db, reminders, int(task.KeySpaceBytes))
		if count > 0 { // don't spam log with zeros
			logging.Infof(ctx, "Successfully processed %d reminder(s)", count)
		}
		lerr.Assign(1, err)
	}()
	wg.Wait()

	// We don't want to return a complex error that may have transient.Tag
	// somewhere inside. See the comment above.
	if lerr.Get() != nil {
		return errors.New("the sweep finished with errors, see logs")
	}
	return nil
}

// enqueueFollowUp enqueues sweep tasks that derive from `orig`.
//
// Logs errors inside.
func (d *Distributed) enqueueFollowUp(ctx context.Context, orig *tqpb.SweepTask, parts partition.SortedPartitions) error {
	return parallel.WorkPool(16, func(work chan<- func() error) {
		for _, part := range parts {
			task := proto.Clone(orig).(*tqpb.SweepTask)
			task.Partition = part.String()
			task.Level += 1 // we need to go deeper
			work <- func() error {
				if err := d.EnqueueSweepTask(ctx, task); err != nil {
					logging.Errorf(ctx, "Failed to enqueue the follow up task %q: %s", task.Partition, err)
					return err
				}
				return nil
			}
		}
	})
}

// processReminders leases sub-ranges of the partition and processes reminders
// there.
//
// Logs errors inside. Returns the total number of successfully processed
// reminders.
func (d *Distributed) processReminders(ctx context.Context, lessor lessor.Lessor, sectionID string, db db.DB, reminders []*reminder.Reminder, keySpaceBytes int) (int, error) {
	l := len(reminders)
	if l == 0 {
		return 0, nil
	}
	desired, err := partition.SpanInclusive(reminders[0].ID, reminders[l-1].ID)
	if err != nil {
		logging.Errorf(ctx, "bug: invalid Reminder ID(s): %s", err)
		return 0, errors.Fmt("invalid Reminder ID(s): %w", err)
	}

	var errProcess error
	var count int
	leaseErr := lessor.WithLease(ctx, sectionID, desired, time.Minute,
		func(leaseCtx context.Context, leased partition.SortedPartitions) {
			reminders := onlyLeased(reminders, leased, keySpaceBytes)
			count, errProcess = d.processLeasedReminders(leaseCtx, db, reminders)
		})
	switch {
	case leaseErr != nil:
		logging.Errorf(ctx, "Failed to acquire the lease: %s", leaseErr)
		return 0, errors.Fmt("failed to acquire the lease: %w", leaseErr)
	case errProcess != nil:
		return count, errors.Fmt("failed to process all reminders: %w", errProcess)
	default:
		return count, nil
	}
}

// processLeasedReminders processes given reminders by splitting them in
// batches and calling internal.SubmitBatch for each batch.
//
// Logs errors inside. Returns the total number of successfully processed
// reminders.
func (d *Distributed) processLeasedReminders(ctx context.Context, db db.DB, reminders []*reminder.Reminder) (int, error) {
	const (
		batchWorkers = 8
		batchSize    = 50
	)
	var total int32
	err := parallel.WorkPool(batchWorkers, func(work chan<- func() error) {
		for {
			var batch []*reminder.Reminder
			switch l := len(reminders); {
			case l == 0:
				return
			case l < batchSize:
				batch, reminders = reminders, nil
			default:
				batch, reminders = reminders[:batchSize], reminders[batchSize:]
			}
			work <- func() error {
				processed, err := internal.SubmitBatch(ctx, d.Submitter, db, batch)
				atomic.AddInt32(&total, int32(processed))
				return err
			}
		}
	})
	return int(atomic.LoadInt32(&total)), err
}
