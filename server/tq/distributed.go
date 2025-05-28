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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/sweep"
	"go.chromium.org/luci/server/tq/internal/tqpb"
)

// DistributedSweeperOptions is configuration for the process of "sweeping" of
// transactional tasks reminders performed in a distributed manner using Cloud
// Tasks service itself to distribute work.
//
// The sweeping process ensures all transactionally committed tasks will have a
// corresponding Cloud Tasks task created. It periodically scans the database
// for "reminder" records created whenever a task is created as part of a
// transaction. A reminder older than a certain age likely indicates that the
// corresponding AddTask call crashed right after the transaction before it had
// a chance to create Cloud Tasks task. For each such old reminder, the sweeping
// will idempotently create a Cloud Task and delete the record in the database.
//
// DistributedSweeperOptions tune some of parameters of this process. Roughly:
//  1. Sweep() call in Dispatcher creates SweepShards jobs that each scan
//     a portion of the database for old reminders.
//  2. Each such job is allowed to process no more than TasksPerScan reminder
//     records. This caps its runtime and memory usage. TasksPerScan should be
//     small enough so that all sweeping jobs finish before the next Sweep()
//     call, but large enough so that it makes meaningful progress.
//  3. If a sweeping job detects there's more than TasksPerScan items it needs
//     to cover, it launches SecondaryScanShards follow-up jobs that cover the
//     remaining items. This should be happening in rare circumstances, only if
//     the database is slow or has a large backlog.
type DistributedSweeperOptions struct {
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
	// Default is "tq-sweep".
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
// sweeping tasks through the given dispatcher.
func NewDistributedSweeper(disp *Dispatcher, opts DistributedSweeperOptions) Sweeper {
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
		opts.TaskQueue = "tq-sweep"
	}
	if opts.TaskPrefix == "" {
		opts.TaskPrefix = "/internal/tasks"
	}

	impl := &distributedSweeper{opts: opts}

	// Make sweep.Distributed submit raw Cloud Tasks requests via `impl`, enqueue
	// sweep jobs via `disp`, which will eventually result in ExecSweepTask.
	distr := &sweep.Distributed{Submitter: impl}
	distr.EnqueueSweepTask = sweepTaskRouting(disp, opts, distr.ExecSweepTask)

	// Make `sweep` submit initial sweep tasks in the same way too.
	impl.enqueue = distr.EnqueueSweepTask

	return impl
}

// distributedSweeper implements Sweeper interface via a callback and Submitter
// by deferring to the current submitter in the Dispatcher.
//
// It will be called concurrently.
type distributedSweeper struct {
	opts    DistributedSweeperOptions
	enqueue func(ctx context.Context, task *tqpb.SweepTask) error
}

// Submit delegates to the submitter in the context.
func (s *distributedSweeper) Submit(ctx context.Context, payload *reminder.Payload) error {
	sub, err := currentSubmitter(ctx)
	if err != nil {
		return status.Errorf(codes.Internal, "%s", err)
	}
	return sub.Submit(ctx, payload)
}

// sweep initiates an asynchronous sweep of the entire reminder keyspace.
func (s *distributedSweeper) sweep(ctx context.Context, _ Submitter, reminderKeySpaceBytes int) error {
	partitions := partition.Universe(reminderKeySpaceBytes).Split(s.opts.SweepShards)
	return parallel.WorkPool(16, func(work chan<- func() error) {
		for _, kind := range db.Kinds() {
			for shard, part := range partitions {
				lessorID := s.opts.LessorID
				if lessorID == "" {
					lessorID = kind
				}
				task := &tqpb.SweepTask{
					Db:                  kind,
					Partition:           part.String(),
					LessorId:            lessorID,
					LeaseSectionId:      fmt.Sprintf("%s_%d_%d", kind, shard, len(partitions)),
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

type sweepEnqueue func(context.Context, *tqpb.SweepTask) error
type sweepExecute func(context.Context, *tqpb.SweepTask) error

// sweepTaskRouting sets up a route so that a task enqueued via the returned
// callback eventually results in `exec` call somewhere.
func sweepTaskRouting(disp *Dispatcher, opts DistributedSweeperOptions, exec sweepExecute) sweepEnqueue {
	disp.RegisterTaskClass(TaskClass{
		ID:            "tq-sweep",
		Prototype:     (*tqpb.SweepTask)(nil),
		Kind:          NonTransactional,
		Quiet:         true,
		Queue:         opts.TaskQueue,
		RoutingPrefix: opts.TaskPrefix,
		TargetHost:    opts.TaskHost,
		Handler: func(ctx context.Context, msg proto.Message) error {
			err := exec(ctx, msg.(*tqpb.SweepTask))
			if err != nil && !transient.Tag.In(err) {
				err = Fatal.Apply(err)
			}
			return err
		},
	})
	return func(ctx context.Context, task *tqpb.SweepTask) error {
		err := disp.AddTask(ctx, &Task{
			Payload: task,
			Title:   fmt.Sprintf("l%d_%d_%d", task.Level, task.ShardIndex, task.ShardCount),
		})
		return errors.WrapIf(err, "adding task to sweeper task queue %q", opts.TaskQueue)
	}
}
