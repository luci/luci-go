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

func (s *resultDBServer) QueryInvocationVariantArtifactGroups(ctx context.Context, req *pb.QueryInvocationVariantArtifactGroupsRequest) (rsp *pb.QueryInvocationVariantArtifactGroupsResponse, err error) {
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
	if err := validateQueryInvocationVariantArtifactGroupsRequest(req, isGoogler); err != nil {
		if errors.Contains(err, insufficientPermissionWithQueryFilter) {
			return nil, appstatus.Errorf(codes.PermissionDenied, err.Error())
		}
		return nil, appstatus.BadRequest(err)
	}

	limit := int(artifactSearchPageSizeLimiter.Adjust(req.PageSize))
	opts := artifacts.ReadArtifactGroupsOpts{
		Project:           req.Project,
		SearchString:      req.SearchString,
		ArtifactIDMatcher: req.ArtifactIdMatcher,
		IsInvocationLevel: true,
		StartTime:         req.StartTime.AsTime(),
		EndTime:           req.EndTime.AsTime(),
		SubRealms:         subRealms,
		Limit:             limit,
		PageToken:         req.PageToken,
	}
	rows, nextPageToken, err := s.artifactBQClient.ReadArtifactGroups(ctx, opts)
	if err != nil {
		return nil, errors.Annotate(err, "read invocation artifacts groups").Err()
	}
	pbGroups, err := toInvocationArtifactGroupsProto(rows, req.SearchString)
	if err != nil {
		return nil, errors.Annotate(err, "to invocation artifact groups proto").Err()
	}
	return &pb.QueryInvocationVariantArtifactGroupsResponse{
		Groups:        pbGroups,
		NextPageToken: nextPageToken,
	}, nil
}

func validateQueryInvocationVariantArtifactGroupsRequest(req *pb.QueryInvocationVariantArtifactGroupsRequest, isGoogler bool) error {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if err := validateSearchString(req.SearchString); err != nil {
		return errors.Annotate(err, "search_string").Err()
	}
	// Non-googler caller have to specify an exact artifact id.
	// Because search with empty artifact id, or artifact id prefix can be expensive.
	// So we want to avoid people outside of google from abusing it.
	allowNonExactMatch := isGoogler
	if err := validateArtifactIDMatcher(req.ArtifactIdMatcher, allowNonExactMatch); err != nil {
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

func toInvocationArtifactGroupsProto(groups []*artifacts.ArtifactGroup, searchString *pb.ArtifactContentMatcher) ([]*pb.QueryInvocationVariantArtifactGroupsResponse_MatchGroup, error) {
	pbGroups := make([]*pb.QueryInvocationVariantArtifactGroupsResponse_MatchGroup, 0, len(groups))
	for _, g := range groups {
		variant, err := pbutil.VariantFromJSON(g.Variant.String())
		if err != nil {
			return nil, errors.Annotate(err, "variant from JSON").Err()
		}
		match := &pb.QueryInvocationVariantArtifactGroupsResponse_MatchGroup{
			VariantUnionHash: g.VariantHash,
			VariantUnion:     variant,
			ArtifactId:       g.ArtifactID,
			Artifacts:        toInvocationArtifactMatchingContents(g.Artifacts, g.ArtifactID, searchString),
			MatchingCount:    g.MatchingCount,
		}
		pbGroups = append(pbGroups, match)
	}
	return pbGroups, nil
}

func toInvocationArtifactMatchingContents(bqArtifacts []*artifacts.MatchingArtifact, artifactID string, searchString *pb.ArtifactContentMatcher) []*pb.ArtifactMatchingContent {
	res := make([]*pb.ArtifactMatchingContent, 0, len(bqArtifacts))
	for _, a := range bqArtifacts {
		snippet, matches := constructSnippetAndMatches(a, searchString)
		res = append(res, &pb.ArtifactMatchingContent{
			Name:          pbutil.InvocationArtifactName(a.InvocationID, artifactID),
			PartitionTime: timestamppb.New(a.PartitionTime),
			Snippet:       snippet,
			Matches:       matches,
		})
	}
	return res
}
