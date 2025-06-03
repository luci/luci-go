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

	"go.chromium.org/luci/common/errors"

	antsinvocationexporter "go.chromium.org/luci/analysis/internal/ants/invocations/exporter"
	"go.chromium.org/luci/analysis/internal/tracing"
)

// AnTSTInvocationExporter is an ingestion stage that exports Android invocations to BigQuery.
// It implements IngestionSink.
type AnTSTInvocationExporter struct {
	exporter *antsinvocationexporter.Exporter
}

// Name returns a unique name for the ingestion stage.
func (AnTSTInvocationExporter) Name() string {
	return "export-ants-invocation"
}

func (e *AnTSTInvocationExporter) Ingest(ctx context.Context, input Inputs) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/verdictingester.AnTSTInvocationExporter.Ingest")
	defer func() { tracing.End(s, err) }()

	// Export AnTS invocation to BigQuery after all test results are exported.
	// This ordering is required by AnTS F1 users, to make sure test results are completed
	// when joined with the invocation table.
	if !input.LastPage {
		return nil
	}
	// only export Android project.
	if input.Payload.Project != "android" {
		return nil
	}

	err = e.exporter.Export(ctx, input.Invocation)
	if err != nil {
		return errors.Fmt("export invocation: %w", err)
	}
	return nil
}
