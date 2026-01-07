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

package pubsub

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/tracing"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// testAggregationsPublisher is a helper struct for publishing test aggregations.
type testAggregationsPublisher struct {
	// task is the task payload.
	task *taskspb.PublishTestAggregationsTask

	// resultDBHostname is the hostname of the ResultDB service.
	resultDBHostname string
}

// handleTestAggregationsPublisher handles the test aggregation publisher task.
func (p *testAggregationsPublisher) handleTestAggregationsPublisher(ctx context.Context) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.handleTestAggregationsPublisher")
	defer func() { tracing.End(s, err) }()

	task := p.task
	rootInvID := rootinvocations.ID(task.RootInvocationId)

	// 1. Reads Root Invocation to check its state.
	rootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
	if err != nil {
		return errors.Fmt("read root invocation %q: %w", rootInvID.Name(), err)
	}

	// 2. Checks StreamingExportState: Only publish if metadata is final.
	if rootInv.StreamingExportState != pb.RootInvocation_METADATA_FINAL {
		logging.Infof(ctx, "Root invocation %q is not ready for streaming export, skipping test aggregation notification", rootInvID.Name())
		return nil
	}

	// 3. Construct the notification.
	// TODO: b/454120520 - Implement test aggregation logic.
	notification := &pb.TestAggregationsNotification{
		RootInvocation: rootInvID.Name(),
		ResultdbHost:   p.resultDBHostname,
	}

	// 4. Generate attributes.
	attrs := generateAttributes(rootInv)

	// 5. Publish notification.
	tasks.NotifyTestAggregations(ctx, notification, attrs)
	return nil
}
