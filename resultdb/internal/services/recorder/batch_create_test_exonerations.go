// Copyright 2019 The LUCI Authors.
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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// verifyBatchCreateTestExonerationsPermissions verifies the caller has provided
// the update-token(s) sufficient to create the given test exonerations.
func verifyBatchCreateTestExonerationsPermissions(ctx context.Context, req *pb.BatchCreateTestExonerationsRequest) error {
	// Three cases are allowed:
	// 1. Parent is specified on the batch request. The child request, if it sets Parent, must set the same value.
	//    Invocation is not specified everywhere.
	// 2. Invocation is specified on the batch request. The child request, if it sets Invocation, must set the same value.
	//    Parent is not specified everywhere.
	// 3. Parent is specified on the child request only. Invocation is not specified anywhere.

	var state string
	if req.Parent != "" {
		// Case 1.
		wuID, err := workunits.ParseName(req.Parent)
		if err != nil {
			return appstatus.BadRequest(errors.Fmt("parent: %w", err))
		}
		if req.Invocation != "" {
			return appstatus.BadRequest(errors.New("invocation: must not be specified if parent is specified"))
		}
		state = workUnitUpdateTokenState(wuID)
	}
	if req.Invocation != "" {
		// Case 2. Go into the legacy validation path.
		return verifyBatchCreateTestExonerationsPermissionLegacy(ctx, req)
	}

	if err := pbutil.ValidateBatchRequestCountAndSize(req.Requests); err != nil {
		return appstatus.BadRequest(errors.Fmt("requests: %w", err))
	}

	var rootInvocationID rootinvocations.ID
	for i, r := range req.Requests {
		if r.Invocation != "" {
			if req.Parent != "" {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: invocation: must not be set if `parent` is set on top-level batch request", i))
			} else {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: invocation: may not be set at the request-level unless also set at the batch-level", i))
			}
		}
		if req.Parent != "" {
			// Case 1. Expect the `parent` field set on the child request, if set, to match that on the batch request.
			if err := emptyOrEqual("parent", r.Parent, req.Parent); err != nil {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: %w", i, err))
			}
		} else {
			// Case 3. Parent is set on a per-request basis.
			parentID, err := workunits.ParseName(r.Parent)
			if err != nil {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: parent: %w", i, err))
			}

			// Check all root invocations are the same. This can generate more helpful errors
			// than checking the token states are equal directly.
			if i == 0 {
				rootInvocationID = parentID.RootInvocationID
			} else if rootInvocationID != parentID.RootInvocationID {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: parent: all test exonerations in a batch must belong to the same root invocation; got %q, want %q", i, parentID.RootInvocationID.Name(), rootInvocationID.Name()))
			}

			s := workUnitUpdateTokenState(parentID)
			if i == 0 {
				state = s
			} else if state != s {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: parent: work unit %q requires a different update token to request[0]'s %q, but this RPC only accepts one update token", i, parentID.Name(), req.Requests[0].Parent))
			}
		}
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	if err := validateWorkUnitUpdateTokenForState(ctx, token, state); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	return nil
}

func verifyBatchCreateTestExonerationsPermissionLegacy(ctx context.Context, req *pb.BatchCreateTestExonerationsRequest) error {
	if err := pbutil.ValidateInvocationName(req.Invocation); err != nil {
		return appstatus.BadRequest(errors.Fmt("invocation: %w", err))
	}
	// TODO: Get rid of the requirement to support an empty requests collection if we can.
	if len(req.Requests) > 0 {
		if err := pbutil.ValidateBatchRequestCountAndSize(req.Requests); err != nil {
			return appstatus.BadRequest(errors.Fmt("requests: %w", err))
		}
	}
	for i, r := range req.Requests {
		if err := emptyOrEqual("invocation", r.Invocation, req.Invocation); err != nil {
			return appstatus.BadRequest(errors.Fmt("requests[%d]: %w", i, err))
		}
		if r.Parent != "" {
			return appstatus.BadRequest(errors.Fmt("requests[%d]: parent: must not be set if `invocation` is set on top-level batch request", i))
		}
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	id := invocations.MustParseName(req.Invocation)
	if err := validateInvocationToken(ctx, token, id); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	return nil
}

func validateBatchCreateTestExonerationsRequest(req *pb.BatchCreateTestExonerationsRequest, cfg *config.CompiledServiceConfig) error {
	// Parent, Invocation and Request length is already validated by
	// verifyBatchCreateTestExonerationsPermissions.

	// If we are operating on work units, enforce stricter validation.
	strictValidation := req.Invocation == ""

	if strictValidation && req.RequestId == "" {
		// Request ID is required to ensure requests are treated idempotently
		// in case of inevitable retries.
		return errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}

	for i, r := range req.Requests {
		if err := emptyOrEqual("request_id", r.RequestId, req.RequestId); err != nil {
			return errors.Fmt("requests[%d]: %w", i, err)
		}
		if err := validateTestExoneration(r.TestExoneration, cfg, strictValidation); err != nil {
			return errors.Fmt("requests[%d]: test_exoneration: %w", i, err)
		}
	}
	return nil
}

// BatchCreateTestExonerations implements pb.RecorderServer.
func (s *recorderServer) BatchCreateTestExonerations(ctx context.Context, in *pb.BatchCreateTestExonerationsRequest) (rsp *pb.BatchCreateTestExonerationsResponse, err error) {
	// Per AIP-211, perform authorisation checks before request validation.
	if err := verifyBatchCreateTestExonerationsPermissions(ctx, in); err != nil {
		return nil, err
	}

	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateBatchCreateTestExonerationsRequest(in, cfg); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Identify which case we are dealing with:
	// - One invocation ID for all request items
	// - One work unit ID for all request items
	// - A work unit ID per request item
	var invID invocations.ID
	var commonWorkUnitID workunits.ID
	var perRequestWorkUnitID bool
	if in.Invocation != "" {
		invID = invocations.MustParseName(in.Invocation)
	} else if in.Parent != "" {
		commonWorkUnitID = workunits.MustParseName(in.Parent)
	} else {
		perRequestWorkUnitID = true
	}

	ret := &pb.BatchCreateTestExonerationsResponse{
		TestExonerations: make([]*pb.TestExoneration, len(in.Requests)),
	}
	ms := make([]*spanner.Mutation, len(in.Requests))
	var workUnitIDs []workunits.ID
	for i, sub := range in.Requests {
		var workUnitID workunits.ID
		if commonWorkUnitID != (workunits.ID{}) {
			workUnitID = commonWorkUnitID
		} else if perRequestWorkUnitID {
			workUnitID = workunits.MustParseName(sub.Parent)
		}
		if workUnitID != (workunits.ID{}) {
			workUnitIDs = append(workUnitIDs, workUnitID)
		}
		ret.TestExonerations[i], ms[i] = insertTestExoneration(ctx, workUnitID, invID, in.RequestId, i, sub.TestExoneration)
	}

	if invID != "" {
		// Legacy request using invocations.
		_, err = mutateInvocation(ctx, invID, func(ctx context.Context) error {
			span.BufferWrite(ctx, ms...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		// Request using work units.
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			// Ensures all work units are active.
			states, err := workunits.ReadFinalizationStates(ctx, workUnitIDs)
			if err != nil {
				return err
			}
			for i, st := range states {
				if st != pb.WorkUnit_ACTIVE {
					return appstatus.Errorf(codes.FailedPrecondition, "requests[%d]: parent %q is not active", i, workUnitIDs[i].Name())
				}
			}

			span.BufferWrite(ctx, ms...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return ret, nil
}
