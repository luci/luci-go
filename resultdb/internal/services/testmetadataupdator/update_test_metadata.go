// Copyright 2023 The LUCI Authors.
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

package testmetadataupdator

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TestMetadataTask describes how to route update test metadata task.
var TestMetadataTask = tq.RegisterTaskClass(tq.TaskClass{
	ID:            "update-test-metadata",
	Prototype:     &taskspb.UpdateTestMetadata{},
	Kind:          tq.Transactional,
	Queue:         "testmetadataupdator",
	RoutingPrefix: "/internal/tasks/testmetadataupdator",
})

// InitServer initializes a testmetadataupdator server.
func InitServer(srv *server.Server) {
	// init() below takes care of everything.
}

func init() {
	TestMetadataTask.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		task := msg.(*taskspb.UpdateTestMetadata)
		return updateTestMetadata(ctx, invocations.ID(task.InvocationId))
	})
}

// updateTestMetadata update the TestMetadata table with test results in the given invocation
// and invocations that inherit its commit source information.
func updateTestMetadata(ctx context.Context, invID invocations.ID) error {
	inv, err := invocations.Read(span.Single(ctx), invID, invocations.ExcludeExtendedProperties)
	if err != nil {
		return err
	}
	if inv.State != pb.Invocation_FINALIZED {
		return errors.Fmt("Invocation is not finalized %s", invID)
	}
	if inv.SourceSpec == nil {
		logging.Infof(ctx, "Skipping metadata ingestion for invocation %s, sourceSpec not available.", inv.Name)
		return nil
	}

	source := inv.SourceSpec.Sources
	if inv.SourceSpec.Inherit {
		// Invocation with inherited sources will be processed with its including invocation.
		logging.Infof(ctx, "Skipping metadata ingestion for invocation %s, sources are inherited.", inv.Name)
		return nil
	}
	if source == nil {
		logging.Infof(ctx, "Skipping metadata ingestion for invocation %s, sources not available.", inv.Name)
		return nil
	}
	if len(source.Changelists) > 0 || source.IsDirty {
		logging.Infof(ctx, "Skipping metadata ingestion for invocation %s, invocation has applied gerrit changes or dirty sources", inv.Name)
		return nil
	}

	invIDs, err := graph.FindInheritSourcesDescendants(ctx, invID)
	if err != nil {
		return errors.Fmt("find descendants invocation %s: %w", invID, err)
	}

	updator := newUpdator(source, invIDs, time.Now())
	return updator.run(ctx)
}

// Schedule schedules tasks for all the finalized invocations.
func Schedule(ctx context.Context, invID invocations.ID) error {
	return tq.AddTask(ctx, &tq.Task{
		Title:   string(invID),
		Payload: &taskspb.UpdateTestMetadata{InvocationId: string(invID)},
	})
}
