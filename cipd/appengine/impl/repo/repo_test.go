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
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/repo/processing"
	"go.chromium.org/luci/cipd/appengine/impl/repo/tasks"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMetadataFetching(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		meta := testutil.MetadataStore{}

		// ACL.
		rootMeta := meta.Populate("", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:admin@example.com"},
				},
			},
		})
		topMeta := meta.Populate("a", &api.PrefixMetadata{
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
			})
			return impl.GetPrefixMetadata(ctx, &api.PrefixRequest{Prefix: prefix})
		}

		callGetInherited := func(prefix string, user identity.Identity) ([]*api.PrefixMetadata, error) {
			ctx := auth.WithState(context.Background(), &authtest.FakeState{
				Identity: user,
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
			So(resp, ShouldResembleProto, []*api.PrefixMetadata{rootMeta, topMeta, leafMeta})
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
			So(resp, ShouldResembleProto, []*api.PrefixMetadata{rootMeta, topMeta})
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
	})
}

func TestMetadataUpdating(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		testTime := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, tc := testclock.UseTime(context.Background(), testTime)

		meta := testutil.MetadataStore{}

		// ACL.
		meta.Populate("", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:admin@example.com"},
				},
			},
		})
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
			So(meta, ShouldResembleProto, expected)

			// Update it a bit later.
			tc.Add(time.Hour)
			updated := *expected
			updated.Acls = nil
			meta, err = callUpdate("user:top-owner@example.com", &updated)
			So(err, ShouldBeNil)
			So(meta, ShouldResembleProto, &api.PrefixMetadata{
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
			m := meta.Populate("a/b", &api.PrefixMetadata{
				UpdateUser: "user:someone@example.com",
			})
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
			m := meta.Populate("a/b", &api.PrefixMetadata{
				UpdateUser: "user:someone@example.com",
			})

			m.Fingerprint = "" // indicates the caller is expecting to create a new one
			meta, err := callUpdate("user:top-owner@example.com", m)
			So(grpc.Code(err), ShouldEqual, codes.AlreadyExists)
			So(meta, ShouldBeNil)
		})

		Convey("Changed midway", func() {
			m := meta.Populate("a/b", &api.PrefixMetadata{
				UpdateUser: "user:someone@example.com",
			})

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

func TestGetRolesInPrefix(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		meta := testutil.MetadataStore{}

		meta.Populate("", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:admin@example.com"},
				},
			},
		})
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_WRITER,
					Principals: []string{"user:writer@example.com"},
				},
			},
		})

		impl := repoImpl{meta: &meta}

		call := func(prefix string, user identity.Identity) (*api.RolesInPrefixResponse, error) {
			ctx := auth.WithState(context.Background(), &authtest.FakeState{
				Identity: user,
			})
			return impl.GetRolesInPrefix(ctx, &api.PrefixRequest{Prefix: prefix})
		}

		Convey("Happy path", func() {
			resp, err := call("a/b/c/d", "user:writer@example.com")
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &api.RolesInPrefixResponse{
				Roles: []*api.RolesInPrefixResponse_RoleInPrefix{
					{Role: api.Role_READER},
					{Role: api.Role_WRITER},
				},
			})
		})

		Convey("Anonymous", func() {
			resp, err := call("a/b/c/d", "anonymous:anonymous")
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &api.RolesInPrefixResponse{})
		})

		Convey("Admin", func() {
			resp, err := call("a/b/c/d", "user:admin@example.com")
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &api.RolesInPrefixResponse{
				Roles: []*api.RolesInPrefixResponse_RoleInPrefix{
					{Role: api.Role_READER},
					{Role: api.Role_WRITER},
					{Role: api.Role_OWNER},
				},
			})
		})

		Convey("Bad prefix", func() {
			_, err := call("///", "user:writer@example.com")
			So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'prefix'")
		})
	})
}

