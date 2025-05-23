// Copyright 2023 The LUCI Authors.
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
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/gerritauth"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runquery"
)

// GetCLRunInfo implements GerritIntegrationServer; it returns ongoing Run information related to the given CL.
func (g *GerritIntegrationServer) GetCLRunInfo(ctx context.Context, req *apiv0pb.GetCLRunInfoRequest) (resp *apiv0pb.GetCLRunInfoResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	gc := req.GetGerritChange()
	eid, err := changelist.GobID(gc.GetHost(), gc.GetChange())
	if err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "invalid GerritChange %v: %s", gc, err)
	}

	if gerritInfo := gerritauth.GetAssertedInfo(ctx); gerritInfo == nil {
		if err := checkCanUseAPI(ctx, "GetCLRunInfo"); err != nil {
			return nil, err
		}
	} else {
		// If Gerrit JWT was provided, check that it matches the request change.
		// The JWT-provided host does not include the "-review" prefix.
		jwtChange := gerritInfo.Change
		if jwtChange.Host+"-review.googlesource.com" != gc.GetHost() || jwtChange.ChangeNumber != gc.GetChange() {
			return nil, appstatus.Errorf(codes.InvalidArgument, "JWT change does not match GerritChange %v: got %s/%d", gc, jwtChange.Host, jwtChange.ChangeNumber)
		}

		preferredEmail := gerritInfo.User.PreferredEmail
		if preferredEmail == "" {
			logging.Warningf(ctx, "jwt provided but user preferred email is missing")
			return &apiv0pb.GetCLRunInfoResponse{}, nil
		}
		userID, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, preferredEmail))
		if err != nil {
			return nil, appstatus.Attachf(err, codes.InvalidArgument, "failed to construct user identity")
		}
		if !common.IsInstantTriggerDogfooder(ctx, userID) {
			return &apiv0pb.GetCLRunInfoResponse{}, nil
		}
	}

	cl, err := eid.Load(ctx)
	switch {
	case err != nil:
		return nil, err
	case cl == nil:
		return nil, appstatus.Errorf(codes.NotFound, "change %s not found", eid)
	}

	qb := runquery.CLQueryBuilder{CLID: cl.ID}
	runs, _, err := qb.LoadRuns(ctx)
	if err != nil {
		return nil, err
	}

	respRunInfo, err := populateRunInfo(ctx, filterOngoingRuns(runs))
	if err != nil {
		return nil, err
	}

	depChangeInfos, err := queryDepChangeInfos(ctx, cl)
	if err != nil {
		return nil, err
	}

	return &apiv0pb.GetCLRunInfoResponse{
		// TODO(crbug.com/1486976): Split RunInfo into RunsAsOrigin and RunsAsDep.
		RunsAsOrigin:   respRunInfo,
		RunsAsDep:      respRunInfo,
		DepChangeInfos: depChangeInfos,
	}, nil
}

// queryDepChangeInfos queries for dependent CLs.
func queryDepChangeInfos(ctx context.Context, cl *changelist.CL) ([]*apiv0pb.GetCLRunInfoResponse_DepChangeInfo, error) {
	if len(cl.Snapshot.Deps) == 0 {
		return nil, nil
	}

	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(8)
	infos := make([]*apiv0pb.GetCLRunInfoResponse_DepChangeInfo, 0, len(cl.Snapshot.Deps))
	var infosMu sync.Mutex
	for _, dep := range cl.Snapshot.Deps {
		eg.Go(func() error {
			depClid := common.CLID(dep.Clid)

			depCl := &changelist.CL{ID: depClid}
			if err := datastore.Get(ectx, depCl); err != nil {
				return err
			}
			gerrit := depCl.Snapshot.GetGerrit()
			switch {
			case gerrit == nil:
				return fmt.Errorf("dep CL %d has non-Gerrit snapshot", depClid)
			case gerrit.GetInfo().GetStatus() != gerritpb.ChangeStatus_NEW:
				return nil // only returns active CLs
			}

			// Query for runs.
			qb := runquery.CLQueryBuilder{CLID: depClid}
			runs, _, err := qb.LoadRuns(ectx)
			if err != nil {
				return nil
			}
			runInfo, err := populateRunInfo(ectx, filterOngoingRuns(runs))
			if err != nil {
				return err
			}
			infosMu.Lock()
			infos = append(infos, &apiv0pb.GetCLRunInfoResponse_DepChangeInfo{
				GerritChange: &apiv0pb.GerritChange{
					Host:     gerrit.Host,
					Change:   gerrit.Info.Number,
					Patchset: depCl.Snapshot.Patchset,
				},
				Runs:        runInfo,
				ChangeOwner: gerrit.Info.Owner.Email,
			})
			infosMu.Unlock()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, appstatus.Errorf(codes.Internal, "%s", err)
	}

	sort.Slice(infos, func(i, j int) bool {
		if infos[i].GetGerritChange().GetHost() == infos[j].GetGerritChange().GetHost() {
			return infos[i].GetGerritChange().GetChange() < infos[j].GetGerritChange().GetChange()
		}
		return strings.Compare(infos[i].GetGerritChange().GetHost(), infos[j].GetGerritChange().GetHost()) < 0
	})
	return infos, nil
}

// filterOngoingRuns filters out ended runs.
func filterOngoingRuns(runs []*run.Run) []*run.Run {
	ongoingRuns := []*run.Run{}
	for _, r := range runs {
		if !run.IsEnded(r.Status) {
			ongoingRuns = append(ongoingRuns, r)
		}
	}
	return ongoingRuns
}
