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

// Package perm implements permission checks.
//
// The API is formulated in terms of LUCI Realms permissions, but it is
// currently implemented on top of native Buildbucket roles (which are
// deprecated).
package perm

import (
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

var (
	// BuildsGet allows to see all information about a build.
	BuildsGet = realms.RegisterPermission("buildbucket.builds.get")
	// BuildsList allows to list and search builds in a bucket.
	BuildsList = realms.RegisterPermission("buildbucket.builds.list")
	// BuildsCancel allows to cancel a build.
	BuildsCancel = realms.RegisterPermission("buildbucket.builds.cancel")

	// BuildersGet allows to see details of a builder (but not its builds).
	BuildersGet = realms.RegisterPermission("buildbucket.builders.get")
	// BuildersList allows to list and search builders (but not builds).
	BuildersList = realms.RegisterPermission("buildbucket.builders.list")
)

// Permission -> a minimal legacy role it requires.
var minRolePerPerm = map[realms.Permission]pb.Acl_Role{
	// Builds.
	BuildsGet:    pb.Acl_READER,
	BuildsList:   pb.Acl_READER,
	BuildsCancel: pb.Acl_WRITER,

	// Builders.
	BuildersGet:  pb.Acl_READER,
	BuildersList: pb.Acl_READER,
}

// HasInBucket checks the caller has the given permission in the bucket.
//
// Returns appstatus errors. If the bucket doesn't exist returns NotFound.
//
// Always checks the read permission first, returning NotFound if the caller
// doesn't have it. Returns PermissionDenied if the caller has the read
// permission, but not the requested `perm`.
func HasInBucket(ctx context.Context, perm realms.Permission, project, bucket string) error {
	bucketID := project + "/" + bucket // for error messages only

	// Verify the permission is known at all.
	minRole, ok := minRolePerPerm[perm]
	if !ok {
		return errors.Reason("checking unknown permission %q in %q", perm, bucketID).Err()
	}

	// Grab ACLs from the Bucket proto.
	//
	// TODO(vadimsh): It may make sense to cache the result in the process memory
	// for a minute or so to reduce load on the datastore, this is a very hot code
	// path.
	bck := &model.Bucket{ID: bucket, Parent: model.ProjectKey(ctx, project)}
	switch err := datastore.Get(ctx, bck); {
	case err == datastore.ErrNoSuchEntity:
		return NotFoundErr(ctx)
	case err != nil:
		return errors.Annotate(err, "failed to fetch %q bucket entity", bucketID).Err()
	}

	id := auth.CurrentIdentity(ctx)

	// Projects can do anything in their own buckets regardless of ACLs.
	if id.Kind() == identity.Project && id.Value() == project {
		return nil
	}

	// Admins can do anything in all buckets regardless of ACLs.
	switch is, err := auth.IsMember(ctx, "administrators"); {
	case err != nil:
		return errors.Annotate(err, "failed to check group membership in %q", "administrators").Err()
	case is:
		return nil
	}

	// Find the "maximum" role of current identity in this bucket.
	switch role, err := getRole(ctx, id, bck.Proto.Acls); {
	case err != nil:
		return err
	case role < pb.Acl_READER:
		return NotFoundErr(ctx)
	case role < minRole:
		return appstatus.Errorf(codes.PermissionDenied, "%q does not have permission %q in bucket %q", id, perm, bucketID)
	default:
		return nil
	}
}

// HasInBuilder checks the caller has the given permission in the builder.
//
// It's just a tiny wrapper around HasInBucket to reduce typing.
func HasInBuilder(ctx context.Context, perm realms.Permission, id *pb.BuilderID) error {
	return HasInBucket(ctx, perm, id.Project, id.Bucket)
}

// NotFoundErr returns an appstatus with a generic error message indicating
// the resource requested was not found with a hint that the user may not have
// permission to view it. By not differentiating between "not found" and
// "permission denied" errors, leaking existence of resources a user doesn't
// have permission to view can be avoided. Should be used everywhere a
// "not found" or "permission denied" error occurs.
func NotFoundErr(ctx context.Context) error {
	return appstatus.Errorf(codes.NotFound, "requested resource not found or %q does not have permission to view it", auth.CurrentIdentity(ctx))
}

// getRole returns the role of an identity based on the given ACLs.
//
// Roles are numerically comparable and role n implies roles [0, n-1] as well.
// May return -1 if the current identity has no defined role in this bucket.
func getRole(ctx context.Context, id identity.Identity, acls []*pb.Acl) (pb.Acl_Role, error) {
	db := auth.GetState(ctx).DB()

	var role pb.Acl_Role = -1
	for _, rule := range acls {
		// Check this rule if it could potentially confer a higher role.
		if rule.Role > role {
			if rule.Identity == string(id) {
				role = rule.Role
			} else if id.Kind() == identity.User && rule.Identity == id.Email() {
				role = rule.Role
			} else if is, err := db.IsMember(ctx, id, []string{rule.Group}); err != nil {
				// Empty group membership checks always return false without
				// any error so it doesn't matter if the group is unspecified.
				return -1, errors.Annotate(err, "failed to check group membership in %q", rule.Group).Err()
			} else if is {
				role = rule.Role
			}
		}
	}

	return role, nil
}
