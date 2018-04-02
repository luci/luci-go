// Copyright 2018 The LUCI Authors.
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

package repo

import (
	"testing"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetadataFetching(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		meta := testutil.MetadataStore{}

		// ACL.
		rootMeta := meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:top-owner@example.com"},
				},
			},
		})

		// The metadata to be fetched.
		leafMeta := meta.Populate("a/b/c/d", &api.PrefixMetadata{
			UpdateUser: "user:someone@example.com",
		})

		impl := repoImpl{meta: &meta}

		callGet := func(prefix string, user identity.Identity) (*api.PrefixMetadata, error) {
			ctx := auth.WithState(context.Background(), &authtest.FakeState{
				Identity: user,
				FakeDB: authtest.FakeDB{
					"user:admin@example.com": {"administrators"},
				},
			})
			return impl.GetPrefixMetadata(ctx, &api.PrefixRequest{Prefix: prefix})
		}

		callGetInherited := func(prefix string, user identity.Identity) ([]*api.PrefixMetadata, error) {
			ctx := auth.WithState(context.Background(), &authtest.FakeState{
				Identity: user,
				FakeDB: authtest.FakeDB{
					"user:admin@example.com": {"administrators"},
				},
			})
			resp, err := impl.GetInheritedPrefixMetadata(ctx, &api.PrefixRequest{Prefix: prefix})
			if err != nil {
				return nil, err
			}
			return resp.PerPrefixMetadata, nil
		}

		Convey("GetPrefixMetadata happy path", func() {
			resp, err := callGet("a/b/c/d", "user:top-owner@example.com")
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, leafMeta)
		})

		Convey("GetInheritedPrefixMetadata happy path", func() {
			resp, err := callGetInherited("a/b/c/d", "user:top-owner@example.com")
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, []*api.PrefixMetadata{rootMeta, leafMeta})
		})

		Convey("GetPrefixMetadata bad prefix", func() {
			resp, err := callGet("a//", "user:top-owner@example.com")
			So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			So(resp, ShouldBeNil)
		})

		Convey("GetInheritedPrefixMetadata bad prefix", func() {
			resp, err := callGetInherited("a//", "user:top-owner@example.com")
			So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			So(resp, ShouldBeNil)
		})

		Convey("GetPrefixMetadata no metadata, caller has access", func() {
			resp, err := callGet("a/b", "user:top-owner@example.com")
			So(grpc.Code(err), ShouldEqual, codes.NotFound)
			So(resp, ShouldBeNil)
		})

		Convey("GetInheritedPrefixMetadata no metadata, caller has access", func() {
			resp, err := callGetInherited("a/b", "user:top-owner@example.com")
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, []*api.PrefixMetadata{rootMeta})
		})

		Convey("GetPrefixMetadata no metadata, caller has no access", func() {
			resp, err := callGet("a/b", "user:someone-else@example.com")
			So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
			// Existing metadata that the caller has no access to produces same error,
			// so unauthorized callers can't easily distinguish between the two.
			resp, err = callGet("a/b/c/d", "user:someone-else@example.com")
			So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
			// Same for completely unknown prefix.
			resp, err = callGet("zzz", "user:someone-else@example.com")
			So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
		})

		Convey("GetInheritedPrefixMetadata no metadata, caller has no access", func() {
			resp, err := callGetInherited("a/b", "user:someone-else@example.com")
			So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
			// Existing metadata that the caller has no access to produces same error,
			// so unauthorized callers can't easily distinguish between the two.
			resp, err = callGetInherited("a/b/c/d", "user:someone-else@example.com")
			So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
			// Same for completely unknown prefix.
			resp, err = callGetInherited("zzz", "user:someone-else@example.com")
			So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
		})

		Convey("GetInheritedPrefixMetadata admin", func() {
			// Admins can see everything, in particular they can see absence of root
			// prefixes: they receive empty metadata list for them. Note that
			// non-admins can't ever see empty metadata list, since there's at least
			// one (perhaps inherited) metadata entry that granted them the access
			// in the first place.
			resp, err := callGetInherited("zzz", "user:admin@example.com")
			So(err, ShouldBeNil)
			So(resp, ShouldBeNil)
		})
	})
}

