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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints/bayesian"
	"go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/sources"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/ingestion/controllegacy"
	"go.chromium.org/luci/analysis/internal/ingestion/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

var (
	verdictCounter = metric.NewCounter(
		"analysis/changepoints/analyze/verdicts",
		"The number of verdicts processed by analysis, classified by project and status.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// Possible values:
		// - "ingested": The verdict was ingested.
		// - "skipped_no_source": The verdict was skipped because it has no source
		//   data.
		// - "skipped_no_commit_data": The verdict was skipped because its source
		//   does not have enough commit data (e.g. commit position).
		// - "skipped_out_of_order": The verdict was skipped because it was too
		//   out of order.
		// - "skipped_unsubmitted_code": The verdict was skipped because is was
		//   from unsubmitted code.
		// - "skipped_all_skipped_or_duplicate":  The verdict was skipped because
		//   it contains only skipped or duplicate results.
		field.String("status"),
	)
)

// CheckPoint represents a single row in the TestVariantBranchCheckpoint table.
type CheckPoint struct {
	InvocationID        string
	StartingTestID      string
	StartingVariantHash string
}

// Analyze performs change point analyses based on incoming test verdicts.
// sourcesMap contains the information about the source code being tested.
func Analyze(ctx context.Context, tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestVerdicts, sourcesMap map[string]*pb.Sources, exporter *bqexporter.Exporter) error {
	logging.Debugf(ctx, "Analyzing %d test variants for build %d", len(tvs), payload.Build.Id)

	// Check that sourcesMap is not empty and has commit position data.
	// This is for fast termination, as there should be only few items in
	// sourcesMap to check.
	if !sources.SourcesMapHasCommitData(sourcesMap) {
		verdictCounter.Add(ctx, int64(len(tvs)), payload.Build.Project, "skipped_no_commit_data")
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
		err := analyzeSingleBatch(ctx, batchTVs, payload, sourcesMap, exporter)
		if err != nil {
			return errors.Annotate(err, "analyzeSingleBatch").Err()
		}
		startIndex = int(endIndex)
	}

	return nil
}

