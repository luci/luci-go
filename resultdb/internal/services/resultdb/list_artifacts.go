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
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/span"
)

// parseParent parses the parent argument as either an invocation name or a
// test result name and returns the corresponding invocation id, a parent id
// regex suitable for artifacts.Query and an error if the arg is not a valid.
func parseParent(parent string) (invocations.ID, string, error) {
	if invIDStr, err := pbutil.ParseInvocationName(parent); err == nil {
		// Fetch only invocation-level artifacts. They have empty ParentId.
		return invocations.ID(invIDStr), "^$", nil
	}
	invIDStr, testID, resultID, err := pbutil.ParseTestResultName(parent)
	if err != nil {
		return "", "", appstatus.BadRequest(
			errors.Reason("parent: neither valid invocation name nor valid test result name").Err())
	}
	return invocations.ID(invIDStr), regexp.QuoteMeta(artifacts.ParentID(testID, resultID)), nil
}

func validateListArtifactsRequest(req *pb.ListArtifactsRequest) error {
	// Do not assume that parent is already validated for permissions checking.
	if _, _, err := parseParent(req.Parent); err != nil {
		return err
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return appstatus.BadRequest(errors.Annotate(err, "page_size").Err())
	}

	return nil
}

// ListArtifacts implements pb.ResultDBServer.
func (s *resultDBServer) ListArtifacts(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	invID, parentIDRegexp, err := parseParent(in.Parent)
	if err != nil {
		return nil, err
	}

	if err := permissions.VerifyInvocation(ctx, invID, rdbperms.PermListArtifacts); err != nil {
		return nil, err
	}

	if err := validateListArtifactsRequest(in); err != nil {
		return nil, err
	}

	// Prepare the query.
	q := artifacts.Query{
		PageSize:       pagination.AdjustPageSize(in.PageSize),
		PageToken:      in.PageToken,
		InvocationIDs:  invocations.NewIDSet(invID),
		ParentIDRegexp: parentIDRegexp,
		WithGcsURI:     true,
	}

	// Read artifacts.
	arts, token, err := q.FetchProtos(ctx)
	if err != nil {
		return nil, err
	}

	realm, err := invocations.ReadRealm(ctx, invID)
	if err != nil {
		return nil, err
	}
	if err := s.populateFetchURLs(ctx, []string{realm}, arts...); err != nil {
		return nil, err
	}

	return &pb.ListArtifactsResponse{
		Artifacts:     arts,
		NextPageToken: token,
	}, nil
}
