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

package main

import (
	"context"
	"strings"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/recorder/chromium"
)

var urlPrefixes = []string{"http://", "https://"}

// validateDeriveInvocationRequest returns an error if req is invalid.
func validateDeriveInvocationRequest(req *pb.DeriveInvocationRequest) error {
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

	if err := pbutil.ValidateVariantDef(req.GetBaseTestVariant()); err != nil {
		return errors.Annotate(err, "base_test_variant").Err()
	}

	return nil
}

// DeriveInvocation derives the invocation associated with the given swarming task.
//
// If the task is a dedup of another task, the invocation returned is the underlying one; otherwise,
// the invocation returned is associated with the swarming task itself.
func (s *recorderServer) DeriveInvocation(ctx context.Context, in *pb.DeriveInvocationRequest) (*pb.Invocation, error) {
	if err := validateDeriveInvocationRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	// Get the swarming service to use.
	swarmingURL := "https://" + in.SwarmingTask.Hostname
	swarmSvc, err := chromium.GetSwarmSvc(internal.HTTPClient(ctx), swarmingURL)
	if err != nil {
		return nil, err
	}

	// Get the swarming task, deduping if necessary.
	task, err := chromium.GetSwarmingTask(ctx, in.SwarmingTask.Id, swarmSvc)
	if err != nil {
		return nil, err
	}
	if task, err = chromium.GetOriginTask(ctx, task, swarmSvc); err != nil {
		return nil, err
	}
	invID, err := chromium.GetInvocationID(ctx, task, in)
	if err != nil {
		return nil, err
	}

	client := span.Client(ctx)

	// Check if we even need to write this invocation: is it finalized?
	doWrite, err := shouldWriteInvocation(ctx, client.Single(), invID)
	switch {
	case err != nil:
		return nil, err
	case !doWrite:
		readTxn, err := client.BatchReadOnlyTransaction(ctx, spanner.StrongRead())
		if err != nil {
			return nil, err
		}
		defer readTxn.Close()
		return span.ReadInvocationFull(ctx, readTxn, invID)
	}

	// Otherwise, get the protos and write them to Spanner.
	inv, results, err := chromium.DeriveProtosForWriting(ctx, task, in)
	if err != nil {
		return nil, err
	}
	inv.Deadline = inv.FinalizeTime

	// TODO(jchinlee): Validate invocation and results.

	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		// Check invocation state again.
		switch doWrite, err := shouldWriteInvocation(ctx, txn, invID); {
		case err != nil:
			return err
		case !doWrite:
			return nil
		}

		// Get Invocation mutation.
		if inv.FinalizeTime == nil {
			panic("missing inv.FinalizeTime")
		}

		invMap := map[string]interface{}{
			"InvocationId": invID,
			"State":        inv.State,
			"Realm":        chromium.Realm,

			"UpdateToken": "",

			"CreateTime":   inv.CreateTime,
			"Deadline":     inv.Deadline,
			"FinalizeTime": inv.FinalizeTime,

			"BaseTestVariantDef": inv.GetBaseTestVariantDef(),
			"Tags":               inv.Tags,
		}
		populateExpirations(invMap, clock.Now(ctx))

		muts := []*spanner.Mutation{spanner.InsertMap("Invocations", span.ToSpannerMap(invMap))}

		// Create InvocationByTags mutations.
		muts = append(muts, getInvocationsByTagMutations(invID, inv)...)

		// Get TestResult mutations.
		for i, tr := range results {
			mut, err := getTestResultMutation(invID, tr, i)
			if err != nil {
				return errors.Annotate(err, "test result #%d %q", i, tr.TestPath).Err()
			}
			muts = append(muts, mut)
		}

		// Write mutations.
		return txn.BufferWrite(muts)
	})

	return inv, err
}

func shouldWriteInvocation(ctx context.Context, txn span.Txn, invID string) (bool, error) {
	state, err := readInvocationState(ctx, txn, invID)
	switch {
	case grpcutil.Code(err) == codes.NotFound:
		// No such invocation found means we may have to write it, so proceed.
		return true, nil

	case err != nil:
		return false, err

	case !pbutil.IsFinalized(state):
		return false, errors.Reason(
			"attempting to derive an existing non-finalized invocation").Err()
	}

	// The invocation exists and is finalized, so no need to write it.
	return false, nil
}
