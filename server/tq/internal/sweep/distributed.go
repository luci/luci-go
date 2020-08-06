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
	logging.Infof(ctx, "Sweeping %s (level %d, %d/%d): %s",
		task.Db, task.Level, task.ShardIndex, task.ShardCount, task.Partition)

	// The corresponding DB must be registered in the process, otherwise we won't
	// know how to enumerate reminders.
	db := db.NonTxnDB(ctx, task.Db)
	if db == nil {
		return errors.Reason("no TQ db kind %q registered in the process", task.Db).Err()
	}

	// Similarly a lessor is needed for coordination.
	lessor, err := lessor.Get(ctx, task.LessorId)
	if err != nil {
		return errors.Annotate(err, "can't initialize lessor %q", task.LessorId).Err()
	}

	part, err := partition.FromString(task.Partition)
	if err != nil {
		return errors.Annotate(err, "bad task payload").Err()
	}

	// Ensure there is time to process reminders produced by the scan.
	scanTimeout := time.Minute
	if d, ok := ctx.Deadline(); ok {
		scanTimeout = d.Sub(clock.Now(ctx)) / 5
	}
	scanCtx, cancel := clock.WithTimeout(ctx, scanTimeout)
	defer cancel()

	// Discover stale reminders and a list of partitions we need to additionally
	// scan. Use the configuration passed with the task. Note that this call may
	// return both results and an error.
	reminders, followUp, err := Scan(scanCtx, ScanParams{
		DB:                  db,
		Partition:           part,
		KeySpaceBytes:       int(task.KeySpaceBytes),
		TasksPerScan:        int(task.TasksPerScan),
		SecondaryScanShards: int(task.SecondaryScanShards),
		Level:               int(task.Level),
	})
	if err != nil {
		if len(reminders) == 0 && len(followUp) == 0 {
			logging.Errorf(ctx, "Scan failed without returning any results: %s", err)
			return errors.New("scan failed without returning any results")
		}
		logging.Warningf(ctx, "Got %d reminders and %d follow-up ranges and then failed with: %s", len(reminders), len(followUp), err)
	} else if len(reminders) != 0 || len(followUp) != 0 {
		logging.Infof(ctx, "Got %d reminders and %d follow-up ranges", len(reminders), len(followUp))
	}

	// Refuse to scan deeper than 2 levels.
	if task.Level >= 2 && len(followUp) != 0 {
		logging.Errorf(ctx, "Refusing to recurse deeper, abandoning scans of %v", followUp)
		followUp = nil
	}

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
		return 0, errors.Annotate(err, "invalid Reminder ID(s)").Err()
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
		return 0, errors.Annotate(leaseErr, "failed to acquire the lease").Err()
	case errProcess != nil:
		return count, errors.Annotate(errProcess, "failed to process all reminders").Err()
	default:
		return count, nil
	}
}

// processLeasedReminders processes given reminders by splitting them in
// batches and calling processBatch for each batch.
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
				processed, err := d.processBatch(ctx, db, batch)
				atomic.AddInt32(&total, int32(processed))
				return err
			}
		}
	})
	return int(atomic.LoadInt32(&total)), err
}

// processBatch process a batch of reminders by submitting corresponding
// Cloud Tasks and deleting reminders.
//
// Reminders batch will be modified to fetch Reminders' Payload. RAM usage is
// equivalent to O(total Payload size of each Reminder in batch).
//
// Logs errors inside. Returns the total number of successfully processed
// reminders.
func (d *Distributed) processBatch(ctx context.Context, db db.DB, batch []*reminder.Reminder) (int, error) {
	payloaded, err := db.FetchReminderPayloads(ctx, batch)
	switch missing := len(batch) - len(payloaded); {
	case missing < 0:
		panic(errors.Reason("%s.FetchReminderPayloads returned %d but asked for %d Reminders",
			db.Kind(), len(payloaded), len(batch)).Err())
	case err != nil:
		logging.Warningf(ctx, "Failed to fetch %d/%d Reminders: %s", missing, len(batch), err)
		// Continue processing whatever was fetched anyway.
	case missing > 0:
		logging.Warningf(ctx, "%d stale Reminders were unexpectedly deleted by something else. "+
			"If this persists, check for a misconfiguration of the sweeping or the happy path timeout",
			missing)
	}

	var success int32

	// Note: this can be optimized further by batching deletion of Reminders,
	// but the current version was good enough in load tests already.
	merr := parallel.WorkPool(16, func(work chan<- func() error) {
		for _, r := range payloaded {
			r := r
			work <- func() error {
				err := internal.SubmitFromReminder(ctx, d.Submitter, db, r, nil)
				if err != nil {
					logging.Errorf(ctx, "Failed to process reminder %q: %s", r.ID, err)
				} else {
					atomic.AddInt32(&success, 1)
				}
				return err
			}
		}
	})

	count := int(atomic.LoadInt32(&success))

	switch {
	case err == nil:
		return count, merr
	case merr == nil:
		return count, err
	default:
		e := merr.(errors.MultiError)
		return count, append(e, err)
	}
}
