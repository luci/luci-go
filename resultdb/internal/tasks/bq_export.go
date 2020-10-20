// Copyright 2020 The LUCI Authors.
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

package tasks

import (
	"context"

	"go.chromium.org/luci/server/experiments"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// BQExportTasks describes how to route bq export tasks.
//
// The handler is implemented in internal/services/bqexporter.
var BQExportTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:                  "bq-export-inv",
	Prototype:           &taskspb.ExportInvocationToBQ{},
	Kind:                tq.Transactional,
	InheritTraceContext: true,
	Queue:               "bqexporter",                 // use a dedicated queue
	RoutingPrefix:       "/internal/tasks/bqexporter", // for routing to "bqexporter" service
})

// UseBQExportTQ experiment enables using server/tq for bq export tasks.
var UseBQExportTQ = experiments.Register("rdb-use-tq-bq-export")

// ScheduleBQExport enqueues a BQExport task.
//
// The caller is responsible for ensuring that the invocation is finalized.
func ScheduleBQExport(ctx context.Context, id invocations.ID, export *pb.BigQueryExport) {
	tq.MustAddTask(ctx, &tq.Task{
		Payload: &taskspb.ExportInvocationToBQ{
			InvocationId: string(id),
			BqExport:     export,
		},
		Title: string(id),
	})
}
