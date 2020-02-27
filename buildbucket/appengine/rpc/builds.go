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
	"regexp"
	"strings"

	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/model"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

// TODO(crbug/1042991): Move to a common location.
const (
	projRegex    = "^[a-z0-9\\-_]+$"
	bucketRegex  = "^[a-z0-9_\\-.]{1,100}$"
	builderRegex = "^[a-zA-Z0-9_\\-. ]{1,128}$"
)

// logDetails logs debug information about the request.
func logDetails(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	logging.Debugf(ctx, "%q called %q with request %s", auth.CurrentIdentity(ctx), methodName, req)
	return ctx, nil
}

// logAndReturnUnimplemented logs the method called, the proto response, and any
// error, but returns that the called method was unimplemented. Used to aid in
// development. Users of this function must ensure called methods do not have
// any side-effects. When removing this function, remember to ensure all methods
// have correct ACLs checks.
// TODO(crbug/1042991): Remove once methods are implemented.
func logAndReturnUnimplemented(ctx context.Context, methodName string, rsp proto.Message, err error) error {
	logging.Debugf(ctx, "%q would have returned %q with response %s", methodName, err, rsp)
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

// GetBuild handles a request to retrieve a build. Implements buildbucketpb.BuildsServer.
func (*Builds) GetBuild(ctx context.Context, req *buildbucketpb.GetBuildRequest) (*buildbucketpb.Build, error) {
	switch {
	case req.GetId() != 0:
		if req.Builder != nil || req.BuildNumber != 0 {
			return nil, status.Errorf(codes.InvalidArgument, "id is mutually exclusive with (builder and build_number)")
		}
	case req.GetBuilder() != nil && req.BuildNumber != 0:
		// TODO(crbug/1042991): Move buildbucketpb.BuilderID validation to a common location.
		projMatch := regexp.MustCompile(projRegex)
		bucketMatch := regexp.MustCompile(bucketRegex)
		builderMatch := regexp.MustCompile(builderRegex)
		switch parts := strings.Split(req.Builder.Bucket, "."); {
		case !projMatch.MatchString(req.Builder.Project):
			return nil, status.Errorf(codes.InvalidArgument, "builder.project must match %q", projRegex)
		case !bucketMatch.MatchString(req.Builder.Bucket):
			return nil, status.Errorf(codes.InvalidArgument, "builder.bucket must match %q", bucketRegex)
		case !builderMatch.MatchString(req.Builder.Builder):
			return nil, status.Errorf(codes.InvalidArgument, "builder.builder must match %q", builderRegex)
		case parts[0] == "luci" && len(parts) > 2:
			return nil, status.Errorf(codes.InvalidArgument, "invalid use of v1 builder.bucket in v2 API (hint: try %q)", parts[2])
		}
	default:
		return nil, status.Errorf(codes.InvalidArgument, "one of id or (builder and build_number) is required")
	}
	// TODO(crbug/1042991): Check that the user can view this build.
	if req.Id > 0 {
		logging.Infof(ctx, "GetBuild by ID: %q", req)
		ent := &model.Build{
			ID: req.Id,
		}
		err := datastore.Get(ctx, ent)
		switch err {
		case nil:
		case datastore.ErrNoSuchEntity:
			return nil, status.Errorf(codes.NotFound, "not found")
		default:
			return nil, errors.Annotate(err, "error fetching build with ID %d", req.Id).Err()
		}
		return &buildbucketpb.Build{
			Builder: &buildbucketpb.BuilderID{
				Project: ent.Project,
				Bucket:  ent.BucketID,
				Builder: ent.BuilderID,
			},
			Id: ent.ID,
		}, nil
	}
	// TODO(crbug/1042991): Implement get by builder/build number.
	return &buildbucketpb.Build{}, nil
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
