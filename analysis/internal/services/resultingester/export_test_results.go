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

package resultingester

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"go.opentelemetry.io/otel/attribute"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/testresults/exporter"
	"go.chromium.org/luci/analysis/internal/tracing"
)

// NewTestResultsExporter initialises a new TestResultsExporter instance.
func NewTestResultsExporter(client exporter.InsertClient) *TestResultsExporter {
	exporter := exporter.NewExporter(client)
	return &TestResultsExporter{
		exporter: exporter,
	}
}

// TestResultsExporter is an ingestion stage that exports test results to BigQuery.
// It implements IngestionSink.
type TestResultsExporter struct {
	exporter *exporter.Exporter
}

// Name returns a unique name for the ingestion stage.
func (TestResultsExporter) Name() string {
	return "export-test-results"
}

// Ingest exports the provided test results to BigQuery.
func (e *TestResultsExporter) Ingest(ctx context.Context, input Inputs) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.TestResultsExporter.Ingest")
	defer func() { tracing.End(s, err) }()

	destinations := []exporter.ExportDestination{
		exporter.ByDayTable,
		exporter.ByMonthTable,
	}
	for _, dest := range destinations {
		err := e.exportTo(ctx, input, dest)
		if err != nil {
			return errors.Fmt("export to %q: %w", dest.Key, err)
		}
	}
	return nil
}

// Ingest exports the provided test results to BigQuery.
func (e *TestResultsExporter) exportTo(ctx context.Context, input Inputs, dest exporter.ExportDestination) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.TestResultsExporter.exportTo",
		attribute.String("destination", dest.Key))
	defer func() { tracing.End(s, err) }()

	key := checkpoints.Key{
		Project:    input.Project,
		ResourceID: fmt.Sprintf("%s/%s/%s", input.ResultDBHost, input.RootInvocationID, input.InvocationID),
		ProcessID:  fmt.Sprintf("result-ingestion/export-test-results/%s", dest.Key),
		Uniquifier: fmt.Sprintf("%v", input.PageNumber),
	}
	exists, err := checkpoints.Exists(span.Single(ctx), key)
	if err != nil {
		return errors.Fmt("test existance of checkpoint: %w", err)
	}
	if exists {
		// We already performed this export previously. Do not perform it
		// again to avoid duplicate rows in the destination table.
		return nil
	}

	// Export test verdicts.
	exportOptions := exporter.Options{
		RootInvocationID: input.RootInvocationID,
		RootRealm:        realms.Join(input.Project, input.SubRealm),
		PartitionTime:    input.PartitionTime,
		Parent:           input.Parent,
		Sources:          input.Sources,
	}
	err = e.exporter.Export(ctx, input.Verdicts, dest, exportOptions)
	if err != nil {
		return errors.Fmt("export: %w", err)
	}

	// Create the checkpoint.
	ms := []*spanner.Mutation{checkpoints.Insert(ctx, key, checkpointTTL)}
	if _, err := span.Apply(ctx, ms); err != nil {
		return errors.Fmt("create checkpoint: %w", err)
	}

	return nil
}
