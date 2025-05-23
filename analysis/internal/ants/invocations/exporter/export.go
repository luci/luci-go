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

package exporter

import (
	"context"

	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

// InsertClient defines an interface for inserting rows into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows *bqpb.AntsInvocationRow) error
}

// Exporter provides methods to stream rows into BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter instantiates a new Exporter. The given client is used
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

// Export exports the given invocation to BigQuery.
func (e *Exporter) Export(ctx context.Context, inv *rdbpb.Invocation) error {
	invID, err := pbutil.ParseInvocationName(inv.Name)
	if err != nil {
		// Should not happen
		return errors.Annotate(err, "parse invocation name").Err()
	}

	// TODO(beining): populate more fields.
	exportRow := &bqpb.AntsInvocationRow{
		InvocationId:   invID,
		SchedulerState: bqpb.AntsInvocationRow_SCHEDULER_STATE_UNSPECIFIED,
		Timing: &bqpb.AntsInvocationRow_Timing{
			CreationTimestamp: inv.CreateTime.AsTime().UnixMilli(),
			CompleteTimestamp: inv.FinalizeTime.AsTime().UnixMilli(),
		},
		CompletionTime: inv.FinalizeTime,
	}
	if err := e.client.Insert(ctx, exportRow); err != nil {
		return errors.Annotate(err, "insert rows").Err()
	}
	return nil
}
