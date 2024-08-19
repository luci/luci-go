// Copyright 2024 The LUCI Authors.
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
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

const googlerOnlyGroup = "googlers"

// The maximium bytes for
const maxMatchWithContextLength = 10 * 1024 // 10KiB.

var artifactSearchPageSizeLimiter = pagination.PageSizeLimiter{
	Default: 20,
	Max:     100,
}

var insufficientPermissionWithQueryFilter = errors.New("insufficient permission to run this query with current filters")

func (s *resultDBServer) QueryTestVariantArtifactGroups(ctx context.Context, req *pb.QueryTestVariantArtifactGroupsRequest) (rsp *pb.QueryTestVariantArtifactGroupsResponse, err error) {
	// Validate project before using it to check permission.
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "project").Err())
	}
	subRealms, err := permissions.QuerySubRealmsNonEmpty(ctx, req.Project, nil, rdbperms.PermListArtifacts)
	if err != nil {
		return nil, err
	}
	isGoogler, err := auth.IsMember(ctx, googlerOnlyGroup)
	if err != nil {
		return nil, errors.Annotate(err, "failed to check ACL").Err()
	}
	if err := validateQueryTestVariantArtifactGroupsRequest(req, isGoogler); err != nil {
		if errors.Contains(err, insufficientPermissionWithQueryFilter) {
			return nil, appstatus.Errorf(codes.PermissionDenied, err.Error())
		}
		return nil, appstatus.BadRequest(err)
	}

	limit := int(artifactSearchPageSizeLimiter.Adjust(req.PageSize))
	opts := artifacts.ReadArtifactGroupsOpts{
		Project:           req.Project,
		SearchString:      req.SearchString,
		TestIDMatcher:     req.TestIdMatcher,
		ArtifactIDMatcher: req.ArtifactIdMatcher,
		StartTime:         req.StartTime.AsTime(),
		EndTime:           req.EndTime.AsTime(),
		SubRealms:         subRealms,
		Limit:             limit,
		PageToken:         req.PageToken,
	}
	rows, nextPageToken, err := s.artifactBQClient.ReadArtifactGroups(ctx, opts)
	if err != nil {
		return nil, errors.Annotate(err, "read test artifacts groups").Err()
	}
	pbGroups, err := toTestArtifactGroupsProto(rows)
	if err != nil {
		return nil, errors.Annotate(err, "to test artifact groups proto").Err()
	}
	return &pb.QueryTestVariantArtifactGroupsResponse{
		Groups:        pbGroups,
		NextPageToken: nextPageToken,
	}, nil
}

func validateQueryTestVariantArtifactGroupsRequest(req *pb.QueryTestVariantArtifactGroupsRequest, isGoogler bool) error {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if req.SearchString.GetExactContain() == "" && req.SearchString.GetRegexContain() == "" {
		return errors.New("search_string: unspecified")
	}
	// Non-googler caller have to specify an exact test id.
	// Because search with empty test id, or test id prefix can uses around 36 BigQuery slot hours.
	// This is expensive and we want to avoid people outside of google from abusing it.
	allowNonExactMatch := isGoogler
	if err := validateTestIDMatcher(req.TestIdMatcher, allowNonExactMatch); err != nil {
		return errors.Annotate(err, "test_id_matcher").Err()
	}
	// Non-exact artifact id match is already allowed here,
	// because this matcher has minimal impact on the query performance for this search,.
	if err := validateArtifactIDMatcher(req.ArtifactIdMatcher, true); err != nil {
		return errors.Annotate(err, "artifact_id_matcher").Err()
	}

	if err := validateStartEndTime(req.StartTime, req.EndTime); err != nil {
		return err
	}
	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}
	return nil
}

func validateTestIDMatcher(m *pb.IDMatcher, allowNonExactMatch bool) error {
	if m == nil {
		if !allowNonExactMatch {
			return errors.Annotate(insufficientPermissionWithQueryFilter, "unspecified").Err()
		}
		return nil
	}
	switch x := m.Matcher.(type) {
	case *pb.IDMatcher_HasPrefix:
		if !allowNonExactMatch {
			return errors.Annotate(insufficientPermissionWithQueryFilter, "search by prefix is not allowed").Err()
		}
		// ValidateTestID can also be used to validate test id prefix.
		return pbutil.ValidateTestID(m.GetHasPrefix())
	case *pb.IDMatcher_ExactEqual:
		return pbutil.ValidateTestID(m.GetExactEqual())
	default:
		return fmt.Errorf("has unexpected type %T", x)
	}
}

