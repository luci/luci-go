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

package resultdb

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/internal/testverdictsv2"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// BatchGetTestVerdicts implements pb.ResultDBServer.
func (s *resultDBServer) BatchGetTestVerdicts(ctx context.Context, req *pb.BatchGetTestVerdictsRequest) (*pb.BatchGetTestVerdictsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// AIP-211: Perform authorization checks before validating the request.
	access, err := verifyBatchGetTestVerdictsAccess(ctx, req)
	if err != nil {
		// Already returns appropriate appstatus-annotated error.
		return nil, err
	}

	if err := validateBatchGetTestVerdictsRequest(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	rootInvID := rootinvocations.MustParseName(req.Parent)

	// Identify the sharding algorithm in use by the root invocation.
	// We can use any shard to find this information.
	shardID := rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: 0}
	shardingInfo, err := rootinvocations.ReadTestShardingInformationFromShard(ctx, shardID)
	if err != nil {
		return nil, err
	}
	alg, err := rootinvocations.ShardingAlgorithmByID(shardingInfo.Algorithm)
	if err != nil {
		return nil, err
	}

	// Derive the verdict IDs.
	testIDs := make([]*pb.TestIdentifier, 0, len(req.Tests))
	verdictIDs := make([]testresultsv2.VerdictID, 0, len(req.Tests))
	for i, tv := range req.Tests {
		var testID *pb.TestIdentifier
		if tv.TestIdStructured != nil {
			// Structured test ID.
			testID = tv.TestIdStructured

			// We allow either the ModuleVariant or ModuleVariantHash to be set.
			// If the ModuleVariantHash isn't set, compute it from the ModuleVariant.
			if testID.ModuleVariantHash == "" {
				testID = proto.Clone(testID).(*pb.TestIdentifier)
				pbutil.PopulateStructuredTestIdentifierHashes(testID)
			}
		} else {
			// Flat test ID. Parse it into a structured ID.
			base, err := pbutil.ParseAndValidateTestID(tv.TestId)
			if err != nil {
				// This should not happen if validateBatchGetTestVerdictsRequest is correct,
				// but check just in case.
				return nil, appstatus.BadRequest(errors.Fmt("test_variants[%d]: test_id: %w", i, err))
			}
			testID = &pb.TestIdentifier{
				ModuleName:        base.ModuleName,
				ModuleScheme:      base.ModuleScheme,
				ModuleVariantHash: tv.VariantHash,
				CoarseName:        base.CoarseName,
				FineName:          base.FineName,
				CaseName:          base.CaseName,
			}
		}
		testIDs = append(testIDs, testID)

		// Identify the shard.
		shardIndex := alg.ShardTestID(rootInvID, testID)

		// Construct the verdict ID.
		verdictIDs = append(verdictIDs, testresultsv2.VerdictID{
			RootInvocationShardID: rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: shardIndex},
			ModuleName:            testID.ModuleName,
			ModuleScheme:          testID.ModuleScheme,
			ModuleVariantHash:     testID.ModuleVariantHash,
			CoarseName:            testID.CoarseName,
			FineName:              testID.FineName,
			CaseName:              testID.CaseName,
		})
	}

	view := req.View
	if view == pb.TestVerdictView_TEST_VERDICT_VIEW_UNSPECIFIED {
		// Default to the basic view.
		view = pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC
	}

	// Fetch verdicts.
	q := &testverdictsv2.IteratorQuery{
		RootInvocationID: rootInvID,
		VerdictIDs:       verdictIDs,
		Access:           access,
		View:             req.View,
	}

	// Fetch all verdicts.
	fetchOpts := testverdictsv2.IteratorFetchOptions{
		FetchOptions: testverdictsv2.FetchOptions{
			// Try to fetch everything in one page.
			PageSize:           len(verdictIDs),
			ResponseLimitBytes: 0, // Do not apply a separate total response limit or test result limit.
			TotalResultLimit:   0,
			// Rely on the maximum number of verdicts supported by the RPC and
			// the per-verdict limits.
			VerdictResultLimit: testverdictsv2.StandardVerdictResultLimit,
			VerdictSizeLimit:   testverdictsv2.StandardVerdictSizeLimit,
		},
	}

	verdicts, _, err := q.Fetch(ctx, testverdictsv2.PageToken{}, fetchOpts)
	if err != nil {
		return nil, err
	}

	if len(verdicts) != len(req.Tests) {
		// This should never happen, this is a guarantee of the query
		// library we are using.
		return nil, errors.Fmt("implementation invariant violated: requested %v verdicts, got %v verdicts", len(req.Tests), len(verdicts))
	}
	for i, v := range verdicts {
		if v == nil {
			testID := pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(testIDs[i]))

			// Insert placeholder verdict for not found verdicts.
			verdicts[i] = &pb.TestVerdict{
				TestIdStructured: testIDs[i],
				TestId:           testID,
			}
		}
	}

	return &pb.BatchGetTestVerdictsResponse{
		TestVerdicts: verdicts,
	}, nil
}

func verifyBatchGetTestVerdictsAccess(ctx context.Context, req *pb.BatchGetTestVerdictsRequest) (permissions.RootInvocationAccess, error) {
	rootInvID, err := rootinvocations.ParseName(req.Parent)
	if err != nil {
		return permissions.RootInvocationAccess{}, appstatus.BadRequest(errors.Fmt("parent: %w", err))
	}
	result, err := permissions.VerifyAllWorkUnitsAccess(ctx, rootInvID, permissions.ListVerdictsAccessModel, permissions.LimitedAccess)
	if err != nil {
		return permissions.RootInvocationAccess{}, err
	}
	return result, nil
}

func validateBatchGetTestVerdictsRequest(req *pb.BatchGetTestVerdictsRequest) error {
	// req.Parent is already validated by verifyBatchGetTestVerdictsAccess.

	limit := 500
	if req.View == pb.TestVerdictView_TEST_VERDICT_VIEW_FULL {
		limit = 50
	}

	if len(req.Tests) == 0 {
		return errors.Fmt("test_variants: unspecified")
	}

	if len(req.Tests) > limit {
		return errors.Fmt("test_variants: a maximum of %d test variants can be requested at once", limit)
	}

	for i, tvID := range req.Tests {
		if tvID.TestIdStructured != nil {
			if err := pbutil.ValidateStructuredTestIdentifierForQuery(tvID.TestIdStructured); err != nil {
				return errors.Fmt("test_variants[%v]: test_id_structured: %w", i, err)
			}
			if tvID.TestId != "" {
				return errors.Fmt("test_variants[%v]: test_id: may not be set at same time as test_id_structured", i)
			}
			if tvID.VariantHash != "" {
				return errors.Fmt("test_variants[%v]: variant_hash: may not be set at same time as test_id_structured", i)
			}
		} else if tvID.TestId != "" {
			// Flat test ID.
			if err := pbutil.ValidateTestID(tvID.TestId); err != nil {
				return errors.Fmt("test_variants[%v]: test_id: %w", i, err)
			}
			if err := pbutil.ValidateVariantHash(tvID.VariantHash); err != nil {
				return errors.Fmt("test_variants[%v]: variant_hash: %w", i, err)
			}
		} else {
			return errors.Fmt("test_variants[%v]: either test_id_structured or (test_id and variant_hash) must be set", i)
		}
	}

	if req.View != pb.TestVerdictView_TEST_VERDICT_VIEW_UNSPECIFIED {
		if err := pbutil.ValidateTestVerdictView(req.View); err != nil {
			return errors.Fmt("view: %w", err)
		}
	}
	return nil
}
