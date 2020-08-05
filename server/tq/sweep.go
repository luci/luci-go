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
	"fmt"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/server/tq/internal/sweep"
	"go.chromium.org/luci/server/tq/internal/sweep/sweeppb"
	"go.chromium.org/luci/ttq/internals/databases"
	"go.chromium.org/luci/ttq/internals/partition"
)

// SweeperOptions configures the process of "sweeping" of transactional tasks
// reminders.
//
// The sweeping process ensures all transactionally committed tasks will have a
// corresponding Cloud Tasks task created. It periodically scans the database
// for "reminder" records created whenever a task is created as part of a
// transaction. A reminder older than a certain age likely indicates that the
// corresponding AddTask call crashed right after the transaction before it had
// a chance to create Cloud Tasks task. For each such old reminder, the sweeping
// will idempotently create a Cloud Task and delete the record in the database.
//
// SweeperOptions tune some of parameters of this process. Roughly:
//   1. Sweep() call in Dispatcher creates SweepShards jobs that each scan
//      a portion of the database for old reminders.
//   2. Each such job is allowed to process no more than TasksPerScan reminder
//      records. This caps its runtime and memory usage. TasksPerScan should be
//      small enough so that all sweeping jobs finish before the next Sweep()
//      call, but large enough so that it makes meaningful progress.
//   3. If a sweeping job detects there's more than TasksPerScan items it needs
//      to cover, it launches SecondaryScanShards follow-up jobs that cover the
//      remaining items. This should be happening in rare circumstances, only if
//      the database is slow or has a large backlog.
type SweeperOptions struct {
	// SweepShards defines how many jobs to produce in each Sweep.
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

	// LessorID identifies an implementation of a system that manages leases on
	// subranges of the database.
	//
	// Default is the same ID as the database implementation ID.
	LessorID string

	// SweepTaskQueue is a Cloud Tasks queue name to use for sweep jobs.
	//
	// Can be in short or full form. See Queue in TaskClass for details. The queue
	// should be configured to allow at least 10 QPS.
	//
	// Default is "default".
	TaskQueue string

	// SweepTaskPrefix is a URL prefix to use for sweep jobs.
	//
	// There should be a Dispatcher instance somewhere that is configured to
	// receive such tasks (via non-default ServingPrefix). This is useful if
	// you want to limit what processes process the sweeps.
	//
	// Default is "/internal/tasks".
	TaskPrefix string

	// TaskHost is a hostname to dispatch sweep jobs to.
	//
	// Default is "", meaning to use whatever is configured as default in
	// the Dispatcher.
	TaskHost string
}

// NewDistributedSweeper creates a sweeper that distributes and executes
// sweeping tasks through the given dispatcher using the given TaskClass.
//
// TaskClass's Payload, Kind and Handler will be set to necessary values. Rest
// of the fields (used to decide how to route the task) are used as given.
//
// Will use the dispatcher's Submitter to submit Cloud Tasks. It should be set
// already.
func NewDistributedSweeper(disp *Dispatcher, opts SweeperOptions) Sweeper {
	if opts.SweepShards <= 0 {
		opts.SweepShards = 16
	}
	if opts.TasksPerScan <= 0 {
		opts.TasksPerScan = 2048
	}
	if opts.SecondaryScanShards <= 0 {
		opts.SecondaryScanShards = 16
	}
	if opts.TaskQueue == "" {
		opts.TaskQueue = "default"
	}
	if opts.TaskPrefix == "" {
		opts.TaskPrefix = "/internal/tasks"
	}

	// We adapt sweep.Distributed to use Dispatcher task routing since it can't
	// do it itself due to package import cycles.
	if disp.Submitter == nil {
		panic("the Dispatcher has no Submitter set")
	}

	impl := &sweeperImpl{disp: disp, opts: opts}

	// Make sweep.Distributed submit raw Cloud Tasks requests via `impl`, enqueue
	// sweep jobs via `disp`, which will eventually result in ExecSweepTask.
	distr := &sweep.Distributed{Submitter: impl}
	distr.EnqueueSweepTask = sweepTaskRouting(disp, opts, distr.ExecSweepTask)

	// Make startSweep submit initial sweep tasks in the same way too.
	impl.enqueue = distr.EnqueueSweepTask

	return impl
}

// sweeperImpl implements Sweeper interface via a callback and Submitter by
// deferring to the current submitter in the Dispatcher.
//
// It will be called concurrently.
type sweeperImpl struct {
	opts    SweeperOptions
	disp    *Dispatcher
	enqueue func(ctx context.Context, task *sweeppb.SweepTask) error
}

// CreateTask delegates to disp.Submitter.CreateTask.
//
// This is useful in tests to make sure changes to disp.Submitter affect
// the sweeper too.
func (s *sweeperImpl) CreateTask(ctx context.Context, req *taskspb.CreateTaskRequest) error {
	return s.disp.Submitter.CreateTask(ctx, req)
}

// startSweep initiates an asynchronous sweep of the entire reminder keyspace.
func (s *sweeperImpl) startSweep(ctx context.Context, reminderKeySpaceBytes int) error {
	partitions := partition.Universe(reminderKeySpaceBytes).Split(s.opts.SweepShards)
	return parallel.WorkPool(16, func(work chan<- func() error) {
		for _, db := range databases.Kinds() {
			for shard, part := range partitions {
				lessorID := s.opts.LessorID
				if lessorID == "" {
					lessorID = db
				}
				task := &sweeppb.SweepTask{
					Db:                  db,
					Partition:           part.String(),
					LessorId:            lessorID,
					LeaseSectionId:      fmt.Sprintf("%s_%d_%d", db, shard, len(partitions)),
					ShardCount:          int32(len(partitions)),
					ShardIndex:          int32(shard),
					Level:               0,
					KeySpaceBytes:       int32(reminderKeySpaceBytes),
					TasksPerScan:        int32(s.opts.TasksPerScan),
					SecondaryScanShards: int32(s.opts.SecondaryScanShards),
				}
				work <- func() error { return s.enqueue(ctx, task) }
			}
		}
	})
}

type sweepEnqueue func(context.Context, *sweeppb.SweepTask) error
type sweepExecute func(context.Context, *sweeppb.SweepTask) error

// sweepTaskRouting sets up a route so that a task enqueued via the returned
// callback eventually results in `exec` call somewhere.
func sweepTaskRouting(disp *Dispatcher, opts SweeperOptions, exec sweepExecute) sweepEnqueue {
	disp.RegisterTaskClass(TaskClass{
		ID:            "tq-sweep",
		Prototype:     (*sweeppb.SweepTask)(nil),
		Kind:          NonTransactional,
		Queue:         opts.TaskQueue,
		RoutingPrefix: opts.TaskPrefix,
		TargetHost:    opts.TaskHost,
		Handler: func(ctx context.Context, msg proto.Message) error {
			return exec(ctx, msg.(*sweeppb.SweepTask))
		},
	})
	return func(ctx context.Context, task *sweeppb.SweepTask) error {
		return disp.AddTask(ctx, &Task{
			Payload: task,
			Title:   fmt.Sprintf("l%d_%d_%d", task.Level, task.ShardIndex, task.ShardCount),
		})
	}
}
