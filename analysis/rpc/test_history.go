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
	"fmt"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/pagination"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/testrealms"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func init() {
	rdbperms.PermListTestResults.AddFlags(realms.UsedInQueryRealms)
	rdbperms.PermListTestExonerations.AddFlags(realms.UsedInQueryRealms)
}

var pageSizeLimiter = pagination.PageSizeLimiter{
	Default: 100,
	Max:     1000,
}

type TestSearchClient interface {
	QueryTests(ctx context.Context, project, testIDSubstring string, opts testrealms.QueryTestsOptions) (testIDs []string, nextPageToken string, err error)
}

// testHistoryServer implements pb.TestHistoryServer.
type testHistoryServer struct {
	searchClient TestSearchClient
}

// NewTestHistoryServer returns a new pb.TestHistoryServer.
func NewTestHistoryServer(searchClient TestSearchClient) pb.TestHistoryServer {
	return &pb.DecoratedTestHistory{
		Service:  &testHistoryServer{searchClient: searchClient},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// Retrieves test verdicts for a given test ID in a given project and in a given
// range of time.
func (s *testHistoryServer) Query(ctx context.Context, req *pb.QueryTestHistoryRequest) (*pb.QueryTestHistoryResponse, error) {
	if err := validateQueryTestHistoryRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}

	subRealms, err := perms.QuerySubRealmsNonEmpty(ctx, req.Project, req.Predicate.SubRealm, nil, perms.ListTestResultsAndExonerations...)
	if err != nil {
		return nil, err
	}
	var previousTestID string
	if req.FollowTestIdRenaming {
		previousTestID, err = queryPreviousTestIDFromResultDB(ctx, req.Project, req.TestId)
		if err != nil {
			return nil, err
		}
	}

	pageSize := int(pageSizeLimiter.Adjust(req.PageSize))
	opts := testresults.ReadTestHistoryOptions{
		Project:                 req.Project,
		TestID:                  req.TestId,
		PreviousTestID:          previousTestID,
		SubRealms:               subRealms,
		VariantPredicate:        req.Predicate.VariantPredicate,
		SubmittedFilter:         req.Predicate.SubmittedFilter,
		TimeRange:               req.Predicate.PartitionTimeRange,
		ExcludeBisectionResults: !req.Predicate.IncludeBisectionResults,
		PageSize:                pageSize,
		PageToken:               req.PageToken,
	}

	verdicts, nextPageToken, err := testresults.ReadTestHistory(span.Single(ctx), opts)
	if err != nil {
		return nil, err
	}

	return &pb.QueryTestHistoryResponse{
		Verdicts:      verdicts,
		NextPageToken: nextPageToken,
	}, nil
}

func validateQueryTestHistoryRequest(req *pb.QueryTestHistoryRequest) error {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := rdbpbutil.ValidateTestID(req.TestId); err != nil {
		return errors.Fmt("test_id: %w", err)
	}

	if err := pbutil.ValidateTestVerdictPredicate(req.GetPredicate()); err != nil {
		return errors.Fmt("predicate: %w", err)
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Fmt("page_size: %w", err)
	}

	return nil
}

// Retrieves a summary of test verdicts for a given test ID in a given project
// and in a given range of times.
func (s *testHistoryServer) QueryStats(ctx context.Context, req *pb.QueryTestHistoryStatsRequest) (*pb.QueryTestHistoryStatsResponse, error) {
	if err := validateQueryTestHistoryStatsRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}

	subRealms, err := perms.QuerySubRealmsNonEmpty(ctx, req.Project, req.Predicate.SubRealm, nil, perms.ListTestResultsAndExonerations...)
	if err != nil {
		return nil, err
	}
	var previousTestID string
	if req.FollowTestIdRenaming {
		previousTestID, err = queryPreviousTestIDFromResultDB(ctx, req.Project, req.TestId)
		if err != nil {
			return nil, err
		}
	}

	pageSize := int(pageSizeLimiter.Adjust(req.PageSize))
	opts := testresults.ReadTestHistoryOptions{
		Project:                 req.Project,
		TestID:                  req.TestId,
		PreviousTestID:          previousTestID,
		SubRealms:               subRealms,
		VariantPredicate:        req.Predicate.VariantPredicate,
		SubmittedFilter:         req.Predicate.SubmittedFilter,
		TimeRange:               req.Predicate.PartitionTimeRange,
		ExcludeBisectionResults: !req.Predicate.IncludeBisectionResults,
		PageSize:                pageSize,
		PageToken:               req.PageToken,
	}

	groups, nextPageToken, err := testresults.ReadTestHistoryStats(span.Single(ctx), opts)
	if err != nil {
		return nil, err
	}

	return &pb.QueryTestHistoryStatsResponse{
		Groups:        groups,
		NextPageToken: nextPageToken,
	}, nil
}

func validateQueryTestHistoryStatsRequest(req *pb.QueryTestHistoryStatsRequest) error {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := rdbpbutil.ValidateTestID(req.TestId); err != nil {
		return errors.Fmt("test_id: %w", err)
	}

	if err := pbutil.ValidateTestVerdictPredicate(req.GetPredicate()); err != nil {
		return errors.Fmt("predicate: %w", err)
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Fmt("page_size: %w", err)
	}

	return nil
}

