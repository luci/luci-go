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
	"slices"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/caching/layered"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const (
	// Administrators is a group of users that have read & cancel permissions
	// in all buckets.
	Administrators = "administrators"
)

var (
	// Cache "<project>/<bucket>" => wirepb-serialized pb.Bucket.
	//
	// Missing buckets are represented by empty pb.Bucket protos.
	bucketCache = layered.RegisterCache(layered.Parameters[*pb.Bucket]{
		ProcessCacheCapacity: 65536,
		GlobalNamespace:      "bucket_cache_v1",
		Marshal: func(item *pb.Bucket) ([]byte, error) {
			return proto.Marshal(item)
		},
		Unmarshal: func(blob []byte) (*pb.Bucket, error) {
			pb := &pb.Bucket{}
			if err := proto.Unmarshal(blob, pb); err != nil {
				return nil, err
			}
			return pb, nil
		},
		AllowNoProcessCacheFallback: true, // allow skipping cache in tests
	})

	// Permissions granted to admin users in all buckets, including canceling
	// a build and reading details of a build or builder.
	AdminPerms = []realms.Permission{
		bbperms.BuildsCancel, bbperms.BuildsGet, bbperms.BuildsList,
		bbperms.BuildersGet, bbperms.BuildersList,
	}
)

// getBucket fetches a cached bucket proto.
//
// The returned value can be up to a minute stale compared to the state in
// the datastore.
//
// Returns:
//
//	bucket, nil - on success.
//	nil, nil - if the bucket is absent.
//	nil, err - on internal errors.
func getBucket(ctx context.Context, project, bucket string) (*pb.Bucket, error) {
	if project == "" {
		return nil, errors.New("project name is empty")
	}
	if bucket == "" {
		return nil, errors.New("bucket name is empty")
	}

	item, err := bucketCache.GetOrCreate(ctx, project+"/"+bucket, func() (*pb.Bucket, time.Duration, error) {
		entity := &model.Bucket{ID: bucket, Parent: model.ProjectKey(ctx, project)}
		switch err := datastore.Get(ctx, entity); {
		case err == nil:
			// We rely on Name to not be empty for existing buckets. Make sure it is
			// not empty.
			bucketPB := entity.Proto
			if bucketPB == nil {
				bucketPB = &pb.Bucket{}
			}
			bucketPB.Name = bucket
			return bucketPB, time.Minute, nil
		case err == datastore.ErrNoSuchEntity:
			// Cache "absence" for a shorter duration to make it harder to overflow
			// the cache with requests for non-existing buckets.
			return &pb.Bucket{}, 15 * time.Second, nil
		default:
			return nil, 0, errors.Fmt("datastore error: %w", err)
		}
	}, layered.WithRandomizedExpiration(10*time.Second))
	if err != nil {
		return nil, err
	}

	// Name is always populated in existing buckets. It is never populated in
	// missing buckets.
	if item.Name == "" {
		return nil, nil
	}
	return item, nil
}

