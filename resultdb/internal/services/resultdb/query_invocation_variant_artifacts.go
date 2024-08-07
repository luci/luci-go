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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func (s *resultDBServer) QueryInvocationVariantArtifacts(ctx context.Context, req *pb.QueryInvocationVariantArtifactsRequest) (rsp *pb.QueryInvocationVariantArtifactsResponse, err error) {
	// Validate project before using it to check permission.
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "project").Err())
	}
	subRealms, err := permissions.QuerySubRealmsNonEmpty(ctx, req.Project, nil, rdbperms.PermListArtifacts)
	if err != nil {
		return nil, err
	}
	if err := validateQueryInvocationVariantArtifactsRequest(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	limit := int(artifactSearchPageSizeLimiter.Adjust(req.PageSize))
	opts := artifacts.ReadArtifactsOpts{
		Project:           req.Project,
		SearchString:      req.SearchString,
		VariantHash:       req.VariantUnionHash,
		IsInvocationLevel: true,
		ArtifactID:        req.ArtifactId,
		StartTime:         req.StartTime.AsTime(),
		EndTime:           req.EndTime.AsTime(),
		SubRealms:         subRealms,
		Limit:             limit,
		PageToken:         req.PageToken,
	}
	rows, nextPageToken, err := s.artifactBQClient.ReadArtifacts(ctx, opts)
	if err != nil {
		return nil, errors.Annotate(err, "read artifacts").Err()
	}
	return &pb.QueryInvocationVariantArtifactsResponse{
		Artifacts:     toInvocationArtifactMatchingContents(rows, req.ArtifactId),
		NextPageToken: nextPageToken,
	}, nil
}

func validateQueryInvocationVariantArtifactsRequest(req *pb.QueryInvocationVariantArtifactsRequest) error {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if req.SearchString.GetExactContain() == "" && req.SearchString.GetRegexContain() == "" {
		return errors.New("search_string: unspecified")
	}
	if err := pbutil.ValidateVariantHash(req.VariantUnionHash); err != nil {
		return errors.Annotate(err, "variant_union_hash").Err()
	}

	if err := pbutil.ValidateArtifactID(req.ArtifactId); err != nil {
		return errors.Annotate(err, "artifact_id").Err()
	}
	if err := validateStartEndTime(req.StartTime, req.EndTime); err != nil {
		return err
	}
	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}
	return nil
}
