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
	"math"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/analysis/internal/changepoints/bayesian"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/ingestion/control"
	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/ingestion/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/pbutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

// CheckPoint represents a single row in the TestVariantBranchCheckpoint table.
type CheckPoint struct {
	InvocationID        string
	StartingTestID      string
	StartingVariantHash string
}

// Analyze performs change point analyses based on incoming test verdicts.
// sourcesMap contains the information about the source code being tested.
func Analyze(ctx context.Context, tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestResults, sourcesMap map[string]*rdbpb.Sources) error {
	logging.Debugf(ctx, "Analyzing %d test variants for build %d", len(tvs), payload.Build.Id)

	// Check that sourcesMap is not empty and has commit position data.
	// This is for fast termination, as there should be only few items in
	// sourcesMap to check.
	if !sourcesMapHasCommitData(sourcesMap) {
		logging.Debugf(ctx, "Sourcemap has no commit data, skipping change point analysis")
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
		err := analyzeSingleBatch(ctx, batchTVs, payload, sourcesMap)
		if err != nil {
			return errors.Annotate(err, "analyzeSingleBatch").Err()
		}
		startIndex = int(endIndex)
	}

	return nil
}

func analyzeSingleBatch(ctx context.Context, tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestResults, sourcesMap map[string]*rdbpb.Sources) error {
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

		duplicateMap, newInvIDs, err := readDuplicateInvocations(ctx, tvs, payload.Build)
		if err != nil {
			return errors.Annotate(err, "duplicate map").Err()
		}

		// Only keep "relevant" test variants, and test variant with commit information.
		filteredTVs, err := filterTestVariants(tvs, payload.PresubmitRun, duplicateMap, sourcesMap)
		if err != nil {
			return errors.Annotate(err, "filter test variants").Err()
		}

		// Query TestVariantBranch from spanner.
		tvbks := testVariantBranchKeys(filteredTVs, payload.Build.Project, sourcesMap)
		tvbs, err := ReadTestVariantBranches(ctx, tvbks)
		if err != nil {
			return errors.Annotate(err, "read test variant branches").Err()
		}

		// The list of mutations for this transaction.
		mutations := []*spanner.Mutation{}

		for i, tv := range filteredTVs {
			tvb := tvbs[i]
			if isOutOfOrderAndShouldBeDiscarded(tvb, sourcesMap[tv.SourcesId]) {
				// TODO(nqmtuan): send metric to tsmon.
				logging.Debugf(ctx, "Out of order verdict in build %d", payload.Build.Id)
				continue
			}
			// "Insert" the new test variant to input buffer.
			tvb, err := insertIntoInputBuffer(tvb, tv, payload, duplicateMap, sourcesMap)
			if err != nil {
				return errors.Annotate(err, "insert into input buffer").Err()
			}
			runChangePointAnalysis(tvb)
			mut, err := tvb.ToMutation()
			if err != nil {
				return errors.Annotate(err, "test variant branch to mutation").Err()
			}
			mutations = append(mutations, mut)
		}

		// Store new Invocations to Invocations table.
		ingestedInvID := control.BuildInvocationID(payload.Build.Id)
		invMuts := invocationsToMutations(ctx, payload.Build.Project, newInvIDs, ingestedInvID)
		mutations = append(mutations, invMuts...)

		// Store checkpoint in TestVariantBranchCheckpoint table.
		mutations = append(mutations, checkPoint.ToMutation())
		span.BufferWrite(ctx, mutations...)
		return nil
	})

	return err
}

// isOutOfOrderAndShouldBeDiscarded returns true if the verdict is out-of-order
// and should be discarded.
// This function returns false if the verdict is out-of-order but can still be
// processed.
// We only keep out-of-order verdict if either condition occurs:
//   - The verdict commit position falls within the input buffer
//     (commit position >= smallest start position), or
//   - There is no finalizing or finalized segment (i.e. the entire known
//     test history is inside the input buffer)
func isOutOfOrderAndShouldBeDiscarded(tvb *TestVariantBranch, sources *rdbpb.Sources) bool {
	// No test variant branch. Should be ok to proceed.
	if tvb == nil {
		return false
	}
	if len(tvb.FinalizedSegments.GetSegments()) == 0 && tvb.FinalizingSegment == nil {
		return false
	}
	position := sourcesCommitPosition(sources)
	hotVerdicts := tvb.InputBuffer.HotBuffer.Verdicts
	coldVerdicts := tvb.InputBuffer.ColdBuffer.Verdicts
	minPos := math.MaxInt
	if len(hotVerdicts) > 0 && minPos > hotVerdicts[0].CommitPosition {
		minPos = hotVerdicts[0].CommitPosition
	}
	if len(coldVerdicts) > 0 && minPos > coldVerdicts[0].CommitPosition {
		minPos = coldVerdicts[0].CommitPosition
	}
	return position < minPos
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
	changePoints := a.ChangePoints(history, bayesian.ConfidenceIntervalTail)
	sib := tvb.InputBuffer.Segmentize(changePoints)
	evictedSegment := sib.EvictSegments()
	tvb.UpdateOutputBuffer(evictedSegment)
	// TODO (nqmtuan): Combine the remaining output buffer segments and remaining
	// segment for BQ exporter.
}

