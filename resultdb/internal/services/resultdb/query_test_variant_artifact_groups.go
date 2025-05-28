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
	"regexp"
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

// The maximum bytes of artifact content in the response.
const maxMatchWithContextLength = 10 * 1024 // 10KiB.

// The cut-off time is when all necessary columns for log search are populated in the text_artifacts table.
// We should only accept query requests logs after this time.
var cutOffTime = time.Date(2024, 7, 20, 0, 0, 0, 0, time.UTC)

var artifactSearchPageSizeLimiter = pagination.PageSizeLimiter{
	Default: 20,
	Max:     100,
}

var insufficientPermissionWithQueryFilter = errors.New("insufficient permission to run this query with current filters")

func (s *resultDBServer) QueryTestVariantArtifactGroups(ctx context.Context, req *pb.QueryTestVariantArtifactGroupsRequest) (rsp *pb.QueryTestVariantArtifactGroupsResponse, err error) {
	// Validate project before using it to check permission.
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return nil, appstatus.BadRequest(errors.Fmt("project: %w", err))
	}
	subRealms, err := permissions.QuerySubRealmsNonEmpty(ctx, req.Project, nil, rdbperms.PermListArtifacts)
	if err != nil {
		return nil, err
	}
	isGoogler, err := auth.IsMember(ctx, googlerOnlyGroup)
	if err != nil {
		return nil, errors.Fmt("failed to check ACL: %w", err)
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
		if errors.Is(err, artifacts.BQQueryTimeOutErr) {
			// Returns bad_request error code to avoid automatic retries.
			return nil, appstatus.BadRequest(err)
		}
		return nil, errors.Fmt("read test artifacts groups: %w", err)
	}
	pbGroups, err := toTestArtifactGroupsProto(rows, req.SearchString)
	if err != nil {
		return nil, errors.Fmt("to test artifact groups proto: %w", err)
	}
	return &pb.QueryTestVariantArtifactGroupsResponse{
		Groups:        pbGroups,
		NextPageToken: nextPageToken,
	}, nil
}

func validateQueryTestVariantArtifactGroupsRequest(req *pb.QueryTestVariantArtifactGroupsRequest, isGoogler bool) error {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := validateSearchString(req.SearchString); err != nil {
		return errors.Fmt("search_string: %w", err)
	}
	// Non-googler caller have to specify an exact test id.
	// Because search with empty test id, or test id prefix can uses around 36 BigQuery slot hours.
	// This is expensive and we want to avoid people outside of google from abusing it.
	allowNonExactMatch := isGoogler
	if err := validateTestIDMatcher(req.TestIdMatcher, allowNonExactMatch); err != nil {
		return errors.Fmt("test_id_matcher: %w", err)
	}
	// Non-exact artifact id match is already allowed here,
	// because this matcher has minimal impact on the query performance for this search,.
	if err := validateArtifactIDMatcher(req.ArtifactIdMatcher, true); err != nil {
		return errors.Fmt("artifact_id_matcher: %w", err)
	}

	if err := validateStartEndTime(req.StartTime, req.EndTime); err != nil {
		return err
	}
	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Fmt("page_size: %w", err)
	}
	return nil
}

func validateSearchString(m *pb.ArtifactContentMatcher) error {
	if m.GetContain() == "" && m.GetRegexContain() == "" {
		return errors.New("unspecified")
	}
	var matchString string
	switch x := m.Matcher.(type) {
	case *pb.ArtifactContentMatcher_Contain:
		matchString = m.GetContain()
	case *pb.ArtifactContentMatcher_RegexContain:
		matchString = m.GetRegexContain()
		if err := validateRegexSearchString(matchString); err != nil {
			return err
		}
	default:
		return fmt.Errorf("has unexpected type %T", x)
	}
	// 2048 bytes (2kib) is an arbitrary limit for this field. It can be increased if needed.
	if len(matchString) > 2048 {
		return errors.New("longer than 2048 bytes")
	}
	return nil
}

func validateRegexSearchString(regexPattern string) error {
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return err
	}
	if re.NumSubexp() > 0 {
		return errors.New("capture group is not allowed")
	}
	return nil
}