func TestMetadataUpdating(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		testTime := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, tc := testclock.UseTime(context.Background(), testTime)

		meta := testutil.MetadataStore{}

		// ACL.
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:top-owner@example.com"},
				},
			},
		})

		impl := repoImpl{meta: &meta}

		callUpdate := func(user identity.Identity, m *api.PrefixMetadata) (*api.PrefixMetadata, error) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: user,
				FakeDB: authtest.FakeDB{
					"user:admin@example.com": {"administrators"},
				},
			})
			return impl.UpdatePrefixMetadata(ctx, m)
		}

		Convey("Happy path", func() {
			// Create new metadata entry.
			meta, err := callUpdate("user:top-owner@example.com", &api.PrefixMetadata{
				Prefix:     "a/b/",
				UpdateTime: google.NewTimestamp(time.Unix(10000, 0)), // should be overwritten
				UpdateUser: "user:zzz@example.com",                   // should be overwritten
				Acls: []*api.PrefixMetadata_ACL{
					{Role: api.Role_READER, Principals: []string{"user:reader@example.com"}},
				},
			})
			So(err, ShouldBeNil)

			expected := &api.PrefixMetadata{
				Prefix:      "a/b",
				Fingerprint: "WZllwc6m8f9C_rfwnspaPIiyPD0",
				UpdateTime:  google.NewTimestamp(testTime),
				UpdateUser:  "user:top-owner@example.com",
				Acls: []*api.PrefixMetadata_ACL{
					{Role: api.Role_READER, Principals: []string{"user:reader@example.com"}},
				},
			}
			So(meta, ShouldResemble, expected)

			// Update it a bit later.
			tc.Add(time.Hour)
			updated := *expected
			updated.Acls = nil
			meta, err = callUpdate("user:top-owner@example.com", &updated)
			So(err, ShouldBeNil)
			So(meta, ShouldResemble, &api.PrefixMetadata{
				Prefix:      "a/b",
				Fingerprint: "oQ2uuVbjV79prXxl4jyJkOpff90",
				UpdateTime:  google.NewTimestamp(testTime.Add(time.Hour)),
				UpdateUser:  "user:top-owner@example.com",
			})
		})

		Convey("Validation works", func() {
			meta, err := callUpdate("user:top-owner@example.com", &api.PrefixMetadata{
				Prefix: "a/b//",
			})
			So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			So(meta, ShouldBeNil)

			meta, err = callUpdate("user:top-owner@example.com", &api.PrefixMetadata{
				Prefix: "a/b",
				Acls: []*api.PrefixMetadata_ACL{
					{Role: api.Role_READER, Principals: []string{"huh?"}},
				},
			})
			So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			So(meta, ShouldBeNil)
		})

		Convey("ACLs work", func() {
			meta, err := callUpdate("user:unknown@example.com", &api.PrefixMetadata{
				Prefix: "a/b",
			})
			So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			So(meta, ShouldBeNil)

			// Same as completely unknown prefix.
			meta, err = callUpdate("user:unknown@example.com", &api.PrefixMetadata{
				Prefix: "zzz",
			})
			So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			So(meta, ShouldBeNil)
		})

		Convey("Deleted concurrently", func() {
			m := meta.Populate("a/b", &api.PrefixMetadata{})
			meta.Purge("a/b")

			// If the caller is a prefix owner, they see NotFound.
			meta, err := callUpdate("user:top-owner@example.com", m)
			So(grpc.Code(err), ShouldEqual, codes.NotFound)
			So(meta, ShouldBeNil)

			// Other callers just see regular PermissionDenined.
			meta, err = callUpdate("user:unknown@example.com", m)
			So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			So(meta, ShouldBeNil)
		})

		Convey("Creating existing", func() {
			m := meta.Populate("a/b", &api.PrefixMetadata{})

			m.Fingerprint = "" // indicates the caller is expecting to create a new one
			meta, err := callUpdate("user:top-owner@example.com", m)
			So(grpc.Code(err), ShouldEqual, codes.AlreadyExists)
			So(meta, ShouldBeNil)
		})

		Convey("Changed midway", func() {
			m := meta.Populate("a/b", &api.PrefixMetadata{})

			// Someone comes and updates it.
			updated, err := callUpdate("user:top-owner@example.com", m)
			So(err, ShouldBeNil)
			So(updated.Fingerprint, ShouldNotEqual, m.Fingerprint)

			// Trying to do it again fails, 'm' is stale now.
			_, err = callUpdate("user:top-owner@example.com", m)
			So(grpc.Code(err), ShouldEqual, codes.FailedPrecondition)
		})
	})
}
