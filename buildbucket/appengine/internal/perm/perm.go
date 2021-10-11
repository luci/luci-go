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
	"sort"
	"sync"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const (
	// UpdateBuildAllowedUsers is a group of users allowed to update builds.
	// They are expected to be robots.
	UpdateBuildAllowedUsers = "buildbucket-update-build-users"

	// Administrators is a group of users that have all permissions in all
	// buckets.
	Administrators = "administrators"
)

var (
	// BuildsAdd allows to schedule new builds in a bucket.
	BuildsAdd = realms.RegisterPermission("buildbucket.builds.add")
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
	BuildsAdd:    pb.Acl_SCHEDULER,
	BuildsGet:    pb.Acl_READER,
	BuildsList:   pb.Acl_READER,
	BuildsCancel: pb.Acl_SCHEDULER,

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
	realm := realms.Join(project, bucket)

	switch yes, err := auth.ShouldEnforceRealmACL(ctx, realm); {
	case err != nil:
		return errors.Annotate(err, "failed to check realms DB").Err()

	case yes:
		logging.Infof(ctx, "crbug.com/1091604: enforcing realm ACLs for %q", realm)
		return hasPermRealms(ctx, perm, project, bucket)

	default:
		// Make the legacy ACL check.
		err := hasPermLegacy(ctx, perm, project, bucket)

		// If got some internal error (not an appstatus one), return it as is.
		// It means the check itself failed.
		if _, ok := appstatus.Get(err); err != nil && !ok {
			return err
		}

		// Compare the result of the legacy ACL check to the realm ACL check. Note
		// that Execute doesn't return an error. It just does best effort logging.
		(auth.HasPermissionDryRun{
			ExpectedResult: err == nil,
			TrackingBug:    "crbug.com/1091604",
			AdminGroup:     Administrators,
		}).Execute(ctx, perm, realm)

		// But still use legacy ACLs.
		return err
	}
}

// hasPermRealms checks realms Buildbucket ACLs.
//
// Returns nil if the caller has the permission, an appstatus error if doesn't,
// or some other error if the check itself failed.
func hasPermRealms(ctx context.Context, perm realms.Permission, project, bucket string) error {
	realm := realms.Join(project, bucket)
	switch has, err := auth.HasPermission(ctx, perm, realm); {
	case err != nil:
		return errors.Annotate(err, "failed to check realm %q ACLs", realm).Err()
	case has:
		return nil
	}

	// For compatibility with legacy ALCs, administrators have implicit access to
	// everything. Log when this rule is invoked, since it's surprising and it
	// something we might want to get rid of after everything is migrated to
	// Realms.
	switch is, err := auth.IsMember(ctx, Administrators); {
	case err != nil:
		return errors.Annotate(err, "failed to check group membership in %q", Administrators).Err()
	case is:
		logging.Warningf(ctx, "ADMIN_ACCESS: %q does not have permission %q in bucket %q, but they are in %q group and are allowed to proceed",
			auth.CurrentIdentity(ctx), perm, project+"/"+bucket, Administrators)
		return nil
	}

	// Give a detailed error message only if the caller is allowed to see
	// the builder. Otherwise return generic "Not found or no permission" error.
	if perm != BuildersGet {
		switch visible, err := auth.HasPermission(ctx, BuildersGet, realm); {
		case err != nil:
			return errors.Annotate(err, "failed to check realm %q ACLs", realm).Err()
		case visible:
			return appstatus.Errorf(codes.PermissionDenied, "%q does not have permission %q in bucket %q",
				auth.CurrentIdentity(ctx), perm, project+"/"+bucket)
		}
	}

	return NotFoundErr(ctx)
}

// hasPermLegacy checks legacy Buildbucket ACLs.
//
// Returns nil if the caller has the permission, an appstatus error if doesn't,
// or some other error if the check itself failed.
func hasPermLegacy(ctx context.Context, perm realms.Permission, project, bucket string) error {
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
	switch is, err := auth.IsMember(ctx, Administrators); {
	case err != nil:
		return errors.Annotate(err, "failed to check group membership in %q", Administrators).Err()
	case is:
		return nil
	}

	// Find the "maximum" role of current identity in this bucket.
	switch role, err := getRole(ctx, id, bck.Proto.GetAcls()); {
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

// BucketsByPerm returns buckets of the project that the caller has the given permission in.
// If the project is empty, it returns all user accessible buckets.
// Note: if the caller doesn't have the permission, it returns empty buckets.
func BucketsByPerm(ctx context.Context, p realms.Permission, project string) (buckets []string, err error) {
	var projKey *datastore.Key
	if project != "" {
		projKey = datastore.KeyForObj(ctx, &model.Project{ID: project})
	}

	var bucketKeys []*datastore.Key
	if err := datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind).Ancestor(projKey), &bucketKeys); err != nil {
		return nil, err
	}

	err = parallel.WorkPool(len(bucketKeys), func(c chan<- func() error) {
		var mu sync.Mutex
		for _, bk := range bucketKeys {
			bk := bk
			c <- func() error {
				if err := HasInBucket(ctx, p, bk.Parent().StringID(), bk.StringID()); err != nil {
					status, ok := appstatus.Get(err)
					if ok && (status.Code() == codes.PermissionDenied || status.Code() == codes.NotFound) {
						return nil
					}
					return err
				}
				mu.Lock()
				buckets = append(buckets, protoutil.FormatBucketID(bk.Parent().StringID(), bk.StringID()))
				mu.Unlock()
				return nil
			}
		}
	})
	sort.Strings(buckets)
	return
}

// CanUpdateBuild returns whether the caller has a permission to update builds.
func CanUpdateBuild(ctx context.Context) (bool, error) {
	return auth.IsMember(ctx, UpdateBuildAllowedUsers)
}
