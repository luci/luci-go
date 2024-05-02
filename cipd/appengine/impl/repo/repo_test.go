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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/repo/processing"
	"go.chromium.org/luci/cipd/appengine/impl/repo/tasks"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	// Using transactional datastore TQ tasks.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

////////////////////////////////////////////////////////////////////////////////
// Prefix metadata RPC methods + related helpers including ACL checks.

func TestMetadataFetching(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		_, _, as := testutil.TestingContext(
			authtest.MockMembership("user:prefixes-viewer@example.com", PrefixesViewers),
		)

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
			return impl.GetPrefixMetadata(as(user.Email()), &api.PrefixRequest{Prefix: prefix})
		}

		callGetInherited := func(prefix string, user identity.Identity) ([]*api.PrefixMetadata, error) {
			resp, err := impl.GetInheritedPrefixMetadata(as(user.Email()), &api.PrefixRequest{Prefix: prefix})
			if err != nil {
				return nil, err
			}
			return resp.PerPrefixMetadata, nil
		}

		Convey("GetPrefixMetadata happy path", func() {
			resp, err := callGet("a/b/c/d", "user:top-owner@example.com")
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, leafMeta)
		})

		Convey("GetPrefixMetadata happy path via global group", func() {
			resp, err := callGet("a/b/c/d", "user:prefixes-viewer@example.com")
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, leafMeta)
		})

		Convey("GetInheritedPrefixMetadata happy path", func() {
			resp, err := callGetInherited("a/b/c/d", "user:top-owner@example.com")
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, []*api.PrefixMetadata{rootMeta, topMeta, leafMeta})
		})

		Convey("GetInheritedPrefixMetadata happy path via global group", func() {
			resp, err := callGetInherited("a/b/c/d", "user:prefixes-viewer@example.com")
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, []*api.PrefixMetadata{rootMeta, topMeta, leafMeta})
		})

		Convey("GetPrefixMetadata bad prefix", func() {
			resp, err := callGet("a//", "user:top-owner@example.com")
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(resp, ShouldBeNil)
		})

		Convey("GetInheritedPrefixMetadata bad prefix", func() {
			resp, err := callGetInherited("a//", "user:top-owner@example.com")
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(resp, ShouldBeNil)
		})

		Convey("GetPrefixMetadata no metadata, caller has access", func() {
			resp, err := callGet("a/b", "user:top-owner@example.com")
			So(status.Code(err), ShouldEqual, codes.NotFound)
			So(resp, ShouldBeNil)
		})

		Convey("GetInheritedPrefixMetadata no metadata, caller has access", func() {
			resp, err := callGetInherited("a/b", "user:top-owner@example.com")
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, []*api.PrefixMetadata{rootMeta, topMeta})
		})

		Convey("GetPrefixMetadata no metadata, caller has no access", func() {
			resp, err := callGet("a/b", "user:someone-else@example.com")
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
			// Existing metadata that the caller has no access to produces same error,
			// so unauthorized callers can't easily distinguish between the two.
			resp, err = callGet("a/b/c/d", "user:someone-else@example.com")
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
			// Same for completely unknown prefix.
			resp, err = callGet("zzz", "user:someone-else@example.com")
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
		})

		Convey("GetInheritedPrefixMetadata no metadata, caller has no access", func() {
			resp, err := callGetInherited("a/b", "user:someone-else@example.com")
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
			// Existing metadata that the caller has no access to produces same error,
			// so unauthorized callers can't easily distinguish between the two.
			resp, err = callGetInherited("a/b/c/d", "user:someone-else@example.com")
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
			// Same for completely unknown prefix.
			resp, err = callGetInherited("zzz", "user:someone-else@example.com")
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(resp, ShouldBeNil)
		})
	})
}

func TestMetadataUpdating(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, tc, as := testutil.TestingContext()

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

		callUpdate := func(user identity.Identity, m *api.PrefixMetadata) (md *api.PrefixMetadata, err error) {
			err = datastore.RunInTransaction(as(user.Email()), func(ctx context.Context) (err error) {
				md, err = impl.UpdatePrefixMetadata(ctx, m)
				return
			}, nil)
			return
		}

		Convey("Happy path", func() {
			// Create new metadata entry.
			meta, err := callUpdate("user:top-owner@example.com", &api.PrefixMetadata{
				Prefix:     "a/b/",
				UpdateTime: timestamppb.New(time.Unix(10000, 0)), // should be overwritten
				UpdateUser: "user:zzz@example.com",               // should be overwritten
				Acls: []*api.PrefixMetadata_ACL{
					{Role: api.Role_READER, Principals: []string{"user:reader@example.com"}},
				},
			})
			So(err, ShouldBeNil)

			expected := &api.PrefixMetadata{
				Prefix:      "a/b",
				Fingerprint: "WZllwc6m8f9C_rfwnspaPIiyPD0",
				UpdateTime:  timestamppb.New(testutil.TestTime),
				UpdateUser:  "user:top-owner@example.com",
				Acls: []*api.PrefixMetadata_ACL{
					{Role: api.Role_READER, Principals: []string{"user:reader@example.com"}},
				},
			}
			So(meta, ShouldResembleProto, expected)

			// Update it a bit later.
			tc.Add(time.Hour)
			updated := proto.Clone(expected).(*api.PrefixMetadata)
			updated.Acls = nil
			meta, err = callUpdate("user:top-owner@example.com", updated)
			So(err, ShouldBeNil)
			So(meta, ShouldResembleProto, &api.PrefixMetadata{
				Prefix:      "a/b",
				Fingerprint: "oQ2uuVbjV79prXxl4jyJkOpff90",
				UpdateTime:  timestamppb.New(testutil.TestTime.Add(time.Hour)),
				UpdateUser:  "user:top-owner@example.com",
			})

			// Have these in the event log as well.
			datastore.GetTestable(ctx).CatchupIndexes()
			ev, err := model.QueryEvents(ctx, model.NewEventsQuery())
			So(err, ShouldBeNil)
			So(ev, ShouldResembleProto, []*api.Event{
				{
					Kind:    api.EventKind_PREFIX_ACL_CHANGED,
					Package: "a/b",
					Who:     "user:top-owner@example.com",
					When:    timestamppb.New(testutil.TestTime.Add(time.Hour)),
					RevokedRole: []*api.PrefixMetadata_ACL{
						{Role: api.Role_READER, Principals: []string{"user:reader@example.com"}},
					},
				},
				{
					Kind:    api.EventKind_PREFIX_ACL_CHANGED,
					Package: "a/b",
					Who:     "user:top-owner@example.com",
					When:    timestamppb.New(testutil.TestTime),
					GrantedRole: []*api.PrefixMetadata_ACL{
						{Role: api.Role_READER, Principals: []string{"user:reader@example.com"}},
					},
				},
			})
		})

		Convey("Validation works", func() {
			meta, err := callUpdate("user:top-owner@example.com", &api.PrefixMetadata{
				Prefix: "a/b//",
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(meta, ShouldBeNil)

			meta, err = callUpdate("user:top-owner@example.com", &api.PrefixMetadata{
				Prefix: "a/b",
				Acls: []*api.PrefixMetadata_ACL{
					{Role: api.Role_READER, Principals: []string{"huh?"}},
				},
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(meta, ShouldBeNil)
		})

		Convey("ACLs work", func() {
			meta, err := callUpdate("user:unknown@example.com", &api.PrefixMetadata{
				Prefix: "a/b",
			})
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(meta, ShouldBeNil)

			// Same as completely unknown prefix.
			meta, err = callUpdate("user:unknown@example.com", &api.PrefixMetadata{
				Prefix: "zzz",
			})
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(meta, ShouldBeNil)
		})

		Convey("Deleted concurrently", func() {
			m := meta.Populate("a/b", &api.PrefixMetadata{
				UpdateUser: "user:someone@example.com",
			})
			meta.Purge("a/b")

			// If the caller is a prefix owner, they see NotFound.
			meta, err := callUpdate("user:top-owner@example.com", m)
			So(status.Code(err), ShouldEqual, codes.NotFound)
			So(meta, ShouldBeNil)

			// Other callers just see regular PermissionDenined.
			meta, err = callUpdate("user:unknown@example.com", m)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(meta, ShouldBeNil)
		})

		Convey("Creating existing", func() {
			m := meta.Populate("a/b", &api.PrefixMetadata{
				UpdateUser: "user:someone@example.com",
			})

			m.Fingerprint = "" // indicates the caller is expecting to create a new one
			meta, err := callUpdate("user:top-owner@example.com", m)
			So(status.Code(err), ShouldEqual, codes.AlreadyExists)
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
			So(status.Code(err), ShouldEqual, codes.FailedPrecondition)
		})
	})
}

func TestGetRolesInPrefixOnBehalfOf(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		rootCtx, _, _ := testutil.TestingContext()

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

		call := func(prefix string, user identity.Identity, callerIsPrefixViewer bool) (*api.RolesInPrefixResponse, error) {
			caller := identity.Identity("user:roles-caller@example.com")
			mocks := []authtest.MockedDatum{}
			if callerIsPrefixViewer {
				mocks = append(mocks, authtest.MockMembership(caller, PrefixesViewers))
			}
			return impl.GetRolesInPrefixOnBehalfOf(auth.WithState(rootCtx, &authtest.FakeState{
				Identity: caller,
				FakeDB:   authtest.NewFakeDB(mocks...),
			}), &api.PrefixRequestOnBehalfOf{Identity: string(user), PrefixRequest: &api.PrefixRequest{Prefix: prefix}})
		}

		Convey("Happy path", func() {
			resp, err := call("a/b/c/d", "user:writer@example.com", true)
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &api.RolesInPrefixResponse{
				Roles: []*api.RolesInPrefixResponse_RoleInPrefix{
					{Role: api.Role_READER},
					{Role: api.Role_WRITER},
				},
			})
		})

		Convey("Anonymous", func() {
			resp, err := call("a/b/c/d", "anonymous:anonymous", true)
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &api.RolesInPrefixResponse{})
		})

		Convey("Admin", func() {
			resp, err := call("a/b/c/d", "user:admin@example.com", true)
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
			_, err := call("///", "user:writer@example.com", true)
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'prefix'")
		})

		Convey("Not prefix viewer", func() {
			_, err := call("a/b/c/d", "user:writer@example.com", false)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("Disallowed identity types", func() {
			for _, kind := range []identity.Kind{identity.Bot, identity.Project, identity.Service} {
				ident, err := identity.MakeIdentity(string(kind) + ":somevalue")
				So(err, ShouldBeNil)

				_, err = call("a/b/c/d", ident, true)
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			}
		})

	})
}

