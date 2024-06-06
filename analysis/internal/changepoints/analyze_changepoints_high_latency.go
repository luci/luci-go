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

package changepoints

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints/analyzer"
	"go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/sources"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/config"
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
		// - "skipped_no_sources": The verdict was skipped because it has no source
		//   data.
		// - "skipped_no_commit_data": The verdict was skipped because its source
		//   does not have enough commit data (e.g. commit position).
		// - "skipped_out_of_order": The verdict was skipped because it was too
		//   out of order.
		// - "skipped_unsubmitted_code": The verdict was skipped because is was
		//   from unsubmitted code.
		// - "skipped_all_skipped_or_unclaimed":  The verdict was skipped because
		//   it contains only skipped or unclaimed results.
		field.String("status"),
	)
)

// Analyze performs change point analyses based on incoming test verdicts.
// sourcesMap contains the information about the source code being tested.
func Analyze(ctx context.Context, tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestVerdicts, sourcesMap map[string]*pb.Sources, exporter *bqexporter.Exporter) error {
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

	invIDs, err := invocationIDsToClaimHighLatency(tvs, sourcesMap)
	if err != nil {
		return errors.Annotate(err, "identify invocation ids to claim").Err()
	}

	rootInvocationID := payload.Invocation.InvocationId
	claimedInvs, err := tryClaimInvocations(ctx, payload.Project, rootInvocationID, invIDs)
	if err != nil {
		return errors.Annotate(err, "try to claim invocations").Err()
	}

	// Only keep "relevant" test variants, and test variant with commit information.
	filteredTVs, err := filterTestVariantsHighLatency(ctx, tvs, payload, claimedInvs, sourcesMap)
	if err != nil {
		return errors.Annotate(err, "filter test variants").Err()
	}

	if len(filteredTVs) == 0 {
		// Exit early.
		return nil
	}

	// Contains the test variant branches to be written to BigQuery.
	bqExporterInput := make([]bqexporter.PartialBigQueryRow, 0, len(tvs))

	firstTV := tvs[0]
	checkpointKey := checkpoints.Key{
		Project:    payload.Project,
		ResourceID: fmt.Sprintf("%s/%s", payload.Invocation.ResultdbHost, payload.Invocation.InvocationId),
		ProcessID:  "verdict-ingestion/analyze-changepoints",
		Uniquifier: fmt.Sprintf("%s/%s", firstTV.TestId, firstTV.VariantHash),
	}

	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Check checkpoints table to see if we have already processed this batch.
		exists, err := checkpoints.Exists(ctx, checkpointKey)
		if err != nil {
			return errors.Annotate(err, "test existence of checkpoint").Err()
		}
		// This batch has been processed, we can skip it.
		if exists {
			return nil
		}

		// Query TestVariantBranch from spanner.
		tvbks := testVariantBranchKeys(filteredTVs, payload.Project, sourcesMap)

		// The list of mutations for this transaction.
		mutations := []*spanner.Mutation{}

		// Buffers allocated once and re-used for processing
		// all test variant branches.
		var hs inputbuffer.HistorySerializer
		var analysis analyzer.Analyzer

		// Handle each read test variant branch.
		f := func(i int, tvb *testvariantbranch.Entry) error {
			tv := filteredTVs[i]
			// "Insert" the new test variant to input buffer.
			tvb, inOrder, err := insertVerdictIntoInputBuffer(tvb, tv, payload, claimedInvs, sourcesMap)
			if err != nil {
				return errors.Annotate(err, "insert into input buffer").Err()
			}
			if !inOrder {
				verdictCounter.Add(ctx, 1, payload.Project, "skipped_out_of_order")
				return nil
			}

			segments := analysis.Run(tvb)
			tvb.ApplyRetentionPolicyForFinalizedSegments(payload.PartitionTime.AsTime())
			mut, err := tvb.ToMutation(&hs)
			if err != nil {
				return errors.Annotate(err, "test variant branch to mutation").Err()
			}
			mutations = append(mutations, mut)
			bqRow, err := bqexporter.ToPartialBigQueryRow(tvb, segments)
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

		// Store checkpoint.
		mutations = append(mutations, checkpoints.Insert(ctx, checkpointKey, checkpointTTL))
		span.BufferWrite(ctx, mutations...)
		verdictCounter.Add(ctx, int64(ingestedVerdictCount), payload.Project, "ingested")
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

// insertVerdictIntoInputBuffer inserts the new test variant tv into the input buffer
// of TestVariantBranch tvb.
// If tvb is nil, it means it is not in spanner. In this case, return a new
// TestVariantBranch object with a single element in the input buffer.
func insertVerdictIntoInputBuffer(tvb *testvariantbranch.Entry, tv *rdbpb.TestVariant, payload *taskspb.IngestTestVerdicts, claimedInvs map[string]bool, sourcesMap map[string]*pb.Sources) (updatedTVB *testvariantbranch.Entry, inOrder bool, err error) {
	src := sourcesMap[tv.SourcesId]
	if tvb == nil {
		ref := pbutil.SourceRefFromSources(src)
		tvb = &testvariantbranch.Entry{
			IsNew:       true,
			Project:     payload.Project,
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

	runs, err := testvariantbranch.ToRuns(tv, payload.PartitionTime.AsTime(), claimedInvs, src)
	if err != nil {
		return nil, false, err
	}

	for _, run := range runs {
		if !tvb.InsertToInputBuffer(run) {
			// Out of order run.
			return nil, false, nil
		}
	}
	return tvb, true, nil
}

// filterTestVariantsHighLatency only keeps test variants that satisfy all following
// conditions:
//   - Have commit position information.
//   - Have at least 1 non-skipped test result from a claimed invocation.
//   - Not from unsubmitted code (i.e. try run that did not result in submitted code)
//   - Are not eligible for ingestion through the low-latency pipeline,
//     i.e. test runs testing an applied changelist.
func filterTestVariantsHighLatency(ctx context.Context, tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestVerdicts, claimedInvs map[string]bool, sourcesMap map[string]*pb.Sources) ([]*rdbpb.TestVariant, error) {
	results := []*rdbpb.TestVariant{}
	presubmit := payload.PresubmitRun
	project := payload.Project
	for _, tv := range tvs {
		// Checks source map.
		src, ok := sourcesMap[tv.SourcesId]
		if !ok {
			verdictCounter.Add(ctx, 1, project, "skipped_no_sources")
			continue
		}
		if !sources.HasCommitData(src) {
			verdictCounter.Add(ctx, 1, project, "skipped_no_commit_data")
			continue
		}
		wasSubmittedByPresubmit := presubmit != nil &&
			presubmit.Status == pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED &&
			presubmit.Mode == pb.PresubmitRunMode_FULL_RUN
		if len(src.Changelists) > 0 && !wasSubmittedByPresubmit {
			verdictCounter.Add(ctx, 1, project, "skipped_unsubmitted_code")
			continue
		}

		// Checks skips and duplicates.
		allSkippedOrUnclaimed := true
		for _, r := range tv.Results {
			invID, err := resultdb.InvocationFromTestResultName(r.Result.Name)
			if err != nil {
				return nil, errors.Annotate(err, "invocation from test result name").Err()
			}
			_, isClaimed := claimedInvs[invID]
			if r.Result.Status != rdbpb.TestStatus_SKIP && isClaimed {
				results = append(results, tv)
				allSkippedOrUnclaimed = false
				break
			}
		}
		if allSkippedOrUnclaimed {
			verdictCounter.Add(ctx, 1, project, "skipped_all_skipped_or_unclaimed")
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
