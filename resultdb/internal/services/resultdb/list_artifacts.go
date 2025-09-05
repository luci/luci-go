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
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

// parseArtifactParent parses the parent argument as either as a work unit name,
// test result name or an invocation name. It returns the corresponding work unit or
// invocation id, a parent id regex suitable for artifacts.Query and an error if
// the arg is not a valid.
func parseArtifactParent(parent string) (invocations.ID, workunits.ID, string, error) {
	if parent == "" {
		return "", workunits.ID{}, "", errors.New("unspecified")
	}
	if strings.HasPrefix(parent, "invocations/") {
		if invIDStr, ok := pbutil.TryParseInvocationName(parent); ok {
			// Fetch only invocation-level artifacts. They have empty ParentId.
			return invocations.ID(invIDStr), workunits.ID{}, "^$", nil
		}
		invIDStr, testID, resultID, err := pbutil.ParseLegacyTestResultName(parent)
		if err == nil {
			return invocations.ID(invIDStr), workunits.ID{}, regexp.QuoteMeta(artifacts.ParentID(testID, resultID)), nil
		}
		// Fall through to error path.
	} else {
		if wuID, ok := workunits.TryParseName(parent); ok {
			return "", wuID, "^$", nil
		}
		parts, err := pbutil.ParseTestResultName(parent)
		if err == nil {
			wuID := workunits.ID{RootInvocationID: rootinvocations.ID(parts.RootInvocationID), WorkUnitID: parts.WorkUnitID}
			return "", wuID, regexp.QuoteMeta(artifacts.ParentID(parts.TestID, parts.ResultID)), nil
		}
		// Fall through to error path.
	}
	return "", workunits.ID{}, "", errors.New("neither a valid work unit name, test result name or legacy invocation name")
}

func validateListArtifactsRequest(req *pb.ListArtifactsRequest) error {
	// Do not assume that parent is already validated for permissions checking.
	if _, _, _, err := parseArtifactParent(req.Parent); err != nil {
		return appstatus.BadRequest(fmt.Errorf("parent: %w", err))
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return appstatus.BadRequest(errors.Fmt("page_size: %w", err))
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

	invID, wuID, parentIDRegexp, err := parseArtifactParent(in.Parent)
	if err != nil {
		return nil, appstatus.BadRequest(fmt.Errorf("parent: %w", err))
	}

	var queryInvocationID invocations.ID
	if invID != "" {
		if err := permissions.VerifyInvocation(ctx, invID, rdbperms.PermListArtifacts); err != nil {
			return nil, err
		}
		queryInvocationID = invID
	} else {
		accessLevel, err := permissions.QueryWorkUnitAccess(ctx, wuID, permissions.ListArtifactsAccessModel)
		if err != nil {
			return nil, err
		}
		if accessLevel == permissions.NoAccess {
			return nil, appstatus.Errorf(codes.PermissionDenied, "caller does not have permission %s or %s in realm of %q", rdbperms.PermListArtifacts, rdbperms.PermListLimitedArtifacts, wuID.RootInvocationID.Name())
		}
		if accessLevel != permissions.FullAccess {
			return nil, appstatus.Errorf(codes.PermissionDenied, "caller does not have permission %s in the realm of %q (trying to upgrade limited artifact access to full access)", rdbperms.PermGetArtifact, wuID.Name())
		}
		queryInvocationID = wuID.LegacyInvocationID()
	}

	if err := validateListArtifactsRequest(in); err != nil {
		return nil, err
	}

	// Prepare the query.
	q := artifacts.Query{
		PageSize:       pagination.AdjustPageSize(in.PageSize),
		PageToken:      in.PageToken,
		InvocationIDs:  invocations.NewIDSet(queryInvocationID),
		ParentIDRegexp: parentIDRegexp,
		WithGcsURI:     true,
		WithRbeURI:     true,
	}

	// Read artifacts.
	arts, token, err := q.FetchProtos(ctx)
	if err != nil {
		return nil, err
	}

	workUnitIDToProject := map[workunits.ID]string{}
	invocationIDToProject := map[invocations.ID]string{}
	if invID != "" {
		realm, err := invocations.ReadRealm(ctx, invID)
		if err != nil {
			return nil, err
		}
		project, _ := realms.Split(realm)
		invocationIDToProject[invID] = project
	} else {
		realm, err := workunits.ReadRealm(ctx, wuID)
		if err != nil {
			return nil, err
		}
		project, _ := realms.Split(realm)
		workUnitIDToProject[wuID] = project
	}

	if err := s.populateFetchURLs(ctx, workUnitIDToProject, invocationIDToProject, arts...); err != nil {
		return nil, err
	}

	return &pb.ListArtifactsResponse{
		Artifacts:     arts,
		NextPageToken: token,
	}, nil
}
