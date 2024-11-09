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

	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

// GerritIntegrationServer implements the v0 API.
type GerritIntegrationServer struct {
	apiv0pb.UnimplementedGerritIntegrationServer
}

// populateRunInfo converts run.Runs to apiv0pb.GetCLRunInfoResponse_RunInfos for the response.
func populateRunInfo(ctx context.Context, runs []*run.Run) ([]*apiv0pb.GetCLRunInfoResponse_RunInfo, error) {
	respRuns := make([]*apiv0pb.GetCLRunInfoResponse_RunInfo, len(runs))
	errs := parallel.WorkPool(min(len(runs), 16), func(work chan<- func() error) {
		for i, r := range runs {
			i, r := i, r
			work <- func() (err error) {
				respRuns[i], err = populateRunInfoResponse(ctx, r)
				return err
			}
		}
	})
	return respRuns, common.MostSevereError(errs)
}

// populateRunInfoResponse constructs and populates a
// apiv0pb.GetCLRunInfoResponse_RunInfo to use in a response.
//
// This includes fetching and populating extra information, including CL info
// to fill in details in each apiv0pb.GetCLRunInfoResponse_RunInfo.
func populateRunInfoResponse(ctx context.Context, r *run.Run) (*apiv0pb.GetCLRunInfoResponse_RunInfo, error) {
	var originChange *apiv0pb.GerritChange
	if r.RootCL != 0 {
		// Fetch the origin CL.
		originCL := &changelist.CL{ID: r.RootCL}
		if err := datastore.Get(ctx, originCL); err != nil {
			return nil, err
		}

		originGerrit := originCL.Snapshot.GetGerrit()
		if originGerrit == nil {
			return nil, fmt.Errorf("root CL %d has non-Gerrit snapshot", r.RootCL)
		}

		originChange = &apiv0pb.GerritChange{
			Host:     originGerrit.Host,
			Change:   originGerrit.Info.Number,
			Patchset: originCL.Snapshot.Patchset,
		}
	}

	return &apiv0pb.GetCLRunInfoResponse_RunInfo{
		Id:           r.ID.PublicID(),
		CreateTime:   common.Time2PBNillable(r.CreateTime),
		StartTime:    common.Time2PBNillable(r.StartTime),
		OriginChange: originChange,
		Mode:         string(r.Mode),
	}, nil
}
