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

// Package resultingester defines the task queue which ingests test results
// from ResultDB and pushes it into:
// - Test results table (for exoneration analysis)
// - Test results BigQuery export
// - Changepoint analysis
package resultingester

import (
	"context"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/lowlatency"
	"go.chromium.org/luci/analysis/internal/tracing"
	"go.chromium.org/luci/analysis/pbutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
)

// IngestForLowLatencyTestResults implements an ingestion stage that writes test
// results to the TestResultsBySourcePosition table.
type IngestForLowLatencyTestResults struct{}

// Name returns a unique name for the ingestion stage.
func (IngestForLowLatencyTestResults) Name() string {
	return "ingest-for-exoneration"
}

// IngestLegacy writes test results to the TestResultsBySourcePosition table.
func (IngestForLowLatencyTestResults) IngestLegacy(ctx context.Context, input LegacyInputs) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.IngestForLowLatencyTestResults.IngestLegacy")
	defer func() { tracing.End(s, err) }()

	if input.Sources.GetBaseSources() == nil {
		// Test results without base sources are not ingested into this process.
		return nil
	}

	testResultSources, err := toTestResultSources(input.Sources)
	if err != nil {
		return errors.Fmt("convert sources: %w", err)
	}

	generator := func(outputC chan batch) error {
		batchTestResults(input, testResultSources, outputC)
		return nil
	}

	err = recordTestResults(ctx, generator)
	if err != nil {
		return errors.Fmt("record test results: %w", err)
	}
	return nil
}

func (IngestForLowLatencyTestResults) IngestRootInvocation(ctx context.Context, input RootInvocationInputs) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.IngestForLowLatencyTestResults.IngestRootInvocation")
	defer func() { tracing.End(s, err) }()

	if input.Sources.GetBaseSources() == nil {
		// Test results without base sources are not ingested into this process.
		return nil
	}

	testResultSources, err := toTestResultSources(input.Sources)
	if err != nil {
		return errors.Fmt("convert sources: %w", err)
	}

	generator := func(outputC chan batch) error {
		batchPubSubTestResults(input.Notification, testResultSources, outputC)
		return nil
	}

	err = recordTestResults(ctx, generator)
	if err != nil {
		return errors.Fmt("record test results: %w", err)
	}
	return nil
}

// recordTestResults records test results into the TestResultsBySourcePosition table.
// This operation is idempotent and may be safely called multiple times.
// This method should only be called if sources are available.
//
// The generator function is called in a separate goroutine and is expected to
// push batches of test results into the output channel. It should not close the
// channel.
func recordTestResults(ctx context.Context, generator func(outputC chan batch) error) error {
	const workerCount = 8

	return parallel.WorkPool(workerCount, func(c chan<- func() error) {
		batchC := make(chan batch)

		c <- func() error {
			defer close(batchC)
			err := generator(batchC)
			if err != nil {
				return errors.Fmt("batch test results: %w", err)
			}
			return nil
		}

		for batch := range batchC {
			c <- func() error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, batch.testResults...)
					return nil
				})
				if err != nil {
					return transient.Tag.Apply(errors.Fmt("inserting test results: %w", err))
				}
				return nil
			}
		}
	})
}

func toTestResultSources(srcs *analysispb.Sources) (testresults.Sources, error) {
	var result testresults.Sources
	result.RefHash = pbutil.SourceRefHash(pbutil.SourceRefFromSources(srcs))
	result.Position = pbutil.SourcePosition(srcs)

	for _, cl := range srcs.Changelists {
		err := testresults.ValidateGerritHostname(cl.Host)
		if err != nil {
			return testresults.Sources{}, err
		}
		result.Changelists = append(result.Changelists, testresults.Changelist{
			Host:      cl.Host,
			Change:    cl.Change,
			Patchset:  cl.Patchset,
			OwnerKind: cl.OwnerKind,
		})
	}
	result.IsDirty = srcs.IsDirty
	return result, nil
}

type batch struct {
	// Test results to insert. Already prepared as Spanner mutations.
	testResults []*spanner.Mutation
}

