// Copyright 2025 The LUCI Authors.
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

// Package verdictingester defines the top-level task queue which ingests
// test verdicts from ResultDB and pushes it into LUCI Analysis's analysis
// and BigQuery export pipelines.
package verdictingester

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	antstestresultexporter "go.chromium.org/luci/analysis/internal/ants/testresults/exporter"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/tracing"
)

// AnTSTestResultExporter is an ingestion stage that exports Android test results to BigQuery.
// It implements IngestionSink.
type AnTSTestResultExporter struct {
	exporter *antstestresultexporter.Exporter
}

// Name returns a unique name for the ingestion stage.
func (AnTSTestResultExporter) Name() string {
	return "export-ants-test-results"
}

func (e *AnTSTestResultExporter) Ingest(ctx context.Context, input Inputs) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/verdictingester.AnTSTestResultExporter.Ingest")
	defer func() { tracing.End(s, err) }()

	// only export Android project.
	if input.Payload.Project != "android" {
		return nil
	}
	payload := input.Payload
	key := checkpoints.Key{
		Project:    input.Payload.Project,
		ResourceID: fmt.Sprintf("%s/%s", payload.Invocation.ResultdbHost, payload.Invocation.InvocationId),
		ProcessID:  fmt.Sprintf("verdict-ingestion/export-ants-test-results"),
		Uniquifier: fmt.Sprintf("%v", payload.TaskIndex),
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

	exportOptions := antstestresultexporter.ExportOptions{
		Invocation: input.Invocation,
	}
	err = e.exporter.Export(ctx, input.Verdicts, exportOptions)
	if err != nil {
		return errors.Fmt("export test_results: %w", err)
	}
	// Create the checkpoint.
	ms := []*spanner.Mutation{checkpoints.Insert(ctx, key, checkpointTTL)}
	if _, err := span.Apply(ctx, ms); err != nil {
		return errors.Fmt("create checkpoint: %w", err)
	}
	return nil
}
