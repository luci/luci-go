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

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// commonPostlude converts an appstatus error to a gRPC error and logs it.
func commonPostlude(ctx context.Context, methodName string, rsp proto.Message, err error) error {
	return appstatus.GRPCifyAndLog(ctx, err)
}

// notFound returns a generic error message indicating the resource requested
// was not found with a hint that the user may not have permission to view
// it. By not differentiating between "not found" and "permission denied"
// errors, leaking existence of resources a user doesn't have permission to
// view can be avoided. Should be used everywhere a "not found" or
// "permission denied" error occurs.
func notFound(ctx context.Context) error {
	return appstatus.Errorf(codes.NotFound, "requested resource not found or %q does not have permission to view it", auth.CurrentIdentity(ctx))
}

// logDetails logs debug information about the request.
func logDetails(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	logging.Debugf(ctx, "%q called %q with request %s", auth.CurrentIdentity(ctx), methodName, proto.MarshalTextString(req))
	return ctx, nil
}

func validatePageSize(pageSize int32) error {
	if pageSize < 0 {
		return errors.Reason("page_size cannot be negative").Err()
	}
	return nil
}

// canRead returns a NotFound error if the requester does not have a reader
// role in the bucket.
func canRead(ctx context.Context, project, bucket string) error {
	bck := &model.Bucket{ID: bucket, Parent: model.ProjectKey(ctx, project)}
	switch err := datastore.Get(ctx, bck); {
	case err == datastore.ErrNoSuchEntity:
		return notFound(ctx)
	case err != nil:
		return err
	}

	switch r, err := bck.GetRole(ctx); {
	case err != nil:
		return err
	case r < pb.Acl_READER:
		return notFound(ctx)
	default:
		return nil
	}
}

// decodeCursor decodes a datastore cursor from a page token.
// The returned error may be appstatus-annotated.
func decodeCursor(ctx context.Context, pageToken string) (datastore.Cursor, error) {
	if pageToken == "" {
		return nil, nil
	}

	cursor, err := datastore.DecodeCursor(ctx, pageToken)
	if err != nil {
		return nil, appstatus.Attachf(err, codes.InvalidArgument, "bad cursor")
	}

	return cursor, nil
}
