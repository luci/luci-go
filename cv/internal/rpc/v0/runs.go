// Copyright 2021 The LUCI Authors.
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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

// TODO: change this group to a v0 API dedicated group, like "v0-api-allowed-users".
const allowGroup = "service-luci-change-verifier-admins"

// RunsServer implements rpc v0 APIs.
type RunsServer struct {
	apiv0pb.UnimplementedRunsServer
}

// GetRun returns the Run.
func (s *RunsServer) GetRun(ctx context.Context, req *apiv0pb.GetRunRequest) (resp *apiv0pb.Run, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "Runs.GetRun"); err != nil {
		return
	}

	id, err := toInternalRunID(req.GetId())
	if err != nil {
		return nil, err
	}

	r := run.Run{ID: id}
	switch err := datastore.Get(ctx, &r); {
	case err == datastore.ErrNoSuchEntity:
		return nil, appstatus.Errorf(codes.NotFound, "run not found")
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch Run").Tag(transient.Tag).Err()
	}

	rcls, err := run.LoadRunCLs(ctx, r.ID, r.CLs)
	if err != nil {
		return nil, err
	}
	gcls := make([]*apiv0pb.GerritChange, len(rcls))
	for i, rcl := range rcls {
		host, change, err := rcl.ExternalID.ParseGobID()
		if err != nil {
			// As of Sep 2, 2021, CV works only with Gerrit (GoB) CL.
			panic(errors.Annotate(err, "ParseGobID").Err())
		}
		gcls[i] = &apiv0pb.GerritChange{
			Host:     host,
			Change:   change,
			Patchset: rcl.Detail.GetPatchset(),
		}
	}

	// TODO(crbug/1233963): check if user has access to this specific Run.
	return &apiv0pb.Run{
		Id:         r.ID.PublicID(),
		Eversion:   int64(r.EVersion),
		Status:     apiv0pb.Run_Status(r.Status),
		Mode:       string(r.Mode),
		CreateTime: common.TspbNillable(r.CreateTime),
		StartTime:  common.TspbNillable(r.StartTime),
		UpdateTime: common.TspbNillable(r.UpdateTime),
		EndTime:    common.TspbNillable(r.EndTime),
		Owner:      string(r.Owner),
		Cls:        gcls,
	}, nil
}

func checkAllowed(ctx context.Context, name string) error {
	// TODO(crbug/1233963): delete this after proper Run ACLs are implemented.
	switch yes, err := auth.IsMember(ctx, allowGroup); {
	case err != nil:
		return appstatus.Errorf(codes.Internal, "failed to check ACL")
	case !yes:
		return appstatus.Errorf(codes.PermissionDenied, "not a member of %s", allowGroup)
	default:
		logging.Warningf(ctx, "%s is calling %s", auth.CurrentIdentity(ctx), name)
		return nil
	}
}

func toInternalRunID(id string) (common.RunID, error) {
	if id == "" {
		return "", appstatus.Errorf(codes.InvalidArgument, "Run ID is required")
	}
	internalID, err := common.FromPublicRunID(id)
	if err != nil {
		return "", appstatus.Errorf(codes.InvalidArgument, err.Error())
	}
	return internalID, nil
}
