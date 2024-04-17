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

// Package artifactexporter handles uploading artifacts to BigQuery.
// This is the replacement of the legacy artifact exporter in bqexp
package artifactexporter

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
)

// ExportArtifactsTask describes how to route export artifact task.
var ExportArtifactsTask = tq.RegisterTaskClass(tq.TaskClass{
	ID:            "export-artifacts",
	Prototype:     &taskspb.ExportArtifacts{},
	Kind:          tq.Transactional,
	Queue:         "artifactexporter",
	RoutingPrefix: "/internal/tasks/artifactexporter", // for routing to "artifactexporter" service
})

// InitServer initializes a artifactexporter server.
func InitServer(srv *server.Server) {
	// init() below takes care of everything.
}

func init() {
	ExportArtifactsTask.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		task := msg.(*taskspb.ExportArtifacts)
		return exportArtifacts(ctx, invocations.ID(task.InvocationId))
	})
}

// Schedule schedules tasks for all the finalized invocations.
func Schedule(ctx context.Context, invID invocations.ID) error {
	return tq.AddTask(ctx, &tq.Task{
		Title:   string(invID),
		Payload: &taskspb.ExportArtifacts{InvocationId: string(invID)},
	})
}

// exportArtifact reads all text artifacts (including artifact content
// in RBE-CAS) for an invocation and exports to BigQuery.
func exportArtifacts(ctx context.Context, invID invocations.ID) error {
	logging.Infof(ctx, "Exporting artifacts for invocation ID %s", invID)
	return nil
}
