// Copyright 2022 The LUCI Authors.
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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

// GetRun fetches information about one Run given a public Run ID.
func (s *RunsServer) GetRun(ctx context.Context, req *apiv0pb.GetRunRequest) (resp *apiv0pb.Run, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkCanUseAPI(ctx, "Runs.GetRun"); err != nil {
		return
	}

	id, err := toInternalRunID(req.GetId())
	if err != nil {
		return nil, err
	}

	r, err := run.LoadRun(ctx, id, acls.NewRunReadChecker())
	if err != nil {
		return nil, err
	}

	ctx = logging.SetField(ctx, "run", r.ID)
	return populateRunResponse(ctx, r)
}

func toInternalRunID(id string) (common.RunID, error) {
	if id == "" {
		return "", appstatus.Errorf(codes.InvalidArgument, "Run ID is required")
	}
	internalID, err := common.FromPublicRunID(id)
	if err != nil {
		return "", appstatus.Errorf(codes.InvalidArgument, "%s", err)
	}
	return internalID, nil
}