func analyzeSingleBatch(ctx context.Context, tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestVerdicts, sourcesMap map[string]*pb.Sources, exporter *bqexporter.Exporter) error {
	// Nothing to analyze.
	if len(tvs) == 0 {
		return nil
	}

	firstTV := tvs[0]
	checkPoint := CheckPoint{
		InvocationID:        controllegacy.BuildInvocationName(payload.GetBuild().Id),
		StartingTestID:      firstTV.TestId,
		StartingVariantHash: firstTV.VariantHash,
	}

	// Contains the test variant branches to be written to BigQuery.
	bqExporterInput := make([]bqexporter.PartialBigQueryRow, 0, len(tvs))

	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
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
		filteredTVs, err := filterTestVariants(ctx, tvs, payload, duplicateMap, sourcesMap)
		if err != nil {
			return errors.Annotate(err, "filter test variants").Err()
		}

		// Query TestVariantBranch from spanner.
		tvbks := testVariantBranchKeys(filteredTVs, payload.Build.Project, sourcesMap)

		// The list of mutations for this transaction.
		mutations := []*spanner.Mutation{}

		// Buffers allocated once and re-used for processing
		// all test variant branches.
		var hs inputbuffer.HistorySerializer
		var analysis Analyzer

		// Handle each read test variant branch.
		f := func(i int, tvb *testvariantbranch.Entry) error {
			tv := filteredTVs[i]
			if isOutOfOrderAndShouldBeDiscarded(tvb, sourcesMap[tv.SourcesId]) {
				verdictCounter.Add(ctx, 1, payload.Build.Project, "skipped_out_of_order")
				logging.Debugf(ctx, "Out of order verdict in build %d", payload.Build.Id)
				return nil
			}
			// "Insert" the new test variant to input buffer.
			tvb, err := insertIntoInputBuffer(tvb, tv, payload, duplicateMap, sourcesMap)
			if err != nil {
				return errors.Annotate(err, "insert into input buffer").Err()
			}
			inputSegments := analysis.Run(tvb)
			tvb.ApplyRetentionPolicyForFinalizedSegments(payload.PartitionTime.AsTime())
			mut, err := tvb.ToMutation(&hs)
			if err != nil {
				return errors.Annotate(err, "test variant branch to mutation").Err()
			}
			mutations = append(mutations, mut)
			bqRow, err := bqexporter.ToPartialBigQueryRow(tvb, inputSegments)
			if err != nil {
				return errors.Annotate(err, "test variant branch to bigquery row").Err()
			}
			bqExporterInput = append(bqExporterInput, bqRow)
			return nil
		}
		if err := testvariantbranch.ReadF(ctx, tvbks, f); err != nil {
			return errors.Annotate(err, "read test variant branches").Err()
		}

		ingestedVerdictCount := len(mutations)

		// Store new Invocations to Invocations table.
		ingestedInvID := controllegacy.BuildInvocationID(payload.Build.Id)
		invMuts := invocationsToMutations(ctx, payload.Build.Project, newInvIDs, ingestedInvID)
		mutations = append(mutations, invMuts...)

		// Store checkpoint in TestVariantBranchCheckpoint table.
		mutations = append(mutations, checkPoint.ToMutation())
		span.BufferWrite(ctx, mutations...)
		verdictCounter.Add(ctx, int64(ingestedVerdictCount), payload.Build.Project, "ingested")
		return nil
	})

	if err != nil {
		return errors.Annotate(err, "analyze change point").Err()
	}
	// Export to BigQuery.
	// Note: exportToBigQuery does not guarantee eventual export, in case it
	// fails. Even though the task may be retried, bqtvbs will be empty, so
	// the data will not be exported.
	// This should not be a concern, since the export will happen again when the
	// next test verdict comes, but it may result in some delay.
	rowInputs := bqexporter.RowInputs{
		Rows:            bqExporterInput,
		CommitTimestamp: commitTimestamp,
	}
	err = exportToBigQuery(ctx, exporter, rowInputs)
	if err != nil {
		return errors.Annotate(err, "export to big query").Err()
	}
	return nil
}

