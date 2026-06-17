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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/turboci/value"
	executorpb "go.chromium.org/turboci/proto/go/graph/executor/v1"

	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// CancelStage implements executorgrpcpb.TurboCIStageExecutorServer.
func (*TurboCIStageExecutor) CancelStage(ctx context.Context, req *executorpb.CancelStageRequest) (*executorpb.CancelStageResponse, error) {
	TurboCICall(ctx).LogDetails(ctx)

	stage := req.GetStage()
	attempt := stage.GetAttempts()[len(stage.GetAttempts())-1]

	// Extract the build ID from attempt details. It must be there, since only
	// a SCHEDULED or RUNNING attempt can be cancelled and we put the build
	// ID as a detail when switching into SCHEDULED/RUNNING states.
	details, err := value.Lookup[*pb.BuildStageDetails](
		value.SimpleDataSource(req.GetValueData()),
		attempt.GetDetails(),
	)
	if err != nil {
		return nil, appstatus.Errorf(codes.Internal, "could not decode BuildStageDetails: %s", err)
	}
	if details == nil {
		return nil, appstatus.Errorf(codes.Internal, "no BuildStageDetails in the attempt details")
	}
	bID := details.GetId()
	if bID == 0 {
		return nil, appstatus.Errorf(codes.Internal, "unexpected BuildStageDetails: %v", details)
	}

	// Initiate graceful build cancellation. Note we don't need to check any
	// ACLs here since Turbo CI Orchestrator already did that.
	bld, err := tasks.StartCancel(ctx, bID, &tasks.CancellationDetails{
		CanceledBy: "turboci",
		Summary:    "Cancelled via Turbo CI API",
	})
	if err != nil {
		return nil, err
	}

	// Acknowledge we processed the cancellation by switching the attempt into
	// the TEARING_DOWN state. If the attempt is already COMPLETE/INCOMPLETE, this
	// will just do nothing. Once the build terminates, Buildbucket will switch
	// the attempt into COMPLETE/INCOMPLETE state as usual.
	creds, err := turboci.ProjectRPCCredentials(ctx, bld.Project)
	if err != nil {
		return nil, appstatus.Errorf(codes.Internal, "project credentials: %s", err)
	}
	err = (&turboci.Client{
		Creds: creds,
		Token: req.GetStageAttemptToken(),
		Build: bld.Proto,
	}).SwitchAttemptToTearingDown(ctx)
	return &executorpb.CancelStageResponse{}, err
}
