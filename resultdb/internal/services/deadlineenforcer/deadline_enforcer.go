// Copyright 2021 The LUCI Authors.
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

// Package deadlineenforcer finalizes tasks with overdue deadlines.
package deadlineenforcer

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/cron"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const maxInvocationsPerShardToEnforceAtOnce = 100
const maxWorkUnitsPerShardToEnforceAtOnce = 100
const enforcerShardCount = 100

var (
	// timeWorkUnitsOverdue tracks the delay between work units expiring and being
	// picked up by deadlineenforcer.
	timeWorkUnitsOverdue = metric.NewCumulativeDistribution(
		"resultdb/deadlineenforcer/delay",
		"Delay between work unit expiration and forced finalization",
		&types.MetricMetadata{Units: types.Milliseconds},
		nil,
		field.String("realm"),
	)

	// timeInvocationsOverdue tracks the delay between invocations expiring and being
	// picked up by deadlineenforcer.
	timeInvocationsOverdue = metric.NewCumulativeDistribution(
		"resultdb/deadlineenforcer/delay_legacy_invocations",
		"Delay between legacy invocation expiration and forced finalization",
		&types.MetricMetadata{Units: types.Milliseconds},
		nil,
		field.String("realm"),
	)

	// overdueWorkUnitsFinalized counts work units finalized by the
	// deadlineenforcer service.
	overdueWorkUnitsFinalized = metric.NewCounter(
		"resultdb/deadlineenforcer/finalized_work_units",
		"Work units finalized by deadline enforcer",
		&types.MetricMetadata{Units: "work units"},
		field.String("realm"),
	)

	// overdueInvocationsFinalized counts invocations finalized by the
	// deadlineenforcer service.
	overdueInvocationsFinalized = metric.NewCounter(
		"resultdb/deadlineenforcer/finalized_invocations",
		"Invocations finalized by deadline enforcer",
		&types.MetricMetadata{Units: "invocations"},
		field.String("realm"),
	)
)

const DeadlineEnforcedMessage = "The task failed to report its state to ResultDB before the reporting deadline expired. To avoid this error, the task scheduler should clean up after tasks using the FinalizeWorkUnit and FinalizeWorkUnitDescendants RPCs."

// Options are for configuring the deadline enforcer.
type Options struct {
	// ForceCronInterval forces minimum interval in cron jobs.
	// Useful in integration tests to reduce the test time.
	ForceCronInterval time.Duration
}

// InitServer initializes a deadline enforcer server.
func InitServer(srv *server.Server, opts Options) {
	minInterval := time.Minute
	if opts.ForceCronInterval > 0 {
		minInterval = opts.ForceCronInterval
	}
	srv.RunInBackground("resultdb.deadlineenforcer", func(ctx context.Context) {
		run(ctx, minInterval)
	})
	srv.RunInBackground("resultdb.deadlineenforcerlegacy", func(ctx context.Context) {
		runLegacy(ctx, minInterval)
	})
}

// run blocks continuously finalizes expired work units.
// It blocks until context is canceled.
func run(ctx context.Context, minInterval time.Duration) {
	cron.Group(ctx, enforcerShardCount, minInterval, enforceOneShard)
}

// enforceOneShard finalizes expired work units on the given shard.
func enforceOneShard(ctx context.Context, shardIndex int) error {
	limit := maxWorkUnitsPerShardToEnforceAtOnce
	for {
		cnt, err := enforce(ctx, shardIndex, limit)
		if err != nil {
			return err
		}
		if cnt != limit {
			// The last page wasn't full, there likely aren't any more
			// overdue work units for now.
			break
		}
	}
	return nil
}

