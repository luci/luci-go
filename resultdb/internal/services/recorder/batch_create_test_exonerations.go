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
	"go.chromium.org/luci/resultdb/internal/testexonerationsv2"
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

	ret := &pb.BatchCreateTestExonerationsResponse{
		TestExonerations: make([]*pb.TestExoneration, len(in.Requests)),
	}
	if in.Invocation != "" {
		// Legacy request using invocations.
		invID := invocations.MustParseName(in.Invocation)
		ms := make([]*spanner.Mutation, 0, len(in.Requests))
		for i, sub := range in.Requests {
			te := normalizeTestExoneration(ctx, workunits.ID{}, invID, in.RequestId, i, sub.TestExoneration)
			ret.TestExonerations[i] = te

			// Insert the legacy test exoneration record.
			ms = append(ms, insertTestExoneration(invID, te))
		}

		_, err = mutateInvocation(ctx, invID, func(ctx context.Context) error {
			span.BufferWrite(ctx, ms...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		var commonWorkUnitID workunits.ID
		if in.Parent != "" {
			commonWorkUnitID = workunits.MustParseName(in.Parent)
		}
		var parentIDs []workunits.ID
		for _, sub := range in.Requests {
			var workUnitID workunits.ID
			if commonWorkUnitID != (workunits.ID{}) {
				workUnitID = commonWorkUnitID
			} else { // Per request ID is set.
				workUnitID = workunits.MustParseName(sub.Parent)
			}
			parentIDs = append(parentIDs, workUnitID)
		}

		// Read the realms of the work units. This can happen outside the main
		// Read/Write transaction as they are immutable.
		realms, err := workunits.ReadRealms(span.Single(ctx), parentIDs)
		if err != nil {
			// NotFound appstatus error or internal error.
			return nil, err
		}

		// Read the test sharding algorithm version from one of the root invocation's shards.
		// We read from a shard instead of the root invocation directly to avoid hotspotting
		// the root invocation record.
		// This read is safe to occur outside the main R/W transaction as the field is immutable.
		testShardingInformation, err := rootinvocations.ReadTestShardingInformationFromShard(span.Single(ctx), parentIDs[0].RootInvocationShardID())
		if err != nil {
			// NotFound appstatus error or internal error.
			return nil, err
		}

		ms := make([]*spanner.Mutation, 0, len(in.Requests)*2)
		for i, sub := range in.Requests {
			parentID := parentIDs[i]
			te := normalizeTestExoneration(ctx, parentID, "", in.RequestId, i, sub.TestExoneration)
			ret.TestExonerations[i] = te

			// Insert the legacy test exoneration record.
			ms = append(ms, insertTestExoneration(parentID.LegacyInvocationID(), te))

			// Create it in the v2 table too.
			teV2, err := prepareTestExonerationV2(parentID, te, realms[parentID], testShardingInformation.Algorithm)
			if err != nil {
				return nil, err
			}
			ms = append(ms, testexonerationsv2.Create(teV2))
		}

		// Request using work units.
		_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			// Ensures all work units are active.
			states, err := workunits.ReadFinalizationStates(ctx, parentIDs)
			if err != nil {
				return err
			}
			for i, st := range states {
				if st != pb.WorkUnit_ACTIVE {
					return appstatus.Errorf(codes.FailedPrecondition, "requests[%d]: parent %q is not active", i, parentIDs[i].Name())
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

func prepareTestExonerationV2(parentID workunits.ID, te *pb.TestExoneration, workUnitRealm string, algID rootinvocations.TestShardingAlgorithmID) (*testexonerationsv2.TestExonerationRow, error) {
	algImpl, err := rootinvocations.ShardingAlgorithmByID(algID)
	if err != nil {
		return nil, err
	}
	shardIndex := algImpl.ShardTestID(parentID.RootInvocationID, te.TestIdStructured)

	row := &testexonerationsv2.TestExonerationRow{
		ID: testexonerationsv2.ID{
			RootInvocationShardID: rootinvocations.ShardID{
				RootInvocationID: parentID.RootInvocationID,
				ShardIndex:       shardIndex,
			},
			ModuleName:        te.TestIdStructured.ModuleName,
			ModuleScheme:      te.TestIdStructured.ModuleScheme,
			ModuleVariantHash: te.TestIdStructured.ModuleVariantHash,
			CoarseName:        te.TestIdStructured.CoarseName,
			FineName:          te.TestIdStructured.FineName,
			CaseName:          te.TestIdStructured.CaseName,
			WorkUnitID:        parentID.WorkUnitID,
			ExonerationID:     te.ExonerationId,
		},
		ModuleVariant:   te.TestIdStructured.ModuleVariant,
		Realm:           workUnitRealm,
		ExplanationHTML: te.ExplanationHtml,
		Reason:          te.Reason,
	}
	return row, nil
}
