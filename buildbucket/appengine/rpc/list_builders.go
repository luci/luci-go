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

	"go.chromium.org/gae/service/datastore"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/proto/paged"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// validateListBuilders validates the given request.
func validateListBuilders(req *pb.ListBuildersRequest) error {
	if err := protoutil.ValidateBuilderID(&pb.BuilderID{Project: req.Project, Bucket: req.Bucket}); err != nil {
		return err
	}

	return validatePageSize(req.PageSize)
}

// ListBuilders handles a request to retrieve builders. Implements pb.BuildersServer.
func (*Builders) ListBuilders(ctx context.Context, req *pb.ListBuildersRequest) (*pb.ListBuildersResponse, error) {
	if err := validateListBuilders(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	// TODO(crbug.com/1091489): add support for project-wide search.
	if req.Bucket == "" {
		return nil, appstatus.Errorf(codes.Unimplemented, "request without bucket is not supported yet")
	}

	if err := canRead(ctx, req.Project, req.Bucket); err != nil {
		return nil, err
	}

	// Fetch the builders.
	q := datastore.NewQuery(model.BuilderKind).
		Ancestor(model.BucketKey(ctx, req.Project, req.Bucket))

	pageSize := req.PageSize
	switch {
	case pageSize <= 0:
		pageSize = 100
	case pageSize > 1000:
		pageSize = 1000
	}

	res := &pb.ListBuildersResponse{}
	err := paged.Query(ctx, pageSize, req.GetPageToken(), res, q, func(b *model.Builder) error {
		res.Builders = append(res.Builders, &pb.BuilderItem{
			Id: &pb.BuilderID{
				Project: req.Project,
				Bucket:  req.Bucket,
				Builder: b.ID,
			},
			Config: &b.Config,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}
