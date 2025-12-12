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
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

// validateQueryArtifactsRequest returns a non-nil error if req is determined
// to be invalid.
func validateQueryArtifactsRequest(req *pb.QueryArtifactsRequest) error {
	if err := pbutil.ValidateArtifactPredicate(req.Predicate, req.Parent != ""); err != nil {
		return errors.Fmt("predicate: %w", err)
	}

	if len(req.Invocations) == 0 && req.Parent == "" {
		return errors.New("invocations or parent: must be specified")
	}

	if len(req.Invocations) > 0 && req.Parent != "" {
		return errors.New("invocations and parent: mutually exclusive")
	}

	for _, name := range req.Invocations {
		if err := pbutil.ValidateInvocationName(name); err != nil {
			return errors.Fmt("invocations: %q: %w", name, err)
		}
	}

	if req.Parent != "" {
		if err := pbutil.ValidateRootInvocationName(req.Parent); err != nil {
			return errors.Fmt("parent: %q: %w", req.Parent, err)
		}
	}

	if err := pagination.ValidatePageSize(req.PageSize); err != nil {
		return errors.Fmt("page_size: %w", err)
	}
	return nil
}

// QueryArtifacts implements pb.ResultDBServer.
func (s *resultDBServer) QueryArtifacts(ctx context.Context, in *pb.QueryArtifactsRequest) (*pb.QueryArtifactsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := permissions.VerifyInvocationsByName(ctx, in.Invocations, rdbperms.PermListArtifacts); err != nil {
		return nil, err
	}

	if err := validateQueryArtifactsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	readMask, err := QueryMask(in.ReadMask)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Query is valid - increment the queryInvocationsCount metric
	queryInvocationsCount.Add(ctx, 1, "QueryArtifacts", len(in.Invocations))

	var reachableInvIDs invocations.IDSet
	var reachableInvs graph.ReachableInvocations
	if in.Parent != "" {
		// For the root invocation.
		reachableInvIDs, reachableInvs, err = resolveRootInvocation(ctx, in)
	} else {
		// For the legacy invocations.
		reachableInvIDs, reachableInvs, err = resolveLegacyInvocations(ctx, in)
	}
	if err != nil {
		return nil, err
	}

	// Query artifacts.
	q := &artifacts.Query{
		InvocationIDs:       reachableInvIDs,
		TestResultPredicate: in.GetPredicate().GetTestResultPredicate(),
		PageSize:            pagination.AdjustPageSize(in.PageSize),
		PageToken:           in.PageToken,
		FollowEdges:         in.GetPredicate().GetFollowEdges(),
		ContentTypeRegexp:   in.GetPredicate().GetContentTypeRegexp(),
		ArtifactIDRegexp:    in.GetPredicate().GetArtifactIdRegexp(),
		ArtifactTypeRegexp:  in.GetPredicate().GetArtifactTypeRegexp(),
		WithGcsURI:          true,
		WithRbeURI:          true,
		Mask:                readMask,
	}

	// Handle ArtifactKind.
	if in.Predicate.GetArtifactKind() == pb.ArtifactPredicate_WORK_UNIT {
		// Only return work unit level artifacts.
		q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{
			IncludedInvocations: true,
			TestResults:         false,
		}
	}

	arts, token, err := q.FetchProtos(ctx)
	if err != nil {
		return nil, err
	}

	workUnitIDToProject := make(map[workunits.ID]string, len(reachableInvs.Invocations))
	invocationIDToProject := make(map[invocations.ID]string, len(reachableInvs.Invocations))
	for invID, ri := range reachableInvs.Invocations {
		p, _ := realms.Split(ri.Realm)
		invocationIDToProject[invID] = p
		if invID.IsWorkUnit() {
			wuID := workunits.MustParseLegacyInvocationID(invID)
			workUnitIDToProject[wuID] = p
		}
	}

	includeFetchURL, err := readMask.Includes("fetch_url")
	if err != nil {
		return nil, err
	}
	if includeFetchURL == mask.IncludeEntirely {
		if err := s.populateFetchURLs(ctx, workUnitIDToProject, invocationIDToProject, arts...); err != nil {
			return nil, err
		}
	}
	return &pb.QueryArtifactsResponse{
		Artifacts:     arts,
		NextPageToken: token,
	}, nil
}