// Retrieves variants for a given test ID in a given project that were recorded
// in the past 90 days.
func (*testHistoryServer) QueryVariants(ctx context.Context, req *pb.QueryVariantsRequest) (*pb.QueryVariantsResponse, error) {
	if err := validateQueryVariantsRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}

	subRealms, err := perms.QuerySubRealmsNonEmpty(ctx, req.Project, req.SubRealm, nil, rdbperms.PermListTestResults)
	if err != nil {
		return nil, err
	}
	var previousTestID string
	if req.FollowTestIdRenaming {
		previousTestID, err = queryPreviousTestIDFromResultDB(ctx, req.Project, req.TestId)
		if err != nil {
			return nil, err
		}
	}

	pageSize := int(pageSizeLimiter.Adjust(req.PageSize))
	opts := testresults.ReadVariantsOptions{
		Project:          req.GetProject(),
		TestID:           req.GetTestId(),
		PreviousTestID:   previousTestID,
		SubRealms:        subRealms,
		VariantPredicate: req.VariantPredicate,
		PageSize:         pageSize,
		PageToken:        req.PageToken,
	}

	variants, nextPageToken, err := testresults.ReadVariants(span.Single(ctx), opts)
	if err != nil {
		return nil, err
	}

	return &pb.QueryVariantsResponse{
		Variants:      variants,
		NextPageToken: nextPageToken,
	}, nil
}

func queryPreviousTestIDFromResultDB(ctx context.Context, project, testID string) (string, error) {
	cl, err := resultdb.NewCredentialForwardingClient(ctx, chromeinfra.ResultDBHost)
	if err != nil {
		return "", err
	}

	req := &resultpb.QueryTestMetadataRequest{
		Project: project,
		Predicate: &resultpb.TestMetadataPredicate{
			TestIds: []string{testID},
		},
	}
	rsp, err := cl.QueryTestMetadata(ctx, req)
	if err != nil {
		return "", fmt.Errorf("query previous test ID from ResultDB: %w", err)
	}

	if len(rsp.TestMetadata) == 0 {
		return "", nil
	}
	const mainGitRef = "refs/heads/main"

	// Default to the first item.
	metadata := rsp.TestMetadata[0]
	for _, md := range rsp.TestMetadata {
		// Prefer something from a main branch, if there is one.
		if md.SourceRef.GetGitiles().GetRef() == mainGitRef {
			metadata = md
			break
		}
	}
	return metadata.TestMetadata.GetPreviousTestId(), nil
}

func validateQueryVariantsRequest(req *pb.QueryVariantsRequest) error {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := rdbpbutil.ValidateTestID(req.TestId); err != nil {
		return errors.Fmt("test_id: %w", err)
	}
	if req.SubRealm != "" {
		if err := realms.ValidateRealmName(req.SubRealm, realms.ProjectScope); err != nil {
			return errors.Fmt("sub_realm: %w", err)
		}
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Fmt("page_size: %w", err)
	}

	if req.GetVariantPredicate() != nil {
		if err := pbutil.ValidateVariantPredicate(req.GetVariantPredicate()); err != nil {
			return errors.Fmt("predicate: %w", err)
		}
	}

	return nil
}

// QueryTests finds all test IDs that contain the given substring in a given
// project that were recorded in the past 90 days.
func (s *testHistoryServer) QueryTests(ctx context.Context, req *pb.QueryTestsRequest) (*pb.QueryTestsResponse, error) {
	if err := validateQueryTestsRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}

	subRealms, err := perms.QuerySubRealmsNonEmpty(ctx, req.Project, req.SubRealm, nil, rdbperms.PermListTestResults)
	if err != nil {
		return nil, err
	}
	authorizedRealms := make([]string, 0, len(subRealms))
	for _, subRealm := range subRealms {
		authorizedRealms = append(authorizedRealms, realms.Join(req.Project, subRealm))
	}

	pageSize := int(pageSizeLimiter.Adjust(req.PageSize))
	opts := testrealms.QueryTestsOptions{
		CaseSensitive: !req.CaseInsensitive,
		Realms:        authorizedRealms,
		PageSize:      pageSize,
		PageToken:     req.GetPageToken(),
	}

	testIDs, nextPageToken, err := s.searchClient.QueryTests(ctx, req.Project, req.TestIdSubstring, opts)
	if err != nil {
		return nil, err
	}

	return &pb.QueryTestsResponse{
		TestIds:       testIDs,
		NextPageToken: nextPageToken,
	}, nil
}

func validateQueryTestsRequest(req *pb.QueryTestsRequest) error {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := validateTestIDPart(req.TestIdSubstring); err != nil {
		return errors.Fmt("test_id_substring: %w", err)
	}
	if req.SubRealm != "" {
		if err := realms.ValidateRealmName(req.SubRealm, realms.ProjectScope); err != nil {
			return errors.Fmt("sub_realm: %w", err)
		}
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Fmt("page_size: %w", err)
	}

	return nil
}

func validateTestIDPart(testIDPart string) error {
	if testIDPart == "" {
		return errors.New("unspecified")
	}
	if len(testIDPart) > 512 {
		return errors.New("length exceeds 512 bytes")
	}
	if !utf8.ValidString(testIDPart) {
		return errors.New("not a valid utf8 string")
	}
	if !norm.NFC.IsNormalString(testIDPart) {
		return errors.New("not in unicode normalized form C")
	}
	for i, rune := range testIDPart {
		if !unicode.IsPrint(rune) {
			return fmt.Errorf("non-printable rune %+q at byte index %d", rune, i)
		}
	}
	return nil
}
