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

// Package resultingester defines an orchestrator for ingesting test results
// from ResultDB and pushing it into:
// - Test results table (for exoneration analysis)
// - Test results BigQuery export
// - Changepoint analysis
//
// The orchestrator is run on a task queue for legacy invocations, and
// on a pub/sub handler for root invocations.
package resultingester

import (
	"context"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

// IngestionSink is the interface implemented by consumers of test results
// in the ingestion pipeline.
type IngestionSink interface {
	// A short identifier for the sink, for use in errors.
	Name() string
	// IngestLegacy the test results from a legacy invocation.
	IngestLegacy(ctx context.Context, input LegacyInputs) error
	// IngestRootInvocation the test results from a root invocation.
	IngestRootInvocation(ctx context.Context, input RootInvocationInputs) error
}

// orchestrator orchestrates the ingestion of test results to the
// specified sinks.
type orchestrator struct {
	// The set of ingestion sinks to write to, in order.
	sinks []IngestionSink
}
