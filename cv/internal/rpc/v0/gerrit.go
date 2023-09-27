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

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

// GerritIntegrationServer implements the v0 API.
type GerritIntegrationServer struct {
	apiv0pb.UnimplementedGerritIntegrationServer
}

// populateRunInfo converts run.Runs to apiv0pb.GetCLRunInfoResponse_RunInfos for the response.
func populateRunInfo(ctx context.Context, runs []*run.Run) []*apiv0pb.GetCLRunInfoResponse_RunInfo {
	respRuns := make([]*apiv0pb.GetCLRunInfoResponse_RunInfo, len(runs))
	for i, r := range runs {
		respRuns[i] = populateRunInfoResponse(ctx, r)
	}
	return respRuns
}

// populateRunInfoResponse constructs and populates a apiv0pb.GetCLRunInfoResponse_RunInfo to use in a response.
func populateRunInfoResponse(ctx context.Context, r *run.Run) *apiv0pb.GetCLRunInfoResponse_RunInfo {
	return &apiv0pb.GetCLRunInfoResponse_RunInfo{
		Id:           r.ID.PublicID(),
		CreateTime:   common.Time2PBNillable(r.CreateTime),
		StartTime:    common.Time2PBNillable(r.StartTime),
		OriginChange: nil, // TODO(crbug.com/1486976): Implement.
		Mode:         string(r.Mode),
	}
}
