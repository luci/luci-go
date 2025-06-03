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

	"go.chromium.org/luci/analysis/internal/changepoints"
	tvbexporter "go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/tracing"
)

// ChangePointExporter is an ingestion stage that exports test results for changepoint analysis.
// It implements IngestionSink.
type ChangePointExporter struct {
	exporter *tvbexporter.Exporter
}

// Name returns a unique name for the ingestion stage.
func (ChangePointExporter) Name() string {
	return "export-changepoints"
}

// Ingest exports the provided test verdicts for changepoint analysis.
func (e *ChangePointExporter) Ingest(ctx context.Context, input Inputs) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/verdictingester.ChangePointExporter.Ingest")
	defer func() { tracing.End(s, err) }()

	cfg, err := config.Get(ctx)
	if err != nil {
		return errors.Fmt("read config: %w", err)
	}
	tvaEnabled := cfg.TestVariantAnalysis != nil && cfg.TestVariantAnalysis.Enabled
	if !tvaEnabled {
		return nil
	}
	err = changepoints.Analyze(ctx, input.Verdicts, input.Payload, input.SourcesByID, e.exporter)
	if err != nil {
		return errors.Fmt("analyze test variants: %w", err)
	}
	return nil
}
