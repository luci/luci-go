// Copyright 2025 The LUCI Authors.
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
	"slices"
	"strings"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// parentsMap holds parent builds references by a batch of ScheduleBuildRequest.
//
// Either `fromRequests` or `fromToken` are present, but not both at the same
// time.
type parentsMap struct {
	fromRequests map[int64]*parent
	fromToken    *parent
}

type parent struct {
	bld   *model.Build
	infra *model.BuildInfra
	// ancestors including self.
	ancestors []int64
	pRunID    string
	err       error
}

// parentForRequest returns the parent of the build to create.
//
// If the request has ParentBuildId specified,
//   - returns the parent by ParentBuildId,
//   - will panic if missing parentsMap or missing parent from the map.
//
// If the request doesn't specify ParentBuildId, tries to get the parent from
// BUILD token. And missing parentsMap is allowed.
func (ps *parentsMap) parentForRequest(req *pb.ScheduleBuildRequest) *parent {
	if ps == nil {
		if req.GetParentBuildId() != 0 {
			panic(fmt.Sprintf("requested parent %d not found", req.ParentBuildId))
		}
		return nil
	}
	if req.GetParentBuildId() != 0 {
		p, ok := ps.fromRequests[req.ParentBuildId]
		if !ok {
			panic(fmt.Sprintf("requested parent %d not found", req.ParentBuildId))
		}
		return p
	}
	return ps.fromToken
}

// parentBuildForRequest returns a parent build for the given request.
func (ps *parentsMap) parentBuildForRequest(req *pb.ScheduleBuildRequest) (*model.Build, error) {
	p := ps.parentForRequest(req)
	switch {
	case p == nil:
		return nil, nil
	case p.err != nil:
		return nil, p.err
	default:
		return p.bld, nil
	}
}

// ancestorsForRequest returns a list of ancestor builds for the given request.
func (ps *parentsMap) ancestorsForRequest(req *pb.ScheduleBuildRequest) ([]int64, error) {
	p := ps.parentForRequest(req)
	switch {
	case p == nil:
		return nil, nil
	case p.err != nil:
		return nil, p.err
	default:
		return p.ancestors, nil
	}
}

// parentRunIDForRequest returns a parent RunID for the given request.
func (ps *parentsMap) parentRunIDForRequest(req *pb.ScheduleBuildRequest) (string, error) {
	p := ps.parentForRequest(req)
	switch {
	case p == nil:
		return "", nil
	case p.err != nil:
		return "", p.err
	default:
		return p.pRunID, nil
	}
}

// parentInfraForRequest returns a parent BuildInfra for the given request.
func (ps *parentsMap) parentInfraForRequest(req *pb.ScheduleBuildRequest) (*model.BuildInfra, error) {
	p := ps.parentForRequest(req)
	switch {
	case p == nil:
		return nil, nil
	case p.err != nil:
		return nil, p.err
	default:
		return p.infra, nil
	}
}

// populateParentFields fills in `p` based on fetch entities.
//
// Updates it to erroneous if the build has finished already.
func populateParentFields(p *parent, pBld *model.Build, pInfra *model.BuildInfra) {
	if pBld == nil || pInfra == nil {
		panic("impossible")
	}
	if protoutil.IsEnded(pBld.Proto.GetStatus()) || protoutil.IsEnded(pBld.Proto.GetOutput().GetStatus()) {
		p.err = appstatus.BadRequest(errors.Fmt("%d has ended, cannot add child to it", pBld.ID))
		return
	}

	p.bld = pBld
	p.infra = pInfra
	p.ancestors = append(slices.Clone(pBld.AncestorIds), pBld.ID)

	pTaskID := pInfra.Proto.GetBackend().GetTask().GetId()
	pTarget := pTaskID.GetTarget()
	if strings.HasPrefix(pTarget, "swarming://") {
		p.pRunID = pTaskID.GetId()
		if p.pRunID != "" {
			p.pRunID = p.pRunID[:len(p.pRunID)-1] + "1"
		}
	}
}

