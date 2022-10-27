// Copyright 2020 The LUCI Authors.
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
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/span"
)

// validateQueryArtifactsRequest returns a non-nil error if req is determined
// to be invalid.
func validateQueryArtifactsRequest(req *pb.QueryArtifactsRequest) error {
	if err := pbutil.ValidateArtifactPredicate(req.GetPredicate()); err != nil {
		return errors.Annotate(err, "predicate").Err()
	}
	return validateQueryRequest(req)
}

// QueryArtifacts implements pb.ResultDBServer.
func (s *resultDBServer) QueryArtifacts(ctx context.Context, in *pb.QueryArtifactsRequest) (*pb.QueryArtifactsResponse, error) {
	if err := permissions.VerifyInvocationsByName(ctx, in.Invocations, rdbperms.PermListArtifacts); err != nil {
		return nil, err
	}

	if err := validateQueryArtifactsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Open a transaction.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Get the transitive closure.
	invs, err := invocations.Reachable(ctx, invocations.MustParseNames(in.Invocations))
	if err != nil {
		return nil, err
	}

	// Query artifacts.
	q := artifacts.Query{
		InvocationIDs:       invs,
		TestResultPredicate: in.GetPredicate().GetTestResultPredicate(),
		PageSize:            pagination.AdjustPageSize(in.PageSize),
		PageToken:           in.PageToken,
		FollowEdges:         in.GetPredicate().GetFollowEdges(),
		ContentTypeRegexp:   in.GetPredicate().GetContentTypeRegexp(),
		ArtifactIDRegexp:    in.GetPredicate().GetArtifactIdRegexp(),
		WithGcsURI:          true,
	}
	arts, token, err := q.FetchProtos(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.populateFetchURLs(ctx, arts...); err != nil {
		return nil, err
	}

	return &pb.QueryArtifactsResponse{
		Artifacts:     arts,
		NextPageToken: token,
	}, nil
}