// HasInBucket checks the caller has the given permission in the bucket.
//
// Returns appstatus errors. If the bucket doesn't exist returns NotFound.
//
// Always checks the read permission (represented by BuildersGet), returning
// NotFound if the caller doesn't have it. Returns PermissionDenied if the
// caller has the read permission, but not the requested `perm`.
func HasInBucket(ctx context.Context, perm realms.Permission, project, bucket string) error {
	// Referring to a non-existing bucket is NotFound, even if the requested
	// permission is available via the @root realm.
	bucketPB, err := getBucket(ctx, project, bucket)
	switch {
	case err != nil:
		return errors.Fmt("failed to fetch bucket %q: %w", project+"/"+bucket, err)
	case bucketPB == nil:
		return NotFoundErr(ctx)
	}

	realm := realms.Join(project, bucket)
	switch yes, err := auth.HasPermission(ctx, perm, realm, nil); {
	case err != nil:
		return errors.Fmt("failed to check realm %q ACLs: %w", realm, err)
	case yes:
		return nil
	}

	// Administrators have the ability to cancel any build, and to read details
	// of any build or builder in any bucket.
	switch is, err := auth.IsMember(ctx, Administrators); {
	case err != nil:
		return errors.Fmt("failed to check group membership in %q: %w", Administrators, err)
	case is:
		if slices.Contains(AdminPerms, perm) {
			// Log when this rule is invoked, since it's surprising.
			logging.Warningf(ctx, "ADMIN_FALLBACK: perm=%q bucket=%q caller=%q",
				perm, project+"/"+bucket, auth.CurrentIdentity(ctx))
			return nil
		}
		// Give a detailed error message because the caller is an administrator.
		return appstatus.Errorf(codes.PermissionDenied,
			"%q does not have permission %q in bucket %q",
			auth.CurrentIdentity(ctx), perm, project+"/"+bucket)
	}

	// The user doesn't have the requested permission. Give a detailed error
	// message only if the caller is allowed to see the builder. Otherwise return
	// generic "Not found or no permission" error.
	if perm != bbperms.BuildersGet {
		switch visible, err := auth.HasPermission(ctx, bbperms.BuildersGet, realm, nil); {
		case err != nil:
			return errors.Fmt("failed to check realm %q ACLs: %w", realm, err)
		case visible:
			return appstatus.Errorf(codes.PermissionDenied,
				"%q does not have permission %q in bucket %q",
				auth.CurrentIdentity(ctx), perm, project+"/"+bucket)
		}
	}

	return NotFoundErr(ctx)
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

// hasInBuilderBoolean is a wrapper around HasInBuilder that handles denied/not-found errors
// and returns false instead of an error in those cases.
func hasInBuilderBoolean(ctx context.Context, p realms.Permission, builderID *pb.BuilderID) (bool, error) {
	err := HasInBuilder(ctx, p, builderID)
	if err == nil {
		return true, nil
	}
	status, ok := appstatus.Get(err)
	if ok && (status.Code() == codes.PermissionDenied || status.Code() == codes.NotFound) {
		return false, nil
	}
	return false, err
}

// GetFirstAvailablePerm returns the first permission in the given list which is granted to
// the user for the given builder. Returns an error if the user has none of the permissions.
func GetFirstAvailablePerm(ctx context.Context, builderID *pb.BuilderID, perms ...realms.Permission) (realms.Permission, error) {
	if len(perms) == 0 {
		panic("at least one permission must be provided")
	}
	// Look at each permission in turn except for the last one.
	for _, perm := range perms[:len(perms)-1] {
		if ok, err := hasInBuilderBoolean(ctx, perm, builderID); err != nil {
			return realms.Permission{}, err
		} else if ok {
			return perm, nil
		}
	}

	// If the user doesn't have any permissions at all, we want to throw an error, so use
	// HasInBuilder instead of the boolean version.
	lastPerm := perms[len(perms)-1]
	if err := HasInBuilder(ctx, lastPerm, builderID); err != nil {
		return realms.Permission{}, err
	}
	return lastPerm, nil
}

// getCachedPerm is a simple caching wrapper to check whether the user has a permission in
// a given bucket cache (map of bucket names to broadest Build read permission).
//
// Returns an error if the user does not have at least bbperms.BuildsList.
func getCachedPerm(ctx context.Context, bucketPermCache map[string]realms.Permission, builderID *pb.BuilderID) (realms.Permission, error) {
	qualifiedBucket := protoutil.FormatBucketID(builderID.GetProject(), builderID.GetBucket())
	_, cached := bucketPermCache[qualifiedBucket]
	if !cached {
		broadestBuildReadPerm, err := GetFirstAvailablePerm(ctx, builderID, bbperms.BuildsGet, bbperms.BuildsGetLimited, bbperms.BuildsList)
		if err != nil {
			return realms.Permission{}, err
		}
		bucketPermCache[qualifiedBucket] = broadestBuildReadPerm
	}
	return bucketPermCache[qualifiedBucket], nil
}

// RedactBuild redacts fields from the given build based on whether the user has
// appropriate permissions to see those fields.
// The relevant permissions are:
//
//	bbperms.BuildsGet: can see all fields
//	bbperms.BuildsGetLimited: can see a limited set of fields excluding detailed builder output
//	bbperms.BuildsList: can see only basic fields required to list builds
//
// Returns an error if the user does not have at least bbperms.BuildsList.
//
// For efficiency in the case where multiple builds are going to be redacted at once, the caller
// may optionally supply a bucket cache (map of bucket names to broadest Build read permission).
func RedactBuild(ctx context.Context, bucketPermCache map[string]realms.Permission, build *pb.Build) error {
	if bucketPermCache == nil {
		bucketPermCache = make(map[string]realms.Permission)
	}
	builderID := build.GetBuilder()
	broadestPerm, err := getCachedPerm(ctx, bucketPermCache, builderID)
	if err != nil {
		return err
	}
	var redactionMask *model.BuildMask
	switch {
	case broadestPerm == bbperms.BuildsGet:
		return nil
	case broadestPerm == bbperms.BuildsGetLimited:
		redactionMask = model.GetLimitedBuildMask
	case broadestPerm == bbperms.BuildsList:
		redactionMask = model.ListOnlyBuildMask
	}
	if err := redactionMask.Trim(build); err != nil {
		return err
	}
	return nil
}
