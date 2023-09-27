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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/appstatus"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/run"
)

// GetCLRunInfo implements GerritIntegrationServer; it returns ongoing Run information related to the given CL.
func (g *GerritIntegrationServer) GetCLRunInfo(ctx context.Context, req *apiv0pb.GetCLRunInfoRequest) (resp *apiv0pb.GetCLRunInfoResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	// TODO(crbug.com/1486976): Extend check to allow all requests from Gerrit.
	if err := checkCanUseAPI(ctx, "GetCLRunInfo"); err != nil {
		return nil, err
	}

	gc := req.GetGerritChange()
	eid, err := changelist.GobID(gc.GetHost(), gc.GetChange())
	if err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "invalid GerritChange %v: %s", gc, err)
	}

	clids, err := changelist.Lookup(ctx, []changelist.ExternalID{eid})
	if err != nil {
		return nil, err
	}
	clid := clids[0]
	if clid == 0 {
		// changelist.Lookup returns 0 for unknown external ID, i.e. CL not found.
		// In that case, no change was found; return empty response.
		return nil, appstatus.Errorf(codes.NotFound, "change %s not found", eid)
	}

	qb := run.CLQueryBuilder{CLID: clid}
	runs, _, err := qb.LoadRuns(ctx)
	if err != nil {
		return nil, err
	}

	// Filter out finished runs.
	ongoingRuns := []*run.Run{}
	for _, r := range runs {
		if !run.IsEnded(r.Status) {
			ongoingRuns = append(ongoingRuns, r)
		}
	}

	respRunInfo := populateRunInfo(ctx, ongoingRuns)

	// TODO(crbug.com/1486976): Query for dependency CLs.
	return &apiv0pb.GetCLRunInfoResponse{
		// TODO(crbug.com/1486976): Split RunInfo into RunsAsOrigin and RunsAsDep.
		RunsAsOrigin:   respRunInfo,
		RunsAsDep:      respRunInfo,
		DepChangeInfos: nil,
	}, nil
}
