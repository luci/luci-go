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

package testhistory

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testresults/lowlatency"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/analysis/rpc"
)

const (
	// defaultQueryRecentPassesLimit is the default number of passing results to return.
	defaultQueryRecentPassesLimit = 5
	// maxQueryRecentPassesLimit is the maximum number of passing results to return.
	maxQueryRecentPassesLimit = 25
)

// QueryRecentPasses queries for recent passing results of a specific test variant.
func (s *testHistoryServer) QueryRecentPasses(ctx context.Context, req *pb.QueryRecentPassesRequest) (*pb.QueryRecentPassesResponse, error) {
	if err := validateQueryRecentPassesRequest(req); err != nil {
		return nil, rpc.InvalidArgumentError(err)
	}

	subRealms, err := perms.QuerySubRealmsNonEmpty(ctx, req.Project, "" /* query all realms*/, nil, rdbperms.PermListTestResults)
	if err != nil {
		return nil, err
	}
	if len(subRealms) == 0 {
		return nil, appstatus.Errorf(codes.PermissionDenied, "caller does not have permission %s in any realm in project %q", rdbperms.PermListTestResults, req.Project)
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = defaultQueryRecentPassesLimit
	} else if limit > maxQueryRecentPassesLimit {
		limit = maxQueryRecentPassesLimit
	}

	spanCtx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Strategy A: Source Position Lookup
	srcRef := pbutil.SourceRefFromSources(req.Sources)
	srcRefHash := pbutil.SourceRefHash(srcRef)
	maxPosition := pbutil.SourcePosition(req.Sources)

	opts := lowlatency.ReadPassingTestResultsBySourceOptions{
		Project:           req.Project,
		SubRealms:         subRealms,
		TestID:            req.TestId,
		VariantHash:       req.VariantHash,
		SourceRefHash:     srcRefHash,
		MaxSourcePosition: maxPosition,
		Limit:             limit,
	}

	ids, err := lowlatency.ReadPassingResultsBySource(spanCtx, opts)
	if err != nil {
		return nil, fmt.Errorf("reading passing invocations by source: %w", err)
	}
	logging.Debugf(ctx, "Found %d passing invocations via source position in %d realms", len(ids), len(subRealms))

	results := make([]*pb.QueryRecentPassesResponse_PassingResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, &pb.QueryRecentPassesResponse_PassingResult{
			Name: id,
		})
	}

	return &pb.QueryRecentPassesResponse{
		PassingResults: results,
	}, nil
}

func validateQueryRecentPassesRequest(req *pb.QueryRecentPassesRequest) error {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := rdbpbutil.ValidateTestID(req.TestId); err != nil {
		return errors.Fmt("test_id: %w", err)
	}
	if err := pbutil.ValidateVariantHash(req.VariantHash); err != nil {
		return errors.Fmt("variant_hash: %w", err)
	}
	if req.Limit < 0 {
		return errors.New("limit: must be non-negative")
	}

	// Verify sources ourselves as pbutil.ValidateSources is more restrictive than what we actually need for this RPC.
	if req.Sources == nil {
		return errors.New("sources: required")
	}
	switch src := req.Sources.GetBaseSources().(type) {
	case *pb.Sources_GitilesCommit:
		if src.GitilesCommit.GetHost() == "" {
			return errors.New("sources.gitiles_commit: host is required")
		}
		if src.GitilesCommit.GetProject() == "" {
			return errors.New("sources.gitiles_commit: project is required")
		}
		if src.GitilesCommit.GetRef() == "" {
			return errors.New("sources.gitiles_commit: ref is required")
		}
	case *pb.Sources_SubmittedAndroidBuild:
		if src.SubmittedAndroidBuild.GetDataRealm() == "" {
			return errors.New("sources.submitted_android_build: data_realm is required")
		}
		if src.SubmittedAndroidBuild.GetBranch() == "" {
			return errors.New("sources.submitted_android_build: branch is required")
		}
		if src.SubmittedAndroidBuild.GetBuildId() == 0 {
			return errors.New("sources.submitted_android_build: build_id is required and must not be 0")
		}
	case nil:
		return errors.New("sources: base_sources is required")
	default:
		// This case should not be reached if the proto is well-defined.
		return errors.New("sources: unknown base_sources type")
	}

	return nil
}
