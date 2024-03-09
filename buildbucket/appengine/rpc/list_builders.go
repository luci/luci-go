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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/pagination"
	"go.chromium.org/luci/common/pagination/dscursor"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

var listBuildersCursorVault = dscursor.NewVault([]byte("buildbucket.v2.Builders.ListBuilders"))

// validateListBuildersReq validates the given request.
func validateListBuildersReq(ctx context.Context, req *pb.ListBuildersRequest) error {
	if req.Project == "" {
		if req.Bucket != "" {
			return errors.Reason("project must be specified when bucket is specified").Err()
		}
	} else {
		err := protoutil.ValidateBuilderID(&pb.BuilderID{Project: req.Project, Bucket: req.Bucket})
		if err != nil {
			return err
		}
	}

	return validatePageSize(req.PageSize)
}

// ListBuilders handles a request to retrieve builders. Implements pb.BuildersServer.
func (*Builders) ListBuilders(ctx context.Context, req *pb.ListBuildersRequest) (*pb.ListBuildersResponse, error) {
	if err := validateListBuildersReq(ctx, req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Parse the cursor from the page token.
	cur, err := listBuildersCursorVault.Cursor(ctx, req.PageToken)
	switch err {
	case pagination.ErrInvalidPageToken:
		return nil, appstatus.BadRequest(err)
	case nil:
		// continue
	default:
		return nil, err
	}

	// ACL checks.
	var key *datastore.Key
	var allowedBuckets []string
	if req.Bucket == "" {
		if req.Project != "" {
			key = model.ProjectKey(ctx, req.Project)
		}

		var err error
		if allowedBuckets, err = perm.BucketsByPerm(ctx, bbperms.BuildersList, req.Project); err != nil {
			return nil, err
		}
	} else {
		key = model.BucketKey(ctx, req.Project, req.Bucket)

		if err := perm.HasInBucket(ctx, bbperms.BuildersList, req.Project, req.Bucket); err != nil {
			return nil, err
		}
		allowedBuckets = []string{protoutil.FormatBucketID(req.Project, req.Bucket)}
	}

	// Fetch the builders.
	q := datastore.NewQuery(model.BuilderKind).Ancestor(key).Start(cur)
	builders, nextCursor, err := fetchBuilders(ctx, q, allowedBuckets, req.PageSize)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch builders").Err()
	}

	// Generate the next page token.
	nextPageToken, err := listBuildersCursorVault.PageToken(ctx, nextCursor)
	if err != nil {
		return nil, err
	}

	// Compose the response.
	res := &pb.ListBuildersResponse{
		Builders:      make([]*pb.BuilderItem, len(builders)),
		NextPageToken: nextPageToken,
	}
	for i, b := range builders {
		res.Builders[i] = &pb.BuilderItem{
			Id: &pb.BuilderID{
				Project: b.Parent.Parent().StringID(),
				Bucket:  b.Parent.StringID(),
				Builder: b.ID,
			},
			Config: b.Config,
		}
	}
	return res, nil
}

// fetchBuilders fetches a page of builders together with a cursor.
//
// buckets in allowedBuckets should use project/bucket format.
func fetchBuilders(ctx context.Context, q *datastore.Query, allowedBuckets []string, pageSize int32) (builders []*model.Builder, nextCursor datastore.Cursor, err error) {
	// Note: this function is fairly generic, but the only reason it is currently
	// Builder-specific is because datastore.Run does not accept callback
	// signature func(any, CursorCB).

	if pageSize <= 0 {
		pageSize = 100
	}

	// Convert allowedBuckets to a set for faster lookup.
	allowedBucketSet := stringset.NewFromSlice(allowedBuckets...)

	// Fetch entities and the cursor if needed.
	err = datastore.Run(ctx, q, func(builder *model.Builder, getCursor datastore.CursorCB) error {
		// Check if the bucket is allowed. Use the fully qualified bucket ID
		// instead of the bucket name so we don't have to assume that the query
		// only returns builders from a single project.
		bucketID := protoutil.FormatBucketID(builder.Parent.Parent().StringID(), builder.Parent.StringID())
		if !allowedBucketSet.Has(bucketID) {
			return nil
		}

		builders = append(builders, builder)
		if len(builders) == int(pageSize) {
			var err error
			if nextCursor, err = getCursor(); err != nil {
				return err
			}
			return datastore.Stop
		}

		return nil
	})

	return
}
