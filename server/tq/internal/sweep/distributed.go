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
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/server/tq/internal/sweep/sweeppb"
	"go.chromium.org/luci/ttq/internals"
	"go.chromium.org/luci/ttq/internals/databases"
	"go.chromium.org/luci/ttq/internals/partition"
	"go.chromium.org/luci/ttq/internals/reminder"
)

// Distributed implements distributed sweeping.
//
// Requires its EnqueueSweepTask callback to be configured in a way that
// enqueued tasks eventually result in ExecSweepTask call (perhaps in a
// different process).
type Distributed struct {
	// EnqueueSweepTask submits the task for execution somewhere in the fleet.
	EnqueueSweepTask func(ctx context.Context, task *sweeppb.SweepTask) error
}

// ExecSweepTask executes a previously enqueued sweep task.
func (d *Distributed) ExecSweepTask(ctx context.Context, task *sweeppb.SweepTask) error {
	logging.Infof(ctx, "Sweeping %s lvl%d %d/%d: %s",
		task.Db, task.Level, task.ShardIndex, task.ShardCount, task.Partition)

	// The corresponding DB must be registered in the process, otherwise we won't
	// know how to enumerate reminders.
	db := databases.NonTxnDB(ctx, task.Db)
	if db == nil {
		return errors.Reason("no TQ db kind %q registered in the process", task.Db).Err()
	}

	// Ensure there is time to process reminders produced by the scan.
	scanTimeout := time.Minute
	if d, ok := ctx.Deadline(); ok {
		scanTimeout = clock.Now(ctx).Sub(d) / 5
	}
	scanCtx, cancel := clock.WithTimeout(ctx, scanTimeout)
	defer cancel()

	part, err := partition.FromString(task.Partition)
	if err != nil {
		return errors.Annotate(err, "bad task payload").Err()
	}

	// Discover stale reminders and a list of partitions we may need to rescan.
	// This call may return both results and an error.
	reminders, moreParts, err := Scan(scanCtx, ScanParams{
		DB:                  db,
		Partition:           part,
		KeySpaceBytes:       int(task.KeySpaceBytes),
		TasksPerScan:        int(task.TasksPerScan),
		SecondaryScanShards: int(task.SecondaryScanShards),
		Level:               int(task.Level),
	})
	if err != nil {
		if len(reminders) == 0 && len(moreParts) == 0 {
			return err // really bad error, couldn't do anything
		}
		logging.Warningf(ctx,
			"Got %d reminders, %d follow-up ranges even though scanning failed with %s",
			len(reminders), len(moreParts), err)
	} else {
		logging.Infof(ctx, "Got %d reminders, %d follow-up ranges", len(reminders), len(moreParts))
	}

	// Refuse to scan deeper than 2 levels.
	if task.Level >= 2 && len(moreParts) != 0 {
		logging.Errorf(ctx, "Refusing to recurse deeper, abandoning scans of: %v", moreParts)
		moreParts = nil
	}

	// In parallel launch follow up scans and process reminders.
	wg := sync.WaitGroup{}
	lerr := errors.NewLazyMultiError(2)

	if len(moreParts) != 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lerr.Assign(0, d.enqueueFollowUpSweeps(ctx, task, moreParts))
		}()
	}

	if len(reminders) != 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lerr.Assign(1, d.processReminders(ctx, db, task.LockId, reminders))
		}()
	}

	wg.Wait()
	return lerr.Get() // nil if no errors
}

// enqueueFollowUpSweeps enqueues more sweep tasks that originated as a follow
// up to the given `orig` task.
func (d *Distributed) enqueueFollowUpSweeps(ctx context.Context, orig *sweeppb.SweepTask, parts partition.SortedPartitions) error {
	return parallel.WorkPool(16, func(work chan<- func() error) {
		for _, part := range parts {
			task := proto.Clone(orig).(*sweeppb.SweepTask)
			task.Partition = part.String()
			task.Level += 1 // we need to go deeper
			work <- func() error { return d.EnqueueSweepTask(ctx, task) }
		}
	})
}

// processReminders leases a subset of given reminders and processes them.
func (d *Distributed) processReminders(ctx context.Context, db databases.Database, lockID string, reminders []*reminder.Reminder) error {
	l := len(reminders)
	if l == 0 {
		return nil
	}
	desired, err := partition.SpanInclusive(reminders[0].Id, reminders[l-1].Id)
	if err != nil {
		return errors.Annotate(err, "invalid Reminder Id(s)").Err()
	}

	var errProcess error
	leaseErr := d.lessor(db).WithLease(ctx, lockID, desired, time.Minute,
		func(leaseCtx context.Context, leased partition.SortedPartitions) {
			reminders := internals.OnlyLeased(reminders, leased)
			errProcess = d.processLeasedReminders(leaseCtx, db, reminders)
		})
	switch {
	case leaseErr != nil:
		return errors.Annotate(leaseErr, "failed to acquire the lease").Err()
	case errProcess != nil:
		return errors.Annotate(errProcess, "failed to process all reminders").Err()
	default:
		return nil
	}
}

func (d *Distributed) lessor(db databases.Database) internals.Lessor {
	// TODO(vadimsh): Implement.
	return nil
}

// processLeasedReminders processes given leased reminders.
func (d *Distributed) processLeasedReminders(ctx context.Context, db databases.Database, reminders []*reminder.Reminder) error {
	// TODO(vadimsh): Implement.
	return nil
}