func validateTestIDMatcher(m *pb.IDMatcher, allowNonExactMatch bool) error {
	if m == nil {
		if !allowNonExactMatch {
			return errors.Fmt("unspecified: %w", insufficientPermissionWithQueryFilter)
		}
		return nil
	}
	switch x := m.Matcher.(type) {
	case *pb.IDMatcher_HasPrefix:
		if !allowNonExactMatch {
			return errors.Fmt("search by prefix is not allowed: %w", insufficientPermissionWithQueryFilter)
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
			return errors.Fmt("unspecified: %w", insufficientPermissionWithQueryFilter)
		}
		return nil
	}
	switch x := m.Matcher.(type) {
	case *pb.IDMatcher_HasPrefix:
		if !allowNonExactMatch {
			return errors.Fmt("search by prefix is not allowed: %w", insufficientPermissionWithQueryFilter)
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
	if startTime.AsTime().Before(cutOffTime) {
		return fmt.Errorf("start_time: must be after %s", cutOffTime)
	}
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

func toTestArtifactGroupsProto(groups []*artifacts.ArtifactGroup, searchString *pb.ArtifactContentMatcher) ([]*pb.QueryTestVariantArtifactGroupsResponse_MatchGroup, error) {
	pbGroups := make([]*pb.QueryTestVariantArtifactGroupsResponse_MatchGroup, 0, len(groups))
	for _, g := range groups {
		variant, err := pbutil.VariantFromJSON(g.Variant.String())
		if err != nil {
			return nil, errors.Fmt("variant from JSON: %w", err)
		}
		match := &pb.QueryTestVariantArtifactGroupsResponse_MatchGroup{
			TestId:        g.TestID,
			VariantHash:   g.VariantHash,
			Variant:       variant,
			ArtifactId:    g.ArtifactID,
			Artifacts:     toTestArtifactMatchingContents(g.Artifacts, g.TestID, g.ArtifactID, searchString),
			MatchingCount: g.MatchingCount,
		}
		pbGroups = append(pbGroups, match)
	}
	return pbGroups, nil
}

func toTestArtifactMatchingContents(bqArtifacts []*artifacts.MatchingArtifact, testID, artifactID string, searchString *pb.ArtifactContentMatcher) []*pb.ArtifactMatchingContent {
	res := make([]*pb.ArtifactMatchingContent, 0, len(bqArtifacts))
	for _, a := range bqArtifacts {
		snippet, matches := constructSnippetAndMatches(a, searchString)
		res = append(res, &pb.ArtifactMatchingContent{
			Name:          pbutil.TestResultArtifactName(a.InvocationID, testID, a.ResultID, artifactID),
			PartitionTime: timestamppb.New(a.PartitionTime),
			TestStatus:    pb.TestStatus(pb.TestStatus_value[a.TestStatus.String()]),
			Snippet:       snippet,
			Matches:       matches,
		})
	}
	return res
}

// constructSnippetAndMatches constructs the snippet contains the match and content immediately before and after,
// and the location of all matches in this snippet.
// It also truncate the snippet so that the total size of it is not greater than maxMatchWithContextLength.
// It will prioritize fiting first match first, divided the remaining bytes equally to fit content immediately before and after.
func constructSnippetAndMatches(artifact *artifacts.MatchingArtifact, searchString *pb.ArtifactContentMatcher) (snippet string, matches []*pb.ArtifactMatchingContent_Match) {
	if len(artifact.Match) >= maxMatchWithContextLength {
		snippet, _ := truncateString(artifact.Match, maxMatchWithContextLength)
		return snippet, []*pb.ArtifactMatchingContent_Match{{
			StartIndex: 0,
			EndIndex:   int32(len(snippet)),
		}}
	}

	// Divide remaining bytes to before and after evenly.
	remainBytes := maxMatchWithContextLength - len(artifact.Match)
	beforeReversed, _ := truncateString(reverseString(artifact.MatchWithContextBefore), remainBytes/2)
	before := reverseString(beforeReversed)
	after, endEllipsis := truncateString(artifact.MatchWithContextAfter, remainBytes/2)
	indexOffset := len(artifact.Match) + len(before)
	// Add the first match to matches.
	matches = []*pb.ArtifactMatchingContent_Match{{
		StartIndex: int32(len(before)), EndIndex: int32(indexOffset),
	}}
	// Because artifact.Match is guaranteed to be the first match in the artifact content.
	// (i.e. no match can be found in MatchWithContextBefore, or in the boundary between match and MatchWithContextAfter)
	// We only need to search the MatchWithContextAfter for remaining matches.
	afterEndIdx := len(after)
	if endEllipsis {
		// We need to remove the ellipsis when perform match finding.
		afterEndIdx = len(after) - 3
	}
	matches = append(matches, findAllMatches(after[0:afterEndIdx], searchString, indexOffset)...)

	return before + artifact.Match + after, matches
}

// findAllMatches finds the location by indexOffset of all matches in str.
func findAllMatches(str string, searchString *pb.ArtifactContentMatcher, indexOffset int) []*pb.ArtifactMatchingContent_Match {
	matches := make([]*pb.ArtifactMatchingContent_Match, 0)
	pattern := artifacts.RegexPattern(searchString)
	re := regexp.MustCompile(pattern)
	matchIndices := re.FindAllStringIndex(str, -1)
	for _, matchIndex := range matchIndices {
		matches = append(matches, &pb.ArtifactMatchingContent_Match{
			StartIndex: int32(matchIndex[0] + indexOffset),
			EndIndex:   int32(matchIndex[1] + indexOffset),
		})
	}
	return matches
}

// truncateString truncates a UTF-8 string to the given number of bytes.
// If the string is truncated AND length is >= 3, ellipsis ("...") are added.
// Truncation is aware of UTF-8 runes and will only truncate whole runes.
// length must be at least 3 (to leave space for ellipsis, if needed).
// It will returns the truncated string and a bool to indicate whether ellipsis is appended.
func truncateString(s string, length int) (result string, ellipsisAppended bool) {
	if len(s) <= length {
		return s, false
	}
	if length < 3 {
		return "", false
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
	return s[:lastIndex] + "...", true
}

// reverseString reverses a UTF-8 string.
func reverseString(str string) string {
	result := ""
	for _, v := range str {
		result = string(v) + result
	}
	return result
}