// QueryMask returns mask.Mask converted from field_mask.FieldMask.
// It returns a default mask with all fields if readMask is empty.
func QueryMask(readMask *field_mask.FieldMask) (*mask.Mask, error) {
	if len(readMask.GetPaths()) == 0 {
		return mask.All(&pb.Artifact{}), nil
	}
	return mask.FromFieldMask(readMask, &pb.Artifact{}, mask.AdvancedSemantics())
}

// resolveRootInvocation resolves the reachable invocations for a query that
// specifies a parent root invocation.
// If work units are specified in the predicate, it filters the reachable
// invocations to only those work units.
func resolveRootInvocation(ctx context.Context, in *pb.QueryArtifactsRequest) (invocations.IDSet, graph.ReachableInvocations, error) {
	rootInvID, err := rootinvocations.ParseName(in.Parent)
	if err != nil {
		return nil, graph.ReachableInvocations{}, appstatus.BadRequest(errors.Fmt("parent: %q: %w", in.Parent, err))
	}

	if len(in.GetPredicate().GetWorkUnits()) > 0 {
		// Filter by included work units if specified.
		wuInvs := invocations.NewIDSet()
		for _, name := range in.Predicate.WorkUnits {
			id, err := workunits.ParseName(name)
			if err != nil {
				return nil, graph.ReachableInvocations{}, appstatus.BadRequest(errors.Fmt("predicate.work_units: %q: %w", name, err))
			}
			if id.RootInvocationID != rootInvID {
				return nil, graph.ReachableInvocations{}, appstatus.BadRequest(errors.Fmt("predicate.work_units: %q is not in parent root invocation %q", name, in.Parent))
			}
			wuInvs.Add(id.LegacyInvocationID())
		}

		// For non-recursive queries, we still need to know the realm of the
		// invocations to populate fetch URLs.
		realms, err := invocations.ReadRealms(ctx, wuInvs)
		if err != nil {
			return nil, graph.ReachableInvocations{}, err
		}
		reachableInvs := graph.NewReachableInvocations()
		for id, realm := range realms {
			reachableInvs.Invocations[id] = graph.ReachableInvocation{
				Realm: realm,
			}
		}
		return wuInvs, reachableInvs, nil
	}

	// Traverse the graph to find all reachable invocations from the root.
	invs := invocations.NewIDSet(rootInvID.LegacyInvocationID())
	reachableInvs, err := graph.Reachable(ctx, invs)
	if err != nil {
		return nil, graph.ReachableInvocations{}, err
	}
	reachableInvIDs, err := reachableInvs.IDSet()
	if err != nil {
		return nil, graph.ReachableInvocations{}, err
	}
	return reachableInvIDs, reachableInvs, nil
}

// resolveLegacyInvocations resolves the reachable invocations for a query that
// specifies a set of legacy invocations.
// It always traverses the inclusion graph to find all reachable invocations.
func resolveLegacyInvocations(ctx context.Context, in *pb.QueryArtifactsRequest) (invocations.IDSet, graph.ReachableInvocations, error) {
	invs := invocations.MustParseNames(in.Invocations)
	reachableInvs, err := graph.Reachable(ctx, invs)
	if err != nil {
		return nil, graph.ReachableInvocations{}, err
	}

	reachableInvIDs, err := reachableInvs.IDSet()
	if err != nil {
		return nil, graph.ReachableInvocations{}, err
	}
	return reachableInvIDs, reachableInvs, nil
}