func validateArtifactIDMatcher(m *pb.IDMatcher, allowNonExactMatch bool) error {
	if m == nil {
		if !allowNonExactMatch {
			return errors.Annotate(insufficientPermissionWithQueryFilter, "unspecified").Err()
		}
		return nil
	}
	switch x := m.Matcher.(type) {
	case *pb.IDMatcher_HasPrefix:
		if !allowNonExactMatch {
			return errors.Annotate(insufficientPermissionWithQueryFilter, "search by prefix is not allowed").Err()
		}
		return pbutil.ValidateArtifactIDPrefix(m.GetHasPrefix())
	case *pb.IDMatcher_ExactEqual:
		return pbutil.ValidateArtifactID(m.GetExactEqual())
	default:
		return fmt.Errorf("has unexpected type %T", x)
	}
}

func validateStartEndTime(startTime, endTime *timestamppb.Timestamp) error {
	if startTime == nil {
		return errors.New("start_time: unspecified")
	}
	// TODO(beining@): Validate start_time against a cut-off time so that start_time can't be less than the cut-off time.
	// The cut-off time is when all necessary columns for log search are populated in the text_artifacts table.
	// Add this vaidation when all necessary columns are been added and rolled to prod, and cut-off time is known.
	if endTime == nil {
		return errors.New("end_time: unspecified")
	}
	if startTime.AsTime().After(endTime.AsTime()) {
		return errors.New("start time must not be later than end time")
	}
	if endTime.AsTime().Sub(startTime.AsTime()) > time.Hour*24*7 {
		return errors.New("difference between start_time and end_time must not be greater than 7 days")
	}
	return nil
}

func toTestArtifactGroupsProto(groups []*artifacts.ArtifactGroup) ([]*pb.QueryTestVariantArtifactGroupsResponse_MatchGroup, error) {
	pbGroups := make([]*pb.QueryTestVariantArtifactGroupsResponse_MatchGroup, 0, len(groups))
	for _, g := range groups {
		variant, err := pbutil.VariantFromJSON(g.Variant.String())
		if err != nil {
			return nil, errors.Annotate(err, "variant from JSON").Err()
		}
		match := &pb.QueryTestVariantArtifactGroupsResponse_MatchGroup{
			TestId:        g.TestID,
			VariantHash:   g.VariantHash,
			Variant:       variant,
			ArtifactId:    g.ArtifactID,
			Artifacts:     toTestArtifactMatchingContents(g.Artifacts, g.TestID, g.ArtifactID),
			MatchingCount: g.MatchingCount,
		}
		pbGroups = append(pbGroups, match)
	}
	return pbGroups, nil
}

func toTestArtifactMatchingContents(bqArtifacts []*artifacts.MatchingArtifact, testID, artifactID string) []*pb.ArtifactMatchingContent {
	res := make([]*pb.ArtifactMatchingContent, 0, len(bqArtifacts))
	for _, a := range bqArtifacts {
		matchStr, before, after := truncateMatchWithContext(a)
		res = append(res, &pb.ArtifactMatchingContent{
			Name:          pbutil.TestResultArtifactName(a.InvocationID, testID, a.ResultID, artifactID),
			PartitionTime: timestamppb.New(a.PartitionTime),
			TestStatus:    pb.TestStatus(pb.TestStatus_value[a.TestStatus.String()]),
			Match:         matchStr,
			BeforeMatch:   before,
			AfterMatch:    after,
		})
	}
	return res
}

// truncateMatchWithContext truncates match, and contents immediately before and after,
// so that the total size of them are not greater than maxMatchWithContextLength.
func truncateMatchWithContext(artifact *artifacts.MatchingArtifact) (match, before, after string) {
	if len(artifact.Match) >= maxMatchWithContextLength {
		return truncateString(artifact.Match, maxMatchWithContextLength), "", ""
	}
	// Divide remaining bytes to before and after evenly.
	remainBytes := maxMatchWithContextLength - len(artifact.Match)

	before = reverseString(truncateString(reverseString(artifact.MatchWithContextBefore), remainBytes/2))
	after = truncateString(artifact.MatchWithContextAfter, remainBytes/2)
	return artifact.Match, before, after
}

// truncateString truncates a UTF-8 string to the given number of bytes.
// If the string is truncated and length is >= 3, ellipsis ("...") are added.
// Truncation is aware of UTF-8 runes and will only truncate whole runes.
// length must be at least 3 (to leave space for ellipsis, if needed).
func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	if length < 3 {
		return ""
	}
	// The index (in bytes) at which to begin truncating the string.
	lastIndex := 0
	// Find the point where we must truncate from. We only want to
	// start truncation at the start/end of a rune, not in the middle.
	// See https://blog.golang.org/strings.
	for i := range s {
		if i <= (length - 3) {
			lastIndex = i
		}
	}
	return s[:lastIndex] + "..."
}

// reverseString reverses a UTF-8 string.
func reverseString(str string) string {
	result := ""
	for _, v := range str {
		result = string(v) + result
	}
	return result
}
