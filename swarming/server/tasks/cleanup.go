// Copyright 2024 The LUCI Authors.
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

package tasks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks/taskspb"
)

const (
	// TaskRequests (and all their child entities) older than this are deleted by
	// the cleanup cron job.
	maxTaskAge = 18 * 31 * 24 * time.Hour

	// How long to scan old tasks in the cron job before giving up.
	//
	// Should be lower than the cron frequency.
	maxCronRunTime = 9 * time.Minute

	// Upper bound on randomized delay for deletion tasks.
	//
	// Should be lower than the cron frequency.
	maxDelay = 8 * time.Minute

	// Maximum number of tasks to delete in a single CleanupOldTasks TQ task.
	cleanupTaskBatchSize = 500
)

// RegisterCleanupHandlers registers cron and TQ handlers that implement old
// task cleanup.
func RegisterCleanupHandlers(crondisp *cron.Dispatcher, tqdisp *tq.Dispatcher) {
	(&tasksCleaner{
		cron:                 crondisp,
		tq:                   tqdisp,
		maxTaskAge:           maxTaskAge,
		maxCronRunTime:       maxCronRunTime,
		maxDelay:             maxDelay,
		cleanupTaskBatchSize: cleanupTaskBatchSize,
	}).register()
}

type tasksCleaner struct {
	cron                 *cron.Dispatcher
	tq                   *tq.Dispatcher
	maxTaskAge           time.Duration
	maxCronRunTime       time.Duration
	maxDelay             time.Duration
	cleanupTaskBatchSize int
}

func (tc *tasksCleaner) register() {
	tc.cron.RegisterHandler("cleanup-old-tasks", func(ctx context.Context) error {
		return tc.triggerCleanupTasks(ctx)
	})
	tc.tq.RegisterTaskClass(tq.TaskClass{
		ID:        "cleanup-old-tasks",
		Kind:      tq.NonTransactional,
		Prototype: (*taskspb.CleanupOldTasks)(nil),
		Queue:     "delete-tasks",
		Handler: func(ctx context.Context, payload proto.Message) error {
			return tc.execCleanupTask(ctx, payload.(*taskspb.CleanupOldTasks))
		},
	})
}

// triggerCleanupTasks enqueues CleanupOldTasks TQ tasks.
//
// TODO: If the enqueued cleanup jobs aren't processed fast enough (i.e. within
// the cron interval), the cron job will keep producing duplicate tasks. This
// should not affect correctness, but it is a waste of time and a potential
// cause of instability (if the queue is already full, producing copies of
// existing tasks will just make the situation worse). We can keep track of
// the last processed timestamp and scan only unprocessed range.
func (tc *tasksCleaner) triggerCleanupTasks(ctx context.Context) error {
	// Enqueues a TQ task to delete a bunch of Swarming tasks. Stagger their
	// execution by randomizing delay to avoid hammering the datastore index all
	// at once. Note that we could do the same by setting a rate limit on the
	// Cloud Tasks queue, but such a rate limit would need a periodic tuning to
	// make sure we don't grow the queue length indefinitely. Randomized delay
	// doesn't need such tuning.
	var batch []string
	flushBatch := func() error {
		if len(batch) != 0 {
			tasks := batch
			batch = nil
			delay := randomizeDelay(ctx, tc.maxDelay)
			logging.Infof(ctx, "Enqueuing deletion of %d entity groups after %.1f sec", len(tasks), delay.Seconds())
			return tc.tq.AddTask(ctx, &tq.Task{
				Payload: &taskspb.CleanupOldTasks{TaskIds: tasks},
				Title:   fmt.Sprintf("%s-%s", tasks[0], tasks[len(tasks)-1]),
				Delay:   delay,
			})
		}
		return nil
	}

	q := datastore.NewQuery("TaskRequest").
		Lte("created_ts", clock.Now(ctx).Add(-tc.maxTaskAge).UTC()).
		KeysOnly(true)

	qctx, cancel := clock.WithTimeout(ctx, tc.maxCronRunTime)
	defer cancel()

	// Note this visits tasks in order of oldest to newest. We abort on the first
	// error to avoid "gaps" in the unprocessed part of the backlog.
	err := datastore.Run(qctx, q, func(key *datastore.Key) error {
		batch = append(batch, model.RequestKeyToTaskID(key, model.AsRequest))
		if len(batch) == tc.cleanupTaskBatchSize {
			return flushBatch()
		}
		return nil
	})

	// Timing out is semi-expected.
	if qctx.Err() != nil {
		logging.Warningf(ctx, "Scan aborted: %s", qctx.Err())
		err = nil
	}

	// Flush the final batch.
	if err == nil {
		err = flushBatch()
	}

	return err
}

