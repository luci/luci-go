// Copyright 2021 The LUCI Authors.
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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/testvariants"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func validateBatchGetTestVariantsRequest(in *pb.BatchGetTestVariantsRequest) error {
	if len(in.TestVariants) > 500 {
		return errors.New("a maximum of 500 test variants can be requested at once")
	}

	if err := pbutil.ValidateInvocationName(in.Invocation); err != nil {
		return errors.Fmt("invocation: %q: %w", in.Invocation, err)
	}

	for i, tvID := range in.TestVariants {
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
		} else if tvID.TestId != "" || tvID.VariantHash != "" {
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

	if err := testvariants.ValidateResultLimit(in.ResultLimit); err != nil {
		return errors.Fmt("result_limit: %w", err)
	}

	return nil
}

// BatchGetTestVariants implements the RPC method of the same name.
func (s *resultDBServer) BatchGetTestVariants(ctx context.Context, in *pb.BatchGetTestVariantsRequest) (*pb.BatchGetTestVariantsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := permissions.VerifyInvocationByName(ctx, in.Invocation, rdbperms.PermListTestResults, rdbperms.PermListTestExonerations); err != nil {
		return nil, err
	}

	// Caller has permissions rdbperms.PermListTestResults and
	// rdbperms.PermListTestExonerations, so they have unrestricted access
	accessLevel := testvariants.AccessLevelUnrestricted

	if err := validateBatchGetTestVariantsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	testIDs := make([]string, len(in.TestVariants))
	type key struct {
		TestID      string
		VariantHash string
	}
	variantIDs := make(map[key]struct{}, len(in.TestVariants))
	for i, tvID := range in.TestVariants {
		var testID string
		var variantHash string
		if tvID.TestIdStructured != nil {
			testID = pbutil.TestIDFromStructuredTestIdentifier(tvID.TestIdStructured)
			variantHash = pbutil.VariantHashFromStructuredTestIdentifier(tvID.TestIdStructured)
		} else {
			testID = tvID.TestId
			variantHash = tvID.VariantHash
		}

		variantIDs[key{TestID: testID, VariantHash: variantHash}] = struct{}{}
		testIDs[i] = testID
	}

	invs, err := graph.Reachable(ctx, invocations.NewIDSet(invocations.MustParseName(in.Invocation)))
	if err != nil {
		return nil, errors.Fmt("failed to fetch invocations: %w", err)
	}

	// Query test variants with an empty predicate and a list of test IDs,
	// which will match all variants with those IDs, regardless of status.
	q := testvariants.Query{
		ReachableInvocations: invs,
		TestIDs:              testIDs,
		Predicate:            &pb.TestVariantPredicate{},
		ResultLimit:          testvariants.AdjustResultLimit(in.ResultLimit),
		ResponseLimitBytes:   testvariants.DefaultResponseLimitBytes,
		AccessLevel:          accessLevel,
		// Prefer to sort by v2 verdict status instead of v1 status, to ensure
		// v2 verdict status is always correct. See comment on OrderBy.
		OrderBy: testvariants.SortOrderStatusV2Effective,
		// Number chosen fairly arbitrarily.
		PageSize:  1000,
		PageToken: "",
	}

	// TODO: This is broken with respect to https://google.aip.dev/231:
	// 1. AIP-231 says the order of results must be the same as
	//   the request and include one message for each item in the request,
	//   but this implementation does not guarantee correct ordering.
	// 2. AIP-231 requires this method must fail for all resources
	//   or succeed for all resources (no partial success). However, this method
	//   silently succeeds even if some resources do not exist.
	// 3. AIP-231 does not say whether requesting the same resource twice is
	//   allowed or not. By not ruling it out in request validation, this method
	//   has chosen to be permissive and allow it. But the implementation does
	//   not handle it (duplicate requests should lead to duplicate responses
	//   as per point 1).
	tvs := make([]*pb.TestVariant, 0, len(in.TestVariants))
	distinctSources := make(map[string]*pb.Sources)
	for len(tvs) < len(in.TestVariants) {
		page, err := q.Fetch(ctx)
		if err != nil {
			return nil, errors.Fmt("failed to fetch test variants: %w", err)
		}

		for _, tv := range page.TestVariants {
			if _, ok := variantIDs[key{TestID: tv.TestId, VariantHash: tv.VariantHash}]; ok {
				// Only return the test variants that were requested. (While we have
				// pre-filtered on the test, we did not pre-filter on the test variant.)
				tvs = append(tvs, tv)
				if tv.SourcesId != "" {
					distinctSources[tv.SourcesId] = page.DistinctSources[tv.SourcesId]
				}
			}
		}

		if page.NextPageToken == "" {
			break
		}

		q.PageToken = page.NextPageToken
	}

	return &pb.BatchGetTestVariantsResponse{
		TestVariants: tvs,
		Sources:      distinctSources,
	}, nil
}
