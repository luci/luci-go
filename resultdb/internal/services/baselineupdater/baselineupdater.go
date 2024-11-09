// Copyright 2023 The LUCI Authors.
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

// Package baselineupdater marks test variants from an invocation as submitted by
// adding the test variants to the set of tests for its baseline identifier.
package baselineupdater

import (
	"context"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/baselines"
	btv "go.chromium.org/luci/resultdb/internal/baselines/testvariants"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	tr "go.chromium.org/luci/resultdb/internal/testresults"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// BaselineUpdaterTasks describes how to route mark submitted tasks.
var BaselineUpdaterTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:            "update-baseline",
	Prototype:     &taskspb.MarkInvocationSubmitted{},
	Kind:          tq.Transactional,
	Queue:         "baselineupdater",                 // use a dedicated queue
	RoutingPrefix: "/internal/tasks/baselineupdater", // for routing to "baselineupdater" service
})

// TransactionLimit is set to 8000 because Cloud Spanner limits 40k mutations per transaction.
// We have 5 columns to write to per row, and 40,000/5 = 8000.
var TransactionLimit = 8000

func Schedule(ctx context.Context, invID string) {
	tq.MustAddTask(ctx, &tq.Task{
		Payload: &taskspb.MarkInvocationSubmitted{InvocationId: invID},
		Title:   invID,
	})
}

// InitServer initializes a baselineupdator server.
func InitServer(srv *server.Server) {
	// init() below takes care of everything.
}

func init() {
	BaselineUpdaterTasks.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		task := msg.(*taskspb.MarkInvocationSubmitted)
		err := tryMarkInvocationSubmitted(ctx, invocations.ID(task.InvocationId))
		if _, ok := status.FromError(errors.Unwrap(err)); ok {
			// Spanner gRPC error.
			return transient.Tag.Apply(err)
		}
		return err
	})
}

// Marking an invocation is asynchronous. Invocations must be finalized prior
// to it being marked submitted. Non-finalized invocations are marked as submitted
// and will be scheduled by the finalizer.
//
// When an invocation is marked submitted, all test variants from that invocation
// are added to the set of test variants for the invocation's baseline. In other
// words, the set of tests expected to run for a baseline are updated with the
// test variants from the provided invocation. Adding all test variants
// from an invocation to its baseline are done recursively.
//
// For example, if invocation A for baseline "try:linux-rel" is finalized with
// test A and B and "try:linux-rel" has A and C in its set of test variants,
// this call would update the final set of test variants to (A, B and C).
//
// Marking an invocation submitted also updates the Baselines table with new
// baselines.
func tryMarkInvocationSubmitted(ctx context.Context, invID invocations.ID) error {
	inv, err := invocations.Read(span.Single(ctx), invID)
	if err != nil {
		return errors.Annotate(err, "read invocation").Err()
	}

	if inv.BaselineId == "" {
		// It's valid for a baseline to not be specified, so this workflow
		// will terminate early.
		return nil
	}

	if err := shouldMarkSubmitted(inv); err != nil {
		return errors.Annotate(err, "mark invocation submitted").Err()
	}

	if err = ensureBaselineExists(ctx, inv); err != nil {
		return errors.Annotate(err, "mark invocation submitted").Err()
	}

	return markInvocationSubmitted(ctx, inv)
}

// shouldMarkSubmitted returns an error if the invocation is ready to be marked
// as submitted.
func shouldMarkSubmitted(inv *pb.Invocation) error {
	// all sub invocations should be finalized if the parent invocation is finalized.
	if inv.State != pb.Invocation_FINALIZED {
		return errors.Reason("the invocation is not yet finalized").Err()
	}

	return nil
}

// markBaselineNew adds the baseline to the Baselines table if it's not present
// in the BaselineTestVariants table. Baselines present will have the LastUpdated
// time reset to the commit timestamp.
func ensureBaselineExists(ctx context.Context, inv *pb.Invocation) error {
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		project, _ := realms.Split(inv.Realm)
		baselineID := inv.BaselineId

		_, err := baselines.Read(ctx, project, baselineID)
		if err != nil {
			if err == baselines.NotFound {
				// if it isn't found, we can create the baseline in the table and terminate.
				span.BufferWrite(ctx, baselines.Create(project, baselineID))
				return nil
			} else {
				return errors.Annotate(err, "read baseline").Err()
			}
		}

		// If the baseline exists, we'll update LastUpdatedTime so that it is not
		// automatically ejected from the table.
		span.BufferWrite(ctx, baselines.UpdateLastUpdatedTime(project, baselineID))
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "ensure baseline").Err()
	}

	return nil
}

func markInvocationSubmitted(ctx context.Context, inv *pb.Invocation) error {
	invID := invocations.MustParseName(inv.Name)
	baselineID := inv.BaselineId
	project, _ := realms.Split(inv.Realm)

	masks := mask.MustFromReadMask(&pb.TestResult{},
		"test_id",
		"variant_hash",
		"status",
	)

	rCtx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	idSet := make([]invocations.ID, 0)
	idSet = append(idSet, invocations.ID(invID))
	invs, err := graph.Reachable(rCtx, invocations.NewIDSet(invID))
	if err != nil {
		return errors.Annotate(err, "discover reachable invocations").Err()
	}
	for invID, reachableInv := range invs.Invocations {
		if !reachableInv.HasTestResults {
			continue
		}
		idSet = append(idSet, invID)
	}
	q := &tr.Query{
		Predicate:     &pb.TestResultPredicate{},
		PageSize:      0,
		InvocationIDs: invocations.NewIDSet(idSet...),
		Mask:          masks,
	}

	// This will sequentially load results and process them when we reach mutation
	// limits. This is not the quickest way to process the results and has room
	// for optimization.
	ms := make([]*spanner.Mutation, 0)
	err = q.Run(rCtx, func(tr *pb.TestResult) error {
		if tr.Status == pb.TestStatus_SKIP {
			// We'll ignore SKIPPED from being BaselineTestVariants. This allows
			// it to be verified for flakiness when it no longer becomes skipped.
			logging.Debugf(ctx, "Skipped adding %s for baselineID %s", tr.TestId, baselineID)
			return nil
		}
		ms = append(ms, btv.InsertOrUpdate(project, baselineID, tr.TestId, tr.VariantHash))

		if len(ms) >= TransactionLimit {
			_, err := span.ReadWriteTransaction(ctx, func(rwCtx context.Context) error {
				span.BufferWrite(rwCtx, ms...)
				return nil
			})
			if err != nil {
				return errors.Annotate(err, "write baseline test variants").Err()
			}
			ms = make([]*spanner.Mutation, 0)
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "query test variants").Err()
	}

	// Insert remaining test variants as a final write transaction.
	if len(ms) > 0 {
		_, err := span.ReadWriteTransaction(ctx, func(rwCtx context.Context) error {
			span.BufferWrite(rwCtx, ms...)
			return nil
		})
		if err != nil {
			return errors.Annotate(err, "write baseline test variants").Err()
		}
	}

	return nil
}
