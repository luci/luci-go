// Copyright 2024 The LUCI Authors.
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
//
// It implements two ingestion paths:
//   - Low latency ingestion: Ingestion for test results that are not
//     testing gerrit changelists. This occurs on the result-ingestion
//     pipeline. It is called low-latency because the result-ingestion
//     pipeline is generally triggered immediately upon invocation
//     finalization.
//   - High latency ingestion: Ingestion for test results testing gerrit
//     changelists. These results are only eligible for ingestion if
//     the presubmit run suceeded and was submitted. This occurs on the
//     verdict-ingestion pipeline because this is the only pipeline that
//     waits until the LUCI CV run has completed before it ingests. This
//     is known as the high-latency pipeline.
package changepoints

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"go.opentelemetry.io/otel/attribute"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints/analyzer"
	"go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/tracing"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

var (
	RunCounter = metric.NewCounter(
		"analysis/changepoints/analyze/runs",
		"The number of runs processed by changepoint analysis, classified by project and status.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// Possible values:
		// - "ingested": The run was ingested.
		// - "skipped_no_sources": The run was skipped because it has no source
		//   data.
		// - "skipped_dirty_sources": The run was skipped because the sources
		//   were dirty and not suitable for changepoint analysis.
		// - "skipped_out_of_order": The run was skipped because it was too
		//   out of order.
		// - "skipped_excluded_from_low_latency_pipeline": The run was skipped
		//   because the sources are not eligible for ingestion through the
		//   low-latency pipeline (sources contain possibly unsubmitted CLs).
		// - "skipped_only_skips": The run was skipped because
		//   it contains only skipped test results.
		// - "skipped_unclaimed": The run was skipped because
		//   it was already ingested under another root invocation.
		field.String("status"),
	)
)

const checkpointTTL = 91 * 24 * time.Hour

type AnalysisOptions struct {
	// The LUCI Project the test results should be ingested into.
	Project string
	// The ResultDB host from which the invocations are from.
	ResultDBHost string
	// The export root under which the test results are being ingested.
	RootInvocationID string
	// The invocation being ingested.
	InvocationID string
	// The sources tested by tests run in invocation InvocationID,
	// as viewed from export root RootInvocationID.
	Sources *pb.Sources
	// The start of the retention period.
	PartitionTime time.Time
}

// AnalyzeRun performs change point analyses based on an incoming test run.
func AnalyzeRun(ctx context.Context, tvs []*rdbpb.RunTestVerdict, opts AnalysisOptions, exporter *bqexporter.Exporter) error {
	if opts.Sources == nil {
		RunCounter.Add(ctx, int64(len(tvs)), opts.Project, "skipped_no_sources")
		return nil
	}
	if opts.Sources.IsDirty {
		RunCounter.Add(ctx, int64(len(tvs)), opts.Project, "skipped_dirty_sources")
		return nil
	}
	if !shouldClaimInLowLatencyPipeline(opts.Sources) {
		// Runs with changelists are ingested through the higher-latency pipeline
		// as we must wait for the CV run result to decide whether they will be
		// ingested.
		RunCounter.Add(ctx, int64(len(tvs)), opts.Project, "skipped_excluded_from_low_latency_pipeline")
		return nil
	}

	// Instead of processing 10,000 test verdicts at a time, we will process by
	// smaller batches. This will increase the robustness of the process, and
	// in case something go wrong, we will not need to reprocess the whole 10,000
	// runs.
	// Also, the number of mutations per transaction is limit to 40,000. The
	// mutations include the primary keys and the fields being updated. So we
	// cannot process 10,000 test runs at once.
	//
	// Note: Changing this value may cause some test variants in retried tasks to
	// get ingested twice.
	batchSize := 1000
	for startIndex := 0; startIndex < len(tvs); {
		endIndex := startIndex + batchSize
		if endIndex > len(tvs) {
			endIndex = len(tvs)
		}
		batchTVs := tvs[startIndex:endIndex]
		err := analyzeSingleRunBatch(ctx, batchTVs, opts, exporter)
		if err != nil {
			return errors.Fmt("analyzeSingleBatch: %w", err)
		}
		startIndex = int(endIndex)
	}

	return nil
}