func TestListPrefix(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx := gaetesting.TestingContext()

		meta := testutil.MetadataStore{}

		meta.Populate("", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:admin@example.com"},
				},
			},
		})
		meta.Populate("1/a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})
		meta.Populate("6", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})
		meta.Populate("7", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		impl := repoImpl{meta: &meta}

		call := func(prefix string, recursive, hidden bool, user identity.Identity) (*api.ListPrefixResponse, error) {
			c := auth.WithState(ctx, &authtest.FakeState{Identity: user})
			return impl.ListPrefix(c, &api.ListPrefixRequest{
				Prefix:        prefix,
				Recursive:     recursive,
				IncludeHidden: hidden,
			})
		}

		const hidden = true
		const visible = false
		mk := func(name string, hidden bool) {
			So(datastore.Put(ctx, &model.Package{
				Name:   name,
				Hidden: hidden,
			}), ShouldBeNil)
		}

		// Note: "1" is both a package and a prefix, this is allowed.
		mk("1", visible)
		mk("1/a", visible) // note: readable to reader@...
		mk("1/b", visible)
		mk("1/c", hidden)
		mk("1/d/a", hidden)
		mk("1/a/b", visible)   // note: readable to reader@...
		mk("1/a/b/c", visible) // note: readable to reader@...
		mk("1/a/c", hidden)    // note: readable to reader@...
		mk("2/a/b/c", visible)
		mk("3", visible)
		mk("4", hidden)
		mk("5/a/b", hidden)
		mk("6", hidden)      // note: readable to reader@...
		mk("6/a/b", visible) // note: readable to reader@...
		mk("7/a", hidden)    // note: readable to reader@...
		datastore.GetTestable(ctx).CatchupIndexes()

		// Note about the test cases names below:
		//  * "Full" means there are no ACL restriction.
		//  * "Restricted" means some results are filtered out by ACLs.
		//  * "Root" means listing root of the repo.
		//  * "Non-root" means listing some prefix.
		//  * "Recursive" is obvious.
		//  * "Non-recursive" is also obvious.
		//  * "Including hidden" means results includes hidden packages.
		//  * "Visible only" means results includes only non-hidden packages.
		//
		// This 4 test dimensions => 16 test cases.

		Convey("Full listing", func() {
			Convey("Root recursive (including hidden)", func() {
				resp, err := call("", true, true, "user:admin@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string{
					"1", "1/a", "1/a/b", "1/a/b/c", "1/a/c", "1/b", "1/c", "1/d/a",
					"2/a/b/c", "3", "4", "5/a/b", "6", "6/a/b", "7/a",
				})
				So(resp.Prefixes, ShouldResemble, []string{
					"1", "1/a", "1/a/b", "1/d", "2", "2/a", "2/a/b", "5", "5/a",
					"6", "6/a", "7",
				})
			})

			Convey("Root recursive (visible only)", func() {
				resp, err := call("", true, false, "user:admin@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string{
					"1", "1/a", "1/a/b", "1/a/b/c", "1/b", "2/a/b/c", "3", "6/a/b",
				})
				So(resp.Prefixes, ShouldResemble, []string{
					"1", "1/a", "1/a/b", "2", "2/a", "2/a/b", "6", "6/a",
				})
			})

			Convey("Root non-recursive (including hidden)", func() {
				resp, err := call("", false, true, "user:admin@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string{"1", "3", "4", "6"})
				So(resp.Prefixes, ShouldResemble, []string{"1", "2", "5", "6", "7"})
			})

			Convey("Root non-recursive (visible only)", func() {
				resp, err := call("", false, false, "user:admin@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string{"1", "3"})
				So(resp.Prefixes, ShouldResemble, []string{"1", "2", "6"})
			})

			Convey("Non-root recursive (including hidden)", func() {
				resp, err := call("1", true, true, "user:admin@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string{
					"1/a", "1/a/b", "1/a/b/c", "1/a/c", "1/b", "1/c", "1/d/a",
				})
				So(resp.Prefixes, ShouldResemble, []string{"1/a", "1/a/b", "1/d"})
			})

			Convey("Non-root recursive (visible only)", func() {
				resp, err := call("1", true, false, "user:admin@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string{
					"1/a", "1/a/b", "1/a/b/c", "1/b",
				})
				So(resp.Prefixes, ShouldResemble, []string{"1/a", "1/a/b"})
			})
		})

		Convey("Restricted listing", func() {
			Convey("Root recursive (including hidden)", func() {
				resp, err := call("", true, true, "user:reader@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string{
					"1/a", "1/a/b", "1/a/b/c", "1/a/c", "6", "6/a/b", "7/a",
				})
				So(resp.Prefixes, ShouldResemble, []string{
					"1", "1/a", "1/a/b", "6", "6/a", "7",
				})
			})

			Convey("Root recursive (visible only)", func() {
				resp, err := call("", true, false, "user:reader@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string{
					"1/a", "1/a/b", "1/a/b/c", "6/a/b",
				})
				So(resp.Prefixes, ShouldResemble, []string{
					"1", "1/a", "1/a/b", "6", "6/a",
				})
			})

			Convey("Root non-recursive (including hidden)", func() {
				resp, err := call("", false, true, "user:reader@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string{"6"})
				So(resp.Prefixes, ShouldResemble, []string{"1", "6", "7"})
			})

			Convey("Root non-recursive (visible only)", func() {
				resp, err := call("", false, false, "user:reader@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string(nil))
				So(resp.Prefixes, ShouldResemble, []string{"1", "6"})
			})

			Convey("Non-root recursive (including hidden)", func() {
				resp, err := call("1", true, true, "user:reader@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string{
					"1/a", "1/a/b", "1/a/b/c", "1/a/c",
				})
				So(resp.Prefixes, ShouldResemble, []string{"1/a", "1/a/b"})
			})

			Convey("Non-root recursive (visible only)", func() {
				resp, err := call("1", true, false, "user:reader@example.com")
				So(err, ShouldBeNil)
				So(resp.Packages, ShouldResemble, []string{
					"1/a", "1/a/b", "1/a/b/c",
				})
				So(resp.Prefixes, ShouldResemble, []string{"1/a", "1/a/b"})
			})
		})

		Convey("The package is not listed when listing its name directly", func() {
			resp, err := call("3", true, true, "user:admin@example.com")
			So(err, ShouldBeNil)
			So(resp.Packages, ShouldHaveLength, 0)
			So(resp.Prefixes, ShouldHaveLength, 0)
		})

	})
}

