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

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/protobuf/field_mask"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var sha1Regex = regexp.MustCompile(`^[a-f0-9]{40}$`)

// defMask is the default field mask to use for GetBuild requests.
// Initialized by init.
var defMask mask.Mask

func init() {
	var err error
	defMask, err = mask.FromFieldMask(&field_mask.FieldMask{
		Paths: []string{
			"builder",
			"canary",
			"create_time",
			"created_by",
			"critical",
			"end_time",
			"id",
			"input.experimental",
			"input.gerrit_changes",
			"input.gitiles_commit",
			"number",
			"start_time",
			"status",
			"status_details",
			"update_time",
			// TODO(nodir): Add user_duration.
		},
	}, &pb.Build{}, false, false)
	if err != nil {
		panic(err)
	}
}

// TODO(crbug/1042991): Move to a common location.
func getFieldMask(fields *field_mask.FieldMask) (mask.Mask, error) {
	if len(fields.GetPaths()) == 0 {
		return defMask, nil
	}
	return mask.FromFieldMask(fields, &pb.Build{}, false, false)
}

// logAndReturnUnimplemented logs the method called, the proto response, and any
// error, but returns that the called method was unimplemented. Used to aid in
// development. Users of this function must ensure called methods do not have
// any side-effects. When removing this function, remember to ensure all methods
// have correct ACLs checks.
// TODO(crbug/1042991): Remove once methods are implemented.
func logAndReturnUnimplemented(ctx context.Context, methodName string, rsp proto.Message, err error) error {
	err = appstatus.GRPCifyAndLog(ctx, err)
	if methodName == "GetBuild" {
		logging.Debugf(ctx, "%q is returning %q with response %s", methodName, err, proto.MarshalTextString(rsp))
		return err
	}
	logging.Debugf(ctx, "%q would have returned %q with response %s", methodName, err, proto.MarshalTextString(rsp))
	return appstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// Builds implements pb.BuildsServer.
type Builds struct {
}

// Ensure Builds implements projects.ProjectsServer.
var _ pb.BuildsServer = &Builds{}

// Batch handles a batch request. Implements pb.BuildsServer.
func (*Builds) Batch(ctx context.Context, req *pb.BatchRequest) (*pb.BatchResponse, error) {
	return nil, appstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// ScheduleBuild handles a request to schedule a build. Implements pb.BuildsServer.
func (*Builds) ScheduleBuild(ctx context.Context, req *pb.ScheduleBuildRequest) (*pb.Build, error) {
	return nil, appstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// UpdateBuild handles a request to update a build. Implements pb.UpdateBuild.
func (*Builds) UpdateBuild(ctx context.Context, req *pb.UpdateBuildRequest) (*pb.Build, error) {
	return nil, appstatus.Errorf(codes.Unimplemented, "method not implemented")
}

// NewBuilds returns a new pb.BuildsServer.
func NewBuilds() pb.BuildsServer {
	return &pb.DecoratedBuilds{
		Prelude:  logDetails,
		Service:  &Builds{},
		Postlude: logAndReturnUnimplemented,
	}
}
