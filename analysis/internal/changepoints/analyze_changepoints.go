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

// Package changepoints handles change point detection and analysis.
// See go/luci-test-variant-analysis-design for details.
package changepoints

import (
	"context"

	"go.chromium.org/luci/analysis/internal/ingestion/control"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

// CheckPoint represents a single row in the TestVariantBranchCheckpoint table.
type CheckPoint struct {
	InvocationID        string
	StartingTestID      string
	StartingVariantHash string
}

func Analyze(ctx context.Context, tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestResults) error {
	logging.Debugf(ctx, "Analyzing %d changepoints for build %d", len(tvs), payload.Build.Id)
	// Instead of processing 10,000 test verdicts at a time, we will process by
	// smaller batches. This will increase the robustness of the process, and
	// in case something go wrong, we will not need to reprocess the whole 10,000
	// verdicts.
	// Also, the number of mutations per transaction is limit to 40,000. The
	// mutations include the primary keys and the fields being updated. So we
	// cannot process 10,000 test verdicts at once.
	// TODO(nqmtuan): Consider putting this in config.
	// Note: Changing this value may cause some test variants in retried tasks to
	// get ingested twice.
	batchSize := 1000
	for startIndex := 0; startIndex < len(tvs); {
		endIndex := startIndex + batchSize
		if endIndex > len(tvs) {
			endIndex = len(tvs)
		}
		batchTVs := tvs[startIndex:endIndex]
		err := analyzeSingleBatch(ctx, batchTVs, payload)
		if err != nil {
			return errors.Annotate(err, "analyzeSingleBatch").Err()
		}
		startIndex = int(endIndex)
	}

	return nil
}

func analyzeSingleBatch(ctx context.Context, tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestResults) error {
	// Nothing to analyze.
	if len(tvs) == 0 {
		return nil
	}

	firstTV := tvs[0]
	checkPoint := CheckPoint{
		InvocationID:        control.BuildInvocationName(payload.GetBuild().Id),
		StartingTestID:      firstTV.TestId,
		StartingVariantHash: firstTV.VariantHash,
	}

	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Check the TestVariantBranch table for the existence of the batch.
		exist, err := hasCheckPoint(ctx, checkPoint)
		if err != nil {
			return errors.Annotate(err, "hasCheckPoint (%s, %s, %s)", checkPoint.InvocationID, firstTV.TestId, firstTV.VariantHash).Err()
		}

		// This batch has been processed, we can skip it.
		if exist {
			return nil
		}
		// TODO (nqmtuan): Run change point detection on batch.

		// Store checkpoint in TestVariantBranchCheckpoint table.
		m := checkPoint.ToMutation()
		span.BufferWrite(ctx, m)
		return nil
	})

	return err
}