// insertIntoInputBuffer inserts the new test variant tv into the input buffer
// of TestVariantBranch tvb.
// If tvb is nil, it means it is not in spanner. In this case, return a new
// TestVariantBranch object with a single element in the input buffer.
func insertIntoInputBuffer(tvb *TestVariantBranch, tv *rdbpb.TestVariant, payload *taskspb.IngestTestResults, duplicateMap map[string]bool, sourcesMap map[string]*rdbpb.Sources) (*TestVariantBranch, error) {
	sources := sourcesMap[tv.SourcesId]
	if tvb == nil {
		tvb = &TestVariantBranch{
			IsNew:       true,
			Project:     payload.GetBuild().GetProject(),
			TestID:      tv.TestId,
			VariantHash: tv.VariantHash,
			RefHash:     refHash(sources),
			Variant:     pbutil.VariantFromResultDB(tv.Variant),
			SourceRef:   sourceRef(sources),
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity:  inputbuffer.DefaultHotBufferCapacity,
				ColdBufferCapacity: inputbuffer.DefaultColdBufferCapacity,
			},
		}
	}

	pv, err := toPositionVerdict(tv, payload, duplicateMap, sources)
	if err != nil {
		return nil, err
	}
	tvb.InsertToInputBuffer(pv)
	return tvb, nil
}

// filterTestVariants only keeps test variants that satisfy all following
// conditions:
//   - Have commit position information.
//   - Have at least 1 non-duplicate and non-skipped test result (the test
//     result needs to be both non-duplicate and non-skipped).
//   - Not from unsubmitted code (i.e. try run that did not result in submitted code)
func filterTestVariants(tvs []*rdbpb.TestVariant, presubmit *controlpb.PresubmitResult, duplicateMap map[string]bool, sourcesMap map[string]*rdbpb.Sources) ([]*rdbpb.TestVariant, error) {
	results := []*rdbpb.TestVariant{}
	for _, tv := range tvs {
		// Checks source map.
		sources, ok := sourcesMap[tv.SourcesId]
		if !ok {
			continue
		}
		if !sourcesHasCommitData(sources) {
			continue
		}
		// Checks unsubmitted code.
		if fromUnsubmittedCode(sources, presubmit) {
			continue
		}
		// Checks skips and duplicates.
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

func testVariantBranchKeys(tvs []*rdbpb.TestVariant, project string, sourcesMap map[string]*rdbpb.Sources) []TestVariantBranchKey {
	results := make([]TestVariantBranchKey, len(tvs))
	for i, tv := range tvs {
		sources := sourcesMap[tv.SourcesId]
		results[i] = TestVariantBranchKey{
			Project:     project,
			TestID:      tv.TestId,
			VariantHash: tv.VariantHash,
			RefHash:     RefHash(refHash(sources)),
		}
	}
	return results
}

// Return true if sources is from unsubmitted code, i.e.
// from try run that did not result in submitted code.
func fromUnsubmittedCode(sources *rdbpb.Sources, presubmit *controlpb.PresubmitResult) bool {
	hasCL := len(sources.GetChangelists()) > 0
	submittedPresubmit := presubmit != nil &&
		presubmit.Status == analysispb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED &&
		presubmit.Mode == analysispb.PresubmitRunMode_FULL_RUN
	return hasCL && !submittedPresubmit
}

// hasCommitData checks if sourcesMap has commit data.
// It returns true if at least one sources in the map has commit position data.
func sourcesMapHasCommitData(sourcesMap map[string]*rdbpb.Sources) bool {
	for _, sources := range sourcesMap {
		if sourcesHasCommitData(sources) {
			return true
		}
	}
	return false
}

func sourcesHasCommitData(sources *rdbpb.Sources) bool {
	if sources.IsDirty {
		return false
	}
	commit := sources.GitilesCommit
	if commit == nil {
		return false
	}
	return commit.GetHost() != "" && commit.GetProject() != "" && commit.GetRef() != "" && commit.GetPosition() != 0
}

func sourcesCommitPosition(sources *rdbpb.Sources) int {
	return int(sources.GitilesCommit.Position)
}

func sourceRef(sources *rdbpb.Sources) *analysispb.SourceRef {
	return &analysispb.SourceRef{
		System: &analysispb.SourceRef_Gitiles{
			Gitiles: &analysispb.GitilesRef{
				Host:    sources.GitilesCommit.Host,
				Project: sources.GitilesCommit.Project,
				Ref:     sources.GitilesCommit.Ref,
			},
		},
	}
}

func refHash(sources *rdbpb.Sources) []byte {
	sourceRef := sourceRef(sources)
	return rdbpbutil.RefHash(pbutil.SourceRefToResultDB(sourceRef))
}
