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
	"regexp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

func validateListArtifactsRequest(req *pb.ListArtifactsRequest) error {
	if pbutil.ValidateInvocationName(req.Parent) != nil && pbutil.ValidateTestResultName(req.Parent) != nil {
		return errors.Reason("parent: neither valid invocation name nor valid test result name").Err()
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}

	return nil
}

// ListArtifacts implements pb.ResultDBServer.
func (s *resultDBServer) ListArtifacts(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
	if err := validateListArtifactsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Prepare the query.
	q := artifacts.Query{
		PageSize:  pagination.AdjustPageSize(in.PageSize),
		PageToken: in.PageToken,
	}
	if invIDStr, err := pbutil.ParseInvocationName(in.Parent); err == nil {
		q.InvocationIDs = span.NewInvocationIDSet(span.InvocationID(invIDStr))
		// Fetch only invocation-level artifacts. They have empty ParentId.
		q.ParentIDRegexp = "^$"
	} else {
		// in.Parent must be a test result name.
		invID, testID, resultID := testresults.MustParseName(in.Parent)
		q.InvocationIDs = span.NewInvocationIDSet(invID)
		q.ParentIDRegexp = regexp.QuoteMeta(artifacts.ParentID(testID, resultID))
	}

	// Read artifacts.
	arts, token, err := q.Fetch(ctx, span.Client(ctx).Single())
	if err != nil {
		return nil, err
	}

	if err := s.populateFetchURLs(ctx, arts...); err != nil {
		return nil, err
	}

	return &pb.ListArtifactsResponse{
		Artifacts:     arts,
		NextPageToken: token,
	}, nil
}