func batchTestResults(input LegacyInputs, sources testresults.Sources, outputC chan batch) {
	// Must be selected such that no more than 80,000 mutations occur in
	// one transaction in the worst case.
	// See https://cloud.google.com/spanner/quotas#limits-for
	// 1000 results requires around 20,000 mutations which is well within
	// this limit.
	const batchSize = 1000

	var trs []*spanner.Mutation
	startBatch := func() {
		trs = make([]*spanner.Mutation, 0, batchSize)
	}
	outputBatch := func() {
		if len(trs) == 0 {
			// This should never happen.
			panic("Pushing empty batch")
		}

		outputC <- batch{
			testResults: trs,
		}
	}

	startBatch()
	for _, tv := range input.Verdicts {
		for _, inputTR := range tv.Results {
			// Limit batch size.
			// Keep all results for one test variant in one batch, so that the
			// TestVariantRealm record is kept together with the test results.
			if len(trs) > batchSize {
				outputBatch()
				startBatch()
			}

			tr := lowlatency.TestResult{
				Project:          input.Project,
				TestID:           tv.TestId,
				VariantHash:      tv.VariantHash,
				Sources:          sources,
				RootInvocationID: lowlatency.RootInvocationID{Value: input.ExportRootInvocationID, IsLegacy: true},
				WorkUnitID:       lowlatency.WorkUnitID{Value: input.InvocationID, IsLegacy: true},
				ResultID:         inputTR.Result.ResultId,
				PartitionTime:    input.PartitionTime,
				SubRealm:         input.SubRealm,
				IsUnexpected:     !inputTR.Result.Expected,
				Status:           pbutil.LegacyTestStatusFromResultDB(inputTR.Result.Status),
			}

			// Convert the test result into a mutation immediately
			// to avoid storing both the TestResult object and
			// mutation object in memory until the transaction
			// commits.
			trs = append(trs, tr.SaveUnverified())
		}
	}
	if len(trs) > 0 {
		outputBatch()
	}
}

func batchPubSubTestResults(n *rdbpb.TestResultsNotification, sources testresults.Sources, outputC chan batch) error {
	// Must be selected such that no more than 80,000 mutations occur in
	// one transaction in the worst case.
	// See https://cloud.google.com/spanner/quotas#limits-for
	// We use a smaller number, ~11K mutations as at writing, as this
	// is sufficient to obtain good performance.
	const batchSize = 1000

	var trs []*spanner.Mutation
	startBatch := func() {
		trs = make([]*spanner.Mutation, 0, batchSize)
	}
	outputBatch := func() {
		if len(trs) == 0 {
			// This should never happen.
			panic("Pushing empty batch")
		}

		outputC <- batch{
			testResults: trs,
		}
	}

	project, subRealm := realms.Split(n.RootInvocationMetadata.Realm)
	partitionTime := n.RootInvocationMetadata.CreateTime.AsTime()

	startBatch()
	for _, workUnit := range n.TestResultsByWorkUnit {
		rootInvocationID, workUnitID, err := rdbpbutil.ParseWorkUnitName(workUnit.WorkUnitName)
		if err != nil {
			return tq.Fatal.Apply(errors.Fmt("invalid work unit name %q: %w", workUnit.WorkUnitName, err))
		}

		for _, inputTR := range workUnit.TestResults {
			// Limit batch size.
			if len(trs) > batchSize {
				outputBatch()
				startBatch()
			}

			tr := lowlatency.TestResult{
				Project:          project,
				TestID:           inputTR.TestId,
				VariantHash:      inputTR.VariantHash,
				Sources:          sources,
				RootInvocationID: lowlatency.RootInvocationID{Value: rootInvocationID},
				WorkUnitID:       lowlatency.WorkUnitID{Value: workUnitID},
				ResultID:         inputTR.ResultId,
				PartitionTime:    partitionTime,
				SubRealm:         subRealm,
				IsUnexpected:     !inputTR.Expected,
				Status:           pbutil.LegacyTestStatusFromResultDB(inputTR.Status),
			}

			// Convert the test result into a mutation immediately
			// to avoid storing both the TestResult object and
			// mutation object in memory until the transaction
			// commits.
			trs = append(trs, tr.SaveUnverified())
		}
	}
	if len(trs) > 0 {
		outputBatch()
	}
	return nil
}
