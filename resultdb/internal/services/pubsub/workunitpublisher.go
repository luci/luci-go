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

package pubsub

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// workUnitPublisher is a helper struct for publishing work units.
type workUnitPublisher struct {
	// task is the task payload.
	task *taskspb.PublishWorkUnitsTask

	// resultDBHostname is the hostname of the ResultDB service.
	resultDBHostname string
}

// handleWorkUnitPublisher handles the work unit publisher task.
func (p *workUnitPublisher) handleWorkUnitPublisher(ctx context.Context) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.handleWorkUnitPublisher")
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
		logging.Infof(ctx, "Root invocation %q is not ready for streaming export, skipping work unit notification", rootInvID.Name())
		return nil
	}

	// 3. Construct the notification.
	notification, err := p.workUnitsNotification(ctx, rootInvID, task.WorkUnitIds)
	if err != nil {
		return err
	}

	// 4. Generate attributes.
	attrs := generateAttributes(rootInv)

	// 5. Publish notification.
	tasks.NotifyWorkUnits(ctx, notification, attrs)
	return nil
}

// workUnitsNotification constructs a WorkUnitsNotification message for the
// given work units.
// It checks if each work unit has artifacts and populates the HasArtifacts
// field accordingly.
func (p *workUnitPublisher) workUnitsNotification(ctx context.Context, rootInvID rootinvocations.ID, workUnitIDs []string) (*pb.WorkUnitsNotification, error) {
	// Collect all invocation IDs to check.
	invIDs := make(invocations.IDSet, len(workUnitIDs))
	for _, wuID := range workUnitIDs {
		wuInvID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: wuID}.LegacyInvocationID()
		invIDs.Add(wuInvID)
	}

	// Batch check for artifacts.
	hasArtifactsSet, err := artifacts.FilterHasArtifacts(span.Single(ctx), invIDs)
	if err != nil {
		return nil, errors.Fmt("check artifacts for work units: %w", err)
	}

	workUnits := make([]*pb.WorkUnitsNotification_WorkUnitDetails, len(workUnitIDs))
	for i, wuID := range workUnitIDs {
		wuInvID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: wuID}.LegacyInvocationID()
		workUnits[i] = &pb.WorkUnitsNotification_WorkUnitDetails{
			WorkUnitName: pbutil.WorkUnitName(string(rootInvID), wuID),
			HasArtifacts: hasArtifactsSet.Has(wuInvID),
		}
	}

	return &pb.WorkUnitsNotification{
		WorkUnits:    workUnits,
		ResultdbHost: p.resultDBHostname,
	}, nil
}