func TestGetRolesInPrefix(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		rootCtx, _, _ := testutil.TestingContext()

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
			return impl.GetRolesInPrefix(auth.WithState(rootCtx, &authtest.FakeState{
				Identity: user,
				FakeDB:   authtest.NewFakeDB(),
			}), &api.PrefixRequest{Prefix: prefix})
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
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'prefix'")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Prefix listing.

func TestListPrefix(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()

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
			return impl.ListPrefix(as(user.Email()), &api.ListPrefixRequest{
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

////////////////////////////////////////////////////////////////////////////////
// Hide/unhide package.

func TestHideUnhidePackage(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()
		ctx = as("owner@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:owner@example.com"},
				},
			},
		})

		So(datastore.Put(ctx, &model.Package{Name: "a/b"}), ShouldBeNil)

		fetch := func(pkg string) *model.Package {
			p := &model.Package{Name: pkg}
			So(datastore.Get(ctx, p), ShouldBeNil)
			return p
		}

		impl := repoImpl{meta: &meta}

		Convey("Hides and unhides", func() {
			_, err := impl.HidePackage(ctx, &api.PackageRequest{Package: "a/b"})
			So(err, ShouldBeNil)
			So(fetch("a/b").Hidden, ShouldBeTrue)

			// Noop is fine.
			_, err = impl.HidePackage(ctx, &api.PackageRequest{Package: "a/b"})
			So(err, ShouldBeNil)
			So(fetch("a/b").Hidden, ShouldBeTrue)

			_, err = impl.UnhidePackage(ctx, &api.PackageRequest{Package: "a/b"})
			So(err, ShouldBeNil)
			So(fetch("a/b").Hidden, ShouldBeFalse)
		})

		Convey("Bad package name", func() {
			_, err := impl.HidePackage(ctx, &api.PackageRequest{Package: "///"})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("No access", func() {
			_, err := impl.HidePackage(ctx, &api.PackageRequest{Package: "zzz"})
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(err, ShouldErrLike, "not allowed to see it")
		})

		Convey("Missing package", func() {
			_, err := impl.HidePackage(ctx, &api.PackageRequest{Package: "a/b/c"})
			So(status.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such package")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Package deletion.

func TestDeletePackage(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()

		meta := testutil.MetadataStore{}
		meta.Populate("", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:root@example.com"},
				},
			},
		})
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:non-root-owner@example.com"},
				},
			},
		})

		So(datastore.Put(ctx, &model.Package{Name: "a/b"}), ShouldBeNil)
		So(model.CheckPackageExists(ctx, "a/b"), ShouldBeNil)

		impl := repoImpl{meta: &meta}

		Convey("Works", func() {
			_, err := impl.DeletePackage(as("root@example.com"), &api.PackageRequest{
				Package: "a/b",
			})
			So(err, ShouldBeNil)

			// Gone now.
			So(model.CheckPackageExists(ctx, "a/b"), ShouldNotBeNil)

			_, err = impl.DeletePackage(as("root@example.com"), &api.PackageRequest{
				Package: "a/b",
			})
			So(status.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such package")
		})

		Convey("Only reader or above can see", func() {
			_, err := impl.DeletePackage(as("someone@example.com"), &api.PackageRequest{
				Package: "a/b",
			})
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(err, ShouldErrLike, "is not allowed to see it")
		})

		Convey("Only root owner can delete", func() {
			_, err := impl.DeletePackage(as("non-root-owner@example.com"), &api.PackageRequest{
				Package: "a/b",
			})
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(err, ShouldErrLike, "allowed only to service administrators")
		})

		Convey("Bad package name", func() {
			_, err := impl.DeletePackage(ctx, &api.PackageRequest{Package: "///"})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "invalid package name")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Package instance registration and post-registration processing.

func TestRegisterInstance(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()
		ctx = as("owner@example.com")

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

		dispatcher := &tq.Dispatcher{}
		ctx, sched := tq.TestingContext(ctx, dispatcher)

		impl := repoImpl{
			tq:   dispatcher,
			meta: &meta,
			cas:  &cas,
		}
		impl.registerTasks()

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
				So(req, ShouldResembleProto, &api.BeginUploadRequest{
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
			So(resp, ShouldResembleProto, &api.RegisterInstanceResponse{
				Status:   api.RegistrationStatus_NOT_UPLOADED,
				UploadOp: &uploadOp,
			})

			// Mock "already have it in the storage" response.
			cas.BeginUploadImpl = func(context.Context, *api.BeginUploadRequest) (*api.UploadOperation, error) {
				return nil, status.Errorf(codes.AlreadyExists, "already uploaded")
			}

			// The instance is already uploaded => registers it in the datastore.
			fullInstProto := &api.Instance{
				Package:      inst.Package,
				Instance:     inst.Instance,
				RegisteredBy: "user:owner@example.com",
				RegisteredTs: timestamppb.New(testutil.TestTime),
			}
			resp, err = impl.RegisterInstance(ctx, inst)
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &api.RegisterInstanceResponse{
				Status:   api.RegistrationStatus_REGISTERED,
				Instance: fullInstProto,
			})

			// Launched post-processors.
			ent := (&model.Instance{}).FromProto(ctx, inst)
			So(datastore.Get(ctx, ent), ShouldBeNil)
			So(ent.ProcessorsPending, ShouldResemble, []string{"proc_id_1"})
			tqt := sched.Tasks()
			So(tqt, ShouldHaveLength, 1)
			So(tqt[0].Payload, ShouldResembleProto, &tasks.RunProcessors{
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
			So(resp, ShouldResembleProto, &api.RegisterInstanceResponse{
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
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
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
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'instance'")
		})

		Convey("No reader access", func() {
			_, err := impl.RegisterInstance(ctx, &api.Instance{
				Package:  "some/other/root",
				Instance: inst.Instance,
			})
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(err, ShouldErrLike, `prefix "some/other/root" doesn't exist or "user:owner@example.com" is not allowed to see it`)
		})

		Convey("No owner access", func() {
			_, err := impl.RegisterInstance(as("reader@example.com"), inst)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(err, ShouldErrLike, `"user:reader@example.com" has no required WRITER role in prefix "a/b"`)
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
		ctx, _, _ := testutil.TestingContext()

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

		goodResult := map[string]string{"result": "OK"}

		// Note: assumes Result is a map[string]string.
		fetchProcSuccess := func(id string) string {
			res := fetchProcRes(id)
			So(res, ShouldNotBeNil)
			So(res.Success, ShouldBeTrue)
			var r map[string]string
			So(res.ReadResult(&r), ShouldBeNil)
			return r["result"]
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
				"ok":   {Result: goodResult},
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
				blob, err := io.ReadAll(rd)
				So(err, ShouldBeNil)
				So(string(blob), ShouldEqual, "blah")

				return processing.Result{Result: goodResult}, nil
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
				Result: processing.Result{Result: goodResult},
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

func (m *mockedProcessor) Applicable(ctx context.Context, inst *model.Instance) (bool, error) {
	return inst.Package.StringID() == m.AppliesTo, nil
}

func (m *mockedProcessor) Run(_ context.Context, i *model.Instance, r *processing.PackageReader) (processing.Result, error) {
	if m.RunCB != nil {
		return m.RunCB(i, r)
	}
	return m.Result, m.Err
}

////////////////////////////////////////////////////////////////////////////////
// Instance listing and querying.

func TestListInstances(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ts := time.Unix(1525136124, 0).UTC()
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		So(datastore.Put(ctx, &model.Package{Name: "a/b"}), ShouldBeNil)
		So(datastore.Put(ctx, &model.Package{Name: "a/empty"}), ShouldBeNil)

		for i := 0; i < 4; i++ {
			So(datastore.Put(ctx, &model.Instance{
				InstanceID:   fmt.Sprintf("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa%d", i),
				Package:      model.PackageKey(ctx, "a/b"),
				RegisteredTs: ts.Add(time.Duration(i) * time.Minute),
			}), ShouldBeNil)
		}

		inst := func(i int) *api.Instance {
			return &api.Instance{
				Package: "a/b",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: fmt.Sprintf("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa%d", i),
				},
				RegisteredTs: timestamppb.New(ts.Add(time.Duration(i) * time.Minute)),
			}
		}

		impl := repoImpl{meta: &meta}

		Convey("Bad package name", func() {
			_, err := impl.ListInstances(ctx, &api.ListInstancesRequest{
				Package: "///",
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Bad page size", func() {
			_, err := impl.ListInstances(ctx, &api.ListInstancesRequest{
				Package:  "a/b",
				PageSize: -1,
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "it should be non-negative")
		})

		Convey("Bad page token", func() {
			_, err := impl.ListInstances(ctx, &api.ListInstancesRequest{
				Package:   "a/b",
				PageToken: "zzzz",
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "invalid cursor")
		})

		Convey("No access", func() {
			_, err := impl.ListInstances(ctx, &api.ListInstancesRequest{
				Package: "z",
			})
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("No package", func() {
			_, err := impl.ListInstances(ctx, &api.ListInstancesRequest{
				Package: "a/missing",
			})
			So(status.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("Empty listing", func() {
			res, err := impl.ListInstances(ctx, &api.ListInstancesRequest{
				Package: "a/empty",
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &api.ListInstancesResponse{})
		})

		Convey("Full listing (no pagination)", func() {
			res, err := impl.ListInstances(ctx, &api.ListInstancesRequest{
				Package: "a/b",
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &api.ListInstancesResponse{
				Instances: []*api.Instance{inst(3), inst(2), inst(1), inst(0)},
			})
		})

		Convey("Listing with pagination", func() {
			// First page.
			res, err := impl.ListInstances(ctx, &api.ListInstancesRequest{
				Package:  "a/b",
				PageSize: 3,
			})
			So(err, ShouldBeNil)
			So(res.Instances, ShouldResembleProto, []*api.Instance{
				inst(3), inst(2), inst(1),
			})
			So(res.NextPageToken, ShouldNotEqual, "")

			// Second page.
			res, err = impl.ListInstances(ctx, &api.ListInstancesRequest{
				Package:   "a/b",
				PageSize:  3,
				PageToken: res.NextPageToken,
			})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &api.ListInstancesResponse{
				Instances: []*api.Instance{inst(0)},
			})
		})
	})
}

func TestSearchInstances(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		So(datastore.Put(ctx, &model.Package{Name: "a/b"}), ShouldBeNil)

		put := func(when int, iid string, tags ...string) {
			inst := &model.Instance{
				InstanceID:   iid,
				Package:      model.PackageKey(ctx, "a/b"),
				RegisteredTs: testutil.TestTime.Add(time.Duration(when) * time.Second),
			}
			ents := make([]*model.Tag, len(tags))
			for i, t := range tags {
				ents[i] = &model.Tag{
					ID:           model.TagID(common.MustParseInstanceTag(t)),
					Instance:     datastore.KeyForObj(ctx, inst),
					Tag:          t,
					RegisteredTs: testutil.TestTime.Add(time.Duration(when) * time.Second),
				}
			}
			So(datastore.Put(ctx, inst, ents), ShouldBeNil)
		}

		iid := func(i int) string {
			ch := string([]byte{'0' + byte(i)})
			return strings.Repeat(ch, 40)
		}

		ids := func(inst []*api.Instance) []string {
			out := make([]string, len(inst))
			for i, obj := range inst {
				out[i] = common.ObjectRefToInstanceID(obj.Instance)
			}
			return out
		}

		expectedIIDs := make([]string, 10)
		for i := 0; i < 10; i++ {
			put(i, iid(i), "a:b")
			expectedIIDs[9-i] = iid(i) // sorted by creation time, most recent first
		}

		impl := repoImpl{meta: &meta}

		Convey("Bad package name", func() {
			_, err := impl.SearchInstances(ctx, &api.SearchInstancesRequest{
				Package: "///",
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Bad page size", func() {
			_, err := impl.SearchInstances(ctx, &api.SearchInstancesRequest{
				Package:  "a/b",
				PageSize: -1,
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "it should be non-negative")
		})

		Convey("Bad page token", func() {
			_, err := impl.SearchInstances(ctx, &api.SearchInstancesRequest{
				Package:   "a/b",
				PageToken: "zzzz",
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "invalid cursor")
		})

		Convey("No tags specified", func() {
			_, err := impl.SearchInstances(ctx, &api.SearchInstancesRequest{
				Package: "a/b",
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'tags': cannot be empty")
		})

		Convey("Bad tag given", func() {
			_, err := impl.SearchInstances(ctx, &api.SearchInstancesRequest{
				Package: "a/b",
				Tags:    []*api.Tag{{Key: "", Value: "zz"}},
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, `bad tag in 'tags': invalid tag key in ":zz"`)
		})

		Convey("No access", func() {
			_, err := impl.SearchInstances(ctx, &api.SearchInstancesRequest{
				Package: "z",
				Tags:    []*api.Tag{{Key: "a", Value: "b"}},
			})
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("No package", func() {
			_, err := impl.SearchInstances(ctx, &api.SearchInstancesRequest{
				Package: "a/missing",
				Tags:    []*api.Tag{{Key: "a", Value: "b"}},
			})
			So(status.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey("Empty results", func() {
			out, err := impl.SearchInstances(ctx, &api.SearchInstancesRequest{
				Package: "a/b",
				Tags:    []*api.Tag{{Key: "a", Value: "missing"}},
			})
			So(err, ShouldBeNil)
			So(ids(out.Instances), ShouldHaveLength, 0)
			So(out.NextPageToken, ShouldEqual, "")
		})

		Convey("Full listing (no pagination)", func() {
			out, err := impl.SearchInstances(ctx, &api.SearchInstancesRequest{
				Package: "a/b",
				Tags:    []*api.Tag{{Key: "a", Value: "b"}},
			})
			So(err, ShouldBeNil)
			So(ids(out.Instances), ShouldResemble, expectedIIDs)
			So(out.NextPageToken, ShouldEqual, "")
		})

		Convey("Listing with pagination", func() {
			out, err := impl.SearchInstances(ctx, &api.SearchInstancesRequest{
				Package:  "a/b",
				Tags:     []*api.Tag{{Key: "a", Value: "b"}},
				PageSize: 6,
			})
			So(err, ShouldBeNil)
			So(ids(out.Instances), ShouldResemble, expectedIIDs[:6])
			So(out.NextPageToken, ShouldNotEqual, "")

			out, err = impl.SearchInstances(ctx, &api.SearchInstancesRequest{
				Package:   "a/b",
				Tags:      []*api.Tag{{Key: "a", Value: "b"}},
				PageSize:  6,
				PageToken: out.NextPageToken,
			})
			So(err, ShouldBeNil)
			So(ids(out.Instances), ShouldResemble, expectedIIDs[6:])
			So(out.NextPageToken, ShouldEqual, "")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Refs support.

func TestRefs(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()
		ctx = as("writer@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_WRITER,
					Principals: []string{"user:writer@example.com"},
				},
			},
		})

		putInst := func(pkg, iid string, pendingProcs, failedProcs []string) {
			So(datastore.Put(ctx,
				&model.Package{Name: pkg},
				&model.Instance{
					InstanceID:        iid,
					Package:           model.PackageKey(ctx, pkg),
					ProcessorsPending: pendingProcs,
					ProcessorsFailure: failedProcs,
				}), ShouldBeNil)
		}

		digest := strings.Repeat("a", 40)
		putInst("a/b/c", digest, nil, nil)

		impl := repoImpl{meta: &meta}

		Convey("CreateRef/ListRefs/DeleteRef happy path", func() {
			_, err := impl.CreateRef(ctx, &api.Ref{
				Name:    "latest",
				Package: "a/b/c",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: digest,
				},
			})
			So(err, ShouldBeNil)

			// Can be listed now.
			refs, err := impl.ListRefs(ctx, &api.ListRefsRequest{Package: "a/b/c"})
			So(err, ShouldBeNil)
			So(refs.Refs, ShouldResembleProto, []*api.Ref{
				{
					Name:    "latest",
					Package: "a/b/c",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: digest,
					},
					ModifiedBy: "user:writer@example.com",
					ModifiedTs: timestamppb.New(testutil.TestTime),
				},
			})

			_, err = impl.DeleteRef(ctx, &api.DeleteRefRequest{
				Name:    "latest",
				Package: "a/b/c",
			})
			So(err, ShouldBeNil)

			// Missing now.
			refs, err = impl.ListRefs(ctx, &api.ListRefsRequest{Package: "a/b/c"})
			So(err, ShouldBeNil)
			So(refs.Refs, ShouldHaveLength, 0)
		})

		Convey("Bad ref", func() {
			Convey("CreateRef", func() {
				_, err := impl.CreateRef(ctx, &api.Ref{
					Name:    "bad:ref:name",
					Package: "a/b/c",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: digest,
					},
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'name'")
			})
			Convey("DeleteRef", func() {
				_, err := impl.DeleteRef(ctx, &api.DeleteRefRequest{
					Name:    "bad:ref:name",
					Package: "a/b/c",
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'name'")
			})
		})

		Convey("Bad package name", func() {
			Convey("CreateRef", func() {
				_, err := impl.CreateRef(ctx, &api.Ref{
					Name:    "latest",
					Package: "///",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: digest,
					},
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'package'")
			})
			Convey("DeleteRef", func() {
				_, err := impl.DeleteRef(ctx, &api.DeleteRefRequest{
					Name:    "latest",
					Package: "///",
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'package'")
			})
			Convey("ListRefs", func() {
				_, err := impl.ListRefs(ctx, &api.ListRefsRequest{
					Package: "///",
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'package'")
			})
		})

		Convey("No access", func() {
			Convey("CreateRef", func() {
				_, err := impl.CreateRef(ctx, &api.Ref{
					Name:    "latest",
					Package: "z",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: digest,
					},
				})
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
				So(err, ShouldErrLike, "is not allowed to see it")
			})
			Convey("DeleteRef", func() {
				_, err := impl.DeleteRef(ctx, &api.DeleteRefRequest{
					Name:    "latest",
					Package: "z",
				})
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
				So(err, ShouldErrLike, "is not allowed to see it")
			})
			Convey("ListRefs", func() {
				_, err := impl.ListRefs(ctx, &api.ListRefsRequest{
					Package: "z",
				})
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
				So(err, ShouldErrLike, "is not allowed to see it")
			})
		})

		Convey("Missing package", func() {
			Convey("CreateRef", func() {
				_, err := impl.CreateRef(ctx, &api.Ref{
					Name:    "latest",
					Package: "a/b/z",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: digest,
					},
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
			Convey("DeleteRef", func() {
				_, err := impl.DeleteRef(ctx, &api.DeleteRefRequest{
					Name:    "latest",
					Package: "a/b/z",
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
			Convey("ListRefs", func() {
				_, err := impl.ListRefs(ctx, &api.ListRefsRequest{
					Package: "a/b/z",
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
		})

		Convey("Bad instance", func() {
			_, err := impl.CreateRef(ctx, &api.Ref{
				Name:    "latest",
				Package: "a/b/c",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: "123",
				},
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'instance'")
		})

		Convey("Missing instance", func() {
			_, err := impl.CreateRef(ctx, &api.Ref{
				Name:    "latest",
				Package: "a/b/c",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: strings.Repeat("b", 40),
				},
			})
			So(status.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such instance")
		})

		Convey("Instance is not ready yet", func() {
			putInst("a/b/c", digest, []string{"proc"}, nil)
			_, err := impl.CreateRef(ctx, &api.Ref{
				Name:    "latest",
				Package: "a/b/c",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: digest,
				},
			})
			So(status.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "the instance is not ready yet, pending processors: proc")
		})

		Convey("Failed processors", func() {
			putInst("a/b/c", digest, nil, []string{"proc"})
			_, err := impl.CreateRef(ctx, &api.Ref{
				Name:    "latest",
				Package: "a/b/c",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: digest,
				},
			})
			So(status.Code(err), ShouldEqual, codes.Aborted)
			So(err, ShouldErrLike, "some processors failed to process this instance: proc")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Tags support.

func TestTags(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
				{
					Role:       api.Role_WRITER,
					Principals: []string{"user:writer@example.com"},
				},
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:owner@example.com"},
				},
			},
		})

		putInst := func(pkg, iid string, pendingProcs, failedProcs []string) *model.Instance {
			inst := &model.Instance{
				InstanceID:        iid,
				Package:           model.PackageKey(ctx, pkg),
				ProcessorsPending: pendingProcs,
				ProcessorsFailure: failedProcs,
			}
			So(datastore.Put(ctx, &model.Package{Name: pkg}, inst), ShouldBeNil)
			return inst
		}

		getTag := func(inst *model.Instance, tag string) *model.Tag {
			t := &model.Tag{
				ID:       model.TagID(common.MustParseInstanceTag(tag)),
				Instance: datastore.KeyForObj(ctx, inst),
			}
			err := datastore.Get(ctx, t)
			if err == datastore.ErrNoSuchEntity {
				return nil
			}
			So(err, ShouldBeNil)
			return t
		}

		tags := func(t ...string) []*api.Tag {
			out := make([]*api.Tag, len(t))
			for i, s := range t {
				out[i] = common.MustParseInstanceTag(s)
			}
			return out
		}

		digest := strings.Repeat("a", 40)
		inst := putInst("a/b/c", digest, nil, nil)
		objRef := &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: digest,
		}

		impl := repoImpl{meta: &meta}

		Convey("AttachTags/DetachTags happy path", func() {
			_, err := impl.AttachTags(as("writer@example.com"), &api.AttachTagsRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Tags:     tags("a:0", "a:1"),
			})
			So(err, ShouldBeNil)

			// Attached both.
			So(getTag(inst, "a:0").RegisteredBy, ShouldEqual, "user:writer@example.com")
			So(getTag(inst, "a:1").RegisteredBy, ShouldEqual, "user:writer@example.com")

			// Detaching requires OWNER.
			_, err = impl.DetachTags(as("owner@example.com"), &api.DetachTagsRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Tags:     tags("a:0", "a:1", "a:missing"),
			})
			So(err, ShouldBeNil)

			// Missing now.
			So(getTag(inst, "a:0"), ShouldBeNil)
			So(getTag(inst, "a:1"), ShouldBeNil)
		})

		Convey("Bad package", func() {
			Convey("AttachTags", func() {
				_, err := impl.AttachTags(as("owner@example.com"), &api.AttachTagsRequest{
					Package:  "a/b///",
					Instance: objRef,
					Tags:     tags("a:0"),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'package'")
			})
			Convey("DetachTags", func() {
				_, err := impl.DetachTags(as("owner@example.com"), &api.DetachTagsRequest{
					Package:  "a/b///",
					Instance: objRef,
					Tags:     tags("a:0"),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'package'")
			})
		})

		Convey("Bad ObjectRef", func() {
			Convey("AttachTags", func() {
				_, err := impl.AttachTags(as("owner@example.com"), &api.AttachTagsRequest{
					Package: "a/b/c",
					Tags:    tags("a:0"),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'instance'")
			})
			Convey("DetachTags", func() {
				_, err := impl.DetachTags(as("owner@example.com"), &api.DetachTagsRequest{
					Package: "a/b/c",
					Tags:    tags("a:0"),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'instance'")
			})
		})

		Convey("Empty tag list", func() {
			Convey("AttachTags", func() {
				_, err := impl.AttachTags(as("owner@example.com"), &api.AttachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "cannot be empty")
			})
			Convey("DetachTags", func() {
				_, err := impl.DetachTags(as("owner@example.com"), &api.DetachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "cannot be empty")
			})
		})

		Convey("Bad tag", func() {
			Convey("AttachTags", func() {
				_, err := impl.AttachTags(as("owner@example.com"), &api.AttachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Tags:     []*api.Tag{{Key: ":"}},
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, `invalid tag key`)
			})
			Convey("DetachTags", func() {
				_, err := impl.DetachTags(as("owner@example.com"), &api.DetachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Tags:     []*api.Tag{{Key: ":"}},
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, `invalid tag key`)
			})
		})

		Convey("No access", func() {
			Convey("AttachTags", func() {
				_, err := impl.AttachTags(as("reader@example.com"), &api.AttachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Tags:     tags("good:tag"),
				})
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
				So(err, ShouldErrLike, "has no required WRITER role")
			})
			Convey("DetachTags", func() {
				_, err := impl.DetachTags(as("writer@example.com"), &api.DetachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Tags:     tags("good:tag"),
				})
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
				So(err, ShouldErrLike, "has no required OWNER role")
			})
		})

		Convey("Missing package", func() {
			Convey("AttachTags", func() {
				_, err := impl.AttachTags(as("owner@example.com"), &api.AttachTagsRequest{
					Package:  "a/b/zzz",
					Instance: objRef,
					Tags:     tags("a:0"),
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
			Convey("DetachTags", func() {
				_, err := impl.DetachTags(as("owner@example.com"), &api.DetachTagsRequest{
					Package:  "a/b/zzz",
					Instance: objRef,
					Tags:     tags("a:0"),
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
		})

		Convey("Missing instance", func() {
			missingRef := &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: strings.Repeat("b", 40),
			}
			Convey("AttachTags", func() {
				_, err := impl.AttachTags(as("owner@example.com"), &api.AttachTagsRequest{
					Package:  "a/b/c",
					Instance: missingRef,
					Tags:     tags("a:0"),
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such instance")
			})
			Convey("DetachTags", func() {
				// DetachTags doesn't care.
				_, err := impl.DetachTags(as("owner@example.com"), &api.DetachTagsRequest{
					Package:  "a/b/c",
					Instance: missingRef,
					Tags:     tags("a:0"),
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such instance")
			})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Instance metadata support.

func TestInstanceMetadata(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
				{
					Role:       api.Role_WRITER,
					Principals: []string{"user:writer@example.com"},
				},
				{
					Role:       api.Role_OWNER,
					Principals: []string{"user:owner@example.com"},
				},
			},
		})

		putInst := func(pkg, iid string, pendingProcs, failedProcs []string) *model.Instance {
			inst := &model.Instance{
				InstanceID:        iid,
				Package:           model.PackageKey(ctx, pkg),
				ProcessorsPending: pendingProcs,
				ProcessorsFailure: failedProcs,
			}
			So(datastore.Put(ctx, &model.Package{Name: pkg}, inst), ShouldBeNil)
			return inst
		}

		getMD := func(inst *model.Instance, kv string) *model.InstanceMetadata {
			split := strings.SplitN(kv, ":", 2)
			t := &model.InstanceMetadata{
				Fingerprint: common.InstanceMetadataFingerprint(split[0], []byte(split[1])),
				Instance:    datastore.KeyForObj(ctx, inst),
			}
			if err := datastore.Get(ctx, t); err != datastore.ErrNoSuchEntity {
				So(err, ShouldBeNil)
				return t
			}
			return nil
		}

		md := func(kv ...string) []*api.InstanceMetadata {
			out := make([]*api.InstanceMetadata, len(kv))
			for i, s := range kv {
				split := strings.SplitN(s, ":", 2)
				out[i] = &api.InstanceMetadata{
					Key:   split[0],
					Value: []byte(split[1]),
				}
			}
			return out
		}

		digest := strings.Repeat("a", 40)
		inst := putInst("a/b/c", digest, nil, nil)
		objRef := &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: digest,
		}

		impl := repoImpl{meta: &meta}

		Convey("AttachMetadata/DetachMetadata/ListMetadata happy path", func() {
			_, err := impl.AttachMetadata(as("writer@example.com"), &api.AttachMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Metadata: md("k0:0", "k1:1"),
			})
			So(err, ShouldBeNil)

			// Attached both.
			So(getMD(inst, "k0:0").AttachedBy, ShouldEqual, "user:writer@example.com")
			So(getMD(inst, "k1:1").AttachedBy, ShouldEqual, "user:writer@example.com")

			// Can retrieve it all.
			resp, err := impl.ListMetadata(as("reader@example.com"), &api.ListMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
			})
			So(err, ShouldBeNil)
			So(resp.Metadata, ShouldHaveLength, 2)

			// Can retrieve an individual key.
			resp, err = impl.ListMetadata(as("reader@example.com"), &api.ListMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Keys:     []string{"k0"},
			})
			So(err, ShouldBeNil)
			So(resp.Metadata, ShouldHaveLength, 1)

			// Detaching requires OWNER. Detach one by giving a KV pair, and another
			// via its fingerprint.
			_, err = impl.DetachMetadata(as("owner@example.com"), &api.DetachMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Metadata: []*api.InstanceMetadata{
					{
						Key:   "k0",
						Value: []byte{'0'},
					},
					{
						Fingerprint: common.InstanceMetadataFingerprint("k1", []byte{'1'}),
					},
				},
			})
			So(err, ShouldBeNil)

			// Missing now.
			So(getMD(inst, "k0:0"), ShouldBeNil)
			So(getMD(inst, "k1:1"), ShouldBeNil)
		})

		Convey("Bad package", func() {
			Convey("AttachMetadata", func() {
				_, err := impl.AttachMetadata(as("writer@example.com"), &api.AttachMetadataRequest{
					Package:  "a/b//",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'package'")
			})
			Convey("DetachMetadata", func() {
				_, err := impl.DetachMetadata(as("owner@example.com"), &api.DetachMetadataRequest{
					Package:  "a/b//",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'package'")
			})
			Convey("ListMetadata", func() {
				_, err := impl.ListMetadata(as("reader@example.com"), &api.ListMetadataRequest{
					Package:  "a/b//",
					Instance: objRef,
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'package'")
			})
		})

		Convey("Bad ObjectRef", func() {
			Convey("AttachMetadata", func() {
				_, err := impl.AttachMetadata(as("writer@example.com"), &api.AttachMetadataRequest{
					Package:  "a/b/c",
					Metadata: md("k:0", "k:1"),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'instance'")
			})
			Convey("DetachMetadata", func() {
				_, err := impl.DetachMetadata(as("owner@example.com"), &api.DetachMetadataRequest{
					Package:  "a/b/c",
					Metadata: md("k:0", "k:1"),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'instance'")
			})
			Convey("ListMetadata", func() {
				_, err := impl.ListMetadata(as("reader@example.com"), &api.ListMetadataRequest{
					Package: "a/b/c",
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'instance'")
			})
		})

		Convey("Empty metadata list", func() {
			Convey("AttachMetadata", func() {
				_, err := impl.AttachMetadata(as("writer@example.com"), &api.AttachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "cannot be empty")
			})
			Convey("DetachMetadata", func() {
				_, err := impl.DetachMetadata(as("owner@example.com"), &api.DetachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "cannot be empty")
			})
		})

		Convey("Bad metadata key", func() {
			Convey("AttachMetadata", func() {
				_, err := impl.AttachMetadata(as("writer@example.com"), &api.AttachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("ZZZ:0"),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, `invalid metadata key`)
			})
			Convey("DetachMetadata", func() {
				_, err := impl.DetachMetadata(as("owner@example.com"), &api.DetachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("ZZZ:0"),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, `invalid metadata key`)
			})
			Convey("ListMetadata", func() {
				_, err := impl.ListMetadata(as("reader@example.com"), &api.ListMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Keys:     []string{"ZZZ"},
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, `invalid metadata key`)
			})
		})

		Convey("Bad metadata value", func() {
			Convey("AttachMetadata", func() {
				_, err := impl.AttachMetadata(as("writer@example.com"), &api.AttachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("k:" + strings.Repeat("z", 512*1024+1)),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, `metadata with key "k": the metadata value is too long`)
			})
			Convey("DetachMetadata", func() {
				_, err := impl.DetachMetadata(as("owner@example.com"), &api.DetachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("k:" + strings.Repeat("z", 512*1024+1)),
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, `metadata with key "k": the metadata value is too long`)
			})
		})

		Convey("Bad metadata content type in AttachMetadata", func() {
			m := md("k:0")
			m[0].ContentType = "zzz zzz"
			_, err := impl.AttachMetadata(as("writer@example.com"), &api.AttachMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Metadata: m,
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, `metadata with key "k": bad content type "zzz zzz`)
		})

		Convey("Bad fingerprint in DetachMetadata", func() {
			_, err := impl.DetachMetadata(as("owner@example.com"), &api.DetachMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Metadata: []*api.InstanceMetadata{
					{Fingerprint: "bad"},
				},
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, `bad metadata fingerprint "bad"`)
		})

		Convey("No access", func() {
			Convey("AttachMetadata", func() {
				_, err := impl.AttachMetadata(as("reader@example.com"), &api.AttachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
				So(err, ShouldErrLike, "has no required WRITER role")
			})
			Convey("DetachMetadata", func() {
				_, err := impl.DetachMetadata(as("writer@example.com"), &api.DetachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
				So(err, ShouldErrLike, "has no required OWNER role")
			})
			Convey("ListMetadata", func() {
				_, err := impl.ListMetadata(as("unknown@example.com"), &api.ListMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
				})
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
				So(err, ShouldErrLike, "is not allowed to see it")
			})
		})

		Convey("Missing package", func() {
			Convey("AttachMetadata", func() {
				_, err := impl.AttachMetadata(as("writer@example.com"), &api.AttachMetadataRequest{
					Package:  "a/b/c/missing",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
			Convey("DetachMetadata", func() {
				_, err := impl.DetachMetadata(as("owner@example.com"), &api.DetachMetadataRequest{
					Package:  "a/b/c/missing",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
			Convey("ListMetadata", func() {
				_, err := impl.ListMetadata(as("reader@example.com"), &api.ListMetadataRequest{
					Package:  "a/b/c/missing",
					Instance: objRef,
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
		})

		Convey("Missing instance", func() {
			missingRef := &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: strings.Repeat("b", 40),
			}
			Convey("AttachMetadata", func() {
				_, err := impl.AttachMetadata(as("writer@example.com"), &api.AttachMetadataRequest{
					Package:  "a/b/c",
					Instance: missingRef,
					Metadata: md("k:0", "k:1"),
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such instance")
			})
			Convey("DetachMetadata", func() {
				_, err := impl.DetachMetadata(as("owner@example.com"), &api.DetachMetadataRequest{
					Package:  "a/b/c",
					Instance: missingRef,
					Metadata: md("k:0", "k:1"),
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such instance")
			})
			Convey("ListMetadata", func() {
				_, err := impl.ListMetadata(as("reader@example.com"), &api.ListMetadataRequest{
					Package:  "a/b/c",
					Instance: missingRef,
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such instance")
			})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Version resolution and instance info fetching.

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})
		impl := repoImpl{meta: &meta}

		pkg := &model.Package{Name: "a/pkg"}
		inst1 := &model.Instance{
			InstanceID:   strings.Repeat("1", 40),
			Package:      model.PackageKey(ctx, "a/pkg"),
			RegisteredBy: "user:1@example.com",
		}
		inst2 := &model.Instance{
			InstanceID:   strings.Repeat("2", 40),
			Package:      model.PackageKey(ctx, "a/pkg"),
			RegisteredBy: "user:2@example.com",
		}

		So(datastore.Put(ctx, pkg, inst1, inst2), ShouldBeNil)
		So(model.SetRef(ctx, "latest", inst2), ShouldBeNil)
		So(model.AttachTags(ctx, inst1, []*api.Tag{
			{Key: "ver", Value: "1"},
			{Key: "ver", Value: "ambiguous"},
		}), ShouldBeNil)
		So(model.AttachTags(ctx, inst2, []*api.Tag{
			{Key: "ver", Value: "2"},
			{Key: "ver", Value: "ambiguous"},
		}), ShouldBeNil)

		Convey("Happy path", func() {
			inst, err := impl.ResolveVersion(ctx, &api.ResolveVersionRequest{
				Package: "a/pkg",
				Version: "latest",
			})
			So(err, ShouldBeNil)
			So(inst, ShouldResembleProto, inst2.Proto())
		})

		Convey("Bad package name", func() {
			_, err := impl.ResolveVersion(ctx, &api.ResolveVersionRequest{
				Package: "///",
				Version: "latest",
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'package'")
		})

		Convey("Bad version name", func() {
			_, err := impl.ResolveVersion(ctx, &api.ResolveVersionRequest{
				Package: "a/pkg",
				Version: "::",
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'version'")
		})

		Convey("No access", func() {
			_, err := impl.ResolveVersion(ctx, &api.ResolveVersionRequest{
				Package: "b",
				Version: "latest",
			})
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(err, ShouldErrLike, "is not allowed to see it")
		})

		Convey("Missing package", func() {
			_, err := impl.ResolveVersion(ctx, &api.ResolveVersionRequest{
				Package: "a/b",
				Version: "latest",
			})
			So(status.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such package")
		})

		Convey("Missing instance", func() {
			_, err := impl.ResolveVersion(ctx, &api.ResolveVersionRequest{
				Package: "a/pkg",
				Version: strings.Repeat("f", 40),
			})
			So(status.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such instance")
		})

		Convey("Missing ref", func() {
			_, err := impl.ResolveVersion(ctx, &api.ResolveVersionRequest{
				Package: "a/pkg",
				Version: "missing",
			})
			So(status.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such ref")
		})

		Convey("Missing tag", func() {
			_, err := impl.ResolveVersion(ctx, &api.ResolveVersionRequest{
				Package: "a/pkg",
				Version: "ver:missing",
			})
			So(status.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such tag")
		})

		Convey("Ambiguous tag", func() {
			_, err := impl.ResolveVersion(ctx, &api.ResolveVersionRequest{
				Package: "a/pkg",
				Version: "ver:ambiguous",
			})
			So(status.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "ambiguity when resolving the tag")
		})
	})
}

func TestGetInstanceURLAndDownloads(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		cas := testutil.MockCAS{}
		impl := repoImpl{meta: &meta, cas: &cas}

		inst := &model.Instance{
			InstanceID:   strings.Repeat("1", 40),
			Package:      model.PackageKey(ctx, "a/pkg"),
			RegisteredBy: "user:1@example.com",
		}
		So(datastore.Put(ctx, &model.Package{Name: "a/pkg"}, inst), ShouldBeNil)

		cas.GetObjectURLImpl = func(_ context.Context, r *api.GetObjectURLRequest) (*api.ObjectURL, error) {
			So(r.Object.HashAlgo, ShouldEqual, api.HashAlgo_SHA1)
			So(r.Object.HexDigest, ShouldEqual, inst.InstanceID)
			return &api.ObjectURL{
				SignedUrl: fmt.Sprintf("http://example.com/%s?d=%s", r.Object.HexDigest, r.DownloadFilename),
			}, nil
		}

		Convey("GetInstanceURL", func() {
			Convey("Happy path", func() {
				resp, err := impl.GetInstanceURL(ctx, &api.GetInstanceURLRequest{
					Package:  inst.Package.StringID(),
					Instance: inst.Proto().Instance,
				})
				So(err, ShouldBeNil)
				So(resp, ShouldResembleProto, &api.ObjectURL{
					SignedUrl: "http://example.com/1111111111111111111111111111111111111111?d=",
				})
			})

			Convey("Bad package name", func() {
				_, err := impl.GetInstanceURL(ctx, &api.GetInstanceURLRequest{
					Package:  "///",
					Instance: inst.Proto().Instance,
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'package'")
			})

			Convey("Bad instance", func() {
				_, err := impl.GetInstanceURL(ctx, &api.GetInstanceURLRequest{
					Package: "a/pkg",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: "huh",
					},
				})
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "bad 'instance'")
			})

			Convey("No access", func() {
				_, err := impl.GetInstanceURL(ctx, &api.GetInstanceURLRequest{
					Package:  "b",
					Instance: inst.Proto().Instance,
				})
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
				So(err, ShouldErrLike, "is not allowed to see it")
			})

			Convey("Missing package", func() {
				_, err := impl.GetInstanceURL(ctx, &api.GetInstanceURLRequest{
					Package:  "a/missing",
					Instance: inst.Proto().Instance,
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})

			Convey("Missing instance", func() {
				_, err := impl.GetInstanceURL(ctx, &api.GetInstanceURLRequest{
					Package: "a/pkg",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: strings.Repeat("f", 40),
					},
				})
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such instance")
			})
		})

		Convey("Raw download handler", func() {
			call := func(path string) *httptest.ResponseRecorder {
				rr := httptest.NewRecorder()
				adaptGrpcErr(impl.handlePackageDownload)(&router.Context{
					Request: (&http.Request{}).WithContext(ctx),
					Params: httprouter.Params{
						{Key: "path", Value: path},
					},
					Writer: rr,
				})
				return rr
			}

			Convey("Happy path", func() {
				rr := call("/a/pkg/+/1111111111111111111111111111111111111111")
				So(rr.Code, ShouldEqual, http.StatusFound)
				So(rr.Header().Get("Location"), ShouldEqual, "http://example.com/1111111111111111111111111111111111111111?d=a-pkg.zip")
				So(rr.Header().Get(cipdInstanceHeader), ShouldEqual, inst.InstanceID)
			})

			Convey("Malformed URL", func() {
				rr := call("huh")
				So(rr.Code, ShouldEqual, http.StatusBadRequest)
				So(rr.Body.String(), ShouldContainSubstring, "the URL should have form")
			})

			Convey("Bad package name", func() {
				rr := call("/???/+/live")
				So(rr.Code, ShouldEqual, http.StatusBadRequest)
				So(rr.Body.String(), ShouldContainSubstring, "invalid package name")
			})

			Convey("Bad version", func() {
				rr := call("/pkg/+/???")
				So(rr.Code, ShouldEqual, http.StatusBadRequest)
				So(rr.Body.String(), ShouldContainSubstring, "bad version")
			})

			Convey("No access", func() {
				rr := call("/b/+/live")
				So(rr.Code, ShouldEqual, http.StatusForbidden)
				So(rr.Body.String(), ShouldContainSubstring, "is not allowed to see it")
			})

			Convey("Missing package", func() {
				rr := call("/a/missing/+/live")
				So(rr.Code, ShouldEqual, http.StatusNotFound)
				So(rr.Body.String(), ShouldContainSubstring, "no such package")
			})

			Convey("Missing instance", func() {
				rr := call("/a/pkg/+/1111111111111111111111111111111111111112")
				So(rr.Code, ShouldEqual, http.StatusNotFound)
				So(rr.Body.String(), ShouldContainSubstring, "no such instance")
			})
		})
	})
}

func TestDescribeInstance(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		impl := repoImpl{meta: &meta}

		inst := &model.Instance{
			InstanceID:   strings.Repeat("1", 40),
			Package:      model.PackageKey(ctx, "a/pkg"),
			RegisteredBy: "user:1@example.com",
		}
		So(datastore.Put(ctx, &model.Package{Name: "a/pkg"}, inst), ShouldBeNil)

		Convey("Happy path, basic info", func() {
			resp, err := impl.DescribeInstance(ctx, &api.DescribeInstanceRequest{
				Package:  "a/pkg",
				Instance: inst.Proto().Instance,
			})
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &api.DescribeInstanceResponse{
				Instance: inst.Proto(),
			})
		})

		Convey("Happy path, full info", func() {
			model.AttachTags(as("tag@example.com"), inst, []*api.Tag{
				{Key: "a", Value: "0"},
				{Key: "a", Value: "1"},
			})

			model.SetRef(as("ref@example.com"), "ref_a", inst)
			model.SetRef(as("ref@example.com"), "ref_b", inst)

			model.AttachMetadata(as("metadata@example.com"), inst, []*api.InstanceMetadata{
				{
					Key:         "foo",
					Value:       []byte("bar"),
					ContentType: "image/png",
				},
				{
					Key:         "bar",
					Value:       []byte("baz"),
					ContentType: "text/plain",
				},
			})

			inst.ProcessorsSuccess = []string{"proc"}
			datastore.Put(ctx, inst, &model.ProcessingResult{
				ProcID:    "proc",
				Instance:  datastore.KeyForObj(ctx, inst),
				Success:   true,
				CreatedTs: testutil.TestTime,
			})

			resp, err := impl.DescribeInstance(ctx, &api.DescribeInstanceRequest{
				Package:            "a/pkg",
				Instance:           inst.Proto().Instance,
				DescribeTags:       true,
				DescribeRefs:       true,
				DescribeProcessors: true,
				DescribeMetadata:   true,
			})
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &api.DescribeInstanceResponse{
				Instance: inst.Proto(),
				Tags: []*api.Tag{
					{
						Key:        "a",
						Value:      "0",
						AttachedBy: "user:tag@example.com",
						AttachedTs: timestamppb.New(testutil.TestTime),
					},
					{
						Key:        "a",
						Value:      "1",
						AttachedBy: "user:tag@example.com",
						AttachedTs: timestamppb.New(testutil.TestTime),
					},
				},
				Refs: []*api.Ref{
					{
						Name:       "ref_a",
						Package:    "a/pkg",
						Instance:   inst.Proto().Instance,
						ModifiedBy: "user:ref@example.com",
						ModifiedTs: timestamppb.New(testutil.TestTime),
					},
					{
						Name:       "ref_b",
						Package:    "a/pkg",
						Instance:   inst.Proto().Instance,
						ModifiedBy: "user:ref@example.com",
						ModifiedTs: timestamppb.New(testutil.TestTime),
					},
				},
				Processors: []*api.Processor{
					{
						Id:         "proc",
						State:      api.Processor_SUCCEEDED,
						FinishedTs: timestamppb.New(testutil.TestTime),
					},
				},
				// Note that metadata is returned LIFO.
				Metadata: []*api.InstanceMetadata{
					{
						Key:         "bar",
						Value:       []byte("baz"),
						ContentType: "text/plain",
						Fingerprint: "aec8a983ee3429852adfcfecacad886d",
						AttachedBy:  "user:metadata@example.com",
						AttachedTs:  timestamppb.New(testutil.TestTime),
					},
					{
						Key:         "foo",
						Value:       []byte("bar"),
						ContentType: "image/png",
						Fingerprint: "a765a8beaa9d561d4c5cbed29d8f4e30",
						AttachedBy:  "user:metadata@example.com",
						AttachedTs:  timestamppb.New(testutil.TestTime),
					},
				},
			})
		})

		Convey("Bad package name", func() {
			_, err := impl.DescribeInstance(ctx, &api.DescribeInstanceRequest{
				Package:  "///",
				Instance: inst.Proto().Instance,
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'package'")
		})

		Convey("Bad instance", func() {
			_, err := impl.DescribeInstance(ctx, &api.DescribeInstanceRequest{
				Package: "a/pkg",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: "huh",
				},
			})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'instance'")
		})

		Convey("No access", func() {
			_, err := impl.DescribeInstance(ctx, &api.DescribeInstanceRequest{
				Package:  "b",
				Instance: inst.Proto().Instance,
			})
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			So(err, ShouldErrLike, "is not allowed to see it")
		})

		Convey("Missing package", func() {
			_, err := impl.DescribeInstance(ctx, &api.DescribeInstanceRequest{
				Package:  "a/missing",
				Instance: inst.Proto().Instance,
			})
			So(status.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such package")
		})

		Convey("Missing instance", func() {
			_, err := impl.DescribeInstance(ctx, &api.DescribeInstanceRequest{
				Package: "a/pkg",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: strings.Repeat("f", 40),
				},
			})
			So(status.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such instance")
		})
	})
}

func TestDescribeBootstrapBundle(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		impl := repoImpl{meta: &meta}

		putInst := func(pkg, extracted string) {
			inst := &model.Instance{
				InstanceID: common.ObjectRefToInstanceID(&api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA256,
					HexDigest: strings.Repeat("1", 64),
				}),
				Package:      model.PackageKey(ctx, pkg),
				RegisteredBy: "user:1@example.com",
			}
			if extracted != "" {
				r := &model.ProcessingResult{
					ProcID:   processing.BootstrapPackageExtractorProcID,
					Instance: datastore.KeyForObj(ctx, inst),
				}
				if extracted != "BROKEN" {
					r.Success = true
					r.WriteResult(processing.BootstrapExtractorResult{
						File:       extracted,
						HashAlgo:   "SHA256",
						HashDigest: strings.Repeat("a", 64),
						Size:       12345,
					})
					inst.ProcessorsSuccess = []string{processing.BootstrapPackageExtractorProcID}
				} else {
					r.Error = "Extraction broken"
					inst.ProcessorsFailure = []string{processing.BootstrapPackageExtractorProcID}
				}
				So(datastore.Put(ctx, r), ShouldBeNil)
			}
			So(datastore.Put(ctx,
				&model.Package{Name: pkg},
				inst,
				&model.Ref{Name: "latest", Package: inst.Package, InstanceID: inst.InstanceID},
			), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
		}

		expectedFile := func(pkg, extracted string) *api.DescribeBootstrapBundleResponse_BootstrapFile {
			return &api.DescribeBootstrapBundleResponse_BootstrapFile{
				Package: pkg,
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA256,
					HexDigest: strings.Repeat("1", 64),
				},
				File: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA256,
					HexDigest: strings.Repeat("a", 64),
				},
				Name: extracted,
				Size: 12345,
			}
		}

		expectedError := func(pkg string, code codes.Code, msg string) *api.DescribeBootstrapBundleResponse_BootstrapFile {
			return &api.DescribeBootstrapBundleResponse_BootstrapFile{
				Package: pkg,
				Status: &statuspb.Status{
					Code:    int32(code),
					Message: msg,
				},
			}
		}

		Convey("Happy path", func() {
			putInst("a/pkg/var-1", "file1")
			putInst("a/pkg/var-2", "file2")
			putInst("a/pkg/var-3", "file3")
			putInst("a/pkg", "")             // will be ignored
			putInst("a/pkg/sun/package", "") // will be ignored

			Convey("With prefix listing", func() {
				resp, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
					Prefix:  "a/pkg",
					Version: "latest",
				})
				So(err, ShouldBeNil)
				So(resp, ShouldResembleProto, &api.DescribeBootstrapBundleResponse{
					Files: []*api.DescribeBootstrapBundleResponse_BootstrapFile{
						expectedFile("a/pkg/var-1", "file1"),
						expectedFile("a/pkg/var-2", "file2"),
						expectedFile("a/pkg/var-3", "file3"),
					},
				})
			})

			Convey("With variants", func() {
				resp, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
					Prefix:   "a/pkg",
					Variants: []string{"var-3", "var-1"},
					Version:  "latest",
				})
				So(err, ShouldBeNil)
				So(resp, ShouldResembleProto, &api.DescribeBootstrapBundleResponse{
					Files: []*api.DescribeBootstrapBundleResponse_BootstrapFile{
						expectedFile("a/pkg/var-3", "file3"),
						expectedFile("a/pkg/var-1", "file1"),
					},
				})
			})
		})

		Convey("Partial failure", func() {
			putInst("a/pkg/var-1", "file1")
			putInst("a/pkg/var-2", "")

			resp, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
				Prefix:  "a/pkg",
				Version: "latest",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &api.DescribeBootstrapBundleResponse{
				Files: []*api.DescribeBootstrapBundleResponse_BootstrapFile{
					expectedFile("a/pkg/var-1", "file1"),
					expectedError("a/pkg/var-2", codes.FailedPrecondition, `"a/pkg/var-2" is not a bootstrap package`),
				},
			})
		})

		Convey("Total failure", func() {
			putInst("a/pkg/var-1", "")
			putInst("a/pkg/var-2", "")
			putInst("a/pkg/var-3", "")

			_, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
				Prefix:  "a/pkg",
				Version: "latest",
			})
			So(err, ShouldHaveRPCCode, codes.FailedPrecondition,
				`"a/pkg/var-1" is not a bootstrap package (and 2 other errors like this)`)
		})

		Convey("Empty prefix", func() {
			_, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
				Prefix:  "a/pkg",
				Version: "latest",
			})
			So(err, ShouldHaveRPCCode, codes.NotFound, `no packages directly under prefix "a/pkg"`)
		})

		Convey("Missing version", func() {
			putInst("a/pkg/var-1", "file1")
			_, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
				Prefix:  "a/pkg",
				Version: "not-latest",
			})
			So(err, ShouldHaveRPCCode, codes.NotFound, `no such ref`)
		})

		Convey("Broken processor", func() {
			putInst("a/pkg/var-1", "BROKEN")
			_, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
				Prefix:  "a/pkg",
				Version: "latest",
			})
			So(err, ShouldHaveRPCCode, codes.Aborted, `some processors failed to process this instance`)
		})

		Convey("Request validation", func() {
			putInst("a/pkg/var-1", "file1")

			Convey("Bad prefix format", func() {
				_, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
					Prefix:  "///",
					Version: "latest",
				})
				So(err, ShouldHaveRPCCode, codes.InvalidArgument)
			})

			Convey("Variant with /", func() {
				_, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
					Prefix:   "a/pkg",
					Variants: []string{"some/thing"},
					Version:  "latest",
				})
				So(err, ShouldHaveRPCCode, codes.InvalidArgument)
			})

			Convey("Empty variant", func() {
				_, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
					Prefix:   "a/pkg",
					Variants: []string{""},
					Version:  "latest",
				})
				So(err, ShouldHaveRPCCode, codes.InvalidArgument)
			})

			Convey("Malformed variant name", func() {
				_, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
					Prefix:   "a/pkg",
					Variants: []string{"BAD"},
					Version:  "latest",
				})
				So(err, ShouldHaveRPCCode, codes.InvalidArgument)
			})

			Convey("Duplicate variants", func() {
				_, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
					Prefix:   "a/pkg",
					Variants: []string{"var-1", "var-1"},
					Version:  "latest",
				})
				So(err, ShouldHaveRPCCode, codes.InvalidArgument)
			})

			Convey("Bad version", func() {
				_, err := impl.DescribeBootstrapBundle(ctx, &api.DescribeBootstrapBundleRequest{
					Prefix:  "a/pkg",
					Version: "bad version ID",
				})
				So(err, ShouldHaveRPCCode, codes.InvalidArgument)
			})

			Convey("Not a reader", func() {
				_, err := impl.DescribeBootstrapBundle(as("someone@example.com"), &api.DescribeBootstrapBundleRequest{
					Prefix:  "a/pkg",
					Version: "latest",
				})
				So(err, ShouldHaveRPCCode, codes.PermissionDenied)
			})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Client bootstrap and legacy API.

func TestClientBootstrap(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		cas := testutil.MockCAS{
			GetObjectURLImpl: func(_ context.Context, r *api.GetObjectURLRequest) (*api.ObjectURL, error) {
				return &api.ObjectURL{
					SignedUrl: fmt.Sprintf("http://fake/%s/%s?d=%s&blah=zzz", r.Object.HashAlgo, r.Object.HexDigest, r.DownloadFilename),
				}, nil
			},
		}

		impl := repoImpl{meta: &meta, cas: &cas}

		goodPlat := "linux-amd64"
		goodDigest := strings.Repeat("f", 64)
		goodIID := common.ObjectRefToInstanceID(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: goodDigest,
		})
		goodPkg, err := processing.GetClientPackage(goodPlat)
		So(err, ShouldBeNil)

		setup := func(res *processing.ClientExtractorResult, fail string) (*model.Instance, *model.ProcessingResult) {
			pkgName, err := processing.GetClientPackage(goodPlat)
			So(err, ShouldBeNil)
			pkg := &model.Package{Name: pkgName}
			inst := &model.Instance{
				InstanceID: goodIID,
				Package:    datastore.KeyForObj(ctx, pkg),
			}
			proc := &model.ProcessingResult{
				ProcID:   processing.ClientExtractorProcID,
				Instance: datastore.KeyForObj(ctx, inst),
			}
			if res != nil {
				proc.Success = true
				proc.WriteResult(res)
				inst.ProcessorsSuccess = []string{proc.ProcID}
			} else {
				proc.Error = fail
				inst.ProcessorsFailure = []string{proc.ProcID}
			}
			So(datastore.Put(ctx, pkg, inst, proc), ShouldBeNil)
			return inst, proc
		}

		res := processing.ClientExtractorResult{}
		res.ClientBinary.HashAlgo = "SHA256"
		res.ClientBinary.HashDigest = strings.Repeat("b", 64)
		res.ClientBinary.AllHashDigests = map[string]string{
			"SHA1":   strings.Repeat("c", 40),
			"SHA256": strings.Repeat("b", 64),
		}
		res.ClientBinary.Size = 123456789101112
		inst, proc := setup(&res, "")

		expectedClientURL := fmt.Sprintf("http://fake/%s/%s?d=cipd&blah=zzz", res.ClientBinary.HashAlgo, res.ClientBinary.HashDigest)

		Convey("Bootstrap endpoint", func() {
			call := func(plat, ver string) *httptest.ResponseRecorder {
				form := url.Values{}
				form.Add("platform", plat)
				form.Add("version", ver)
				rr := httptest.NewRecorder()
				adaptGrpcErr(impl.handleClientBootstrap)(&router.Context{
					Request: (&http.Request{Form: form}).WithContext(ctx),
					Writer:  rr,
				})
				return rr
			}

			Convey("Happy path", func() {
				rr := call(goodPlat, goodIID)
				So(rr.Code, ShouldEqual, http.StatusFound)
				So(rr.Header().Get("Location"), ShouldEqual, expectedClientURL)
				So(rr.Header().Get(cipdInstanceHeader), ShouldEqual, goodIID)
			})

			Convey("No plat", func() {
				rr := call("", goodIID)
				So(rr.Code, ShouldEqual, http.StatusBadRequest)
				So(rr.Body.String(), ShouldContainSubstring, "no 'platform' specified")
			})

			Convey("Bad plat", func() {
				rr := call("...", goodIID)
				So(rr.Code, ShouldEqual, http.StatusBadRequest)
				So(rr.Body.String(), ShouldContainSubstring, "bad platform name")
			})

			Convey("No ver", func() {
				rr := call(goodPlat, "")
				So(rr.Code, ShouldEqual, http.StatusBadRequest)
				So(rr.Body.String(), ShouldContainSubstring, "no 'version' specified")
			})

			Convey("Bad ver", func() {
				rr := call(goodPlat, "!!!!")
				So(rr.Code, ShouldEqual, http.StatusBadRequest)
				So(rr.Body.String(), ShouldContainSubstring, "bad version")
			})

			Convey("No access", func() {
				ctx = as("someone@example.com")
				rr := call(goodPlat, goodIID)
				So(rr.Code, ShouldEqual, http.StatusForbidden)
				So(rr.Body.String(), ShouldContainSubstring, "is not allowed to see it")
			})

			Convey("Missing ver", func() {
				rr := call(goodPlat, "missing")
				So(rr.Code, ShouldEqual, http.StatusNotFound)
				So(rr.Body.String(), ShouldContainSubstring, "no such ref")
			})

			Convey("Missing instance ID", func() {
				rr := call(goodPlat, strings.Repeat("b", 40))
				So(rr.Code, ShouldEqual, http.StatusNotFound)
				So(rr.Body.String(), ShouldContainSubstring, "no such instance")
			})

			Convey("Not extracted yet", func() {
				inst.ProcessorsPending = []string{proc.ProcID}
				datastore.Delete(ctx, proc)
				datastore.Put(ctx, inst)

				rr := call(goodPlat, goodIID)
				So(rr.Code, ShouldEqual, http.StatusNotFound)
				So(rr.Body.String(), ShouldContainSubstring, "is not extracted yet")
			})

			Convey("Fatal error during extraction", func() {
				setup(nil, "BOOM")

				rr := call(goodPlat, goodIID)
				So(rr.Code, ShouldEqual, http.StatusNotFound)
				So(rr.Body.String(), ShouldContainSubstring, "BOOM")
			})
		})

		Convey("DescribeClient RPC", func() {
			call := func(pkg, sha256Digest string) (*api.DescribeClientResponse, error) {
				return impl.DescribeClient(ctx, &api.DescribeClientRequest{
					Package: pkg,
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA256,
						HexDigest: sha256Digest,
					},
				})
			}

			Convey("Happy path", func() {
				resp, err := call(goodPkg, goodDigest)
				So(err, ShouldBeNil)
				So(resp, ShouldResembleProto, &api.DescribeClientResponse{
					Instance: &api.Instance{
						Package: goodPkg,
						Instance: &api.ObjectRef{
							HashAlgo:  api.HashAlgo_SHA256,
							HexDigest: goodDigest,
						},
					},
					ClientRef: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA256,
						HexDigest: res.ClientBinary.AllHashDigests["SHA256"],
					},
					ClientBinary: &api.ObjectURL{
						SignedUrl: expectedClientURL,
					},
					ClientSize: res.ClientBinary.Size,
					LegacySha1: res.ClientBinary.AllHashDigests["SHA1"],
					ClientRefAliases: []*api.ObjectRef{
						{
							HashAlgo:  api.HashAlgo_SHA1,
							HexDigest: res.ClientBinary.AllHashDigests["SHA1"],
						},
						{
							HashAlgo:  api.HashAlgo_SHA256,
							HexDigest: res.ClientBinary.AllHashDigests["SHA256"],
						},
					},
				})
			})

			Convey("Bad package name", func() {
				_, err := call("not/a/client", goodDigest)
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "not a CIPD client package")
			})

			Convey("Bad instance ref", func() {
				_, err := call(goodPkg, "not-an-id")
				So(status.Code(err), ShouldEqual, codes.InvalidArgument)
				So(err, ShouldErrLike, "invalid SHA256 hex digest")
			})

			Convey("Missing instance", func() {
				_, err := call(goodPkg, strings.Repeat("e", 64))
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such instance")
			})

			Convey("No access", func() {
				ctx = as("someone@example.com")
				_, err := call(goodPkg, goodDigest)
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
				So(err, ShouldErrLike, "not allowed to see it")
			})

			Convey("Not extracted yet", func() {
				inst.ProcessorsPending = []string{proc.ProcID}
				datastore.Delete(ctx, proc)
				datastore.Put(ctx, inst)

				_, err := call(goodPkg, goodDigest)
				So(status.Code(err), ShouldEqual, codes.FailedPrecondition)
				So(err, ShouldErrLike, "the instance is not ready yet")
			})

			Convey("Fatal error during extraction", func() {
				setup(nil, "BOOM")

				_, err := call(goodPkg, goodDigest)
				So(status.Code(err), ShouldEqual, codes.Aborted)
				So(err, ShouldErrLike, "some processors failed to process this instance")
			})
		})

		Convey("Legacy API", func() {
			call := func(pkg, iid, ct string) (code int, body string) {
				rr := httptest.NewRecorder()
				adaptGrpcErr(impl.handleLegacyClientInfo)(&router.Context{
					Request: (&http.Request{Form: url.Values{
						"package_name": {pkg},
						"instance_id":  {iid},
					}}).WithContext(ctx),
					Writer: rr,
				})
				expCT := "text/plain; charset=utf-8"
				if ct == "json" {
					expCT = "application/json; charset=utf-8"
				}
				So(rr.Header().Get("Content-Type"), ShouldEqual, expCT)
				code = rr.Code
				body = strings.TrimSpace(rr.Body.String())
				return
			}

			Convey("Happy path", func() {
				code, body := call(goodPkg, goodIID, "json")
				So(code, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual,
					fmt.Sprintf(`{
  "client_binary": {
    "fetch_url": "%s",
    "file_name": "cipd",
    "sha1": "%s",
    "size": "123456789101112"
  },
  "instance": {
    "package_name": "infra/tools/cipd/linux-amd64",
    "instance_id": "%s"
  },
  "status": "SUCCESS"
}`,
						expectedClientURL,
						res.ClientBinary.AllHashDigests["SHA1"],
						inst.InstanceID))
			})

			Convey("Bad package name", func() {
				code, body := call("not/a/client", goodIID, "text")
				So(code, ShouldEqual, http.StatusBadRequest)
				So(body, ShouldContainSubstring, "not a CIPD client package")
			})

			Convey("Bad instance ID", func() {
				code, body := call(goodPkg, "not-an-id", "text")
				So(code, ShouldEqual, http.StatusBadRequest)
				So(body, ShouldContainSubstring, "not a valid package instance ID")
			})

			Convey("Missing instance", func() {
				badIID := common.ObjectRefToInstanceID(&api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA256,
					HexDigest: strings.Repeat("d", 64),
				})
				code, body := call(goodPkg, badIID, "json")
				So(code, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, `{
  "error_message": "no such instance",
  "status": "INSTANCE_NOT_FOUND"
}`)
			})

			Convey("No access", func() {
				ctx = as("someone@example.com")
				code, body := call(goodPkg, goodIID, "text")
				So(code, ShouldEqual, http.StatusForbidden)
				So(body, ShouldContainSubstring, "not allowed to see it")
			})

			Convey("Not extracted yet", func() {
				inst.ProcessorsPending = []string{proc.ProcID}
				datastore.Delete(ctx, proc)
				datastore.Put(ctx, inst)

				code, body := call(goodPkg, goodIID, "json")
				So(code, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, `{
  "error_message": "the client binary is not extracted yet, try later",
  "status": "NOT_EXTRACTED_YET"
}`)
			})

			Convey("Fatal error during extraction", func() {
				setup(nil, "BOOM")

				code, body := call(goodPkg, goodIID, "json")
				So(code, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, `{
  "error_message": "the client binary is not available: some processors failed to process this instance: cipd_client_binary:v1",
  "status": "ERROR"
}`)
			})
		})
	})
}

func TestLegacyHandlers(t *testing.T) {
	t.Parallel()

	Convey("With fakes", t, func() {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		cas := testutil.MockCAS{
			GetObjectURLImpl: func(_ context.Context, r *api.GetObjectURLRequest) (*api.ObjectURL, error) {
				return &api.ObjectURL{
					SignedUrl: "http://fake/" + common.ObjectRefToInstanceID(r.Object),
				}, nil
			},
		}
		impl := repoImpl{meta: &meta, cas: &cas}

		inst1 := &model.Instance{
			InstanceID:   strings.Repeat("a", 40),
			Package:      model.PackageKey(ctx, "a/b"),
			RegisteredBy: "user:reg@example.com",
			RegisteredTs: testutil.TestTime,
		}
		inst2 := &model.Instance{
			InstanceID: strings.Repeat("b", 40),
			Package:    model.PackageKey(ctx, "a/b"),
		}
		So(datastore.Put(ctx, &model.Package{Name: "a/b"}, inst1, inst2), ShouldBeNil)

		// Make an ambiguous tag.
		model.AttachTags(ctx, inst1, []*api.Tag{{Key: "k", Value: "v"}})
		model.AttachTags(ctx, inst2, []*api.Tag{{Key: "k", Value: "v"}})

		callHandler := func(h router.Handler, f url.Values, ct string) (code int, body string) {
			rr := httptest.NewRecorder()
			h(&router.Context{
				Request: (&http.Request{Form: f}).WithContext(ctx),
				Writer:  rr,
			})
			expCT := "text/plain; charset=utf-8"
			if ct == "json" {
				expCT = "application/json; charset=utf-8"
			}
			So(rr.Header().Get("Content-Type"), ShouldEqual, expCT)
			code = rr.Code
			body = strings.TrimSpace(rr.Body.String())
			return
		}

		Convey("handleLegacyInstance works", func() {
			callInstance := func(pkg, iid, ct string) (code int, body string) {
				return callHandler(adaptGrpcErr(impl.handleLegacyInstance), url.Values{
					"package_name": {pkg},
					"instance_id":  {iid},
				}, ct)
			}

			Convey("Happy path", func() {
				code, body := callInstance("a/b", strings.Repeat("a", 40), "json")
				So(code, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, `{
  "fetch_url": "http://fake/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "instance": {
    "package_name": "a/b",
    "instance_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "registered_by": "user:reg@example.com",
    "registered_ts": "1454472306000000"
  },
  "status": "SUCCESS"
}`)
			})

			Convey("Bad package", func() {
				code, body := callInstance("///", strings.Repeat("a", 40), "plain")
				So(code, ShouldEqual, http.StatusBadRequest)
				So(body, ShouldContainSubstring, "invalid package name")
			})

			Convey("Bad instance ID", func() {
				code, body := callInstance("a/b", strings.Repeat("a", 99), "plain")
				So(code, ShouldEqual, http.StatusBadRequest)
				So(body, ShouldContainSubstring, "not a valid package instance ID")
			})

			Convey("No access", func() {
				code, body := callInstance("z/z/z", strings.Repeat("a", 40), "plain")
				So(code, ShouldEqual, http.StatusForbidden)
				So(body, ShouldContainSubstring, "not allowed to see")
			})

			Convey("Missing pkg", func() {
				code, body := callInstance("a/z/z", strings.Repeat("a", 40), "json")
				So(code, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, `{
  "error_message": "no such package: a/z/z",
  "status": "INSTANCE_NOT_FOUND"
}`)
			})
		})

		Convey("handleLegacyResolve works", func() {
			callResolve := func(pkg, ver, ct string) (code int, body string) {
				return callHandler(adaptGrpcErr(impl.handleLegacyResolve), url.Values{
					"package_name": {pkg},
					"version":      {ver},
				}, ct)
			}

			Convey("Happy path", func() {
				code, body := callResolve("a/b", strings.Repeat("a", 40), "json")
				So(code, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, `{
  "instance_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "status": "SUCCESS"
}`)
			})

			Convey("Bad request", func() {
				code, body := callResolve("///", strings.Repeat("a", 40), "plain")
				So(code, ShouldEqual, http.StatusBadRequest)
				So(body, ShouldContainSubstring, "invalid package name")
			})

			Convey("No access", func() {
				code, body := callResolve("z/z/z", strings.Repeat("a", 40), "plain")
				So(code, ShouldEqual, http.StatusForbidden)
				So(body, ShouldContainSubstring, "not allowed to see")
			})

			Convey("Missing pkg", func() {
				code, body := callResolve("a/z/z", strings.Repeat("a", 40), "json")
				So(code, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, `{
  "error_message": "no such package: a/z/z",
  "status": "INSTANCE_NOT_FOUND"
}`)
			})

			Convey("Ambiguous version", func() {
				code, body := callResolve("a/b", "k:v", "json")
				So(code, ShouldEqual, http.StatusOK)
				So(body, ShouldEqual, `{
  "error_message": "ambiguity when resolving the tag, more than one instance has it",
  "status": "AMBIGUOUS_VERSION"
}`)
			})
		})
	})
}

func TestParseDownloadPath(t *testing.T) {
	t.Parallel()

	Convey("OK", t, func() {
		pkg, ver, err := parseDownloadPath("/a/b/c/+/latest")
		So(err, ShouldBeNil)
		So(pkg, ShouldEqual, "a/b/c")
		So(ver, ShouldEqual, "latest")

		pkg, ver, err = parseDownloadPath("/a/b/c/+/repo:https://abc")
		So(err, ShouldBeNil)
		So(pkg, ShouldEqual, "a/b/c")
		So(ver, ShouldEqual, "repo:https://abc")

		pkg, ver, err = parseDownloadPath("/a/b/c/+/" + strings.Repeat("a", 40))
		So(err, ShouldBeNil)
		So(pkg, ShouldEqual, "a/b/c")
		So(ver, ShouldEqual, strings.Repeat("a", 40))
	})

	Convey("Bad chunks", t, func() {
		_, _, err := parseDownloadPath("/+/latest")
		So(err, ShouldErrLike, `should have form`)

		_, _, err = parseDownloadPath("latest")
		So(err, ShouldErrLike, `should have form`)

		_, _, err = parseDownloadPath("/a/b/c")
		So(err, ShouldErrLike, `should have form`)
	})

	Convey("Bad package", t, func() {
		_, _, err := parseDownloadPath("BAD/+/latest")
		So(err, ShouldErrLike, `invalid package name`)

		_, _, err = parseDownloadPath("/a//b/+/latest")
		So(err, ShouldErrLike, `invalid package name`)

		_, _, err = parseDownloadPath("//+/latest")
		So(err, ShouldErrLike, `invalid package name`)
	})

	Convey("Bad version", t, func() {
		_, _, err := parseDownloadPath("a/+/")
		So(err, ShouldErrLike, `bad version`)

		_, _, err = parseDownloadPath("a/+/!!!!")
		So(err, ShouldErrLike, `bad version`)
	})
}