// enforce finalizes a batch of expired work units on the given shard.
// It returns the number of expired work units processed.
func enforce(ctx context.Context, shardIndex, limit int) (int, error) {
	opts := workunits.ReadDeadlineExpiredOptions{
		ShardIndex: shardIndex,
		ShardCount: enforcerShardCount,
		Limit:      limit,
	}

	// Read a batch of work units with expired deadlines.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	expiredWUs, err := workunits.ReadDeadlineExpired(ctx, opts)
	if err != nil {
		return 0, err
	}

	// Finalize the batch of work units.
	// Stores whether expiredWUs[i] was ultimately finalized.
	var finalized []bool
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Reset on each transaction attempt to avoid state from
		// previous aborted R/W transactions leaking out.
		finalized = make([]bool, len(expiredWUs))

		ids := make([]workunits.ID, 0, len(expiredWUs))
		for _, wu := range expiredWUs {
			ids = append(ids, wu.ID)
		}
		states, err := workunits.ReadFinalizationStates(ctx, ids)
		if err != nil {
			return err
		}
		rootInvocationsScheduled := rootinvocations.NewIDSet()
		for i, state := range states {
			if state != pb.WorkUnit_ACTIVE {
				// Finalization already started (possible race with explicit
				// finalization). Do not start finalization again as doing
				// so would overwrite the existing FinalizeStartTime
				// and create an unnecessary task.
				finalized[i] = false
				continue
			}

			wu := expiredWUs[i]

			// Transition the work unit to a FAILED state. This also transitions
			// it to FINALIZING finalizaton state.
			mb := workunits.NewMutationBuilder(wu.ID)
			mb.UpdateState(pb.WorkUnit_FAILED)
			mb.UpdateSummaryMarkdown(DeadlineEnforcedMessage)
			span.BufferWrite(ctx, mb.Build()...)

			finalized[i] = true

			if _, ok := rootInvocationsScheduled[wu.ID.RootInvocationID]; !ok {
				// Transactionally schedule a work unit finalization task for each
				// impacted root invocation.
				if err := tasks.ScheduleWorkUnitsFinalization(ctx, wu.ID.RootInvocationID); err != nil {
					return err
				}
				rootInvocationsScheduled.Add(wu.ID.RootInvocationID)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for i, wu := range expiredWUs {
		if finalized[i] {
			overdueWorkUnitsFinalized.Add(ctx, 1, wu.Realm)
			timeWorkUnitsOverdue.Add(ctx, float64(clock.Now(ctx).Sub(wu.ActiveDeadline).Milliseconds()), wu.Realm)
		}
	}
	return len(expiredWUs), err
}

// blocks continuously finalizes expired invocations.
// It blocks until context is canceled.
func runLegacy(ctx context.Context, minInterval time.Duration) {
	maxShard, err := invocations.CurrentMaxShard(ctx)
	switch {
	case err == spanutil.ErrNoResults:
		maxShard = invocations.Shards - 1
	case err != nil:
		panic(errors.Fmt("failed to determine number of shards: %w", err))
	}

	// Start one cron job for each shard of the database.
	cron.Group(ctx, maxShard+1, minInterval, enforceOneShardLegacy)
}

func enforceOneShardLegacy(ctx context.Context, shard int) error {
	limit := maxInvocationsPerShardToEnforceAtOnce
	for {
		cnt, err := enforceLegacy(ctx, shard, limit)
		if err != nil {
			return err
		}
		if cnt != limit {
			// The last page wasn't full, there likely aren't any more
			// overdue invocations for now.
			break
		}
	}
	return nil
}

func enforceLegacy(ctx context.Context, shard, limit int) (int, error) {
	st := spanner.NewStatement(`
		SELECT InvocationId, ActiveDeadline, Realm
		FROM Invocations@{FORCE_INDEX=InvocationsByActiveDeadline, spanner_emulator.disable_query_null_filtered_index_check=true}
		WHERE ShardId = @shardId
			AND ActiveDeadline <= @now
			AND InvocationID NOT LIKE '%:workunit:%'
			AND InvocationID NOT LIKE '%:root:%'
		LIMIT @limit
	`)
	st.Params["now"] = clock.Now(ctx)
	st.Params["shardId"] = shard
	st.Params["limit"] = limit
	rowCount := 0

	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	err := spanutil.Query(ctx, st, func(row *spanner.Row) error {
		rowCount++
		var id invocations.ID
		var ts *timestamppb.Timestamp
		var realm string
		if err := spanutil.FromSpanner(row, &id, &ts, &realm); err != nil {
			return err
		}
		// TODO(crbug.com/1207606): Increase parallelism.
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			state, err := invocations.ReadState(ctx, id)
			if err != nil {
				return err
			}

			if state != pb.Invocation_ACTIVE {
				// Finalization already started (possible race with explicit
				// finalization). Do not start finalization again as doing
				// so would overwrite the existing FinalizeStartTime
				// and create an unnecessary task.
				return nil
			}

			if id.IsRootInvocation() || id.IsWorkUnit() {
				// Do nothing, root invocation and work units should not be handled by this path.
			} else {
				span.BufferWrite(ctx, invocations.MarkFinalizing(id))
				tasks.StartInvocationFinalization(ctx, id)
			}
			return nil
		})
		if err == nil {
			overdueInvocationsFinalized.Add(ctx, 1, realm)
			timeInvocationsOverdue.Add(ctx, float64(clock.Now(ctx).Sub(ts.AsTime()).Milliseconds()), realm)
		}
		return err
	})
	return rowCount, err
}
