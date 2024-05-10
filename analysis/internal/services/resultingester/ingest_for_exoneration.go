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
	// Add support for Spanner transactions in TQ.
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/lowlatency"
	"go.chromium.org/luci/analysis/internal/tracing"
	"go.chromium.org/luci/analysis/pbutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"

	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

type IngestForExoneration struct{}

func (IngestForExoneration) Name() string {
	return "ingest-for-exoneration"
}

func (IngestForExoneration) Ingest(ctx context.Context, input Inputs) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.IngestForExoneration.Ingest")
	defer func() { tracing.End(s, err) }()

	if input.Sources == nil {
		// Test results without sources are not ingested into this process.
		return nil
	}
	err = recordTestResults(ctx, input)
	if err != nil {
		return transient.Tag.Apply(errors.Annotate(err, "record test results").Err())
	}
	return nil
}

// recordTestResults records test results into the TestResultsBySourcePosition table.
// This operation is idempotent and may be safely called multiple times.
// This method should only be called if sources are available.
func recordTestResults(ctx context.Context, input Inputs) error {
	if input.Sources == nil {
		panic("ingestion.Sources must not be nil")
	}

	const workerCount = 8

	testResultSources, err := toTestResultSources(input.Sources)
	if err != nil {
		return errors.Annotate(err, "convert sources").Err()
	}

	return parallel.WorkPool(workerCount, func(c chan<- func() error) {
		batchC := make(chan batch)

		c <- func() error {
			defer close(batchC)
			batchTestResults(input, testResultSources, batchC)
			return nil
		}

		for batch := range batchC {
			// Bind to a local variable so it can be used in a goroutine without being
			// overwritten. See https://go.dev/doc/faq#closures_and_goroutines
			batch := batch

			c <- func() error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, batch.testResults...)
					return nil
				})
				if err != nil {
					return errors.Annotate(err, "inserting test results").Err()
				}
				return nil
			}
		}
	})
}

func toTestResultSources(srcs *analysispb.Sources) (testresults.Sources, error) {
	var result testresults.Sources

	// We only know how to extract source position for gitiles-based
	// sources for now.
	if srcs.GitilesCommit != nil {
		result.RefHash = pbutil.SourceRefHash(pbutil.SourceRefFromSources(srcs))
		result.Position = srcs.GitilesCommit.Position
	}
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

func batchTestResults(input Inputs, sources testresults.Sources, outputC chan batch) {
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

	startBatch()
	for _, tv := range input.Verdicts {
		// Limit batch size.
		// Keep all results for one test variant in one batch, so that the
		// TestVariantRealm record is kept together with the test results.
		if len(trs) > batchSize {
			outputBatch()
			startBatch()
		}

		for _, inputTR := range tv.Results {
			tr := lowlatency.TestResult{
				Project:          input.Project,
				TestID:           tv.TestId,
				VariantHash:      tv.VariantHash,
				Sources:          sources,
				RootInvocationID: input.RootInvocationID,
				InvocationID:     input.InvocationID,
				ResultID:         inputTR.Result.ResultId,
				PartitionTime:    input.PartitionTime,
				SubRealm:         input.SubRealm,
				IsUnexpected:     !inputTR.Result.Expected,
				Status:           pbutil.TestResultStatusFromResultDB(inputTR.Result.Status),
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