func analyzeSingleRunBatch(ctx context.Context, tvs []*rdbpb.RunTestVerdict, opts AnalysisOptions, exporter *bqexporter.Exporter) (err error) {
	if len(tvs) == 0 {
		panic("at least one test variant must be provided")
	}
	firstTV := tvs[0]

	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/changepoints.analyzeSingleRunBatch")
	s.SetAttributes(attribute.String("project", opts.Project))
	s.SetAttributes(attribute.String("test_id", firstTV.TestId))
	s.SetAttributes(attribute.String("variant_hash", firstTV.VariantHash))
	defer func() { tracing.End(s, err) }()

	// Claim the invocation for the root invocation ID, if it is unclaimed.
	isClaimed, err := tryClaimInvocation(ctx, opts.Project, opts.InvocationID, opts.RootInvocationID)
	if err != nil {
		return errors.Fmt("test existance of checkpoint: %w", err)
	}
	if !isClaimed {
		// Already claimed by another root invocation. Do not ingest.
		RunCounter.Add(ctx, 1, opts.Project, "skipped_unclaimed")
		return nil
	}

	filteredTVs := filterRunTestVerdictsLowLatency(ctx, opts.Project, tvs)
	if len(filteredTVs) == 0 {
		// Exit early.
		return nil
	}

	checkpointKey := checkpoints.Key{
		Project:    opts.Project,
		ResourceID: fmt.Sprintf("%s/%s/%s", opts.ResultDBHost, opts.RootInvocationID, opts.InvocationID),
		ProcessID:  "result-ingestion/analyze-changepoints",
		Uniquifier: fmt.Sprintf("%s/%s", firstTV.TestId, firstTV.VariantHash),
	}

	// Contains the test variant branches to be written to BigQuery.
	bqExporterInput := make([]bqexporter.PartialBigQueryRow, 0, len(tvs))

	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Check checkpoints table to see if we have already processed this batch.
		exists, err := checkpoints.Exists(ctx, checkpointKey)
		if err != nil {
			return errors.Fmt("test existance of checkpoint: %w", err)
		}
		// This batch has been processed, we can skip it.
		if exists {
			return nil
		}

		// Query TestVariantBranch from spanner.
		tvbks := runTestVariantBranchKeys(filteredTVs, opts.Project, opts.Sources)

		// The list of mutations for this transaction.
		mutations := []*spanner.Mutation{}

		// Buffers allocated once and re-used for processing
		// all test variant branches.
		var hs inputbuffer.HistorySerializer
		var analysis analyzer.Analyzer

		// Handle each read test variant branch.
		f := func(i int, tvb *testvariantbranch.Entry) error {
			tv := filteredTVs[i]
			// "Insert" the new test run to input buffer.
			tvb, inOrder, err := insertRunIntoInputBuffer(tvb, tv, opts)
			if err != nil {
				return errors.Fmt("insert into input buffer: %w", err)
			}
			if !inOrder {
				RunCounter.Add(ctx, 1, opts.Project, "skipped_out_of_order")
				return nil
			}

			segments := analysis.Run(tvb)
			tvb.ApplyRetentionPolicyForFinalizedSegments(opts.PartitionTime)
			mut, err := tvb.ToMutation(&hs)
			if err != nil {
				return errors.Fmt("test variant branch to mutation: %w", err)
			}
			mutations = append(mutations, mut)
			bqRow, err := bqexporter.ToPartialBigQueryRow(tvb, segments)
			if err != nil {
				return errors.Fmt("test variant branch to bigquery row: %w", err)
			}
			bqExporterInput = append(bqExporterInput, bqRow)
			return nil
		}
		if err := testvariantbranch.ReadF(ctx, tvbks, f); err != nil {
			return errors.Fmt("read test variant branches: %w", err)
		}

		ingestedRunCount := len(mutations)

		// Store checkpoint.
		mutations = append(mutations, checkpoints.Insert(ctx, checkpointKey, checkpointTTL))
		span.BufferWrite(ctx, mutations...)
		RunCounter.Add(ctx, int64(ingestedRunCount), opts.Project, "ingested")
		return nil
	})

	if err != nil {
		return errors.Fmt("analyze change point: %w", err)
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
		return errors.Fmt("export to big query: %w", err)
	}
	return nil
}

// filterTestVerdictsLowLatency only keeps test verdicts with
// non-skipped results.
func filterRunTestVerdictsLowLatency(ctx context.Context, project string, tvs []*rdbpb.RunTestVerdict) []*rdbpb.RunTestVerdict {
	var results []*rdbpb.RunTestVerdict
	for _, tv := range tvs {
		allSkipped := true
		for _, r := range tv.Results {
			if r.Result.Status != rdbpb.TestStatus_SKIP {
				results = append(results, tv)
				allSkipped = false
				break
			}
		}
		if allSkipped {
			RunCounter.Add(ctx, 1, project, "skipped_only_skips")
		}
	}
	return results
}

// insertRunIntoInputBuffer inserts the new run test verdict tv into the input buffer
// of TestVariantBranch tvb.
// If tvb is nil, it means it is not in spanner. In this case, return a new
// TestVariantBranch object with a single element in the input buffer.
func insertRunIntoInputBuffer(tvb *testvariantbranch.Entry, v *rdbpb.RunTestVerdict, opts AnalysisOptions) (updatedTVB *testvariantbranch.Entry, inOrder bool, err error) {
	if tvb == nil {
		ref := pbutil.SourceRefFromSources(opts.Sources)
		tvb = &testvariantbranch.Entry{
			IsNew:       true,
			Project:     opts.Project,
			TestID:      v.TestId,
			VariantHash: v.VariantHash,
			RefHash:     pbutil.SourceRefHash(ref),
			Variant:     pbutil.VariantFromResultDB(v.Variant),
			SourceRef:   ref,
			InputBuffer: &inputbuffer.Buffer{
				HotBufferCapacity:  inputbuffer.DefaultHotBufferCapacity,
				ColdBufferCapacity: inputbuffer.DefaultColdBufferCapacity,
			},
		}
	}

	run := testvariantbranch.ToRun(v, opts.PartitionTime, opts.Sources)

	if !tvb.InsertToInputBuffer(run) {
		// Out of order run.
		return nil, false, nil
	}
	return tvb, true, nil
}

func runTestVariantBranchKeys(tvs []*rdbpb.RunTestVerdict, project string, sources *pb.Sources) []testvariantbranch.Key {
	results := make([]testvariantbranch.Key, len(tvs))
	refHash := testvariantbranch.RefHash(pbutil.SourceRefHash(pbutil.SourceRefFromSources(sources)))
	for i, tv := range tvs {
		results[i] = testvariantbranch.Key{
			Project:     project,
			TestID:      tv.TestId,
			VariantHash: tv.VariantHash,
			RefHash:     refHash,
		}
	}
	return results
}
