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

	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
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

	// Check permissions.
	if err := perm.HasInBucket(ctx, perm.BuildersList, req.Project, req.Bucket); err != nil {
		return nil, err
	}

	// Fetch the builders.
	q := datastore.NewQuery(model.BuilderKind).
		Ancestor(model.BucketKey(ctx, req.Project, req.Bucket))
	builders, nextPageToken, err := fetchBuilders(ctx, q, req.PageToken, req.PageSize)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch builders").Err()
	}

	// Compose the response.
	res := &pb.ListBuildersResponse{
		Builders:      make([]*pb.BuilderItem, len(builders)),
		NextPageToken: nextPageToken,
	}
	for i, b := range builders {
		res.Builders[i] = &pb.BuilderItem{
			Id: &pb.BuilderID{
				Project: req.Project,
				Bucket:  req.Bucket,
				Builder: b.ID,
			},
			Config: b.Config,
		}
	}
	return res, nil
}

// fetchBuilders fetches a page of builders together with a cursor.
func fetchBuilders(ctx context.Context, q *datastore.Query, pageToken string, pageSize int32) (builders []*model.Builder, nextPageToken string, err error) {
	// Note: this function is fairly generic, but the only reason it is currently
	// Builder-specific is because datastore.Run does not accept callback
	// signature func(interface{}, CursorCB).

	// Respect the page token.
	cur, err := decodeCursor(ctx, pageToken)
	if err != nil {
		return
	}
	q = q.Start(cur)

	// Respect the page size.
	if pageSize <= 0 {
		pageSize = 100
	}
	q = q.Limit(pageSize)

	// Fetch entities and the cursor if needed.
	var nextCursor datastore.Cursor
	err = datastore.Run(ctx, q, func(builder *model.Builder, getCursor datastore.CursorCB) error {
		builders = append(builders, builder)
		if len(builders) == int(pageSize) {
			var err error
			if nextCursor, err = getCursor(); err != nil {
				return err
			}
		}

		return nil
	})

	if nextCursor != nil {
		nextPageToken = nextCursor.String()
	}
	return
}
