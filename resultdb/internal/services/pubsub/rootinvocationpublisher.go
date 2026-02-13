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
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/tracing"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// rootInvocationPublisher is a helper struct for publishing root invocations.
type rootInvocationPublisher struct {
	// task is the task payload.
	task *taskspb.PublishRootInvocationTask

	// resultDBHostname is the hostname of the ResultDB service.
	resultDBHostname string
}

// handleRootInvocationPublisher handles the root invocation publisher task.
func (p *rootInvocationPublisher) handleRootInvocationPublisher(ctx context.Context) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.handleRootInvocationPublisher")
	defer func() { tracing.End(s, err) }()

	task := p.task
	rootInvID := rootinvocations.ID(task.RootInvocationId)

	// 1. Reads Root Invocation to check its state.
	rootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
	if err != nil {
		return errors.Fmt("read root invocation %q: %w", rootInvID.Name(), err)
	}

	// 2. Checks if it is finalized.
	if rootInv.FinalizationState != pb.RootInvocation_FINALIZED {
		return tq.Fatal.Apply(errors.Fmt("root invocation %q is not finalized", rootInvID.Name()))
	}

	cfg, err := config.Service(ctx)
	if err != nil {
		return errors.Fmt("read service config: %w", err)
	}

	// 3. Construct the notification.
	notification := &pb.RootInvocationFinalizedNotification{
		RootInvocation: masking.RootInvocation(rootInv, cfg),
		ResultdbHost:   p.resultDBHostname,
	}

	// 4. Publish notification.
	tasks.NotifyRootInvocationFinalized(ctx, notification)
	return nil
}
