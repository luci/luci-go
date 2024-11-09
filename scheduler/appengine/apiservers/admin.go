// Copyright 2018 The LUCI Authors.
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

package apiservers

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	schedulerpb "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
	"go.chromium.org/luci/scheduler/appengine/internal"
)

// AdminServer implements internal.admin.Admin API.
type AdminServer struct {
	internal.UnimplementedAdminServer

	Engine     engine.EngineInternal
	Catalog    catalog.Catalog
	AdminGroup string
}

// GetDebugJobState implements the corresponding RPC method.
func (s *AdminServer) GetDebugJobState(ctx context.Context, r *schedulerpb.JobRef) (resp *internal.DebugJobState, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()
	if err := s.checkAdmin(ctx, "GetDebugJobState"); err != nil {
		return nil, err
	}

	switch state, err := s.Engine.GetDebugJobState(ctx, r.Project+"/"+r.Job); {
	case err == engine.ErrNoSuchJob:
		return nil, status.Errorf(codes.NotFound, "no such job")
	case err != nil:
		return nil, err
	default:
		return &internal.DebugJobState{
			Enabled:    state.Job.Enabled,
			Paused:     state.Job.Paused,
			LastTriage: timestamppb.New(state.Job.LastTriage),
			CronState: &internal.DebugJobState_CronState{
				Enabled:       state.Job.Cron.Enabled,
				Generation:    state.Job.Cron.Generation,
				LastRewind:    timestamppb.New(state.Job.Cron.LastRewind),
				LastTickWhen:  timestamppb.New(state.Job.Cron.LastTick.When),
				LastTickNonce: state.Job.Cron.LastTick.TickNonce,
			},
			ManagerState:        state.ManagerState,
			ActiveInvocations:   state.Job.ActiveInvocations,
			FinishedInvocations: state.FinishedInvocations,
			RecentlyFinishedSet: state.RecentlyFinishedSet,
			PendingTriggersSet:  state.PendingTriggersSet,
		}, nil
	}
}

// checkAdmin verifies the caller is in the administrators group.
func (s *AdminServer) checkAdmin(ctx context.Context, methodName string) error {
	caller := auth.CurrentIdentity(ctx)
	logging.Warningf(ctx, "Admin call %q by %q", methodName, caller)
	switch yes, err := auth.IsMember(ctx, s.AdminGroup); {
	case err != nil:
		return status.Errorf(codes.Internal, "failed to check ACL")
	case !yes:
		return status.Errorf(codes.PermissionDenied, "not an administrator")
	default:
		return nil
	}
}
