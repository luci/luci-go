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

package recorder

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// FinalizeWorkUnitDescendants implements pb.RecorderServer.
func (s *recorderServer) FinalizeWorkUnitDescendants(ctx context.Context, in *pb.FinalizeWorkUnitDescendantsRequest) (*emptypb.Empty, error) {
	// As per google.aip.dev/211, authorize request before validating it.
	if err := verifyFinalizeWorkUnitDescendantsPermissions(ctx, in); err != nil {
		return nil, err
	}
	if err := validateFinalizeWorkUnitDescendantsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Work unit ID is already validated in validateFinalizeWorkUnitDescendantsRequest.
	workUnitID := workunits.MustParseName(in.Name)

	// Read all descendants.
	// Use a single-use read-only transaction for reading descendants.
	// This avoids long-running transactions.
	var descendantIDs []workunits.ID
	descendantIDs, err := workunits.ReadPrefixedDescendants(span.Single(ctx), workUnitID)
	if err != nil {
		return nil, errors.Fmt("read descendants: %w", err)
	}

	// If no descendants, we are done.
	if len(descendantIDs) == 0 {
		return &emptypb.Empty{}, nil
	}

	// Process in batches to remain within Spanner mutation and request size limits:
	// https://docs.cloud.google.com/spanner/quotas#limits-for
	// At the maximum summary markdown size of 4KB, 1,000 would produce a commit size
	// around 4 MB. At around 10 cells updated per work unit, this would produce
	// around 10,000 mutations per commit. Both are well within limits.
	batchSize := 1_000
	for i := 0; i < len(descendantIDs); i += batchSize {
		end := i + batchSize
		if end > len(descendantIDs) {
			end = len(descendantIDs)
		}
		batch := descendantIDs[i:end]

		if err := finalizeBatch(ctx, workUnitID.RootInvocationID, batch, in.State, in.SummaryMarkdown); err != nil {
			return nil, errors.Fmt("finalizeBatch (%v to %v) failed: %w", i, end, err)
		}
	}

	return &emptypb.Empty{}, nil
}

func finalizeBatch(ctx context.Context, invID rootinvocations.ID, ids []workunits.ID, state pb.WorkUnit_State, summary string) error {
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Read the work units to check their current state.
		// We need to check if they are already finalized to ensure idempotency.
		finalizationStates, err := workunits.ReadFinalizationStates(ctx, ids)
		if err != nil {
			return err
		}

		hasWorkUnitFinalizing := false
		for i, id := range ids {
			finalizationState := finalizationStates[i]
			if finalizationState != pb.WorkUnit_ACTIVE {
				// Finalization already started. Do not start finalization
				// again as doing so would overwrite the existing FinalizeStartTime,
				// state, summary markdown, and create an unnecessary task.
				continue
			}

			mb := workunits.NewMutationBuilder(id)
			// As the state is a terminal state, this will also transition the
			// work unit to FINALIZING.
			mb.UpdateState(state)
			mb.UpdateSummaryMarkdown(pbutil.TruncateSummaryMarkdown(summary))
			span.BufferWrite(ctx, mb.Build()...)

			hasWorkUnitFinalizing = true
		}

		if hasWorkUnitFinalizing {
			// Transactionally schedule work unit finalization task for the root invocation.
			if err := tasks.ScheduleWorkUnitsFinalization(ctx, invID); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func verifyFinalizeWorkUnitDescendantsPermissions(ctx context.Context, req *pb.FinalizeWorkUnitDescendantsRequest) error {
	workUnitID, err := workunits.ParseName(req.Name)
	if err != nil {
		return appstatus.BadRequest(errors.Fmt("name: %w", err))
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	if err := validateWorkUnitUpdateToken(ctx, token, workUnitID); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	return nil
}

func validateFinalizeWorkUnitDescendantsRequest(req *pb.FinalizeWorkUnitDescendantsRequest) error {
	// Validate the work unit ID.
	_, workUnitID, err := pbutil.ParseWorkUnitName(req.Name)
	if err != nil {
		return errors.Fmt("name: %w", err)
	}

	// State is a required field.
	if err := pbutil.ValidateWorkUnitState(req.State); err != nil {
		return errors.Fmt("state: %w", err)
	}
	if !pbutil.IsFinalWorkUnitState(req.State) {
		return errors.New("state: must be a terminal state")
	}

	// We do not enforce length limits via the FinalizeWorkUnit RPCs.
	// While clients should truncate on their side to avoid request size errors,
	// we handle truncation silently here as a fallback. It is foreseeable
	// that clients have implementation bugs and we'd rather have the error
	// to show users than reject the request.
	const enforceLength = false
	if err := pbutil.ValidateSummaryMarkdown(req.SummaryMarkdown, enforceLength); err != nil {
		return errors.Fmt("summary_markdown: %w", err)
	}

	if err := validateFinalizationScope(req.FinalizationScope); err != nil {
		return errors.Fmt("finalization_scope: %w", err)
	}
	_, ok := workUnitIDPrefix(workUnitID)
	if ok {
		return errors.Fmt("name: %q is a prefixed work unit ID, expected non-prefixed work unit ID", workUnitID)
	}
	return nil
}

// validateFinalizationScope validates the finalization scope is a valid value.
func validateFinalizationScope(scope pb.FinalizeWorkUnitDescendantsRequest_FinalizationScope) error {
	if scope == pb.FinalizeWorkUnitDescendantsRequest_FINALIZATION_SCOPE_UNSPECIFIED {
		return validate.Unspecified()
	}
	if _, ok := pb.FinalizeWorkUnitDescendantsRequest_FinalizationScope_name[int32(scope)]; !ok {
		return errors.Fmt("unknown finalization scope %v", scope)
	}
	return nil
}
