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

// IngestLegacy exports the provided test results (from a legacy invocation)
// to BigQuery.
func (e *TestResultsExporter) IngestLegacy(ctx context.Context, input LegacyInputs) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.TestResultsExporter.IngestLegacy")
	defer func() { tracing.End(s, err) }()

	destinations := []exporter.ExportDestination{
		exporter.ByDayTable,
		exporter.ByMonthTable,
	}
	for _, dest := range destinations {
		err := e.exportToLegacy(ctx, input, dest)
		if err != nil {
			return errors.Fmt("export to %q: %w", dest.Key, err)
		}
	}
	return nil
}

// exportToLegacy exports the provided test results to BigQuery.
func (e *TestResultsExporter) exportToLegacy(ctx context.Context, input LegacyInputs, dest exporter.ExportDestination) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.TestResultsExporter.exportToLegacy",
		attribute.String("destination", dest.Key))
	defer func() { tracing.End(s, err) }()

	key := checkpoints.Key{
		Project:    input.Project,
		ResourceID: fmt.Sprintf("%s/%s/%s", input.ResultDBHost, input.ExportRootInvocationID, input.InvocationID),
		ProcessID:  fmt.Sprintf("result-ingestion/export-test-results/%s", dest.Key),
		Uniquifier: fmt.Sprintf("%v", input.PageNumber),
	}
	exists, err := checkpoints.Exists(span.Single(ctx), key)
	if err != nil {
		return errors.Fmt("test existence of checkpoint: %w", err)
	}
	if exists {
		// We already performed this export previously. Do not perform it
		// again to avoid duplicate rows in the destination table.
		return nil
	}

	// Export test verdicts.
	exportOptions := exporter.LegacyOptions{
		// The legacy invocation marked "export root". Not to be confused
		// with ResultDB's Root Invocations.
		ExportRootInvocationID: input.ExportRootInvocationID,
		RootRealm:              realms.Join(input.Project, input.SubRealm),
		PartitionTime:          input.PartitionTime,
		Parent:                 input.Parent,
		Sources:                input.Sources,
	}
	err = e.exporter.ExportLegacy(ctx, input.Verdicts, dest, exportOptions)
	if err != nil {
		return errors.Fmt("export legacy: %w", err)
	}

	// Create the checkpoint.
	ms := []*spanner.Mutation{checkpoints.Insert(ctx, key, checkpointTTL)}
	if _, err := span.Apply(ctx, ms); err != nil {
		return errors.Fmt("create checkpoint: %w", err)
	}

	return nil
}

// IngestRootInvocation exports the provided test results from a root invocation to BigQuery.
func (e *TestResultsExporter) IngestRootInvocation(ctx context.Context, input RootInvocationInputs) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.TestResultsExporter.IngestRootInvocation")
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

// exportTo exports the provided test results to BigQuery.
func (e *TestResultsExporter) exportTo(ctx context.Context, input RootInvocationInputs, dest exporter.ExportDestination) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.TestResultsExporter.exportTo",
		attribute.String("destination", dest.Key))
	defer func() { tracing.End(s, err) }()

	project, _ := realms.Split(input.Notification.RootInvocationMetadata.Realm)

	key := checkpoints.Key{
		Project:    project,
		ResourceID: fmt.Sprintf("%s/%s", input.Notification.ResultdbHost, input.Notification.RootInvocationMetadata.RootInvocationId),
		ProcessID:  fmt.Sprintf("result-ingestion/export-test-results/%s", dest.Key),
		Uniquifier: input.Notification.DeduplicationKey,
	}
	exists, err := checkpoints.Exists(span.Single(ctx), key)
	if err != nil {
		return errors.Fmt("test existence of checkpoint: %w", err)
	}
	if exists {
		// We already performed this export previously. Do not perform it
		// again to avoid duplicate rows in the destination table.
		return nil
	}

	// Export test verdicts.
	exportOptions := exporter.Options{
		RootInvocation: input.Notification.RootInvocationMetadata,
		Sources:        input.Sources,
	}
	err = e.exporter.Export(ctx, input.Notification.TestResultsByWorkUnit, dest, exportOptions)
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
