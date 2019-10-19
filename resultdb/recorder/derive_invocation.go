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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc/codes"

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

	if err := pbutil.ValidateVariantDef(req.BaseTestVariant); err != nil {
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
	skipWrite, err := shouldSkipWriteInvocation(ctx, client.Single(), invID)
	if err != nil {
		return nil, err
	}
	if skipWrite {
		readTxn, err := client.BatchReadOnlyTransaction(ctx, spanner.StrongRead())
		defer readTxn.Close()
		if err != nil {
			return nil, err
		}
		return span.ReadInvocationFull(ctx, readTxn, invID)
	}

	// Otherwise, get the protos and write them to Spanner.
	inv, results, err := chromium.DeriveProtosForWriting(ctx, task, in)
	if err != nil {
		return nil, err
	}
	inv.Deadline = inv.FinalizeTime

	// TODO(jchinlee): Validate invocation and results.

	// Get expiration times.
	exp := getExpirations(time.Now())

	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		// Check invocation state again.
		skipWrite, err := shouldSkipWriteInvocation(ctx, txn, invID)
		if err != nil {
			return err
		}
		if skipWrite {
			return nil
		}

		// Get Invocation mutation.
		invMap := map[string]interface{}{
			"InvocationId": invID,
			"State":        int(inv.State),
			"Realm":        "chromium",

			"InvocationExpirationTime":          exp.InvocationTime,
			"InvocationExpirationWeek":          exp.InvocationWeek,
			"ExpectedTestResultsExpirationTime": exp.ResultsTime,
			"ExpectedTestResultsExpirationWeek": exp.ResultsWeek,

			"UpdateToken": "",

			"BaseTestVariantDef": pbutil.VariantDefPairs(inv.BaseTestVariantDef),
			"Tags":               pbutil.StringPairsToStrings(inv.Tags...),
		}

		if invMap["CreateTime"], err = ptypes.Timestamp(inv.CreateTime); err != nil {
			return errors.Annotate(err, "inv.CreateTime").Err()
		}
		if invMap["Deadline"], err = ptypes.Timestamp(inv.Deadline); err != nil {
			return errors.Annotate(err, "inv.Deadline").Err()
		}
		if inv.FinalizeTime != nil {
			finalizeTime, err := ptypes.Timestamp(inv.FinalizeTime)
			if err != nil {
				return errors.Annotate(err, "inv.FinalizeTime").Err()
			}
			invMap["FinalizeTime"] = finalizeTime
		}

		muts := []*spanner.Mutation{spanner.InsertMap("Invocations", invMap)}

		// Create InvocationByTags mutations.
		for _, tag := range inv.Tags {
			muts = append(muts, spanner.InsertMap("InvocationsByTag", map[string]interface{}{
				"TagId":        tagID(tag),
				"InvocationId": invID,
			}))
		}

		// Get TestResult mutations.
		for i, tr := range results {
			trMap := map[string]interface{}{
				"InvocationId": invID,
				"TestPath":     tr.TestPath,
				"ResultId":     generateTestResultID(tr.TestPath, i),

				"ExtraVariantPairs": pbutil.VariantDefPairs(tr.ExtraVariantPairs),

				"CommitTimestamp": spanner.CommitTimestamp,

				"Status":          int(tr.Status),
				"SummaryMarkdown": tr.SummaryMarkdown,
				"RunDurationUsec": 1e6*tr.Duration.Seconds + int64(1e-3*float64(tr.Duration.Nanos)),
				"Tags":            pbutil.StringPairsToStrings(tr.Tags...),
			}

			if tr.StartTime != nil {
				startTime, err := ptypes.Timestamp(tr.StartTime)
				if err != nil {
					return errors.Annotate(err, "test result #%d %q", i, tr.TestPath).Err()
				}
				trMap["StartTime"] = startTime
			}

			var err error
			if trMap["InputArtifacts"], err = pbutil.ArtifactsToByteArrays(tr.InputArtifacts); err != nil {
				return errors.Annotate(err, "test result #%d %q", i, tr.TestPath).Err()
			}
			if trMap["OutputArtifacts"], err = pbutil.ArtifactsToByteArrays(tr.OutputArtifacts); err != nil {
				return errors.Annotate(err, "test result #%d %q", i, tr.TestPath).Err()
			}

			// Populate IsUnexpected /only/ if true, to keep the index thin.
			if !tr.Expected {
				trMap["IsUnexpected"] = true
			}

			muts = append(muts, spanner.InsertMap("TestResults", trMap))
		}

		// Write mutations.
		return txn.BufferWrite(muts)
	})

	return inv, err
}

func shouldSkipWriteInvocation(ctx context.Context, txn span.Txn, invID string) (bool, error) {
	state, err := readInvocationState(ctx, txn, invID)
	switch {
	case grpcutil.Code(err) == codes.NotFound:
		// No such invocation found means we may have to write it, so proceed.
		return false, nil

	case err != nil:
		return true, err

	case !pbutil.IsFinalized(state):
		return true, errors.Reason(
			"attempting to derive an existing non-finalized invocation").Err()
	}

	// The invocation exists and is finalized, so no need to write it.
	return true, nil
}

// generateTestResultID returns the ID of the TestResult with given path and index in invocation.
// Has format "${sha256_hex(${testPath}:${index})}".
func generateTestResultID(testPath string, i int) string {
	h := sha256.New()
	io.WriteString(h, testPath+":")
	io.WriteString(h, string(i))
	return hex.EncodeToString(h.Sum(nil))
}

// tagID returns the ID of the StringPair tag, with format "${sha256_hex(tag)}_${key}:${value}".
func tagID(tag *pb.StringPair) string {
	tagStr := pbutil.StringPairToString(tag)
	h := sha256.New()
	io.WriteString(h, tagStr)
	return fmt.Sprintf("%s_%s", hex.EncodeToString(h.Sum(nil)), tagStr)
}
