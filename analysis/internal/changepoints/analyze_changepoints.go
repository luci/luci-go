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

	"go.chromium.org/luci/analysis/internal/changepoints/bayesian"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/ingestion/control"
	"go.chromium.org/luci/analysis/internal/ingestion/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testresults/gitreferences"
	"go.chromium.org/luci/analysis/pbutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
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
	logging.Debugf(ctx, "Analyzing %d test variants for build %d", len(tvs), payload.Build.Id)

	// Check if the build has commit data. If not, then skip it.
	if !hasCommitData(payload) {
		logging.Debugf(ctx, "Build %d has no commit data, skipping changepoint analysis", payload.GetBuild().GetId())
		return nil
	}

	// Check if build is from unsubmitted code.
	if fromUnsubmittedCode(payload) {
		logging.Debugf(ctx, "Build %d from unsubmitted code, skipping changepoint analysis", payload.GetBuild().GetId())
		return nil
	}

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

		duplicateMap, err := duplicateMap(ctx, tvs, payload)
		if err != nil {
			return errors.Annotate(err, "duplicate map").Err()
		}

		// Only keep "relevant" test variants.
		filteredTVs, err := filterTestVariants(tvs, duplicateMap)
		if err != nil {
			return errors.Annotate(err, "filter test variants").Err()
		}

		// Query TestVariantBranch from spanner.
		tvbks := testVariantBranchKeys(filteredTVs, payload)
		tvbs, err := ReadTestVariantBranches(ctx, tvbks)
		if err != nil {
			return errors.Annotate(err, "read test variant branches").Err()
		}

		for i, tv := range filteredTVs {
			// "Insert" the new test variant to input buffer.
			_, err := insertIntoInputBuffer(tvbs[i], tv, payload, duplicateMap)
			if err != nil {
				return errors.Annotate(err, "insert into input buffer").Err()
			}
			// TODO(nqmtuan): Run changepoint analysis on test variant branch.
			// TODO(nqmtuan): Store test variant branch in spanner.
		}

		// TODO (nqmtuan): Store non-duplicate runs to Invocations table.

		// Store checkpoint in TestVariantBranchCheckpoint table.
		m := checkPoint.ToMutation()
		span.BufferWrite(ctx, m)
		return nil
	})

	return err
}

func runChangePointAnalysis(tvb *TestVariantBranch) {
	a := bayesian.ChangepointPredictor{
		ChangepointLikelihood: 0.0001,
		// We are leaning toward consistently passing test results.
		HasUnexpectedPrior: bayesian.BetaDistribution{
			Alpha: 0.3,
			Beta:  0.5,
		},
		UnexpectedAfterRetryPrior: bayesian.BetaDistribution{
			Alpha: 0.5,
			Beta:  0.5,
		},
	}
	history := tvb.InputBuffer.MergeBuffer()
	changepoints := a.IdentifyChangepoints(history)
	sib := tvb.InputBuffer.Segmentize(changepoints)
	sib.EvictSegments()
	// TODO (nqmtuan): Combine the evicted segments with the segments from the
	// output buffers and update the output buffer.
	// TODO (nqmtuan): Combine the remaining output buffer segments and remaining
	// segment for BQ exporter.
}

// insertIntoInputBuffer inserts the new test variant tv into the input buffer
// of TestVariantBranch tvb.
// If tvb is nil, it means it is not in spanner. In this case, return a new
// TestVariantBranch object with a single element in the input buffer.
func insertIntoInputBuffer(tvb *TestVariantBranch, tv *rdbpb.TestVariant, payload *taskspb.IngestTestResults, duplicateMap map[string]bool) (*TestVariantBranch, error) {
	if tvb == nil {
		tvb = &TestVariantBranch{
			IsNew:            true,
			Project:          payload.GetBuild().GetProject(),
			TestID:           tv.TestId,
			VariantHash:      tv.VariantHash,
			GitReferenceHash: gitReferenceHash(payload),
			Variant:          pbutil.VariantFromResultDB(tv.Variant),
			InputBuffer:      &inputbuffer.Buffer{},
		}
	}

	pv, err := toPositionVerdict(tv, payload, duplicateMap)
	if err != nil {
		return nil, err
	}
	tvb.InsertToInputBuffer(pv)
	return tvb, nil
}

// filterTestVariants only keeps test variants that have at least 1 non-duplicate
// and non-skipped test result (the test result needs to be both non-duplicate
// and non-skipped).
func filterTestVariants(tvs []*rdbpb.TestVariant, duplicateMap map[string]bool) ([]*rdbpb.TestVariant, error) {
	results := []*rdbpb.TestVariant{}
	for _, tv := range tvs {
		for _, r := range tv.Results {
			invID, err := resultdb.InvocationFromTestResultName(r.Result.Name)
			if err != nil {
				return nil, errors.Annotate(err, "invocation from test result name").Err()
			}
			_, isDuplicate := duplicateMap[invID]
			if r.Result.Status != rdbpb.TestStatus_SKIP && !isDuplicate {
				results = append(results, tv)
				break
			}
		}
	}
	return results, nil
}

func testVariantBranchKeys(tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestResults) []TestVariantBranchKey {
	results := make([]TestVariantBranchKey, len(tvs))
	for i, tv := range tvs {
		results[i] = TestVariantBranchKey{
			Project:          payload.Build.Project,
			TestID:           tv.TestId,
			VariantHash:      tv.VariantHash,
			GitReferenceHash: string(gitReferenceHash(payload)),
		}
	}
	return results
}

// Return true if the build is from unsubmitted code, i.e.
// from try run that did not result in submitted code.
func fromUnsubmittedCode(payload *taskspb.IngestTestResults) bool {
	hasCL := len(payload.GetBuild().GetChangelists()) > 0
	submittedPresubmit := payload.PresubmitRun != nil &&
		payload.PresubmitRun.Status == analysispb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED &&
		payload.PresubmitRun.Mode == analysispb.PresubmitRunMode_FULL_RUN
	return hasCL && !submittedPresubmit
}

// hasCommitData checks if the build has commit data. It checks for
// branch information and commit position data.
func hasCommitData(payload *taskspb.IngestTestResults) bool {
	commit := gitilesCommit(payload)
	return commit != nil && commit.GetHost() != "" && commit.GetProject() != "" && commit.GetRef() != "" && commit.GetPosition() != 0
}

// gitilesCommit returns the commit for the payload.
func gitilesCommit(payload *taskspb.IngestTestResults) *buildbucketpb.GitilesCommit {
	// TODO (nqmtuan): Support projects like ChromeOS, where commit data may not
	// be in GetBuild().GetCommit().
	return payload.GetBuild().GetCommit()
}

func gitReferenceHash(payload *taskspb.IngestTestResults) []byte {
	return gitreferences.GitReferenceHash(payload.Build.Commit.Host, payload.Build.Commit.Project, payload.Build.Commit.Ref)
}
