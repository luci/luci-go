// Copyright 2019 The LUCI Authors.
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

package deriver

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	swarmingAPI "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/services/deriver/chromium"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/tasks"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// testResultBatchSizeMax is the maximum number of TestResults to include per transaction.
const testResultBatchSizeMax = 1000

var urlPrefixes = []string{"http://", "https://"}

// validateDeriveChromiumInvocationRequest returns an error if req is invalid.
func validateDeriveChromiumInvocationRequest(req *pb.DeriveChromiumInvocationRequest) error {
	if req.SwarmingTask == nil {
		return errors.Reason("swarming_task missing").Err()
	}

	if req.SwarmingTask.Hostname == "" {
		return errors.Reason("swarming_task.hostname missing").Err()
	}

	for _, prefix := range urlPrefixes {
		if strings.HasPrefix(req.SwarmingTask.Hostname, prefix) {
			return errors.Reason("swarming_task.hostname should not have prefix %q", prefix).Err()
		}
	}

	if req.SwarmingTask.Id == "" {
		return errors.Reason("swarming_task.id missing").Err()
	}

	return nil
}

// DeriveChromiumInvocation derives the invocation associated with the given swarming task.
//
// The invocation returned is associated with the swarming task itself.
// If the task is deduped against another task, the invocation returned includes the underlying one.
func (s *deriverServer) DeriveChromiumInvocation(ctx context.Context, in *pb.DeriveChromiumInvocationRequest) (*pb.Invocation, error) {
	if err := validateDeriveChromiumInvocationRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Get the swarming service to use.
	swarmingURL := "https://" + in.SwarmingTask.Hostname
	swarmSvc, err := chromium.GetSwarmSvc(internal.HTTPClient(ctx), swarmingURL)
	if err != nil {
		return nil, errors.Annotate(err, "creating swarming client for %q", swarmingURL).Err()
	}

	// Get the swarming task.
	task, err := chromium.GetSwarmingTask(ctx, in.SwarmingTask.Id, swarmSvc)
	if err != nil {
		return nil, errors.Annotate(err, "getting swarming task %q on %q",
			in.SwarmingTask.Id, in.SwarmingTask.Hostname).Err()
	}
	invID := chromium.GetInvocationID(task, in)

	client := span.Client(ctx)

	// Check if we need to write this invocation.
	switch doWrite, err := shouldWriteInvocation(ctx, client.Single(), invID); {
	case err != nil:
		return nil, err
	case !doWrite:
		readTxn := client.ReadOnlyTransaction()
		defer readTxn.Close()
		return span.ReadInvocationFull(ctx, readTxn, invID)
	}

	inv, err := chromium.DeriveChromiumInvocation(task, in)
	if err != nil {
		return nil, err
	}

	// Derive the origin invocation and results.
	var originInv *pb.Invocation
	switch originInv, err = s.deriveInvocationForOriginTask(ctx, in, task, swarmSvc, client); {
	case err != nil:
		return nil, err
	case inv.Name == originInv.Name: // origin task is the task itself, we're done.
		return originInv, nil
	}

	// Include originInv into inv.
	inv.IncludedInvocations = []string{originInv.Name}
	invMs := []*spanner.Mutation{
		span.InsertMap("Invocations", s.rowOfInvocation(ctx, inv, "")),
		span.InsertMap("IncludedInvocations", map[string]interface{}{
			"InvocationId":         invID,
			"IncludedInvocationId": span.MustParseInvocationName(originInv.Name),
		}),
	}
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		switch doWrite, err := shouldWriteInvocation(ctx, txn, invID); {
		case err != nil:
			return err
		case !doWrite:
			return nil
		default:
			return txn.BufferWrite(invMs)
		}
	})
	if err != nil {
		return nil, err
	}
	span.IncRowCount(ctx, 1, span.Invocations, span.Inserted)
	return inv, nil
}

func shouldWriteInvocation(ctx context.Context, txn span.Txn, id span.InvocationID) (bool, error) {
	state, err := span.ReadInvocationState(ctx, txn, id)
	s, _ := appstatus.Get(err)
	switch {
	case s.Code() == codes.NotFound:
		// No such invocation found means we may have to write it, so proceed.
		return true, nil

	case err != nil:
		return false, err

	case state != pb.Invocation_FINALIZED:
		return false, errors.Reason(
			"attempting to derive an existing non-finalized invocation").Err()
	}

	// The invocation exists and is finalized, so no need to write it.
	return false, nil
}

