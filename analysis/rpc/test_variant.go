// Copyright 2022 The LUCI Authors.
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

package rpc

import (
	"context"
	"regexp"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

var variantHashRe = regexp.MustCompile("^[0-9a-f]{16}$")

// testVariantsServer implements pb.TestVariantServer.
type testVariantsServer struct {
}

// NewTestVariantsServer returns a new pb.TestVariantServer.
func NewTestVariantsServer() pb.TestVariantsServer {
	return &pb.DecoratedTestVariants{
		Prelude:  checkAllowedPrelude,
		Service:  &testVariantsServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// QueryFailureRate queries the failure rate of specified test variants,
// returning signals indicating if the test variant is flaky and/or
// deterministically failing.
func (*testVariantsServer) QueryFailureRate(ctx context.Context, req *pb.QueryTestVariantFailureRateRequest) (*pb.QueryTestVariantFailureRateResponse, error) {
	now := clock.Now(ctx)
	if err := validateQueryTestVariantFailureRateRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}

	opts := testresults.QueryFailureRateOptions{
		Project:      req.Project,
		TestVariants: req.TestVariants,
		AsAtTime:     now,
	}

	var err error
	// Query all subrealms the caller can see test results in.
	const subRealm = ""
	opts.SubRealms, err = perms.QuerySubRealmsNonEmpty(ctx, req.Project, subRealm, nil, rdbperms.PermListTestResults)
	if err != nil {
		return nil, err
	}

	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	response, err := testresults.QueryFailureRate(ctx, opts)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func validateQueryTestVariantFailureRateRequest(req *pb.QueryTestVariantFailureRateRequest) error {
	// MaxTestVariants is the maximum number of test variants to be queried in one request.
	const MaxTestVariants = 100

	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if len(req.TestVariants) == 0 {
		return errors.Reason("test_variants: unspecified").Err()
	}
	if len(req.TestVariants) > MaxTestVariants {
		return errors.Reason("test_variants: no more than %v may be queried at a time", MaxTestVariants).Err()
	}
	type testVariant struct {
		testID      string
		variantHash string
	}
	uniqueTestVariants := make(map[testVariant]struct{})
	for i, tv := range req.TestVariants {
		if tv.GetTestId() == "" {
			return errors.Reason("test_variants[%v]: test_id: unspecified", i).Err()
		}
		var variantHash string
		if tv.VariantHash != "" {
			if !variantHashRe.MatchString(tv.VariantHash) {
				return errors.Reason("test_variants[%v]: variant_hash: must match %s", i, variantHashRe).Err()
			}
			variantHash = tv.VariantHash
		}

		// Variant may be nil as not all tests have variants.
		if tv.Variant != nil || tv.VariantHash == "" {
			calculatedHash := pbutil.VariantHash(tv.Variant)
			if tv.VariantHash != "" && calculatedHash != tv.VariantHash {
				return errors.Reason("test_variants[%v]: variant and variant_hash mismatch, variant hashed to %s, expected %s", i, calculatedHash, tv.VariantHash).Err()
			}
			variantHash = calculatedHash
		}

		key := testVariant{testID: tv.TestId, variantHash: variantHash}
		if _, ok := uniqueTestVariants[key]; ok {
			return errors.Reason("test_variants[%v]: already requested in the same request", i).Err()
		}
		uniqueTestVariants[key] = struct{}{}
	}
	return nil
}

func (*testVariantsServer) QueryStability(ctx context.Context, req *pb.QueryTestVariantStabilityRequest) (*pb.QueryTestVariantStabilityResponse, error) {
	if err := validateQueryTestVariantStabilityRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}

	var err error
	// Query all subrealms the caller can see test results in.
	const subRealm = ""
	_, err = perms.QuerySubRealmsNonEmpty(ctx, req.Project, subRealm, nil, rdbperms.PermListTestResults)
	if err != nil {
		return nil, err
	}

	return nil, appstatus.Error(codes.Unimplemented, "not implemented")
}

func validateQueryTestVariantStabilityRequest(req *pb.QueryTestVariantStabilityRequest) error {
	// MaxTestVariants is the maximum number of test variants to be queried in one request.
	const MaxTestVariants = 100

	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if len(req.TestVariants) == 0 {
		return errors.Reason("test_variants: unspecified").Err()
	}
	if len(req.TestVariants) > MaxTestVariants {
		return errors.Reason("test_variants: no more than %v may be queried at a time", MaxTestVariants).Err()
	}
	seenTestVariants := make(map[testVariant]int)
	for i, tv := range req.TestVariants {
		if err := validateTestVariantPosition(tv, i, seenTestVariants); err != nil {
			return errors.Annotate(err, "test_variants[%v]", i).Err()
		}
	}
	return nil
}

type testVariant struct {
	testID      string
	variantHash string
}

// validateTestVariantPosition validates the given test variant position at the given offset
// in the request. seenTestVariants is used to track the previously seen test variants
// and their offsets so that duplicates can be identified.
func validateTestVariantPosition(tv *pb.QueryTestVariantStabilityRequest_TestVariantPosition, offset int, seenTestVariants map[testVariant]int) error {
	if tv.GetTestId() == "" {
		return errors.Reason("test_id: unspecified").Err()
	}
	var variantHash string
	if tv.VariantHash != "" {
		if !variantHashRe.MatchString(tv.VariantHash) {
			return errors.Reason("variant_hash: must match %s", variantHashRe).Err()
		}
		variantHash = tv.VariantHash
	}

	// This RPC allows the Variant or VariantHash (or both)
	// to be set to specify the variant. If both are specified,
	// they must be consistent.
	if tv.Variant != nil {
		calculatedHash := pbutil.VariantHash(tv.Variant)
		if tv.VariantHash != "" && calculatedHash != tv.VariantHash {
			return errors.Reason("variant and variant_hash mismatch, variant hashed to %s, expected %s", calculatedHash, tv.VariantHash).Err()
		}
		variantHash = calculatedHash
	}
	// It is possible neither the VariantHash nor Variant is set.
	// In this case, we interpret the request as being for the
	// nil Variant (which is a valid variant).
	if tv.VariantHash == "" && tv.Variant == nil {
		variantHash = pbutil.VariantHash(nil)
	}

	// Each test variant may appear in the request only once.
	key := testVariant{testID: tv.TestId, variantHash: variantHash}
	if previousOffset, ok := seenTestVariants[key]; ok {
		return errors.Reason("same test variant already requested at index %v", previousOffset).Err()
	}
	seenTestVariants[key] = offset

	if err := pbutil.ValidateSources(tv.Sources); err != nil {
		return errors.Annotate(err, "sources").Err()
	}
	return nil
}