func TestRegisterInstance(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		testTime := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx := gaetesting.TestingContext()
		ctx, _ = testclock.UseTime(ctx, testTime)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:owner@example.com",
		})

		cas := testutil.MockCAS{}

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:owner@example.com"},
				},
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		impl := repoImpl{
			tq:   &tq.Dispatcher{BaseURL: "/internal/tq/"},
			meta: &meta,
			cas:  &cas,
		}
		impl.registerTasks()

		tq := tqtesting.GetTestable(ctx, impl.tq)
		tq.CreateQueues()

		digest := strings.Repeat("a", 40)
		inst := &api.Instance{
			Package: "a/b",
			Instance: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: digest,
			},
		}

		Convey("Happy path", func() {
			impl.registerProcessor(&mockedProcessor{
				ProcID:    "proc_id_1",
				AppliesTo: inst.Package,
			})
			impl.registerProcessor(&mockedProcessor{
				ProcID:    "proc_id_2",
				AppliesTo: "something else",
			})

			uploadOp := api.UploadOperation{
				OperationId: "op_id",
				UploadUrl:   "http://fake.example.com",
				Status:      api.UploadStatus_UPLOADING,
			}

			// Mock "successfully started upload op".
			cas.BeginUploadImpl = func(_ context.Context, req *api.BeginUploadRequest) (*api.UploadOperation, error) {
				So(req, ShouldResemble, &api.BeginUploadRequest{
					Object: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: digest,
					},
				})
				return &uploadOp, nil
			}

			// The instance is not uploaded yet => asks to upload.
			resp, err := impl.RegisterInstance(ctx, inst)
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &api.RegisterInstanceResponse{
				Status:   api.RegistrationStatus_NOT_UPLOADED,
				UploadOp: &uploadOp,
			})

			// Mock "already have it in the storage" response.
			cas.BeginUploadImpl = func(context.Context, *api.BeginUploadRequest) (*api.UploadOperation, error) {
				return nil, grpc.Errorf(codes.AlreadyExists, "already uploaded")
			}

			// The instance is already uploaded => registers it in the datastore.
			fullInstProto := &api.Instance{
				Package:      inst.Package,
				Instance:     inst.Instance,
				RegisteredBy: "user:owner@example.com",
				RegisteredTs: google.NewTimestamp(testTime),
			}
			resp, err = impl.RegisterInstance(ctx, inst)
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &api.RegisterInstanceResponse{
				Status:   api.RegistrationStatus_REGISTERED,
				Instance: fullInstProto,
			})

			// Launched post-processors.
			ent := (&model.Instance{}).FromProto(ctx, inst)
			So(datastore.Get(ctx, ent), ShouldBeNil)
			So(ent.ProcessorsPending, ShouldResemble, []string{"proc_id_1"})
			tqt := tq.GetScheduledTasks()
			So(tqt, ShouldHaveLength, 1)
			So(tqt[0].Payload, ShouldResemble, &tasks.RunProcessors{
				Instance: fullInstProto,
			})
		})

		Convey("Already registered", func() {
			instance := (&model.Instance{
				RegisteredBy: "user:someone@example.com",
			}).FromProto(ctx, inst)
			_, _, err := model.RegisterInstance(ctx, instance, nil)
			So(err, ShouldBeNil)

			resp, err := impl.RegisterInstance(ctx, inst)
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &api.RegisterInstanceResponse{
				Status: api.RegistrationStatus_ALREADY_REGISTERED,
				Instance: &api.Instance{
					Package:      inst.Package,
					Instance:     inst.Instance,
					RegisteredBy: "user:someone@example.com",
				},
			})
		})

		Convey("Bad package name", func() {
			_, err := impl.RegisterInstance(ctx, &api.Instance{
				Package: "//a",
			})
			So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'package'")
		})

		Convey("Bad instance ID", func() {
			_, err := impl.RegisterInstance(ctx, &api.Instance{
				Package: "a/b",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: "abc",
				},
			})
			So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'instance'")
		})

		Convey("No reader access", func() {
			_, err := impl.RegisterInstance(ctx, &api.Instance{
				Package:  "some/other/root",
				Instance: inst.Instance,
			})
			So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			So(err, ShouldErrLike, `prefix "some/other/root" doesn't exist or the caller is not allowed to see it`)
		})

		Convey("No owner access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:reader@example.com",
			})
			_, err := impl.RegisterInstance(ctx, inst)
			So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			So(err, ShouldErrLike, `caller has no required WRITER role in prefix "a/b"`)
		})
	})
}