// execCleanupTask deletes a bunch of TaskRequest entity groups in parallel.
func (tc *tasksCleaner) execCleanupTask(ctx context.Context, t *taskspb.CleanupOldTasks) error {
	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(16)

	var (
		mu    sync.Mutex
		fails []string
	)

	for _, taskID := range t.TaskIds {
		key, err := model.TaskIDToRequestKey(ctx, taskID)
		if err != nil {
			// This should not be happening.
			logging.Errorf(ctx, "Skipping malformed task ID %q: %s", taskID, err)
			continue
		}
		eg.Go(func() error {
			lctx := logging.SetField(ctx, "taskID", taskID)
			if !cleanupEntityGroup(lctx, key, 100) {
				mu.Lock()
				fails = append(fails, taskID)
				mu.Unlock()
			}
			return nil
		})
	}

	_ = eg.Wait()

	if len(fails) == 0 {
		return nil
	}

	logging.Warningf(ctx, "Submitting a task to retry deletion of %d remaining entity groups", len(fails))
	return tc.tq.AddTask(ctx, &tq.Task{
		Payload: &taskspb.CleanupOldTasks{
			TaskIds: fails,
			Retries: t.Retries + 1,
		},
		Title: fmt.Sprintf("retry-%d-%s-%s", t.Retries+1, fails[0], fails[len(fails)-1]),
	})
}

// cleanupEntityGroup deletes TaskRequest entity group.
//
// Deletes as many entities as possible (even in presence of errors).
//
// Returns true if the entity group was deleted or false if there were errors
// and some entities remained. Errors are logged inside.
func cleanupEntityGroup(ctx context.Context, reqKey *datastore.Key, batchSize int) bool {
	q := datastore.NewQuery("").Ancestor(reqKey).KeysOnly(true)

	var batch []*datastore.Key
	var deleted int
	var failed int

	flushBatch := func() {
		if len(batch) != 0 {
			if err := datastore.Delete(ctx, batch); err != nil {
				logging.Errorf(ctx, "Failed to deleted %d entities: %s", len(batch), err)
				failed += len(batch)
			} else {
				deleted += len(batch)
			}
			batch = nil
		}
	}

	err := datastore.Run(ctx, q, func(key *datastore.Key) error {
		// The TaskRequest entity itself will be deleted separately last. Its
		// presence in the datastore is an indicator that there's some entities
		// under its entity group. For that reason we must delete it last, after
		// confirming all other entities are successfully deleted.
		if !key.Equal(reqKey) {
			batch = append(batch, key)
			if len(batch) == batchSize {
				flushBatch()
			}
		}
		return nil
	})
	if err != nil {
		logging.Errorf(ctx, "Failed to fully scan the entity group: %s", err)
	}

	flushBatch()

	if failed == 0 && err == nil {
		// Now that all other entities are successfully deleted it is fine to delete
		// the root key as well, since we won't need it anymore.
		if err = datastore.Delete(ctx, reqKey); err != nil {
			logging.Errorf(ctx, "Failed to deleted TaskRequest: %s", err)
			failed++
		} else {
			deleted++
		}
	}

	if failed != 0 {
		logging.Infof(ctx, "Deleted %d entities, failed to delete %d entities", deleted, failed)
	} else {
		logging.Infof(ctx, "Deleted %d entities", deleted)
	}
	return failed == 0 && err == nil
}

// randomizeDelay returns a random duration up to `max`.
func randomizeDelay(ctx context.Context, max time.Duration) time.Duration {
	if max == 0 {
		return 0
	}
	return time.Duration(mathrand.Int63n(ctx, int64(max)))
}