// exportToBigQuery exports the data in bqRows to BigQuery.
// commitTimestamp is the Spanner commit timestamp of the
// test variant branches.
func exportToBigQuery(ctx context.Context, exporter *bqexporter.Exporter, rowInputs bqexporter.RowInputs) error {
	if len(rowInputs.Rows) == 0 {
		return nil
	}
	cfg, err := config.Get(ctx)
	if err != nil {
		return errors.Annotate(err, "read config").Err()
	}
	if !cfg.GetTestVariantAnalysis().GetBigqueryExportEnabled() {
		return nil
	}

	err = exporter.ExportTestVariantBranches(ctx, rowInputs)
	if err != nil {
		return errors.Annotate(err, "export test variant branches").Err()
	}
	return nil
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
func isOutOfOrderAndShouldBeDiscarded(tvb *testvariantbranch.Entry, src *pb.Sources) bool {
	// No test variant branch. Should be ok to proceed.
	if tvb == nil {
		return false
	}
	if len(tvb.FinalizedSegments.GetSegments()) == 0 && tvb.FinalizingSegment == nil {
		return false
	}
	position := sources.CommitPosition(src)
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

type Analyzer struct {
	// MergeBuffer is a preallocated buffer used to store the result of
	// merging hot and cold input buffers. Reusing the same buffer avoids
	// allocating a new buffer for each test variant branch processed.
	mergeBuffer []inputbuffer.PositionVerdict
}

// Run runs change point analysis, performs any required
// evictions (from the hot input buffer to the cold input buffer,
// and from the input buffer to the output buffer), and returns the
// remaining segment in the input buffer after eviction.
func (a *Analyzer) Run(tvb *testvariantbranch.Entry) []*inputbuffer.Segment {
	predictor := bayesian.ChangepointPredictor{
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
	tvb.InputBuffer.CompactIfRequired()
	tvb.InputBuffer.MergeBuffer(&a.mergeBuffer)
	changePoints := predictor.ChangePoints(a.mergeBuffer, bayesian.ConfidenceIntervalTail)
	sib := tvb.InputBuffer.Segmentize(a.mergeBuffer, changePoints)
	evictedSegment := sib.EvictSegments()
	tvb.UpdateOutputBuffer(evictedSegment)
	return sib.Segments
}

// insertIntoInputBuffer inserts the new test variant tv into the input buffer
// of TestVariantBranch tvb.
// If tvb is nil, it means it is not in spanner. In this case, return a new
// TestVariantBranch object with a single element in the input buffer.
func insertIntoInputBuffer(tvb *testvariantbranch.Entry, tv *rdbpb.TestVariant, payload *taskspb.IngestTestVerdicts, duplicateMap map[string]bool, sourcesMap map[string]*pb.Sources) (*testvariantbranch.Entry, error) {
	src := sourcesMap[tv.SourcesId]
	if tvb == nil {
		ref := pbutil.SourceRefFromSources(src)
		tvb = &testvariantbranch.Entry{
			IsNew:       true,
			Project:     payload.GetBuild().GetProject(),
			TestID:      tv.TestId,
			VariantHash: tv.VariantHash,
			RefHash:     pbutil.SourceRefHash(ref),
			Variant:     pbutil.VariantFromResultDB(tv.Variant),
			SourceRef:   ref,
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity:  inputbuffer.DefaultHotBufferCapacity,
				ColdBufferCapacity: inputbuffer.DefaultColdBufferCapacity,
			},
		}
	}

	pv, err := testvariantbranch.ToPositionVerdict(tv, payload, duplicateMap, src)
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
func filterTestVariants(ctx context.Context, tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestVerdicts, duplicateMap map[string]bool, sourcesMap map[string]*pb.Sources) ([]*rdbpb.TestVariant, error) {
	results := []*rdbpb.TestVariant{}
	presubmit := payload.PresubmitRun
	project := payload.Build.Project
	for _, tv := range tvs {
		// Checks source map.
		src, ok := sourcesMap[tv.SourcesId]
		if !ok {
			verdictCounter.Add(ctx, 1, project, "skipped_no_source")
			continue
		}
		if !sources.HasCommitData(src) {
			verdictCounter.Add(ctx, 1, project, "skipped_no_commit_data")
			continue
		}
		// Checks unsubmitted code.
		if sources.FromUnsubmittedCode(src, presubmit) {
			verdictCounter.Add(ctx, 1, project, "skipped_unsubmitted_code")
			continue
		}
		// Checks skips and duplicates.
		allSkippedAndDuplicate := true
		for _, r := range tv.Results {
			invID, err := resultdb.InvocationFromTestResultName(r.Result.Name)
			if err != nil {
				return nil, errors.Annotate(err, "invocation from test result name").Err()
			}
			_, isDuplicate := duplicateMap[invID]
			if r.Result.Status != rdbpb.TestStatus_SKIP && !isDuplicate {
				results = append(results, tv)
				allSkippedAndDuplicate = false
				break
			}
		}
		if allSkippedAndDuplicate {
			verdictCounter.Add(ctx, 1, project, "skipped_all_skipped_or_duplicate")
		}
	}
	return results, nil
}

func testVariantBranchKeys(tvs []*rdbpb.TestVariant, project string, sourcesMap map[string]*pb.Sources) []testvariantbranch.Key {
	results := make([]testvariantbranch.Key, len(tvs))
	for i, tv := range tvs {
		src := sourcesMap[tv.SourcesId]
		results[i] = testvariantbranch.Key{
			Project:     project,
			TestID:      tv.TestId,
			VariantHash: tv.VariantHash,
			RefHash:     testvariantbranch.RefHash(pbutil.SourceRefHash(pbutil.SourceRefFromSources(src))),
		}
	}
	return results
}