func TestProcessors(t *testing.T) {
	t.Parallel()

	testZip := testutil.MakeZip(map[string]string{
		"file1": strings.Repeat("hello", 50),
		"file2": "blah",
	})

	Convey("With mocks", t, func() {
		testTime := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx := gaetesting.TestingContext()
		ctx, _ = testclock.UseTime(ctx, testTime)

		cas := testutil.MockCAS{}
		impl := repoImpl{cas: &cas}

		inst := &api.Instance{
			Package: "a/b/c",
			Instance: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: strings.Repeat("a", 40),
			},
		}

		storeInstance := func(pending []string) {
			i := (&model.Instance{ProcessorsPending: pending}).FromProto(ctx, inst)
			So(datastore.Put(ctx, i), ShouldBeNil)
		}

		fetchInstance := func() *model.Instance {
			i := (&model.Instance{}).FromProto(ctx, inst)
			So(datastore.Get(ctx, i), ShouldBeNil)
			return i
		}

		fetchProcRes := func(id string) *model.ProcessingResult {
			i := (&model.Instance{}).FromProto(ctx, inst)
			p := &model.ProcessingResult{
				ProcID:   id,
				Instance: datastore.KeyForObj(ctx, i),
			}
			So(datastore.Get(ctx, p), ShouldBeNil)
			return p
		}

		// Note: assumes Result is a string.
		fetchProcSuccess := func(id string) string {
			res := fetchProcRes(id)
			So(res, ShouldNotBeNil)
			So(res.Success, ShouldBeTrue)
			var r string
			So(res.ReadResult(&r), ShouldBeNil)
			return r
		}

		fetchProcFail := func(id string) string {
			res := fetchProcRes(id)
			So(res, ShouldNotBeNil)
			So(res.Success, ShouldBeFalse)
			return res.Error
		}

		Convey("Noop updateProcessors", func() {
			storeInstance([]string{"a", "b"})
			So(impl.updateProcessors(ctx, inst, map[string]processing.Result{
				"some-another": {Err: fmt.Errorf("fail")},
			}), ShouldBeNil)
			So(fetchInstance().ProcessorsPending, ShouldResemble, []string{"a", "b"})
		})

		Convey("Updates processors successfully", func() {
			storeInstance([]string{"ok", "fail", "pending"})

			So(impl.updateProcessors(ctx, inst, map[string]processing.Result{
				"ok":   {Result: "OK"},
				"fail": {Err: fmt.Errorf("failed")},
			}), ShouldBeNil)

			// Updated the Instance entity.
			inst := fetchInstance()
			So(inst.ProcessorsPending, ShouldResemble, []string{"pending"})
			So(inst.ProcessorsSuccess, ShouldResemble, []string{"ok"})
			So(inst.ProcessorsFailure, ShouldResemble, []string{"fail"})

			// Created ProcessingResult entities.
			So(fetchProcSuccess("ok"), ShouldEqual, "OK")
			So(fetchProcFail("fail"), ShouldEqual, "failed")
		})

		Convey("Missing entity in updateProcessors", func() {
			err := impl.updateProcessors(ctx, inst, map[string]processing.Result{
				"proc": {Err: fmt.Errorf("fail")},
			})
			So(err, ShouldErrLike, "the entity is unexpectedly gone")
		})

		Convey("runProcessorsTask happy path", func() {
			// Setup two pending processors that read 'file2'.
			runCB := func(i *model.Instance, r *processing.PackageReader) (processing.Result, error) {
				So(i.Proto(), ShouldResembleProto, inst)

				rd, _, err := r.Open("file2")
				So(err, ShouldBeNil)
				defer rd.Close()
				blob, err := ioutil.ReadAll(rd)
				So(err, ShouldBeNil)
				So(string(blob), ShouldEqual, "blah")

				return processing.Result{Result: "OK"}, nil
			}

			impl.registerProcessor(&mockedProcessor{ProcID: "proc1", RunCB: runCB})
			impl.registerProcessor(&mockedProcessor{ProcID: "proc2", RunCB: runCB})
			storeInstance([]string{"proc1", "proc2"})

			// Setup the package.
			cas.GetReaderImpl = func(_ context.Context, ref *api.ObjectRef) (gs.Reader, error) {
				So(inst.Instance, ShouldResembleProto, ref)
				return testutil.NewMockGSReader(testZip), nil
			}

			// Run the processor.
			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			So(err, ShouldBeNil)

			// Both succeeded.
			inst := fetchInstance()
			So(inst.ProcessorsPending, ShouldHaveLength, 0)
			So(inst.ProcessorsSuccess, ShouldResemble, []string{"proc1", "proc2"})

			// And have the result.
			So(fetchProcSuccess("proc1"), ShouldEqual, "OK")
			So(fetchProcSuccess("proc2"), ShouldEqual, "OK")
		})

		Convey("runProcessorsTask no entity", func() {
			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			So(err, ShouldErrLike, "unexpectedly gone from the datastore")
		})

		Convey("runProcessorsTask no processor", func() {
			storeInstance([]string{"proc"})

			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			So(err, ShouldBeNil)

			// Failed.
			So(fetchProcFail("proc"), ShouldEqual, `unknown processor "proc"`)
		})

		Convey("runProcessorsTask broken package", func() {
			impl.registerProcessor(&mockedProcessor{
				ProcID: "proc",
				Result: processing.Result{Result: "must not be called"},
			})
			storeInstance([]string{"proc"})

			cas.GetReaderImpl = func(_ context.Context, ref *api.ObjectRef) (gs.Reader, error) {
				return testutil.NewMockGSReader([]byte("im not a zip")), nil
			}

			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			So(err, ShouldBeNil)

			// Failed.
			So(fetchProcFail("proc"), ShouldEqual, `error when opening the package: zip: not a valid zip file`)
		})

		Convey("runProcessorsTask propagates transient proc errors", func() {
			impl.registerProcessor(&mockedProcessor{
				ProcID: "good-proc",
				Result: processing.Result{Result: "OK"},
			})
			impl.registerProcessor(&mockedProcessor{
				ProcID: "bad-proc",
				Err:    fmt.Errorf("failed transiently"),
			})
			storeInstance([]string{"good-proc", "bad-proc"})

			cas.GetReaderImpl = func(_ context.Context, ref *api.ObjectRef) (gs.Reader, error) {
				return testutil.NewMockGSReader(testZip), nil
			}

			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			So(transient.Tag.In(err), ShouldBeTrue)
			So(err, ShouldErrLike, "failed transiently")

			// bad-proc is still pending.
			So(fetchInstance().ProcessorsPending, ShouldResemble, []string{"bad-proc"})
			// good-proc is done.
			So(fetchProcSuccess("good-proc"), ShouldEqual, "OK")
		})

		Convey("runProcessorsTask handles fatal errors", func() {
			impl.registerProcessor(&mockedProcessor{
				ProcID: "proc",
				Result: processing.Result{Err: fmt.Errorf("boom")},
			})
			storeInstance([]string{"proc"})

			cas.GetReaderImpl = func(_ context.Context, ref *api.ObjectRef) (gs.Reader, error) {
				return testutil.NewMockGSReader(testZip), nil
			}

			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			So(err, ShouldBeNil)

			// Failed.
			So(fetchProcFail("proc"), ShouldEqual, "boom")
		})
	})
}

// mockedProcessor implements processing.Processor interface.
type mockedProcessor struct {
	ProcID    string
	AppliesTo string

	RunCB  func(*model.Instance, *processing.PackageReader) (processing.Result, error)
	Result processing.Result
	Err    error
}

func (m *mockedProcessor) ID() string {
	return m.ProcID
}

func (m *mockedProcessor) Applicable(inst *model.Instance) bool {
	return inst.Package.StringID() == m.AppliesTo
}

func (m *mockedProcessor) Run(_ context.Context, i *model.Instance, r *processing.PackageReader) (processing.Result, error) {
	if m.RunCB != nil {
		return m.RunCB(i, r)
	}
	return m.Result, m.Err
}
