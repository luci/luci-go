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

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/changepoints"
	tvbexporter "go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/tracing"
)

// IngestForChangepointAnalysis implements an ingestion stage that ingests test
// results into the changepoint analysis.
type IngestForChangepointAnalysis struct {
	exporter *tvbexporter.Exporter
}

// NewIngestForChangepointAnalysis initialises a new IngestForChangepointAnalysis.
func NewIngestForChangepointAnalysis(exporter *tvbexporter.Exporter) *IngestForChangepointAnalysis {
	return &IngestForChangepointAnalysis{
		exporter: exporter,
	}
}

// Name returns a unique name for the ingestion stage.
func (IngestForChangepointAnalysis) Name() string {
	return "ingest-for-changepoint-analysis"
}

// Ingest ingests test results into the changepoint analysis.
func (a *IngestForChangepointAnalysis) Ingest(ctx context.Context, input Inputs) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.IngestForChangepointAnalysis.Ingest")
	defer func() { tracing.End(s, err) }()

	cfg, err := config.Get(ctx)
	if err != nil {
		return errors.Fmt("read config: %w", err)
	}
	tvaEnabled := cfg.TestVariantAnalysis != nil && cfg.TestVariantAnalysis.Enabled
	if !tvaEnabled {
		return nil
	}
	opts := changepoints.AnalysisOptions{
		Project:          input.Project,
		ResultDBHost:     input.ResultDBHost,
		RootInvocationID: input.RootInvocationID,
		InvocationID:     input.InvocationID,
		Sources:          input.Sources,
		PartitionTime:    input.PartitionTime,
	}
	err = changepoints.AnalyzeRun(ctx, input.Verdicts, opts, a.exporter)
	if err != nil {
		return errors.Fmt("analyze test variants: %w", err)
	}

	return nil
}
