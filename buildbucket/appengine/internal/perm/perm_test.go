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

package perm

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func testingContext() context.Context {
	ctx := context.Background()
	ctx = memory.Use(ctx)
	ctx = caching.WithEmptyProcessCache(ctx)
	return ctx
}

func TestHasInBucket(t *testing.T) {
	t.Parallel()

	Convey("With mocked auth DB", t, func() {
		const (
			anon   = identity.AnonymousIdentity
			admin  = identity.Identity("user:admin@example.com")
			reader = identity.Identity("user:reader@example.com")
			writer = identity.Identity("user:writer@example.com")

			appID            = "buildbucket-app-id"
			projectID        = "some-project"
			existingBucketID = "existing-bucket"
			missingBucketID  = "missing-bucket"

			existingRealmID = projectID + ":" + existingBucketID
			missingRealmID  = projectID + ":" + missingBucketID
		)

		ctx := testingContext()

		So(datastore.Put(ctx, &model.Bucket{
			ID:     existingBucketID,
			Parent: model.ProjectKey(ctx, projectID),
			Proto:  &pb.Bucket{},
		}), ShouldBeNil)

		s := &authtest.FakeState{
			FakeDB: authtest.NewFakeDB(
				authtest.MockMembership(admin, Administrators),
			),
		}
		ctx = auth.WithState(ctx, s)

		check := func(bucketID string, perm realms.Permission, caller identity.Identity) codes.Code {
			s.Identity = caller
			err := HasInBucket(ctx, perm, projectID, bucketID)
			if err == nil {
				return codes.OK
			}
			status, ok := appstatus.Get(err)
			if !ok {
				return codes.Internal
			}
			return status.Code()
		}

		Convey("No realm ACLs", func() {
			So(check(existingBucketID, bbperms.BuildsGet, anon), ShouldEqual, codes.NotFound)
			So(check(existingBucketID, bbperms.BuildsGet, admin), ShouldEqual, codes.OK)
			So(check(existingBucketID, bbperms.BuildsGet, reader), ShouldEqual, codes.NotFound)
			So(check(existingBucketID, bbperms.BuildsGet, writer), ShouldEqual, codes.NotFound)

			So(check(missingBucketID, bbperms.BuildsGet, anon), ShouldEqual, codes.NotFound)
			So(check(missingBucketID, bbperms.BuildsGet, admin), ShouldEqual, codes.NotFound)
			So(check(missingBucketID, bbperms.BuildsGet, reader), ShouldEqual, codes.NotFound)
			So(check(missingBucketID, bbperms.BuildsGet, writer), ShouldEqual, codes.NotFound)
		})

		Convey("With realm ACLs", func() {
			s.FakeDB.(*authtest.FakeDB).AddMocks(
				authtest.MockPermission(reader, existingRealmID, bbperms.BuildersGet),
				authtest.MockPermission(reader, existingRealmID, bbperms.BuildsGet),
				authtest.MockPermission(writer, existingRealmID, bbperms.BuildersGet),
				authtest.MockPermission(writer, existingRealmID, bbperms.BuildsGet),
				authtest.MockPermission(writer, existingRealmID, bbperms.BuildsCancel),

				authtest.MockPermission(reader, missingRealmID, bbperms.BuildersGet),
				authtest.MockPermission(reader, missingRealmID, bbperms.BuildsGet),
				authtest.MockPermission(writer, missingRealmID, bbperms.BuildersGet),
				authtest.MockPermission(writer, missingRealmID, bbperms.BuildsGet),
				authtest.MockPermission(writer, missingRealmID, bbperms.BuildsCancel),
			)

			Convey("Read perm", func() {
				So(check(existingBucketID, bbperms.BuildsGet, anon), ShouldEqual, codes.NotFound)
				So(check(existingBucketID, bbperms.BuildsGet, admin), ShouldEqual, codes.OK)
				So(check(existingBucketID, bbperms.BuildsGet, reader), ShouldEqual, codes.OK)
				So(check(existingBucketID, bbperms.BuildsGet, writer), ShouldEqual, codes.OK)
			})

			Convey("Write perm", func() {
				So(check(existingBucketID, bbperms.BuildsCancel, anon), ShouldEqual, codes.NotFound)
				So(check(existingBucketID, bbperms.BuildsCancel, admin), ShouldEqual, codes.OK)
				So(check(existingBucketID, bbperms.BuildsCancel, reader), ShouldEqual, codes.PermissionDenied)
				So(check(existingBucketID, bbperms.BuildsCancel, writer), ShouldEqual, codes.OK)
			})

			Convey("Missing bucket", func() {
				So(check(missingBucketID, bbperms.BuildsGet, anon), ShouldEqual, codes.NotFound)
				So(check(missingBucketID, bbperms.BuildsGet, admin), ShouldEqual, codes.NotFound)
				So(check(missingBucketID, bbperms.BuildsGet, reader), ShouldEqual, codes.NotFound)
				So(check(missingBucketID, bbperms.BuildsGet, writer), ShouldEqual, codes.NotFound)

				So(check(missingBucketID, bbperms.BuildsCancel, anon), ShouldEqual, codes.NotFound)
				So(check(missingBucketID, bbperms.BuildsCancel, admin), ShouldEqual, codes.NotFound)
				So(check(missingBucketID, bbperms.BuildsCancel, reader), ShouldEqual, codes.NotFound)
				So(check(missingBucketID, bbperms.BuildsCancel, writer), ShouldEqual, codes.NotFound)
			})
		})
	})
}

func TestBucketsByPerm(t *testing.T) {
	t.Parallel()

	Convey("GetAccessibleBuckets", t, func() {
		const reader = identity.Identity("user:reader@example.com")

		ctx := testingContext()
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: reader,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(reader, "project:bucket1", bbperms.BuildersList),
				authtest.MockPermission(reader, "project:bucket2", bbperms.BuildersList),
				authtest.MockPermission(reader, "project2:bucket1", bbperms.BuildersList),
				authtest.MockPermission(reader, "project:bucket2", bbperms.BuildsCancel),
			),
		})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		So(datastore.Put(ctx,
			&model.Bucket{
				ID:     "bucket1",
				Parent: model.ProjectKey(ctx, "project"),
				Proto:  &pb.Bucket{},
			},
			&model.Bucket{
				ID:     "bucket1",
				Parent: model.ProjectKey(ctx, "project2"),
				Proto:  &pb.Bucket{},
			},
			&model.Bucket{
				ID:     "bucket2",
				Parent: model.ProjectKey(ctx, "project"),
				Proto:  &pb.Bucket{},
			},
		), ShouldBeNil)

		buckets1, err := BucketsByPerm(ctx, bbperms.BuildersList, "")
		So(err, ShouldBeNil)
		So(buckets1, ShouldResemble, []string{"project/bucket1", "project/bucket2", "project2/bucket1"})

		buckets2, err := BucketsByPerm(ctx, bbperms.BuildsCancel, "")
		So(err, ShouldBeNil)
		So(buckets2, ShouldResemble, []string{"project/bucket2"})

		buckets3, err := BucketsByPerm(ctx, bbperms.BuildersList, "project2")
		So(err, ShouldBeNil)
		So(buckets3, ShouldResemble, []string{"project2/bucket1"})

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.Identity("user:unknown@example.com"),
		})
		buckets4, err := BucketsByPerm(ctx, bbperms.BuildersList, "")
		So(err, ShouldBeNil)
		So(buckets4, ShouldBeNil)
	})
}
