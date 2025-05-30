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
	"encoding/hex"
	"regexp"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/stability"
	"go.chromium.org/luci/analysis/pbutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
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
		return errors.Fmt("project: %w", err)
	}
	if len(req.TestVariants) == 0 {
		return errors.New("test_variants: unspecified")
	}
	if len(req.TestVariants) > MaxTestVariants {
		return errors.Fmt("test_variants: no more than %v may be queried at a time", MaxTestVariants)
	}
	type testVariant struct {
		testID      string
		variantHash string
	}
	uniqueTestVariants := make(map[testVariant]struct{})
	for i, tv := range req.TestVariants {
		if tv.GetTestId() == "" {
			return errors.Fmt("test_variants[%v]: test_id: unspecified", i)
		}
		var variantHash string
		if tv.VariantHash != "" {
			if !variantHashRe.MatchString(tv.VariantHash) {
				return errors.Fmt("test_variants[%v]: variant_hash: must match %s", i, variantHashRe)
			}
			variantHash = tv.VariantHash
		}

		// Variant may be nil as not all tests have variants.
		if tv.Variant != nil || tv.VariantHash == "" {
			calculatedHash := pbutil.VariantHash(tv.Variant)
			if tv.VariantHash != "" && calculatedHash != tv.VariantHash {
				return errors.Fmt("test_variants[%v]: variant and variant_hash mismatch, variant hashed to %s, expected %s", i, calculatedHash, tv.VariantHash)
			}
			variantHash = calculatedHash
		}

		key := testVariant{testID: tv.TestId, variantHash: variantHash}
		if _, ok := uniqueTestVariants[key]; ok {
			return errors.Fmt("test_variants[%v]: already requested in the same request", i)
		}
		uniqueTestVariants[key] = struct{}{}
	}
	return nil
}

func (*testVariantsServer) QueryStability(ctx context.Context, req *pb.QueryTestVariantStabilityRequest) (*pb.QueryTestVariantStabilityResponse, error) {
	if err := validateQueryTestVariantStabilityRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}

	if err := perms.VerifyProjectPermissions(ctx, req.Project, perms.PermGetConfig); err != nil {
		return nil, err
	}

	// Fetch a recent project configuration.
	// (May be a recent value that was cached.)
	cfg, err := readProjectConfig(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	criteria, err := fromTestStabilityCriteriaConfig(cfg.Config)
	if err != nil {
		return nil, err
	}

	// Query all subrealms the caller can see test results in.
	const subRealm = ""
	subRealms, err := perms.QuerySubRealmsNonEmpty(ctx, req.Project, subRealm, nil, rdbperms.PermListTestResults)
	if err != nil {
		return nil, err
	}

	opts := stability.QueryStabilityOptions{
		Project:              req.Project,
		SubRealms:            subRealms,
		TestVariantPositions: req.TestVariants,
		Criteria:             criteria,
		AsAtTime:             clock.Now(ctx),
	}

	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	stabilityAnalysis, err := stability.QueryStability(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &pb.QueryTestVariantStabilityResponse{
		TestVariants: stabilityAnalysis,
		Criteria:     criteria,
	}, nil
}

func fromTestStabilityCriteriaConfig(cfg *configpb.ProjectConfig) (*pb.TestStabilityCriteria, error) {
	if cfg.TestStabilityCriteria == nil {
		return nil, failedPreconditionError(errors.New("project has not defined test stability criteria; set test_stability_criteria in project configuration and try again"))
	}

	criteria := cfg.TestStabilityCriteria

	// We have test stability criteria. We can rely on project configuration
	// validation to ensure mandatory fields have been set.
	return &pb.TestStabilityCriteria{
		FailureRate: &pb.TestStabilityCriteria_FailureRateCriteria{
			FailureThreshold:            criteria.FailureRate.FailureThreshold,
			ConsecutiveFailureThreshold: criteria.FailureRate.ConsecutiveFailureThreshold,
		},
		FlakeRate: &pb.TestStabilityCriteria_FlakeRateCriteria{
			MinWindow:          criteria.FlakeRate.MinWindow,
			FlakeThreshold:     criteria.FlakeRate.FlakeThreshold,
			FlakeRateThreshold: criteria.FlakeRate.FlakeRateThreshold,
			FlakeThreshold_1Wd: criteria.FlakeRate.FlakeThreshold_1Wd,
		},
	}, nil
}

func validateQueryTestVariantStabilityRequest(req *pb.QueryTestVariantStabilityRequest) error {
	// MaxTestVariants is the maximum number of test variants to be queried in one request.
	const MaxTestVariants = 100

	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if len(req.TestVariants) == 0 {
		return errors.New("test_variants: unspecified")
	}
	if len(req.TestVariants) > MaxTestVariants {
		return errors.Fmt("test_variants: no more than %v may be queried at a time", MaxTestVariants)
	}
	seenTestVariantBranches := make(map[testVariantBranch]int)
	for i, tv := range req.TestVariants {
		if err := validateTestVariantPosition(tv, i, seenTestVariantBranches); err != nil {
			return errors.Fmt("test_variants[%v]: %w", i, err)
		}
	}
	return nil
}

type testVariantBranch struct {
	testID        string
	variantHash   string
	sourceRefHash string
}

// validateTestVariantPosition validates the given test variant position at the given offset
// in the request. seenTestVariants is used to track the previously seen test variants
// and their offsets so that duplicates can be identified.
func validateTestVariantPosition(tv *pb.QueryTestVariantStabilityRequest_TestVariantPosition, offset int, seenTestVariantBranches map[testVariantBranch]int) error {
	if tv.GetTestId() == "" {
		return errors.New("test_id: unspecified")
	}
	var variantHash string
	if tv.VariantHash != "" {
		if !variantHashRe.MatchString(tv.VariantHash) {
			return errors.Fmt("variant_hash: must match %s", variantHashRe)
		}
		variantHash = tv.VariantHash
	}

	// This RPC allows the Variant or VariantHash (or both)
	// to be set to specify the variant. If both are specified,
	// they must be consistent.
	if tv.Variant != nil {
		calculatedHash := pbutil.VariantHash(tv.Variant)
		if tv.VariantHash != "" && calculatedHash != tv.VariantHash {
			return errors.Fmt("variant and variant_hash mismatch, variant hashed to %s, expected %s", calculatedHash, tv.VariantHash)
		}
		variantHash = calculatedHash
	}
	// It is possible neither the VariantHash nor Variant is set.
	// In this case, we interpret the request as being for the
	// nil Variant (which is a valid variant).
	if tv.VariantHash == "" && tv.Variant == nil {
		variantHash = pbutil.VariantHash(nil)
	}

	if err := pbutil.ValidateSources(tv.Sources); err != nil {
		return errors.Fmt("sources: %w", err)
	}

	sourceRefHash := hex.EncodeToString(pbutil.SourceRefHash(pbutil.SourceRefFromSources(tv.Sources)))

	// Each test variant branch may appear in the request only once.
	key := testVariantBranch{testID: tv.TestId, variantHash: variantHash, sourceRefHash: sourceRefHash}
	if previousOffset, ok := seenTestVariantBranches[key]; ok {
		return errors.Fmt("same test variant branch already requested at index %v", previousOffset)
	}
	seenTestVariantBranches[key] = offset

	return nil
}