// validateParentViaToken validates the parent build referenced by the BUILD
// token in the request context.
//
// If there is no token present in `ctx` returns nil.
//
// Errors indicating incorrect tokens, broken tokens, non-BUILD tokens, missing
// builds, etc. all are returned status-annotated inside `p`.
func validateParentViaToken(ctx context.Context) *parent {
	p := &parent{}
	buildTok, err, hasToken := getBuildbucketToken(ctx, false)
	if errors.Is(err, errBadTokenAuth) {
		if hasToken {
			p.err = err
		}
		return p
	}

	// NOTE: We pass buildid == 0 here because we are relying on the token itself
	// to tell us what the parent build ID is. Do not do this in other locations
	// or they will be suceptible to accepting tokens generated for other builds.
	tok, err := buildtoken.ParseToTokenBody(ctx, buildTok, 0, pb.TokenBody_BUILD)
	if err != nil {
		// We don't return `err` here because it will include the Unauthenticated
		// gRPC tag, which isn't accurate.
		p.err = appstatus.BadRequest(errors.New("invalid parent buildbucket token"))
		return p
	}

	entities, err := common.GetBuildEntities(ctx, tok.BuildId, model.BuildKind, model.BuildInfraKind)
	if err != nil {
		p.err = err
		return p
	}
	populateParentFields(p, entities[0].(*model.Build), entities[1].(*model.BuildInfra))
	return p
}

// validateParents validates the given set of parent build(s), as extracted from
// a batch of ScheduleBuildRequests, as well as the build referenced by
// the BUILD token in the context.
//
// Verifies that if a token is present, then ScheduleBuildRequests do not
// explicitly set parent builds (i.e. `pIDs` is empty).
func validateParents(ctx context.Context, pIDs []int64) (*parentsMap, error) {
	p := validateParentViaToken(ctx)
	switch {
	case p.err != nil:
		return nil, p.err
	case p.bld == nil && len(pIDs) == 0:
		return nil, nil
	case p.bld != nil && len(pIDs) > 0:
		return nil, appstatus.BadRequest(errors.New("parent buildbucket token and parent_build_id are mutually exclusive"))
	case p.bld != nil:
		return &parentsMap{fromToken: p}, nil
	}

	// Get parent builds using pIDs.
	ps := make(map[int64]*parent, len(pIDs))
	blds := make([]*model.Build, len(pIDs))
	infras := make([]*model.BuildInfra, len(pIDs))
	for i, pID := range pIDs {
		ps[pID] = &parent{}
		b := &model.Build{ID: pID}
		blds[i] = b
		infras[i] = &model.BuildInfra{Build: datastore.KeyForObj(ctx, b)}
	}

	asMultiErr := func(err error) errors.MultiError {
		var me errors.MultiError
		if !errors.As(err, &me) {
			me = make(errors.MultiError, len(pIDs))
			for i := range pIDs {
				me[i] = err
			}
		}
		return me
	}

	if err := datastore.Get(ctx, blds, infras); err != nil {
		var merr errors.MultiError
		if !errors.As(err, &merr) {
			logging.Errorf(ctx, "Failed to fetch parent builds %q: %s", pIDs, err)
			return nil, appstatus.Error(codes.Internal, "failed to fetch parent builds")
		}
		bldsMErr := asMultiErr(merr[0])
		infrasMErr := asMultiErr(merr[1])
		mergeErrs(bldsMErr, infrasMErr, "BuildInfra", func(i int) int { return i })
		for i, pID := range pIDs {
			err := bldsMErr[i]
			if err != nil {
				if errors.Is(err, datastore.ErrNoSuchEntity) {
					ps[pID].err = perm.NotFoundErr(ctx)
				} else {
					logging.Errorf(ctx, "failed to fetch parent build %d, %s", pID, err)
					ps[pID].err = appstatus.Errorf(codes.Internal, "failed to fetch parent build %d", pID)
				}
			}
		}
	}

	for i, pID := range pIDs {
		p := ps[pID]
		if p.err == nil {
			populateParentFields(p, blds[i], infras[i])
		}
	}
	return &parentsMap{fromRequests: ps}, nil
}
