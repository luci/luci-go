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

package rpc

import (
	"context"

	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

// logDetails logs debug information about the request.
func logDetails(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	logging.Debugf(ctx, "%q called %q with request %s", auth.CurrentIdentity(ctx), methodName, proto.MarshalTextString(req))
	return ctx, nil
}

// logAndReturnUnimplemented logs the method called, the proto response, and any
// error, but returns that the called method was unimplemented. Used to aid in
// development. Users of this function must ensure called methods do not have
// any side-effects. When removing this function, remember to ensure all methods
// have correct ACLs checks.
// TODO(crbug/1042991): Remove once methods are implemented.
func logAndReturnUnimplemented(ctx context.Context, methodName string, rsp proto.Message, err error) error {
	logging.Debugf(ctx, "%q would have returned %q with response %s", methodName, err, proto.MarshalTextString(rsp))
	return status.Errorf(codes.Unimplemented, "method not implemented")
}

// Builds implements buildbucketpb.BuildsServer.
type Builds struct {
}

// Ensure Builds implements projects.ProjectsServer.
var _ buildbucketpb.BuildsServer = &Builds{}

// Batch handles a batch request. Implements buildbucketpb.BuildsServer.
func (*Builds) Batch(ctx context.Context, req *buildbucketpb.BatchRequest) (*buildbucketpb.BatchResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

// CancelBuild handles a request to cancel a build. Implements buildbucketpb.BuildsServer.
func (*Builds) CancelBuild(ctx context.Context, req *buildbucketpb.CancelBuildRequest) (*buildbucketpb.Build, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

// SearchBuilds handles a request to search for builds. Implements buildbucketpb.BuildsServer.
func (*Builds) SearchBuilds(ctx context.Context, req *buildbucketpb.SearchBuildsRequest) (*buildbucketpb.SearchBuildsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

// ScheduleBuilds handles a request to schedule a build. Implements buildbucketpb.BuildsServer
func (*Builds) ScheduleBuild(ctx context.Context, req *buildbucketpb.ScheduleBuildRequest) (*buildbucketpb.Build, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

// UpdateBuilds handles a request to update a build. Implements buildbucketpb.UpdateBuild.
func (*Builds) UpdateBuild(ctx context.Context, req *buildbucketpb.UpdateBuildRequest) (*buildbucketpb.Build, error) {
	return nil, status.Errorf(codes.Unimplemented, "method not implemented")
}

// New returns a new buildbucketpb.BuildsServer.
func New() buildbucketpb.BuildsServer {
	return &buildbucketpb.DecoratedBuilds{
		Prelude:  logDetails,
		Service:  &Builds{},
		Postlude: logAndReturnUnimplemented,
	}
}
