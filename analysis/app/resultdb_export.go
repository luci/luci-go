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

// Package app contains pub/sub handlers.
package app

import (
	"context"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/pubsub"

	"go.chromium.org/luci/analysis/internal/services/resultingester"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

var (
	invocationsReadyForExportCounter = metric.NewCounter(
		"analysis/ingestion/pubsub/invocations_ready_for_export",
		"The number of 'ready for export' invocations received by LUCI Analysis from PubSub.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// "success", "transient-failure" or "permanent-failure".
		field.String("status"))
)

// InvocationReadyForExportHandler accepts and processes ResultDB
// Invocation Ready for Export Pub/Sub messages.
type InvocationReadyForExportHandler struct {
}

// NewInvocationReadyForExportHandler initialises a new InvocationReadyForExportHandler.
func NewInvocationReadyForExportHandler() *InvocationReadyForExportHandler {
	return &InvocationReadyForExportHandler{}
}

func (h *InvocationReadyForExportHandler) Handle(ctx context.Context, message pubsub.Message, notification *rdbpb.InvocationReadyForExportNotification) error {
	status := "unknown"
	project := "unknown"
	defer func() {
		// Closure for late binding.
		invocationsReadyForExportCounter.Add(ctx, 1, project, status)
	}()

	project, _ = realms.Split(notification.RootInvocationRealm)

	// Throw result ingestion tasks onto a task queue rather than attempting
	// ingestion inline so that we can rate-limit outbound RPCs to ResultDB
	// and so that we can split the work up into multiple subtasks (one
	// per page).
	resultingester.Schedule(ctx, &taskspb.IngestTestResults{
		Notification: notification,
		PageToken:    "",
		TaskIndex:    1,
	})
	status = "success"
	return nil
}
