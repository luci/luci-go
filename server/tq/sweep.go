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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/server/tq/internal/sweep"
	"go.chromium.org/luci/server/tq/internal/sweep/sweeppb"
	"go.chromium.org/luci/ttq/internals/databases"
	"go.chromium.org/luci/ttq/internals/partition"
)

// NewDistributedSweeper creates a sweeper that distributes and executes
// sweeping tasks through the given dispatcher using the given TaskClass.
//
// TaskClass's Payload, Kind and Handler will be set to necessary values. Rest
// of the fields (used to decide how to route the task) are used as given.
//
// Will use the dispatcher's Submitter to submit Cloud Tasks. It should be set
// already.
func NewDistributedSweeper(disp *Dispatcher, tc TaskClass) Sweeper {
	// We adapt sweep.Distributed to use Dispatcher task routing since it can't
	// do it itself due to package import cycles.
	if disp.Submitter == nil {
		panic("the Dispatcher has no Submitter set")
	}
	distr := &sweep.Distributed{Submitter: disp.Submitter}
	distr.EnqueueSweepTask = sweepTaskRouting(disp, tc, distr.ExecSweepTask)
	return sweeperImpl{enqueue: distr.EnqueueSweepTask}
}

// sweeperImpl implements Sweeper interface via a callback.
//
// It will be called concurrently.
type sweeperImpl struct {
	enqueue func(ctx context.Context, task *sweeppb.SweepTask) error
}

// startSweep initiates an asynchronous sweep of given partitions across all
// registered databases.
func (s sweeperImpl) startSweep(ctx context.Context, partitions []*partition.Partition) error {
	return parallel.WorkPool(16, func(work chan<- func() error) {
		for _, db := range databases.Kinds() {
			for shard, part := range partitions {
				task := &sweeppb.SweepTask{
					Db:                  db,
					Partition:           part.String(),
					LessorId:            db, // TODO: make configurable
					LeaseSectionId:      fmt.Sprintf("%s_%d_%d", db, shard, len(partitions)),
					ShardCount:          int32(len(partitions)),
					ShardIndex:          int32(shard),
					Level:               0,
					KeySpaceBytes:       int32(reminderKeySpaceBytes),
					TasksPerScan:        2048, // TODO: make configurable
					SecondaryScanShards: 16,   // TODO: make configurable
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
func sweepTaskRouting(disp *Dispatcher, tc TaskClass, exec sweepExecute) sweepEnqueue {
	tc.Prototype = (*sweeppb.SweepTask)(nil)
	tc.Kind = NonTransactional
	tc.Handler = func(ctx context.Context, msg proto.Message) error {
		return exec(ctx, msg.(*sweeppb.SweepTask))
	}
	disp.RegisterTaskClass(tc)
	return func(ctx context.Context, task *sweeppb.SweepTask) error {
		return disp.AddTask(ctx, &Task{
			Payload: task,
			Title:   fmt.Sprintf("l%d_%d_%d", task.Level, task.ShardIndex, task.ShardCount),
		})
	}
}
