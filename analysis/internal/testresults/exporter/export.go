// Copyright 2026 The LUCI Authors.
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

// Package exporter contains methods to export test results to BigQuery.
package exporter

import (
	"context"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"

	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// InsertClient defines an interface for inserting rows into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.TestResultRow, dest ExportDestination) error
}

// Exporter provides methods to stream test results to BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter instantiates a new Exporter. The given client is used
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

type Options struct {
	// Metadata about the root invocation the test results exist inside.
	// The partition time for the export is the root invocation's creation time.
	RootInvocation *resultpb.RootInvocationMetadata
	// The sources of the test results.
	Sources *pb.Sources
}

func (e *Exporter) Export(ctx context.Context, results []*rdbpb.TestResultsNotification_TestResultsByWorkUnit, dest ExportDestination, opts Options) error {
	// TODO(b/475387150): Implement BigQuery export.
	return nil
}