// deriveInvocationForOriginTask derives an invocation and test results
// from a given task and returns derived origin invocation.
func (s *deriverServer) deriveInvocationForOriginTask(ctx context.Context, in *pb.DeriveChromiumInvocationRequest, task *swarmingAPI.SwarmingRpcsTaskResult, swarmSvc *swarmingAPI.Service, client *spanner.Client) (*pb.Invocation, error) {
	// Get the origin task that the task is deduped against. Or the task
	// itself if it's not deduped.
	originTask, err := chromium.GetOriginTask(ctx, task, swarmSvc)
	if err != nil {
		return nil, errors.Annotate(err, "getting origin for swarming task %q on %q",
			in.SwarmingTask.Id, in.SwarmingTask.Hostname).Err()
	}
	originInvID := chromium.GetInvocationID(originTask, in)

	// Check if we need to write origin invocation.
	switch doWrite, err := shouldWriteInvocation(ctx, client.Single(), originInvID); {
	case err != nil:
		return nil, err
	case !doWrite: // Origin invocation is already in Spanner. Return it.
		readTxn := client.ReadOnlyTransaction()
		defer readTxn.Close()
		return span.ReadInvocationFull(ctx, readTxn, originInvID)
	}
	originInv, err := chromium.DeriveChromiumInvocation(originTask, in)
	if err != nil {
		return nil, err
	}

	// Get the protos and prepare to write them to Spanner.
	logging.Infof(ctx, "Deriving task %q on %q", originTask.TaskId, in.SwarmingTask.Hostname)
	results, err := chromium.DeriveTestResults(ctx, originTask, in, originInv)
	if err != nil {
		return nil, errors.Annotate(err,
			"task %q on %q named %q", in.SwarmingTask.Id, in.SwarmingTask.Hostname, originTask.Name).Err()
	}
	// TODO(jchinlee): Validate invocation and results.

	// Write test results in batches concurrently, updating inv with the names of the invocations
	// that will be included.
	batchInvs, err := s.batchInsertTestResults(ctx, originInv, results, testResultBatchSizeMax)
	if err != nil {
		return nil, err
	}
	originInv.IncludedInvocations = batchInvs.Names()

	// Prepare mutations.
	ms := make([]*spanner.Mutation, 0, len(batchInvs)+4)
	ms = append(ms, span.InsertMap("Invocations", s.rowOfInvocation(ctx, originInv, "")))
	for includedID := range batchInvs {
		ms = append(ms, span.InsertMap("IncludedInvocations", map[string]interface{}{
			"InvocationId":         originInvID,
			"IncludedInvocationId": includedID,
		}))
	}
	ms = append(ms, tasks.EnqueueBQExport(originInvID, s.InvBQTable, clock.Now(ctx).UTC()))

	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		// Check origin invocation state again.
		switch doWrite, err := shouldWriteInvocation(ctx, txn, originInvID); {
		case err != nil:
			return err
		case !doWrite:
			return nil
		default:
			return txn.BufferWrite(ms)
		}
	})
	if err != nil {
		return nil, err
	}

	span.IncRowCount(ctx, 1, span.Invocations, span.Inserted)
	return originInv, nil
}

// batchInsertTestResults inserts the given TestResults in batches under container Invocations,
// returning container ids.
func (s *deriverServer) batchInsertTestResults(ctx context.Context, inv *pb.Invocation, trs []*chromium.TestResult, batchSize int) (span.InvocationIDSet, error) {
	batches := batchTestResults(trs, batchSize)
	includedInvs := make(span.InvocationIDSet, len(batches))

	invID := span.MustParseInvocationName(inv.Name)
	eg, ctx := errgroup.WithContext(ctx)
	client := span.Client(ctx)
	for i, batch := range batches {
		i := i
		batch := batch

		batchID := batchInvocationID(invID, i)
		includedInvs.Add(batchID)

		eg.Go(func() error {
			muts := make([]*spanner.Mutation, 0, len(batch)+1)

			// Convert the container Invocation in the batch.
			batchInv := &pb.Invocation{
				Name:         batchID.Name(),
				State:        pb.Invocation_FINALIZED,
				CreateTime:   inv.CreateTime,
				FinalizeTime: inv.FinalizeTime,
				Deadline:     inv.Deadline,
			}
			muts = append(muts, span.InsertOrUpdateMap(
				"Invocations", s.rowOfInvocation(ctx, batchInv, "")),
			)

			// Convert the TestResults in the batch.
			for k, tr := range batch {
				muts = append(muts, insertOrUpdateTestResult(batchID, tr.TestResult, k))
				// TODO(crbug.com/1071258): write artifacts.
			}

			if _, err := client.Apply(ctx, muts); err != nil {
				return err
			}

			span.IncRowCount(ctx, len(batch), span.TestResults, span.Inserted)
			span.IncRowCount(ctx, 1, span.Invocations, span.Inserted)
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return includedInvs, nil
}

// batchInvocationID returns an InvocationID for the Invocation containing the referenced batch.
func batchInvocationID(invID span.InvocationID, batchInd int) span.InvocationID {
	return span.InvocationID(fmt.Sprintf("%s::batch::%d", invID, batchInd))
}

// batchTestResults batches the given TestResults given the maximum batch size.
func batchTestResults(trs []*chromium.TestResult, batchSize int) [][]*chromium.TestResult {
	batches := make([][]*chromium.TestResult, 0, len(trs)/batchSize+1)
	for len(trs) > 0 {
		end := batchSize
		if end > len(trs) {
			end = len(trs)
		}

		batches = append(batches, trs[:end])
		trs = trs[end:]
	}

	return batches
}
