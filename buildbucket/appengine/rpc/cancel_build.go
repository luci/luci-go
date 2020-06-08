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

	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// validateCancel validates the given request.
func validateCancel(req *pb.CancelBuildRequest) error {
	switch {
	case req.GetId() == 0:
		return appstatus.Errorf(codes.InvalidArgument, "id is required")
	case req.SummaryMarkdown == "":
		return appstatus.Errorf(codes.InvalidArgument, "summary_markdown is required")
	}
	return nil
}

// CancelBuild handles a request to cancel a build. Implements pb.BuildsServer.
func (*Builds) CancelBuild(ctx context.Context, req *pb.CancelBuildRequest) (*pb.Build, error) {
	if err := validateCancel(req); err != nil {
		return nil, err
	}
	m, err := getFieldMask(req.Fields)
	if err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "invalid field mask")
	}
	bld, bck, err := model.GetBuildAndBucket(ctx, req.Id)
	switch {
	case err == datastore.ErrNoSuchEntity:
		return nil, notFound(ctx)
	case err != nil:
		return nil, err
	}
	switch r, err := bck.GetRole(ctx); {
	case err != nil:
		return nil, err
	case r < pb.Acl_READER:
		return nil, notFound(ctx)
	case r < pb.Acl_WRITER:
		return nil, appstatus.Errorf(codes.PermissionDenied, "%q does not have permission to cancel builds in bucket %q", auth.CurrentIdentity(ctx), bld.BucketID)
	}
	// TODO(crbug/1042991): Cancel the build.
	return bld.ToProto(ctx, m)
}
