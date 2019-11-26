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
	"fmt"
	"strconv"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// testResultBatchSizeMax is the maximum number of TestResults to include per transaction.
const testResultBatchSizeMax = 1999

func insertTestResult(invID span.InvocationID, tr *pb.TestResult, i int) *spanner.Mutation {
	trMap := map[string]interface{}{
		"InvocationId": invID,
		"TestPath":     tr.TestPath,
		"ResultId":     strconv.Itoa(i),

		"Variant":     tr.Variant,
		"VariantHash": pbutil.VariantHash(tr.Variant),

		"CommitTimestamp": spanner.CommitTimestamp,

		"Status":          tr.Status,
		"SummaryMarkdown": span.Snappy([]byte(tr.SummaryMarkdown)),
		"StartTime":       tr.StartTime,
		"RunDurationUsec": span.ToMicros(tr.Duration),
		"Tags":            tr.Tags,

		"InputArtifacts":  tr.InputArtifacts,
		"OutputArtifacts": tr.OutputArtifacts,
	}

	// Populate IsUnexpected /only/ if true, to keep the index thin.
	if !tr.Expected {
		trMap["IsUnexpected"] = true
	}

	return span.InsertMap("TestResults", trMap)
}

// testResultBatch holds all the protos associated with the batch of
// test results that are to be committed transactionally.
type testResultBatch struct {
	// BatchInv holds the finalized invocation associated with this batch specifically.
	BatchInv *pb.Invocation

	// TestResults holds the test results in the batch.
	TestResults []*pb.TestResult
}

// ToMutations converts the protos stored in the testResultBatchMutation to a single slice of
// mutations that will be committed in the same transaction.
func (b *testResultBatch) ToMutations(ctx context.Context) []*spanner.Mutation {
	muts := make([]*spanner.Mutation, 0, len(b.TestResults)+1)

	// Convert the containing invocation.
	muts = append(muts, insertInvocation(ctx, b.BatchInv, "", ""))

	// Convert the test results.
	for i, tr := range b.TestResults {
		muts = append(muts, insertTestResult(span.MustParseInvocationName(b.BatchInv.Name), tr, i))
	}

	return muts
}

// batchTestResults batches the given TestResults for insert under the given invocation.
func batchTestResults(ctx context.Context, inv *pb.Invocation, trs []*pb.TestResult) []*testResultBatch {
	baseID := span.MustParseInvocationName(inv.Name)
	batches := make(
		[]*testResultBatch, (len(trs)+testResultBatchSizeMax-1)/testResultBatchSizeMax)

	for i := range batches {
		end := len(trs)
		if i < len(batches)-1 {
			end = (i + 1) * testResultBatchSizeMax
		}

		batches[i] = &testResultBatch{
			BatchInv: &pb.Invocation{
				Name:         span.InvocationID(fmt.Sprintf("%s_%d", baseID, i)).Name(),
				State:        pb.Invocation_COMPLETED,
				CreateTime:   inv.CreateTime,
				FinalizeTime: inv.FinalizeTime,
				Deadline:     inv.Deadline,
			},
			TestResults: trs[i*testResultBatchSizeMax : end],
		}
	}

	return batches
}
