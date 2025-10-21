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
	"flag"
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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/repo/processing"
	"go.chromium.org/luci/cipd/appengine/impl/repo/tasks"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/appengine/impl/vsa"
	vsapb "go.chromium.org/luci/cipd/appengine/impl/vsa/api"
	"go.chromium.org/luci/cipd/common"

	// Using transactional datastore TQ tasks.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

////////////////////////////////////////////////////////////////////////////////
// Prefix metadata RPC methods + related helpers including ACL checks.

func TestMetadataFetching(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		_, _, as := testutil.TestingContext(
			authtest.MockMembership("user:prefixes-viewer@example.com", PrefixesViewers),
		)

		meta := testutil.MetadataStore{}

		// ACL.
		rootMeta := meta.Populate("", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:admin@example.com"},
				},
			},
		})
		topMeta := meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:top-owner@example.com"},
				},
			},
		})

		// The metadata to be fetched.
		leafMeta := meta.Populate("a/b/c/d", &repopb.PrefixMetadata{
			UpdateUser: "user:someone@example.com",
		})

		impl := repoImpl{meta: &meta}

		callGet := func(prefix string, user identity.Identity) (*repopb.PrefixMetadata, error) {
			return impl.GetPrefixMetadata(as(user.Email()), &repopb.PrefixRequest{Prefix: prefix})
		}

		callGetInherited := func(prefix string, user identity.Identity) ([]*repopb.PrefixMetadata, error) {
			resp, err := impl.GetInheritedPrefixMetadata(as(user.Email()), &repopb.PrefixRequest{Prefix: prefix})
			if err != nil {
				return nil, err
			}
			return resp.PerPrefixMetadata, nil
		}

		t.Run("GetPrefixMetadata happy path", func(t *ftt.Test) {
			resp, err := callGet("a/b/c/d", "user:top-owner@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(leafMeta))
		})

		t.Run("GetPrefixMetadata happy path via global group", func(t *ftt.Test) {
			resp, err := callGet("a/b/c/d", "user:prefixes-viewer@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(leafMeta))
		})

		t.Run("GetInheritedPrefixMetadata happy path", func(t *ftt.Test) {
			resp, err := callGetInherited("a/b/c/d", "user:top-owner@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match([]*repopb.PrefixMetadata{rootMeta, topMeta, leafMeta}))
		})

		t.Run("GetInheritedPrefixMetadata happy path via global group", func(t *ftt.Test) {
			resp, err := callGetInherited("a/b/c/d", "user:prefixes-viewer@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match([]*repopb.PrefixMetadata{rootMeta, topMeta, leafMeta}))
		})

		t.Run("GetPrefixMetadata bad prefix", func(t *ftt.Test) {
			resp, err := callGet("a//", "user:top-owner@example.com")
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, resp, should.BeNil)
		})

		t.Run("GetInheritedPrefixMetadata bad prefix", func(t *ftt.Test) {
			resp, err := callGetInherited("a//", "user:top-owner@example.com")
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, resp, should.BeNil)
		})

		t.Run("GetPrefixMetadata no metadata, caller has access", func(t *ftt.Test) {
			resp, err := callGet("a/b", "user:top-owner@example.com")
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, resp, should.BeNil)
		})

		t.Run("GetInheritedPrefixMetadata no metadata, caller has access", func(t *ftt.Test) {
			resp, err := callGetInherited("a/b", "user:top-owner@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match([]*repopb.PrefixMetadata{rootMeta, topMeta}))
		})

		t.Run("GetPrefixMetadata no metadata, caller has no access", func(t *ftt.Test) {
			resp, err := callGet("a/b", "user:someone-else@example.com")
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, resp, should.BeNil)
			// Existing metadata that the caller has no access to produces same error,
			// so unauthorized callers can't easily distinguish between the two.
			resp, err = callGet("a/b/c/d", "user:someone-else@example.com")
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, resp, should.BeNil)
			// Same for completely unknown prefix.
			resp, err = callGet("zzz", "user:someone-else@example.com")
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, resp, should.BeNil)
		})

		t.Run("GetInheritedPrefixMetadata no metadata, caller has no access", func(t *ftt.Test) {
			resp, err := callGetInherited("a/b", "user:someone-else@example.com")
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, resp, should.BeNil)
			// Existing metadata that the caller has no access to produces same error,
			// so unauthorized callers can't easily distinguish between the two.
			resp, err = callGetInherited("a/b/c/d", "user:someone-else@example.com")
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, resp, should.BeNil)
			// Same for completely unknown prefix.
			resp, err = callGetInherited("zzz", "user:someone-else@example.com")
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, resp, should.BeNil)
		})
	})
}

func TestMetadataUpdating(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, tc, as := testutil.TestingContext()

		meta := testutil.MetadataStore{}

		// ACL.
		meta.Populate("", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:admin@example.com"},
				},
			},
		})
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:top-owner@example.com"},
				},
			},
		})

		impl := repoImpl{meta: &meta}

		callUpdate := func(user identity.Identity, m *repopb.PrefixMetadata) (md *repopb.PrefixMetadata, err error) {
			err = datastore.RunInTransaction(as(user.Email()), func(ctx context.Context) (err error) {
				md, err = impl.UpdatePrefixMetadata(ctx, m)
				return
			}, nil)
			return
		}

		t.Run("Happy path", func(t *ftt.Test) {
			// Create new metadata entry.
			meta, err := callUpdate("user:top-owner@example.com", &repopb.PrefixMetadata{
				Prefix:     "a/b/",
				UpdateTime: timestamppb.New(time.Unix(10000, 0)), // should be overwritten
				UpdateUser: "user:zzz@example.com",               // should be overwritten
				Acls: []*repopb.PrefixMetadata_ACL{
					{Role: repopb.Role_READER, Principals: []string{"user:reader@example.com"}},
				},
			})
			assert.Loosely(t, err, should.BeNil)

			expected := &repopb.PrefixMetadata{
				Prefix:      "a/b",
				Fingerprint: "WZllwc6m8f9C_rfwnspaPIiyPD0",
				UpdateTime:  timestamppb.New(testutil.TestTime),
				UpdateUser:  "user:top-owner@example.com",
				Acls: []*repopb.PrefixMetadata_ACL{
					{Role: repopb.Role_READER, Principals: []string{"user:reader@example.com"}},
				},
			}
			assert.Loosely(t, meta, should.Match(expected))

			// Update it a bit later.
			tc.Add(time.Hour)
			updated := proto.Clone(expected).(*repopb.PrefixMetadata)
			updated.Acls = nil
			meta, err = callUpdate("user:top-owner@example.com", updated)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, meta, should.Match(&repopb.PrefixMetadata{
				Prefix:      "a/b",
				Fingerprint: "oQ2uuVbjV79prXxl4jyJkOpff90",
				UpdateTime:  timestamppb.New(testutil.TestTime.Add(time.Hour)),
				UpdateUser:  "user:top-owner@example.com",
			}))

			// Have these in the event log as well.
			datastore.GetTestable(ctx).CatchupIndexes()
			ev, err := model.QueryEvents(ctx, model.NewEventsQuery())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ev, should.Match([]*repopb.Event{
				{
					Kind:    repopb.EventKind_PREFIX_ACL_CHANGED,
					Package: "a/b",
					Who:     "user:top-owner@example.com",
					When:    timestamppb.New(testutil.TestTime.Add(time.Hour)),
					RevokedRole: []*repopb.PrefixMetadata_ACL{
						{Role: repopb.Role_READER, Principals: []string{"user:reader@example.com"}},
					},
				},
				{
					Kind:    repopb.EventKind_PREFIX_ACL_CHANGED,
					Package: "a/b",
					Who:     "user:top-owner@example.com",
					When:    timestamppb.New(testutil.TestTime),
					GrantedRole: []*repopb.PrefixMetadata_ACL{
						{Role: repopb.Role_READER, Principals: []string{"user:reader@example.com"}},
					},
				},
			}))
		})

		t.Run("Validation works", func(t *ftt.Test) {
			meta, err := callUpdate("user:top-owner@example.com", &repopb.PrefixMetadata{
				Prefix: "a/b//",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, meta, should.BeNil)

			meta, err = callUpdate("user:top-owner@example.com", &repopb.PrefixMetadata{
				Prefix: "a/b",
				Acls: []*repopb.PrefixMetadata_ACL{
					{Role: repopb.Role_READER, Principals: []string{"huh?"}},
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, meta, should.BeNil)
		})

		t.Run("ACLs work", func(t *ftt.Test) {
			meta, err := callUpdate("user:unknown@example.com", &repopb.PrefixMetadata{
				Prefix: "a/b",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, meta, should.BeNil)

			// Same as completely unknown prefix.
			meta, err = callUpdate("user:unknown@example.com", &repopb.PrefixMetadata{
				Prefix: "zzz",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, meta, should.BeNil)
		})

		t.Run("Deleted concurrently", func(t *ftt.Test) {
			m := meta.Populate("a/b", &repopb.PrefixMetadata{
				UpdateUser: "user:someone@example.com",
			})
			meta.Purge("a/b")

			// If the caller is a prefix owner, they see NotFound.
			meta, err := callUpdate("user:top-owner@example.com", m)
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, meta, should.BeNil)

			// Other callers just see regular PermissionDenined.
			meta, err = callUpdate("user:unknown@example.com", m)
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, meta, should.BeNil)
		})

		t.Run("Creating existing", func(t *ftt.Test) {
			m := meta.Populate("a/b", &repopb.PrefixMetadata{
				UpdateUser: "user:someone@example.com",
			})

			m.Fingerprint = "" // indicates the caller is expecting to create a new one
			meta, err := callUpdate("user:top-owner@example.com", m)
			assert.Loosely(t, status.Code(err), should.Equal(codes.AlreadyExists))
			assert.Loosely(t, meta, should.BeNil)
		})

		t.Run("Changed midway", func(t *ftt.Test) {
			m := meta.Populate("a/b", &repopb.PrefixMetadata{
				UpdateUser: "user:someone@example.com",
			})

			// Someone comes and updates it.
			updated, err := callUpdate("user:top-owner@example.com", m)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updated.Fingerprint, should.NotEqual(m.Fingerprint))

			// Trying to do it again fails, 'm' is stale now.
			_, err = callUpdate("user:top-owner@example.com", m)
			assert.Loosely(t, status.Code(err), should.Equal(codes.FailedPrecondition))
		})
	})
}

func TestGetRolesInPrefixOnBehalfOf(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		rootCtx, _, _ := testutil.TestingContext()

		meta := testutil.MetadataStore{}

		meta.Populate("", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:admin@example.com"},
				},
			},
		})
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_WRITER,
					Principals: []string{"user:writer@example.com"},
				},
			},
		})

		impl := repoImpl{meta: &meta}

		call := func(prefix string, user identity.Identity, callerIsPrefixViewer bool) (*repopb.RolesInPrefixResponse, error) {
			caller := identity.Identity("user:roles-caller@example.com")
			mocks := []authtest.MockedDatum{}
			if callerIsPrefixViewer {
				mocks = append(mocks, authtest.MockMembership(caller, PrefixesViewers))
			}
			return impl.GetRolesInPrefixOnBehalfOf(auth.WithState(rootCtx, &authtest.FakeState{
				Identity: caller,
				FakeDB:   authtest.NewFakeDB(mocks...),
			}), &repopb.PrefixRequestOnBehalfOf{Identity: string(user), PrefixRequest: &repopb.PrefixRequest{Prefix: prefix}})
		}

		t.Run("Happy path", func(t *ftt.Test) {
			resp, err := call("a/b/c/d", "user:writer@example.com", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.RolesInPrefixResponse{
				Roles: []*repopb.RolesInPrefixResponse_RoleInPrefix{
					{Role: repopb.Role_READER},
					{Role: repopb.Role_WRITER},
				},
			}))
		})

		t.Run("Anonymous", func(t *ftt.Test) {
			resp, err := call("a/b/c/d", "anonymous:anonymous", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.RolesInPrefixResponse{}))
		})

		t.Run("Admin", func(t *ftt.Test) {
			resp, err := call("a/b/c/d", "user:admin@example.com", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.RolesInPrefixResponse{
				Roles: []*repopb.RolesInPrefixResponse_RoleInPrefix{
					{Role: repopb.Role_READER},
					{Role: repopb.Role_WRITER},
					{Role: repopb.Role_OWNER},
				},
			}))
		})

		t.Run("Bad prefix", func(t *ftt.Test) {
			_, err := call("///", "user:writer@example.com", true)
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'prefix'"))
		})

		t.Run("Not prefix viewer", func(t *ftt.Test) {
			_, err := call("a/b/c/d", "user:writer@example.com", false)
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
		})

		t.Run("Disallowed identity types", func(t *ftt.Test) {
			for _, kind := range []identity.Kind{identity.Bot, identity.Project, identity.Service} {
				ident, err := identity.MakeIdentity(string(kind) + ":somevalue")
				assert.Loosely(t, err, should.BeNil)

				_, err = call("a/b/c/d", ident, true)
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			}
		})
	})
}

func TestGetRolesInPrefix(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		rootCtx, _, _ := testutil.TestingContext()

		meta := testutil.MetadataStore{}

		meta.Populate("", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:admin@example.com"},
				},
			},
		})
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_WRITER,
					Principals: []string{"user:writer@example.com"},
				},
			},
		})

		impl := repoImpl{meta: &meta}

		call := func(prefix string, user identity.Identity) (*repopb.RolesInPrefixResponse, error) {
			return impl.GetRolesInPrefix(auth.WithState(rootCtx, &authtest.FakeState{
				Identity: user,
				FakeDB:   authtest.NewFakeDB(),
			}), &repopb.PrefixRequest{Prefix: prefix})
		}

		t.Run("Happy path", func(t *ftt.Test) {
			resp, err := call("a/b/c/d", "user:writer@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.RolesInPrefixResponse{
				Roles: []*repopb.RolesInPrefixResponse_RoleInPrefix{
					{Role: repopb.Role_READER},
					{Role: repopb.Role_WRITER},
				},
			}))
		})

		t.Run("Anonymous", func(t *ftt.Test) {
			resp, err := call("a/b/c/d", "anonymous:anonymous")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.RolesInPrefixResponse{}))
		})

		t.Run("Admin", func(t *ftt.Test) {
			resp, err := call("a/b/c/d", "user:admin@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.RolesInPrefixResponse{
				Roles: []*repopb.RolesInPrefixResponse_RoleInPrefix{
					{Role: repopb.Role_READER},
					{Role: repopb.Role_WRITER},
					{Role: repopb.Role_OWNER},
				},
			}))
		})

		t.Run("Bad prefix", func(t *ftt.Test) {
			_, err := call("///", "user:writer@example.com")
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'prefix'"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Prefix listing.

func TestListPrefix(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()

		meta := testutil.MetadataStore{}

		meta.Populate("", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:admin@example.com"},
				},
			},
		})
		meta.Populate("1/a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})
		meta.Populate("6", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})
		meta.Populate("7", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		impl := repoImpl{meta: &meta}

		call := func(prefix string, recursive, hidden bool, user identity.Identity) (*repopb.ListPrefixResponse, error) {
			return impl.ListPrefix(as(user.Email()), &repopb.ListPrefixRequest{
				Prefix:        prefix,
				Recursive:     recursive,
				IncludeHidden: hidden,
			})
		}

		const hidden = true
		const visible = false
		mk := func(name string, hidden bool) {
			assert.Loosely(t, datastore.Put(ctx, &model.Package{
				Name:   name,
				Hidden: hidden,
			}), should.BeNil)
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

		t.Run("Full listing", func(t *ftt.Test) {
			t.Run("Root recursive (including hidden)", func(t *ftt.Test) {
				resp, err := call("", true, true, "user:admin@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string{
					"1", "1/a", "1/a/b", "1/a/b/c", "1/a/c", "1/b", "1/c", "1/d/a",
					"2/a/b/c", "3", "4", "5/a/b", "6", "6/a/b", "7/a",
				}))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{
					"1", "1/a", "1/a/b", "1/d", "2", "2/a", "2/a/b", "5", "5/a",
					"6", "6/a", "7",
				}))
			})

			t.Run("Root recursive (visible only)", func(t *ftt.Test) {
				resp, err := call("", true, false, "user:admin@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string{
					"1", "1/a", "1/a/b", "1/a/b/c", "1/b", "2/a/b/c", "3", "6/a/b",
				}))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{
					"1", "1/a", "1/a/b", "2", "2/a", "2/a/b", "6", "6/a",
				}))
			})

			t.Run("Root non-recursive (including hidden)", func(t *ftt.Test) {
				resp, err := call("", false, true, "user:admin@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string{"1", "3", "4", "6"}))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{"1", "2", "5", "6", "7"}))
			})

			t.Run("Root non-recursive (visible only)", func(t *ftt.Test) {
				resp, err := call("", false, false, "user:admin@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string{"1", "3"}))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{"1", "2", "6"}))
			})

			t.Run("Non-root recursive (including hidden)", func(t *ftt.Test) {
				resp, err := call("1", true, true, "user:admin@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string{
					"1/a", "1/a/b", "1/a/b/c", "1/a/c", "1/b", "1/c", "1/d/a",
				}))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{"1/a", "1/a/b", "1/d"}))
			})

			t.Run("Non-root recursive (visible only)", func(t *ftt.Test) {
				resp, err := call("1", true, false, "user:admin@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string{
					"1/a", "1/a/b", "1/a/b/c", "1/b",
				}))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{"1/a", "1/a/b"}))
			})
		})

		t.Run("Restricted listing", func(t *ftt.Test) {
			t.Run("Root recursive (including hidden)", func(t *ftt.Test) {
				resp, err := call("", true, true, "user:reader@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string{
					"1/a", "1/a/b", "1/a/b/c", "1/a/c", "6", "6/a/b", "7/a",
				}))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{
					"1", "1/a", "1/a/b", "6", "6/a", "7",
				}))
			})

			t.Run("Root recursive (visible only)", func(t *ftt.Test) {
				resp, err := call("", true, false, "user:reader@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string{
					"1/a", "1/a/b", "1/a/b/c", "6/a/b",
				}))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{
					"1", "1/a", "1/a/b", "6", "6/a",
				}))
			})

			t.Run("Root non-recursive (including hidden)", func(t *ftt.Test) {
				resp, err := call("", false, true, "user:reader@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string{"6"}))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{"1", "6", "7"}))
			})

			t.Run("Root non-recursive (visible only)", func(t *ftt.Test) {
				resp, err := call("", false, false, "user:reader@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string(nil)))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{"1", "6"}))
			})

			t.Run("Non-root recursive (including hidden)", func(t *ftt.Test) {
				resp, err := call("1", true, true, "user:reader@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string{
					"1/a", "1/a/b", "1/a/b/c", "1/a/c",
				}))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{"1/a", "1/a/b"}))
			})

			t.Run("Non-root recursive (visible only)", func(t *ftt.Test) {
				resp, err := call("1", true, false, "user:reader@example.com")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.Packages, should.Match([]string{
					"1/a", "1/a/b", "1/a/b/c",
				}))
				assert.Loosely(t, resp.Prefixes, should.Match([]string{"1/a", "1/a/b"}))
			})
		})

		t.Run("The package is not listed when listing its name directly", func(t *ftt.Test) {
			resp, err := call("3", true, true, "user:admin@example.com")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Packages, should.HaveLength(0))
			assert.Loosely(t, resp.Prefixes, should.HaveLength(0))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Hide/unhide package.

func TestHideUnhidePackage(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()
		ctx = as("owner@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:owner@example.com"},
				},
			},
		})

		assert.Loosely(t, datastore.Put(ctx, &model.Package{Name: "a/b"}), should.BeNil)

		fetch := func(pkg string) *model.Package {
			p := &model.Package{Name: pkg}
			assert.Loosely(t, datastore.Get(ctx, p), should.BeNil)
			return p
		}

		impl := repoImpl{meta: &meta}

		t.Run("Hides and unhides", func(t *ftt.Test) {
			_, err := impl.HidePackage(ctx, &repopb.PackageRequest{Package: "a/b"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetch("a/b").Hidden, should.BeTrue)

			// Noop is fine.
			_, err = impl.HidePackage(ctx, &repopb.PackageRequest{Package: "a/b"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetch("a/b").Hidden, should.BeTrue)

			_, err = impl.UnhidePackage(ctx, &repopb.PackageRequest{Package: "a/b"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetch("a/b").Hidden, should.BeFalse)
		})

		t.Run("Bad package name", func(t *ftt.Test) {
			_, err := impl.HidePackage(ctx, &repopb.PackageRequest{Package: "///"})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid package name"))
		})

		t.Run("No access", func(t *ftt.Test) {
			_, err := impl.HidePackage(ctx, &repopb.PackageRequest{Package: "zzz"})
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("not allowed to see it"))
		})

		t.Run("Missing package", func(t *ftt.Test) {
			_, err := impl.HidePackage(ctx, &repopb.PackageRequest{Package: "a/b/c"})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such package"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Package deletion.

func TestDeletePackage(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()

		meta := testutil.MetadataStore{}
		meta.Populate("", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:root@example.com"},
				},
			},
		})
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:non-root-owner@example.com"},
				},
			},
		})

		assert.Loosely(t, datastore.Put(ctx, &model.Package{Name: "a/b"}), should.BeNil)
		assert.Loosely(t, model.CheckPackageExists(ctx, "a/b"), should.BeNil)

		impl := repoImpl{meta: &meta}

		t.Run("Works", func(t *ftt.Test) {
			_, err := impl.DeletePackage(as("root@example.com"), &repopb.PackageRequest{
				Package: "a/b",
			})
			assert.Loosely(t, err, should.BeNil)

			// Gone now.
			assert.Loosely(t, model.CheckPackageExists(ctx, "a/b"), should.NotBeNil)

			_, err = impl.DeletePackage(as("root@example.com"), &repopb.PackageRequest{
				Package: "a/b",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such package"))
		})

		t.Run("Only reader or above can see", func(t *ftt.Test) {
			_, err := impl.DeletePackage(as("someone@example.com"), &repopb.PackageRequest{
				Package: "a/b",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("is not allowed to see it"))
		})

		t.Run("Only root owner can delete", func(t *ftt.Test) {
			_, err := impl.DeletePackage(as("non-root-owner@example.com"), &repopb.PackageRequest{
				Package: "a/b",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("allowed only to service administrators"))
		})

		t.Run("Bad package name", func(t *ftt.Test) {
			_, err := impl.DeletePackage(ctx, &repopb.PackageRequest{Package: "///"})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid package name"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Package instance registration and post-registration processing.

func TestRegisterInstance(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()
		ctx = as("owner@example.com")

		cas := testutil.MockCAS{}

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:owner@example.com"},
				},
				{
					Role:       repopb.Role_READER,
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
		inst := &repopb.Instance{
			Package: "a/b",
			Instance: &caspb.ObjectRef{
				HashAlgo:  caspb.HashAlgo_SHA1,
				HexDigest: digest,
			},
		}

		t.Run("Happy path", func(t *ftt.Test) {
			impl.registerProcessor(&mockedProcessor{
				ProcID:    "proc_id_1",
				AppliesTo: inst.Package,
			})
			impl.registerProcessor(&mockedProcessor{
				ProcID:    "proc_id_2",
				AppliesTo: "something else",
			})

			uploadOp := caspb.UploadOperation{
				OperationId: "op_id",
				UploadUrl:   "http://fake.example.com",
				Status:      caspb.UploadStatus_UPLOADING,
			}

			// Mock "successfully started upload op".
			cas.BeginUploadImpl = func(_ context.Context, req *caspb.BeginUploadRequest) (*caspb.UploadOperation, error) {
				assert.Loosely(t, req, should.Match(&caspb.BeginUploadRequest{
					Object: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA1,
						HexDigest: digest,
					},
				}))
				return &uploadOp, nil
			}

			// The instance is not uploaded yet => asks to upload.
			resp, err := impl.RegisterInstance(ctx, inst)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.RegisterInstanceResponse{
				Status:   repopb.RegistrationStatus_NOT_UPLOADED,
				UploadOp: &uploadOp,
			}))

			// Mock "already have it in the storage" response.
			cas.BeginUploadImpl = func(context.Context, *caspb.BeginUploadRequest) (*caspb.UploadOperation, error) {
				return nil, status.Errorf(codes.AlreadyExists, "already uploaded")
			}

			// The instance is already uploaded => registers it in the datastore.
			fullInstProto := &repopb.Instance{
				Package:      inst.Package,
				Instance:     inst.Instance,
				RegisteredBy: "user:owner@example.com",
				RegisteredTs: timestamppb.New(testutil.TestTime),
			}
			resp, err = impl.RegisterInstance(ctx, inst)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.RegisterInstanceResponse{
				Status:   repopb.RegistrationStatus_REGISTERED,
				Instance: fullInstProto,
			}))

			// Launched post-processors.
			ent := (&model.Instance{}).FromProto(ctx, inst)
			assert.Loosely(t, datastore.Get(ctx, ent), should.BeNil)
			assert.Loosely(t, ent.ProcessorsPending, should.Match([]string{"proc_id_1"}))
			tqt := sched.Tasks()
			assert.Loosely(t, tqt, should.HaveLength(1))
			assert.Loosely(t, tqt[0].Payload, should.Match(&tasks.RunProcessors{
				Instance: fullInstProto,
			}))
		})

		t.Run("Already registered", func(t *ftt.Test) {
			instance := (&model.Instance{
				RegisteredBy: "user:someone@example.com",
			}).FromProto(ctx, inst)
			_, _, err := model.RegisterInstance(ctx, instance, nil)
			assert.Loosely(t, err, should.BeNil)

			resp, err := impl.RegisterInstance(ctx, inst)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.RegisterInstanceResponse{
				Status: repopb.RegistrationStatus_ALREADY_REGISTERED,
				Instance: &repopb.Instance{
					Package:      inst.Package,
					Instance:     inst.Instance,
					RegisteredBy: "user:someone@example.com",
				},
			}))
		})

		t.Run("Bad package name", func(t *ftt.Test) {
			_, err := impl.RegisterInstance(ctx, &repopb.Instance{
				Package: "//a",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'package'"))
		})

		t.Run("Bad instance ID", func(t *ftt.Test) {
			_, err := impl.RegisterInstance(ctx, &repopb.Instance{
				Package: "a/b",
				Instance: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA1,
					HexDigest: "abc",
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'instance'"))
		})

		t.Run("No reader access", func(t *ftt.Test) {
			_, err := impl.RegisterInstance(ctx, &repopb.Instance{
				Package:  "some/other/root",
				Instance: inst.Instance,
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(`prefix "some/other/root" doesn't exist or "user:owner@example.com" is not allowed to see it`))
		})

		t.Run("No owner access", func(t *ftt.Test) {
			_, err := impl.RegisterInstance(as("reader@example.com"), inst)
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(`"user:reader@example.com" has no required WRITER role in prefix "a/b"`))
		})
	})
}

func TestProcessors(t *testing.T) {
	t.Parallel()

	testZip := testutil.MakeZip(map[string]string{
		"file1": strings.Repeat("hello", 50),
		"file2": "blah",
	})

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		cas := testutil.MockCAS{}
		impl := repoImpl{cas: &cas}

		inst := &repopb.Instance{
			Package: "a/b/c",
			Instance: &caspb.ObjectRef{
				HashAlgo:  caspb.HashAlgo_SHA1,
				HexDigest: strings.Repeat("a", 40),
			},
		}

		storeInstance := func(pending []string) {
			i := (&model.Instance{ProcessorsPending: pending}).FromProto(ctx, inst)
			assert.Loosely(t, datastore.Put(ctx, i), should.BeNil)
		}

		fetchInstance := func() *model.Instance {
			i := (&model.Instance{}).FromProto(ctx, inst)
			assert.Loosely(t, datastore.Get(ctx, i), should.BeNil)
			return i
		}

		fetchProcRes := func(id string) *model.ProcessingResult {
			i := (&model.Instance{}).FromProto(ctx, inst)
			p := &model.ProcessingResult{
				ProcID:   id,
				Instance: datastore.KeyForObj(ctx, i),
			}
			assert.Loosely(t, datastore.Get(ctx, p), should.BeNil)
			return p
		}

		goodResult := map[string]string{"result": "OK"}

		// Note: assumes Result is a map[string]string.
		fetchProcSuccess := func(id string) string {
			res := fetchProcRes(id)
			assert.Loosely(t, res, should.NotBeNil)
			assert.Loosely(t, res.Success, should.BeTrue)
			var r map[string]string
			assert.Loosely(t, res.ReadResult(&r), should.BeNil)
			return r["result"]
		}

		fetchProcFail := func(id string) string {
			res := fetchProcRes(id)
			assert.Loosely(t, res, should.NotBeNil)
			assert.Loosely(t, res.Success, should.BeFalse)
			return res.Error
		}

		t.Run("Noop updateProcessors", func(t *ftt.Test) {
			storeInstance([]string{"a", "b"})
			assert.Loosely(t, impl.updateProcessors(ctx, inst, map[string]processing.Result{
				"some-another": {Err: fmt.Errorf("fail")},
			}), should.BeNil)
			assert.Loosely(t, fetchInstance().ProcessorsPending, should.Match([]string{"a", "b"}))
		})

		t.Run("Updates processors successfully", func(t *ftt.Test) {
			storeInstance([]string{"ok", "fail", "pending"})

			assert.Loosely(t, impl.updateProcessors(ctx, inst, map[string]processing.Result{
				"ok":   {Result: goodResult},
				"fail": {Err: fmt.Errorf("failed")},
			}), should.BeNil)

			// Updated the Instance entity.
			inst := fetchInstance()
			assert.Loosely(t, inst.ProcessorsPending, should.Match([]string{"pending"}))
			assert.Loosely(t, inst.ProcessorsSuccess, should.Match([]string{"ok"}))
			assert.Loosely(t, inst.ProcessorsFailure, should.Match([]string{"fail"}))

			// Created ProcessingResult entities.
			assert.Loosely(t, fetchProcSuccess("ok"), should.Equal("OK"))
			assert.Loosely(t, fetchProcFail("fail"), should.Equal("failed"))
		})

		t.Run("Missing entity in updateProcessors", func(t *ftt.Test) {
			err := impl.updateProcessors(ctx, inst, map[string]processing.Result{
				"proc": {Err: fmt.Errorf("fail")},
			})
			assert.Loosely(t, err, should.ErrLike("the entity is unexpectedly gone"))
		})

		t.Run("runProcessorsTask happy path", func(t *ftt.Test) {
			// Setup two pending processors that read 'file2'.
			runCB := func(i *model.Instance, r *processing.PackageReader) (processing.Result, error) {
				assert.Loosely(t, i.Proto(), should.Match(inst))

				rd, _, err := r.Open("file2")
				assert.Loosely(t, err, should.BeNil)
				defer rd.Close()
				blob, err := io.ReadAll(rd)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(blob), should.Equal("blah"))

				return processing.Result{Result: goodResult}, nil
			}

			impl.registerProcessor(&mockedProcessor{ProcID: "proc1", RunCB: runCB})
			impl.registerProcessor(&mockedProcessor{ProcID: "proc2", RunCB: runCB})
			storeInstance([]string{"proc1", "proc2"})

			// Setup the package.
			cas.GetReaderImpl = func(_ context.Context, ref *caspb.ObjectRef) (gs.Reader, error) {
				assert.Loosely(t, inst.Instance, should.Match(ref))
				return testutil.NewMockGSReader(testZip), nil
			}

			// Run the processor.
			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			assert.Loosely(t, err, should.BeNil)

			// Both succeeded.
			inst := fetchInstance()
			assert.Loosely(t, inst.ProcessorsPending, should.HaveLength(0))
			assert.Loosely(t, inst.ProcessorsSuccess, should.Match([]string{"proc1", "proc2"}))

			// And have the result.
			assert.Loosely(t, fetchProcSuccess("proc1"), should.Equal("OK"))
			assert.Loosely(t, fetchProcSuccess("proc2"), should.Equal("OK"))
		})

		t.Run("runProcessorsTask no entity", func(t *ftt.Test) {
			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			assert.Loosely(t, err, should.ErrLike("unexpectedly gone from the datastore"))
		})

		t.Run("runProcessorsTask no processor", func(t *ftt.Test) {
			storeInstance([]string{"proc"})

			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			assert.Loosely(t, err, should.BeNil)

			// Failed.
			assert.Loosely(t, fetchProcFail("proc"), should.Equal(`unknown processor "proc"`))
		})

		t.Run("runProcessorsTask broken package", func(t *ftt.Test) {
			impl.registerProcessor(&mockedProcessor{
				ProcID: "proc",
				Result: processing.Result{Result: "must not be called"},
			})
			storeInstance([]string{"proc"})

			cas.GetReaderImpl = func(_ context.Context, ref *caspb.ObjectRef) (gs.Reader, error) {
				return testutil.NewMockGSReader([]byte("im not a zip")), nil
			}

			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			assert.Loosely(t, err, should.BeNil)

			// Failed.
			assert.Loosely(t, fetchProcFail("proc"), should.Equal(`error when opening the package: zip: not a valid zip file`))
		})

		t.Run("runProcessorsTask propagates transient proc errors", func(t *ftt.Test) {
			impl.registerProcessor(&mockedProcessor{
				ProcID: "good-proc",
				Result: processing.Result{Result: goodResult},
			})
			impl.registerProcessor(&mockedProcessor{
				ProcID: "bad-proc",
				Err:    fmt.Errorf("failed transiently"),
			})
			storeInstance([]string{"good-proc", "bad-proc"})

			cas.GetReaderImpl = func(_ context.Context, ref *caspb.ObjectRef) (gs.Reader, error) {
				return testutil.NewMockGSReader(testZip), nil
			}

			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			assert.Loosely(t, err, should.ErrLike("failed transiently"))

			// bad-proc is still pending.
			assert.Loosely(t, fetchInstance().ProcessorsPending, should.Match([]string{"bad-proc"}))
			// good-proc is done.
			assert.Loosely(t, fetchProcSuccess("good-proc"), should.Equal("OK"))
		})

		t.Run("runProcessorsTask handles fatal errors", func(t *ftt.Test) {
			impl.registerProcessor(&mockedProcessor{
				ProcID: "proc",
				Result: processing.Result{Err: fmt.Errorf("boom")},
			})
			storeInstance([]string{"proc"})

			cas.GetReaderImpl = func(_ context.Context, ref *caspb.ObjectRef) (gs.Reader, error) {
				return testutil.NewMockGSReader(testZip), nil
			}

			err := impl.runProcessorsTask(ctx, &tasks.RunProcessors{Instance: inst})
			assert.Loosely(t, err, should.BeNil)

			// Failed.
			assert.Loosely(t, fetchProcFail("proc"), should.Equal("boom"))
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

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ts := time.Unix(1525136124, 0).UTC()
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		assert.Loosely(t, datastore.Put(ctx, &model.Package{Name: "a/b"}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.Package{Name: "a/empty"}), should.BeNil)

		for i := range 4 {
			assert.Loosely(t, datastore.Put(ctx, &model.Instance{
				InstanceID:   fmt.Sprintf("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa%d", i),
				Package:      model.PackageKey(ctx, "a/b"),
				RegisteredTs: ts.Add(time.Duration(i) * time.Minute),
			}), should.BeNil)
		}

		inst := func(i int) *repopb.Instance {
			return &repopb.Instance{
				Package: "a/b",
				Instance: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA1,
					HexDigest: fmt.Sprintf("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa%d", i),
				},
				RegisteredTs: timestamppb.New(ts.Add(time.Duration(i) * time.Minute)),
			}
		}

		impl := repoImpl{meta: &meta}

		t.Run("Bad package name", func(t *ftt.Test) {
			_, err := impl.ListInstances(ctx, &repopb.ListInstancesRequest{
				Package: "///",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid package name"))
		})

		t.Run("Bad page size", func(t *ftt.Test) {
			_, err := impl.ListInstances(ctx, &repopb.ListInstancesRequest{
				Package:  "a/b",
				PageSize: -1,
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("it should be non-negative"))
		})

		t.Run("Bad page token", func(t *ftt.Test) {
			_, err := impl.ListInstances(ctx, &repopb.ListInstancesRequest{
				Package:   "a/b",
				PageToken: "zzzz",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid cursor"))
		})

		t.Run("No access", func(t *ftt.Test) {
			_, err := impl.ListInstances(ctx, &repopb.ListInstancesRequest{
				Package: "z",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
		})

		t.Run("No package", func(t *ftt.Test) {
			_, err := impl.ListInstances(ctx, &repopb.ListInstancesRequest{
				Package: "a/missing",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
		})

		t.Run("Empty listing", func(t *ftt.Test) {
			res, err := impl.ListInstances(ctx, &repopb.ListInstancesRequest{
				Package: "a/empty",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match(&repopb.ListInstancesResponse{}))
		})

		t.Run("Full listing (no pagination)", func(t *ftt.Test) {
			res, err := impl.ListInstances(ctx, &repopb.ListInstancesRequest{
				Package: "a/b",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match(&repopb.ListInstancesResponse{
				Instances: []*repopb.Instance{inst(3), inst(2), inst(1), inst(0)},
			}))
		})

		t.Run("Listing with pagination", func(t *ftt.Test) {
			// First page.
			res, err := impl.ListInstances(ctx, &repopb.ListInstancesRequest{
				Package:  "a/b",
				PageSize: 3,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Instances, should.Match([]*repopb.Instance{
				inst(3), inst(2), inst(1),
			}))
			assert.Loosely(t, res.NextPageToken, should.NotEqual(""))

			// Second page.
			res, err = impl.ListInstances(ctx, &repopb.ListInstancesRequest{
				Package:   "a/b",
				PageSize:  3,
				PageToken: res.NextPageToken,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match(&repopb.ListInstancesResponse{
				Instances: []*repopb.Instance{inst(0)},
			}))
		})
	})
}

func TestSearchInstances(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		assert.Loosely(t, datastore.Put(ctx, &model.Package{Name: "a/b"}), should.BeNil)

		put := func(when int, iid string, tags ...string) {
			inst := &model.Instance{
				InstanceID:   iid,
				Package:      model.PackageKey(ctx, "a/b"),
				RegisteredTs: testutil.TestTime.Add(time.Duration(when) * time.Second),
			}
			ents := make([]*model.Tag, len(tags))
			for i, mTag := range tags {
				ents[i] = &model.Tag{
					ID:           model.TagID(common.MustParseInstanceTag(mTag)),
					Instance:     datastore.KeyForObj(ctx, inst),
					Tag:          mTag,
					RegisteredTs: testutil.TestTime.Add(time.Duration(when) * time.Second),
				}
			}
			assert.Loosely(t, datastore.Put(ctx, inst, ents), should.BeNil)
		}

		iid := func(i int) string {
			ch := string([]byte{'0' + byte(i)})
			return strings.Repeat(ch, 40)
		}

		ids := func(inst []*repopb.Instance) []string {
			out := make([]string, len(inst))
			for i, obj := range inst {
				out[i] = common.ObjectRefToInstanceID(obj.Instance)
			}
			return out
		}

		expectedIIDs := make([]string, 10)
		for i := range 10 {
			put(i, iid(i), "a:b")
			expectedIIDs[9-i] = iid(i) // sorted by creation time, most recent first
		}

		impl := repoImpl{meta: &meta}

		t.Run("Bad package name", func(t *ftt.Test) {
			_, err := impl.SearchInstances(ctx, &repopb.SearchInstancesRequest{
				Package: "///",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid package name"))
		})

		t.Run("Bad page size", func(t *ftt.Test) {
			_, err := impl.SearchInstances(ctx, &repopb.SearchInstancesRequest{
				Package:  "a/b",
				PageSize: -1,
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("it should be non-negative"))
		})

		t.Run("Bad page token", func(t *ftt.Test) {
			_, err := impl.SearchInstances(ctx, &repopb.SearchInstancesRequest{
				Package:   "a/b",
				PageToken: "zzzz",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid cursor"))
		})

		t.Run("No tags specified", func(t *ftt.Test) {
			_, err := impl.SearchInstances(ctx, &repopb.SearchInstancesRequest{
				Package: "a/b",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'tags': cannot be empty"))
		})

		t.Run("Bad tag given", func(t *ftt.Test) {
			_, err := impl.SearchInstances(ctx, &repopb.SearchInstancesRequest{
				Package: "a/b",
				Tags:    []*repopb.Tag{{Key: "", Value: "zz"}},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`bad tag in 'tags': invalid tag key in ":zz"`))
		})

		t.Run("No access", func(t *ftt.Test) {
			_, err := impl.SearchInstances(ctx, &repopb.SearchInstancesRequest{
				Package: "z",
				Tags:    []*repopb.Tag{{Key: "a", Value: "b"}},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
		})

		t.Run("No package", func(t *ftt.Test) {
			_, err := impl.SearchInstances(ctx, &repopb.SearchInstancesRequest{
				Package: "a/missing",
				Tags:    []*repopb.Tag{{Key: "a", Value: "b"}},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
		})

		t.Run("Empty results", func(t *ftt.Test) {
			out, err := impl.SearchInstances(ctx, &repopb.SearchInstancesRequest{
				Package: "a/b",
				Tags:    []*repopb.Tag{{Key: "a", Value: "missing"}},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids(out.Instances), should.HaveLength(0))
			assert.Loosely(t, out.NextPageToken, should.BeEmpty)
		})

		t.Run("Full listing (no pagination)", func(t *ftt.Test) {
			out, err := impl.SearchInstances(ctx, &repopb.SearchInstancesRequest{
				Package: "a/b",
				Tags:    []*repopb.Tag{{Key: "a", Value: "b"}},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids(out.Instances), should.Match(expectedIIDs))
			assert.Loosely(t, out.NextPageToken, should.BeEmpty)
		})

		t.Run("Listing with pagination", func(t *ftt.Test) {
			out, err := impl.SearchInstances(ctx, &repopb.SearchInstancesRequest{
				Package:  "a/b",
				Tags:     []*repopb.Tag{{Key: "a", Value: "b"}},
				PageSize: 6,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids(out.Instances), should.Match(expectedIIDs[:6]))
			assert.Loosely(t, out.NextPageToken, should.NotEqual(""))

			out, err = impl.SearchInstances(ctx, &repopb.SearchInstancesRequest{
				Package:   "a/b",
				Tags:      []*repopb.Tag{{Key: "a", Value: "b"}},
				PageSize:  6,
				PageToken: out.NextPageToken,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids(out.Instances), should.Match(expectedIIDs[6:]))
			assert.Loosely(t, out.NextPageToken, should.BeEmpty)
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Refs support.

func TestRefs(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()
		ctx = as("writer@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_WRITER,
					Principals: []string{"user:writer@example.com"},
				},
			},
		})

		putInst := func(pkg, iid string, pendingProcs, failedProcs []string) {
			assert.Loosely(t, datastore.Put(ctx,
				&model.Package{Name: pkg},
				&model.Instance{
					InstanceID:        iid,
					Package:           model.PackageKey(ctx, pkg),
					ProcessorsPending: pendingProcs,
					ProcessorsFailure: failedProcs,
				}), should.BeNil)
		}

		digest := strings.Repeat("a", 40)
		putInst("a/b/c", digest, nil, nil)

		impl := repoImpl{meta: &meta}

		t.Run("CreateRef/ListRefs/DeleteRef happy path", func(t *ftt.Test) {
			_, err := impl.CreateRef(ctx, &repopb.Ref{
				Name:    "latest",
				Package: "a/b/c",
				Instance: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA1,
					HexDigest: digest,
				},
			})
			assert.Loosely(t, err, should.BeNil)

			// Can be listed now.
			refs, err := impl.ListRefs(ctx, &repopb.ListRefsRequest{Package: "a/b/c"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, refs.Refs, should.Match([]*repopb.Ref{
				{
					Name:    "latest",
					Package: "a/b/c",
					Instance: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA1,
						HexDigest: digest,
					},
					ModifiedBy: "user:writer@example.com",
					ModifiedTs: timestamppb.New(testutil.TestTime),
				},
			}))

			_, err = impl.DeleteRef(ctx, &repopb.DeleteRefRequest{
				Name:    "latest",
				Package: "a/b/c",
			})
			assert.Loosely(t, err, should.BeNil)

			// Missing now.
			refs, err = impl.ListRefs(ctx, &repopb.ListRefsRequest{Package: "a/b/c"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, refs.Refs, should.HaveLength(0))
		})

		t.Run("Bad ref", func(t *ftt.Test) {
			t.Run("CreateRef", func(t *ftt.Test) {
				_, err := impl.CreateRef(ctx, &repopb.Ref{
					Name:    "bad:ref:name",
					Package: "a/b/c",
					Instance: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA1,
						HexDigest: digest,
					},
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'name'"))
			})
			t.Run("DeleteRef", func(t *ftt.Test) {
				_, err := impl.DeleteRef(ctx, &repopb.DeleteRefRequest{
					Name:    "bad:ref:name",
					Package: "a/b/c",
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'name'"))
			})
		})

		t.Run("Bad package name", func(t *ftt.Test) {
			t.Run("CreateRef", func(t *ftt.Test) {
				_, err := impl.CreateRef(ctx, &repopb.Ref{
					Name:    "latest",
					Package: "///",
					Instance: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA1,
						HexDigest: digest,
					},
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'package'"))
			})
			t.Run("DeleteRef", func(t *ftt.Test) {
				_, err := impl.DeleteRef(ctx, &repopb.DeleteRefRequest{
					Name:    "latest",
					Package: "///",
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'package'"))
			})
			t.Run("ListRefs", func(t *ftt.Test) {
				_, err := impl.ListRefs(ctx, &repopb.ListRefsRequest{
					Package: "///",
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'package'"))
			})
		})

		t.Run("No access", func(t *ftt.Test) {
			t.Run("CreateRef", func(t *ftt.Test) {
				_, err := impl.CreateRef(ctx, &repopb.Ref{
					Name:    "latest",
					Package: "z",
					Instance: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA1,
						HexDigest: digest,
					},
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("is not allowed to see it"))
			})
			t.Run("DeleteRef", func(t *ftt.Test) {
				_, err := impl.DeleteRef(ctx, &repopb.DeleteRefRequest{
					Name:    "latest",
					Package: "z",
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("is not allowed to see it"))
			})
			t.Run("ListRefs", func(t *ftt.Test) {
				_, err := impl.ListRefs(ctx, &repopb.ListRefsRequest{
					Package: "z",
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("is not allowed to see it"))
			})
		})

		t.Run("Missing package", func(t *ftt.Test) {
			t.Run("CreateRef", func(t *ftt.Test) {
				_, err := impl.CreateRef(ctx, &repopb.Ref{
					Name:    "latest",
					Package: "a/b/z",
					Instance: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA1,
						HexDigest: digest,
					},
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})
			t.Run("DeleteRef", func(t *ftt.Test) {
				_, err := impl.DeleteRef(ctx, &repopb.DeleteRefRequest{
					Name:    "latest",
					Package: "a/b/z",
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})
			t.Run("ListRefs", func(t *ftt.Test) {
				_, err := impl.ListRefs(ctx, &repopb.ListRefsRequest{
					Package: "a/b/z",
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})
		})

		t.Run("Bad instance", func(t *ftt.Test) {
			_, err := impl.CreateRef(ctx, &repopb.Ref{
				Name:    "latest",
				Package: "a/b/c",
				Instance: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA1,
					HexDigest: "123",
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'instance'"))
		})

		t.Run("Missing instance", func(t *ftt.Test) {
			_, err := impl.CreateRef(ctx, &repopb.Ref{
				Name:    "latest",
				Package: "a/b/c",
				Instance: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA1,
					HexDigest: strings.Repeat("b", 40),
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such instance"))
		})

		t.Run("Instance is not ready yet", func(t *ftt.Test) {
			putInst("a/b/c", digest, []string{"proc"}, nil)
			_, err := impl.CreateRef(ctx, &repopb.Ref{
				Name:    "latest",
				Package: "a/b/c",
				Instance: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA1,
					HexDigest: digest,
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("the instance is not ready yet, pending processors: proc"))
		})

		t.Run("Failed processors", func(t *ftt.Test) {
			putInst("a/b/c", digest, nil, []string{"proc"})
			_, err := impl.CreateRef(ctx, &repopb.Ref{
				Name:    "latest",
				Package: "a/b/c",
				Instance: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA1,
					HexDigest: digest,
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.Aborted))
			assert.Loosely(t, err, should.ErrLike("some processors failed to process this instance: proc"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Tags support.

func TestTags(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
				{
					Role:       repopb.Role_WRITER,
					Principals: []string{"user:writer@example.com"},
				},
				{
					Role:       repopb.Role_OWNER,
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
			assert.Loosely(t, datastore.Put(ctx, &model.Package{Name: pkg}, inst), should.BeNil)
			return inst
		}

		getTag := func(inst *model.Instance, tag string) *model.Tag {
			mTag := &model.Tag{
				ID:       model.TagID(common.MustParseInstanceTag(tag)),
				Instance: datastore.KeyForObj(ctx, inst),
			}
			err := datastore.Get(ctx, mTag)
			if err == datastore.ErrNoSuchEntity {
				return nil
			}
			assert.Loosely(t, err, should.BeNil)
			return mTag
		}

		tags := func(tag ...string) []*repopb.Tag {
			out := make([]*repopb.Tag, len(tag))
			for i, s := range tag {
				out[i] = common.MustParseInstanceTag(s)
			}
			return out
		}

		digest := strings.Repeat("a", 40)
		inst := putInst("a/b/c", digest, nil, nil)
		objRef := &caspb.ObjectRef{
			HashAlgo:  caspb.HashAlgo_SHA1,
			HexDigest: digest,
		}

		impl := repoImpl{meta: &meta}

		t.Run("AttachTags/DetachTags happy path", func(t *ftt.Test) {
			_, err := impl.AttachTags(as("writer@example.com"), &repopb.AttachTagsRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Tags:     tags("a:0", "a:1"),
			})
			assert.Loosely(t, err, should.BeNil)

			// Attached both.
			assert.Loosely(t, getTag(inst, "a:0").RegisteredBy, should.Equal("user:writer@example.com"))
			assert.Loosely(t, getTag(inst, "a:1").RegisteredBy, should.Equal("user:writer@example.com"))

			// Detaching requires OWNER.
			_, err = impl.DetachTags(as("owner@example.com"), &repopb.DetachTagsRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Tags:     tags("a:0", "a:1", "a:missing"),
			})
			assert.Loosely(t, err, should.BeNil)

			// Missing now.
			assert.Loosely(t, getTag(inst, "a:0"), should.BeNil)
			assert.Loosely(t, getTag(inst, "a:1"), should.BeNil)
		})

		t.Run("Bad package", func(t *ftt.Test) {
			t.Run("AttachTags", func(t *ftt.Test) {
				_, err := impl.AttachTags(as("owner@example.com"), &repopb.AttachTagsRequest{
					Package:  "a/b///",
					Instance: objRef,
					Tags:     tags("a:0"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'package'"))
			})
			t.Run("DetachTags", func(t *ftt.Test) {
				_, err := impl.DetachTags(as("owner@example.com"), &repopb.DetachTagsRequest{
					Package:  "a/b///",
					Instance: objRef,
					Tags:     tags("a:0"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'package'"))
			})
		})

		t.Run("Bad ObjectRef", func(t *ftt.Test) {
			t.Run("AttachTags", func(t *ftt.Test) {
				_, err := impl.AttachTags(as("owner@example.com"), &repopb.AttachTagsRequest{
					Package: "a/b/c",
					Tags:    tags("a:0"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'instance'"))
			})
			t.Run("DetachTags", func(t *ftt.Test) {
				_, err := impl.DetachTags(as("owner@example.com"), &repopb.DetachTagsRequest{
					Package: "a/b/c",
					Tags:    tags("a:0"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'instance'"))
			})
		})

		t.Run("Empty tag list", func(t *ftt.Test) {
			t.Run("AttachTags", func(t *ftt.Test) {
				_, err := impl.AttachTags(as("owner@example.com"), &repopb.AttachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("cannot be empty"))
			})
			t.Run("DetachTags", func(t *ftt.Test) {
				_, err := impl.DetachTags(as("owner@example.com"), &repopb.DetachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("cannot be empty"))
			})
		})

		t.Run("Bad tag", func(t *ftt.Test) {
			t.Run("AttachTags", func(t *ftt.Test) {
				_, err := impl.AttachTags(as("owner@example.com"), &repopb.AttachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Tags:     []*repopb.Tag{{Key: ":"}},
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`invalid tag key`))
			})
			t.Run("DetachTags", func(t *ftt.Test) {
				_, err := impl.DetachTags(as("owner@example.com"), &repopb.DetachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Tags:     []*repopb.Tag{{Key: ":"}},
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`invalid tag key`))
			})
		})

		t.Run("No access", func(t *ftt.Test) {
			t.Run("AttachTags", func(t *ftt.Test) {
				_, err := impl.AttachTags(as("reader@example.com"), &repopb.AttachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Tags:     tags("good:tag"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("has no required WRITER role"))
			})
			t.Run("DetachTags", func(t *ftt.Test) {
				_, err := impl.DetachTags(as("writer@example.com"), &repopb.DetachTagsRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Tags:     tags("good:tag"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("has no required OWNER role"))
			})
		})

		t.Run("Missing package", func(t *ftt.Test) {
			t.Run("AttachTags", func(t *ftt.Test) {
				_, err := impl.AttachTags(as("owner@example.com"), &repopb.AttachTagsRequest{
					Package:  "a/b/zzz",
					Instance: objRef,
					Tags:     tags("a:0"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})
			t.Run("DetachTags", func(t *ftt.Test) {
				_, err := impl.DetachTags(as("owner@example.com"), &repopb.DetachTagsRequest{
					Package:  "a/b/zzz",
					Instance: objRef,
					Tags:     tags("a:0"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})
		})

		t.Run("Missing instance", func(t *ftt.Test) {
			missingRef := &caspb.ObjectRef{
				HashAlgo:  caspb.HashAlgo_SHA1,
				HexDigest: strings.Repeat("b", 40),
			}
			t.Run("AttachTags", func(t *ftt.Test) {
				_, err := impl.AttachTags(as("owner@example.com"), &repopb.AttachTagsRequest{
					Package:  "a/b/c",
					Instance: missingRef,
					Tags:     tags("a:0"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such instance"))
			})
			t.Run("DetachTags", func(t *ftt.Test) {
				// DetachTags doesn't care.
				_, err := impl.DetachTags(as("owner@example.com"), &repopb.DetachTagsRequest{
					Package:  "a/b/c",
					Instance: missingRef,
					Tags:     tags("a:0"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such instance"))
			})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Instance metadata support.

func TestInstanceMetadata(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
				{
					Role:       repopb.Role_WRITER,
					Principals: []string{"user:writer@example.com"},
				},
				{
					Role:       repopb.Role_OWNER,
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
			assert.Loosely(t, datastore.Put(ctx, &model.Package{Name: pkg}, inst), should.BeNil)
			return inst
		}

		getMD := func(inst *model.Instance, kv string) *model.InstanceMetadata {
			split := strings.SplitN(kv, ":", 2)
			meta := &model.InstanceMetadata{
				Fingerprint: common.InstanceMetadataFingerprint(split[0], []byte(split[1])),
				Instance:    datastore.KeyForObj(ctx, inst),
			}
			if err := datastore.Get(ctx, meta); err != datastore.ErrNoSuchEntity {
				assert.Loosely(t, err, should.BeNil)
				return meta
			}
			return nil
		}

		md := func(kv ...string) []*repopb.InstanceMetadata {
			out := make([]*repopb.InstanceMetadata, len(kv))
			for i, s := range kv {
				split := strings.SplitN(s, ":", 2)
				out[i] = &repopb.InstanceMetadata{
					Key:   split[0],
					Value: []byte(split[1]),
				}
			}
			return out
		}

		digest := strings.Repeat("a", 40)
		inst := putInst("a/b/c", digest, nil, nil)
		objRef := &caspb.ObjectRef{
			HashAlgo:  caspb.HashAlgo_SHA1,
			HexDigest: digest,
		}

		impl := repoImpl{meta: &meta}

		t.Run("AttachMetadata/DetachMetadata/ListMetadata happy path", func(t *ftt.Test) {
			_, err := impl.AttachMetadata(as("writer@example.com"), &repopb.AttachMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Metadata: md("k0:0", "k1:1"),
			})
			assert.Loosely(t, err, should.BeNil)

			// Attached both.
			assert.Loosely(t, getMD(inst, "k0:0").AttachedBy, should.Equal("user:writer@example.com"))
			assert.Loosely(t, getMD(inst, "k1:1").AttachedBy, should.Equal("user:writer@example.com"))

			// Can retrieve it all.
			resp, err := impl.ListMetadata(as("reader@example.com"), &repopb.ListMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Metadata, should.HaveLength(2))

			// Can retrieve an individual key.
			resp, err = impl.ListMetadata(as("reader@example.com"), &repopb.ListMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Keys:     []string{"k0"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Metadata, should.HaveLength(1))

			// Detaching requires OWNER. Detach one by giving a KV pair, and another
			// via its fingerprint.
			_, err = impl.DetachMetadata(as("owner@example.com"), &repopb.DetachMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Metadata: []*repopb.InstanceMetadata{
					{
						Key:   "k0",
						Value: []byte{'0'},
					},
					{
						Fingerprint: common.InstanceMetadataFingerprint("k1", []byte{'1'}),
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)

			// Missing now.
			assert.Loosely(t, getMD(inst, "k0:0"), should.BeNil)
			assert.Loosely(t, getMD(inst, "k1:1"), should.BeNil)
		})

		t.Run("Bad package", func(t *ftt.Test) {
			t.Run("AttachMetadata", func(t *ftt.Test) {
				_, err := impl.AttachMetadata(as("writer@example.com"), &repopb.AttachMetadataRequest{
					Package:  "a/b//",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'package'"))
			})
			t.Run("DetachMetadata", func(t *ftt.Test) {
				_, err := impl.DetachMetadata(as("owner@example.com"), &repopb.DetachMetadataRequest{
					Package:  "a/b//",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'package'"))
			})
			t.Run("ListMetadata", func(t *ftt.Test) {
				_, err := impl.ListMetadata(as("reader@example.com"), &repopb.ListMetadataRequest{
					Package:  "a/b//",
					Instance: objRef,
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'package'"))
			})
		})

		t.Run("Bad ObjectRef", func(t *ftt.Test) {
			t.Run("AttachMetadata", func(t *ftt.Test) {
				_, err := impl.AttachMetadata(as("writer@example.com"), &repopb.AttachMetadataRequest{
					Package:  "a/b/c",
					Metadata: md("k:0", "k:1"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'instance'"))
			})
			t.Run("DetachMetadata", func(t *ftt.Test) {
				_, err := impl.DetachMetadata(as("owner@example.com"), &repopb.DetachMetadataRequest{
					Package:  "a/b/c",
					Metadata: md("k:0", "k:1"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'instance'"))
			})
			t.Run("ListMetadata", func(t *ftt.Test) {
				_, err := impl.ListMetadata(as("reader@example.com"), &repopb.ListMetadataRequest{
					Package: "a/b/c",
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'instance'"))
			})
		})

		t.Run("Empty metadata list", func(t *ftt.Test) {
			t.Run("AttachMetadata", func(t *ftt.Test) {
				_, err := impl.AttachMetadata(as("writer@example.com"), &repopb.AttachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("cannot be empty"))
			})
			t.Run("DetachMetadata", func(t *ftt.Test) {
				_, err := impl.DetachMetadata(as("owner@example.com"), &repopb.DetachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("cannot be empty"))
			})
		})

		t.Run("Bad metadata key", func(t *ftt.Test) {
			t.Run("AttachMetadata", func(t *ftt.Test) {
				_, err := impl.AttachMetadata(as("writer@example.com"), &repopb.AttachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("ZZZ:0"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`invalid metadata key`))
			})
			t.Run("DetachMetadata", func(t *ftt.Test) {
				_, err := impl.DetachMetadata(as("owner@example.com"), &repopb.DetachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("ZZZ:0"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`invalid metadata key`))
			})
			t.Run("ListMetadata", func(t *ftt.Test) {
				_, err := impl.ListMetadata(as("reader@example.com"), &repopb.ListMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Keys:     []string{"ZZZ"},
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`invalid metadata key`))
			})
		})

		t.Run("Bad metadata value", func(t *ftt.Test) {
			t.Run("AttachMetadata", func(t *ftt.Test) {
				_, err := impl.AttachMetadata(as("writer@example.com"), &repopb.AttachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("k:" + strings.Repeat("z", 512*1024+1)),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`metadata with key "k": the metadata value is too long`))
			})
			t.Run("DetachMetadata", func(t *ftt.Test) {
				_, err := impl.DetachMetadata(as("owner@example.com"), &repopb.DetachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("k:" + strings.Repeat("z", 512*1024+1)),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`metadata with key "k": the metadata value is too long`))
			})
		})

		t.Run("Bad metadata content type in AttachMetadata", func(t *ftt.Test) {
			m := md("k:0")
			m[0].ContentType = "zzz zzz"
			_, err := impl.AttachMetadata(as("writer@example.com"), &repopb.AttachMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Metadata: m,
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`metadata with key "k": bad content type "zzz zzz`))
		})

		t.Run("Bad fingerprint in DetachMetadata", func(t *ftt.Test) {
			_, err := impl.DetachMetadata(as("owner@example.com"), &repopb.DetachMetadataRequest{
				Package:  "a/b/c",
				Instance: objRef,
				Metadata: []*repopb.InstanceMetadata{
					{Fingerprint: "bad"},
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`bad metadata fingerprint "bad"`))
		})

		t.Run("No access", func(t *ftt.Test) {
			t.Run("AttachMetadata", func(t *ftt.Test) {
				_, err := impl.AttachMetadata(as("reader@example.com"), &repopb.AttachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("has no required WRITER role"))
			})
			t.Run("DetachMetadata", func(t *ftt.Test) {
				_, err := impl.DetachMetadata(as("writer@example.com"), &repopb.DetachMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("has no required OWNER role"))
			})
			t.Run("ListMetadata", func(t *ftt.Test) {
				_, err := impl.ListMetadata(as("unknown@example.com"), &repopb.ListMetadataRequest{
					Package:  "a/b/c",
					Instance: objRef,
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("is not allowed to see it"))
			})
		})

		t.Run("Missing package", func(t *ftt.Test) {
			t.Run("AttachMetadata", func(t *ftt.Test) {
				_, err := impl.AttachMetadata(as("writer@example.com"), &repopb.AttachMetadataRequest{
					Package:  "a/b/c/missing",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})
			t.Run("DetachMetadata", func(t *ftt.Test) {
				_, err := impl.DetachMetadata(as("owner@example.com"), &repopb.DetachMetadataRequest{
					Package:  "a/b/c/missing",
					Instance: objRef,
					Metadata: md("k:0", "k:1"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})
			t.Run("ListMetadata", func(t *ftt.Test) {
				_, err := impl.ListMetadata(as("reader@example.com"), &repopb.ListMetadataRequest{
					Package:  "a/b/c/missing",
					Instance: objRef,
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})
		})

		t.Run("Missing instance", func(t *ftt.Test) {
			missingRef := &caspb.ObjectRef{
				HashAlgo:  caspb.HashAlgo_SHA1,
				HexDigest: strings.Repeat("b", 40),
			}
			t.Run("AttachMetadata", func(t *ftt.Test) {
				_, err := impl.AttachMetadata(as("writer@example.com"), &repopb.AttachMetadataRequest{
					Package:  "a/b/c",
					Instance: missingRef,
					Metadata: md("k:0", "k:1"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such instance"))
			})
			t.Run("DetachMetadata", func(t *ftt.Test) {
				_, err := impl.DetachMetadata(as("owner@example.com"), &repopb.DetachMetadataRequest{
					Package:  "a/b/c",
					Instance: missingRef,
					Metadata: md("k:0", "k:1"),
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such instance"))
			})
			t.Run("ListMetadata", func(t *ftt.Test) {
				_, err := impl.ListMetadata(as("reader@example.com"), &repopb.ListMetadataRequest{
					Package:  "a/b/c",
					Instance: missingRef,
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such instance"))
			})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Version resolution and instance info fetching.

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
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

		assert.Loosely(t, datastore.Put(ctx, pkg, inst1, inst2), should.BeNil)
		assert.Loosely(t, model.SetRef(ctx, "latest", inst2), should.BeNil)
		assert.Loosely(t, model.AttachTags(ctx, inst1, []*repopb.Tag{
			{Key: "ver", Value: "1"},
			{Key: "ver", Value: "ambiguous"},
		}), should.BeNil)
		assert.Loosely(t, model.AttachTags(ctx, inst2, []*repopb.Tag{
			{Key: "ver", Value: "2"},
			{Key: "ver", Value: "ambiguous"},
		}), should.BeNil)

		t.Run("Happy path", func(t *ftt.Test) {
			inst, err := impl.ResolveVersion(ctx, &repopb.ResolveVersionRequest{
				Package: "a/pkg",
				Version: "latest",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inst, should.Match(inst2.Proto()))
		})

		t.Run("Bad package name", func(t *ftt.Test) {
			_, err := impl.ResolveVersion(ctx, &repopb.ResolveVersionRequest{
				Package: "///",
				Version: "latest",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'package'"))
		})

		t.Run("Bad version name", func(t *ftt.Test) {
			_, err := impl.ResolveVersion(ctx, &repopb.ResolveVersionRequest{
				Package: "a/pkg",
				Version: "::",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'version'"))
		})

		t.Run("No access", func(t *ftt.Test) {
			_, err := impl.ResolveVersion(ctx, &repopb.ResolveVersionRequest{
				Package: "b",
				Version: "latest",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("is not allowed to see it"))
		})

		t.Run("Missing package", func(t *ftt.Test) {
			_, err := impl.ResolveVersion(ctx, &repopb.ResolveVersionRequest{
				Package: "a/b",
				Version: "latest",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such package"))
		})

		t.Run("Missing instance", func(t *ftt.Test) {
			_, err := impl.ResolveVersion(ctx, &repopb.ResolveVersionRequest{
				Package: "a/pkg",
				Version: strings.Repeat("f", 40),
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such instance"))
		})

		t.Run("Missing ref", func(t *ftt.Test) {
			_, err := impl.ResolveVersion(ctx, &repopb.ResolveVersionRequest{
				Package: "a/pkg",
				Version: "missing",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such ref"))
		})

		t.Run("Missing tag", func(t *ftt.Test) {
			_, err := impl.ResolveVersion(ctx, &repopb.ResolveVersionRequest{
				Package: "a/pkg",
				Version: "ver:missing",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such tag"))
		})

		t.Run("Ambiguous tag", func(t *ftt.Test) {
			_, err := impl.ResolveVersion(ctx, &repopb.ResolveVersionRequest{
				Package: "a/pkg",
				Version: "ver:ambiguous",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("ambiguity when resolving the tag"))
		})
	})
}

func TestGetInstanceURLAndDownloads(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		cas := testutil.MockCAS{}
		impl := repoImpl{meta: &meta, cas: &cas, vsa: vsa.NewClient()}

		inst := &model.Instance{
			InstanceID:   strings.Repeat("1", 40),
			Package:      model.PackageKey(ctx, "a/pkg"),
			RegisteredBy: "user:1@example.com",
		}
		assert.Loosely(t, datastore.Put(ctx, &model.Package{Name: "a/pkg"}, inst), should.BeNil)

		cas.GetObjectURLImpl = func(_ context.Context, r *caspb.GetObjectURLRequest) (*caspb.ObjectURL, error) {
			assert.Loosely(t, r.Object.HashAlgo, should.Equal(caspb.HashAlgo_SHA1))
			assert.Loosely(t, r.Object.HexDigest, should.Equal(inst.InstanceID))
			return &caspb.ObjectURL{
				SignedUrl: fmt.Sprintf("http://example.com/%s?d=%s", r.Object.HexDigest, r.DownloadFilename),
			}, nil
		}

		t.Run("GetInstanceURL", func(t *ftt.Test) {
			t.Run("Happy path", func(t *ftt.Test) {
				resp, err := impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
					Package:  inst.Package.StringID(),
					Instance: inst.Proto().Instance,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(&caspb.ObjectURL{
					SignedUrl: "http://example.com/1111111111111111111111111111111111111111?d=",
				}))
			})

			t.Run("Bad package name", func(t *ftt.Test) {
				_, err := impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
					Package:  "///",
					Instance: inst.Proto().Instance,
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'package'"))
			})

			t.Run("Bad instance", func(t *ftt.Test) {
				_, err := impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
					Package: "a/pkg",
					Instance: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA1,
						HexDigest: "huh",
					},
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad 'instance'"))
			})

			t.Run("No access", func(t *ftt.Test) {
				_, err := impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
					Package:  "b",
					Instance: inst.Proto().Instance,
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("is not allowed to see it"))
			})

			t.Run("Missing package", func(t *ftt.Test) {
				_, err := impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
					Package:  "a/missing",
					Instance: inst.Proto().Instance,
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
			})

			t.Run("Missing instance", func(t *ftt.Test) {
				_, err := impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
					Package: "a/pkg",
					Instance: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA1,
						HexDigest: strings.Repeat("f", 40),
					},
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such instance"))
			})
		})

		t.Run("Raw download handler", func(t *ftt.Test) {
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

			t.Run("Happy path", func(t *ftt.Test) {
				rr := call("/a/pkg/+/1111111111111111111111111111111111111111")
				assert.Loosely(t, rr.Code, should.Equal(http.StatusFound))
				assert.Loosely(t, rr.Header().Get("Location"), should.Equal("http://example.com/1111111111111111111111111111111111111111?d=a-pkg.zip"))
				assert.Loosely(t, rr.Header().Get(cipdInstanceHeader), should.Equal(inst.InstanceID))
			})

			t.Run("Malformed URL", func(t *ftt.Test) {
				rr := call("huh")
				assert.Loosely(t, rr.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("the URL should have form"))
			})

			t.Run("Bad package name", func(t *ftt.Test) {
				rr := call("/???/+/live")
				assert.Loosely(t, rr.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("invalid package name"))
			})

			t.Run("Bad version", func(t *ftt.Test) {
				rr := call("/pkg/+/???")
				assert.Loosely(t, rr.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("bad version"))
			})

			t.Run("No access", func(t *ftt.Test) {
				rr := call("/b/+/live")
				assert.Loosely(t, rr.Code, should.Equal(http.StatusForbidden))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("is not allowed to see it"))
			})

			t.Run("Missing package", func(t *ftt.Test) {
				rr := call("/a/missing/+/live")
				assert.Loosely(t, rr.Code, should.Equal(http.StatusNotFound))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("no such package"))
			})

			t.Run("Missing instance", func(t *ftt.Test) {
				rr := call("/a/pkg/+/1111111111111111111111111111111111111112")
				assert.Loosely(t, rr.Code, should.Equal(http.StatusNotFound))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("no such instance"))
			})
		})
	})
}

func TestDescribeInstance(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
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
		assert.Loosely(t, datastore.Put(ctx, &model.Package{Name: "a/pkg"}, inst), should.BeNil)

		t.Run("Happy path, basic info", func(t *ftt.Test) {
			resp, err := impl.DescribeInstance(ctx, &repopb.DescribeInstanceRequest{
				Package:  "a/pkg",
				Instance: inst.Proto().Instance,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.DescribeInstanceResponse{
				Instance: inst.Proto(),
			}))
		})

		t.Run("Happy path, full info", func(t *ftt.Test) {
			model.AttachTags(as("tag@example.com"), inst, []*repopb.Tag{
				{Key: "a", Value: "0"},
				{Key: "a", Value: "1"},
			})

			model.SetRef(as("ref@example.com"), "ref_a", inst)
			model.SetRef(as("ref@example.com"), "ref_b", inst)

			model.AttachMetadata(as("metadata@example.com"), inst, []*repopb.InstanceMetadata{
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

			resp, err := impl.DescribeInstance(ctx, &repopb.DescribeInstanceRequest{
				Package:            "a/pkg",
				Instance:           inst.Proto().Instance,
				DescribeTags:       true,
				DescribeRefs:       true,
				DescribeProcessors: true,
				DescribeMetadata:   true,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.DescribeInstanceResponse{
				Instance: inst.Proto(),
				Tags: []*repopb.Tag{
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
				Refs: []*repopb.Ref{
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
				Processors: []*repopb.Processor{
					{
						Id:         "proc",
						State:      repopb.Processor_SUCCEEDED,
						FinishedTs: timestamppb.New(testutil.TestTime),
					},
				},
				// Note that metadata is returned LIFO.
				Metadata: []*repopb.InstanceMetadata{
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
			}))
		})

		t.Run("Bad package name", func(t *ftt.Test) {
			_, err := impl.DescribeInstance(ctx, &repopb.DescribeInstanceRequest{
				Package:  "///",
				Instance: inst.Proto().Instance,
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'package'"))
		})

		t.Run("Bad instance", func(t *ftt.Test) {
			_, err := impl.DescribeInstance(ctx, &repopb.DescribeInstanceRequest{
				Package: "a/pkg",
				Instance: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA1,
					HexDigest: "huh",
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'instance'"))
		})

		t.Run("No access", func(t *ftt.Test) {
			_, err := impl.DescribeInstance(ctx, &repopb.DescribeInstanceRequest{
				Package:  "b",
				Instance: inst.Proto().Instance,
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("is not allowed to see it"))
		})

		t.Run("Missing package", func(t *ftt.Test) {
			_, err := impl.DescribeInstance(ctx, &repopb.DescribeInstanceRequest{
				Package:  "a/missing",
				Instance: inst.Proto().Instance,
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such package"))
		})

		t.Run("Missing instance", func(t *ftt.Test) {
			_, err := impl.DescribeInstance(ctx, &repopb.DescribeInstanceRequest{
				Package: "a/pkg",
				Instance: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA1,
					HexDigest: strings.Repeat("f", 40),
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such instance"))
		})
	})
}

func TestDescribeBootstrapBundle(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		impl := repoImpl{meta: &meta}

		putInst := func(pkg, extracted string) {
			inst := &model.Instance{
				InstanceID: common.ObjectRefToInstanceID(&caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA256,
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
				assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			}
			assert.Loosely(t, datastore.Put(ctx,
				&model.Package{Name: pkg},
				inst,
				&model.Ref{Name: "latest", Package: inst.Package, InstanceID: inst.InstanceID},
			), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
		}

		expectedFile := func(pkg, extracted string) *repopb.DescribeBootstrapBundleResponse_BootstrapFile {
			return &repopb.DescribeBootstrapBundleResponse_BootstrapFile{
				Package: pkg,
				Instance: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA256,
					HexDigest: strings.Repeat("1", 64),
				},
				File: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA256,
					HexDigest: strings.Repeat("a", 64),
				},
				Name: extracted,
				Size: 12345,
			}
		}

		expectedError := func(pkg string, code codes.Code, msg string) *repopb.DescribeBootstrapBundleResponse_BootstrapFile {
			return &repopb.DescribeBootstrapBundleResponse_BootstrapFile{
				Package: pkg,
				Status: &statuspb.Status{
					Code:    int32(code),
					Message: msg,
				},
			}
		}

		t.Run("Happy path", func(t *ftt.Test) {
			putInst("a/pkg/var-1", "file1")
			putInst("a/pkg/var-2", "file2")
			putInst("a/pkg/var-3", "file3")
			putInst("a/pkg", "")             // will be ignored
			putInst("a/pkg/sun/package", "") // will be ignored

			t.Run("With prefix listing", func(t *ftt.Test) {
				resp, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
					Prefix:  "a/pkg",
					Version: "latest",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(&repopb.DescribeBootstrapBundleResponse{
					Files: []*repopb.DescribeBootstrapBundleResponse_BootstrapFile{
						expectedFile("a/pkg/var-1", "file1"),
						expectedFile("a/pkg/var-2", "file2"),
						expectedFile("a/pkg/var-3", "file3"),
					},
				}))
			})

			t.Run("With variants", func(t *ftt.Test) {
				resp, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
					Prefix:   "a/pkg",
					Variants: []string{"var-3", "var-1"},
					Version:  "latest",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(&repopb.DescribeBootstrapBundleResponse{
					Files: []*repopb.DescribeBootstrapBundleResponse_BootstrapFile{
						expectedFile("a/pkg/var-3", "file3"),
						expectedFile("a/pkg/var-1", "file1"),
					},
				}))
			})
		})

		t.Run("Partial failure", func(t *ftt.Test) {
			putInst("a/pkg/var-1", "file1")
			putInst("a/pkg/var-2", "")

			resp, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
				Prefix:  "a/pkg",
				Version: "latest",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&repopb.DescribeBootstrapBundleResponse{
				Files: []*repopb.DescribeBootstrapBundleResponse_BootstrapFile{
					expectedFile("a/pkg/var-1", "file1"),
					expectedError("a/pkg/var-2", codes.FailedPrecondition, `"a/pkg/var-2" is not a bootstrap package`),
				},
			}))
		})

		t.Run("Total failure", func(t *ftt.Test) {
			putInst("a/pkg/var-1", "")
			putInst("a/pkg/var-2", "")
			putInst("a/pkg/var-3", "")

			_, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
				Prefix:  "a/pkg",
				Version: "latest",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike(`"a/pkg/var-1" is not a bootstrap package (and 2 other errors like this)`))
		})

		t.Run("Empty prefix", func(t *ftt.Test) {
			_, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
				Prefix:  "a/pkg",
				Version: "latest",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`no packages directly under prefix "a/pkg"`))
		})

		t.Run("Missing version", func(t *ftt.Test) {
			putInst("a/pkg/var-1", "file1")
			_, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
				Prefix:  "a/pkg",
				Version: "not-latest",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`no such ref`))
		})

		t.Run("Broken processor", func(t *ftt.Test) {
			putInst("a/pkg/var-1", "BROKEN")
			_, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
				Prefix:  "a/pkg",
				Version: "latest",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.Aborted))
			assert.Loosely(t, err, should.ErrLike(`some processors failed to process this instance`))
		})

		t.Run("Request validation", func(t *ftt.Test) {
			putInst("a/pkg/var-1", "file1")

			t.Run("Bad prefix format", func(t *ftt.Test) {
				_, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
					Prefix:  "///",
					Version: "latest",
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})

			t.Run("Variant with /", func(t *ftt.Test) {
				_, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
					Prefix:   "a/pkg",
					Variants: []string{"some/thing"},
					Version:  "latest",
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})

			t.Run("Empty variant", func(t *ftt.Test) {
				_, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
					Prefix:   "a/pkg",
					Variants: []string{""},
					Version:  "latest",
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})

			t.Run("Malformed variant name", func(t *ftt.Test) {
				_, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
					Prefix:   "a/pkg",
					Variants: []string{"BAD"},
					Version:  "latest",
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})

			t.Run("Duplicate variants", func(t *ftt.Test) {
				_, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
					Prefix:   "a/pkg",
					Variants: []string{"var-1", "var-1"},
					Version:  "latest",
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})

			t.Run("Bad version", func(t *ftt.Test) {
				_, err := impl.DescribeBootstrapBundle(ctx, &repopb.DescribeBootstrapBundleRequest{
					Prefix:  "a/pkg",
					Version: "bad version ID",
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})

			t.Run("Not a reader", func(t *ftt.Test) {
				_, err := impl.DescribeBootstrapBundle(as("someone@example.com"), &repopb.DescribeBootstrapBundleRequest{
					Prefix:  "a/pkg",
					Version: "latest",
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Client bootstrap and legacy API.

func TestClientBootstrap(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		cas := testutil.MockCAS{
			GetObjectURLImpl: func(_ context.Context, r *caspb.GetObjectURLRequest) (*caspb.ObjectURL, error) {
				return &caspb.ObjectURL{
					SignedUrl: fmt.Sprintf("http://fake/%s/%s?d=%s&blah=zzz", r.Object.HashAlgo, r.Object.HexDigest, r.DownloadFilename),
				}, nil
			},
		}

		impl := repoImpl{meta: &meta, cas: &cas}

		goodPlat := "linux-amd64"
		goodDigest := strings.Repeat("f", 64)
		goodIID := common.ObjectRefToInstanceID(&caspb.ObjectRef{
			HashAlgo:  caspb.HashAlgo_SHA256,
			HexDigest: goodDigest,
		})
		goodPkg, err := processing.GetClientPackage(goodPlat)
		assert.Loosely(t, err, should.BeNil)

		setup := func(res *processing.ClientExtractorResult, fail string) (*model.Instance, *model.ProcessingResult) {
			pkgName, err := processing.GetClientPackage(goodPlat)
			assert.Loosely(t, err, should.BeNil)
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
			assert.Loosely(t, datastore.Put(ctx, pkg, inst, proc), should.BeNil)
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

		t.Run("Bootstrap endpoint", func(t *ftt.Test) {
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

			t.Run("Happy path", func(t *ftt.Test) {
				rr := call(goodPlat, goodIID)
				assert.Loosely(t, rr.Code, should.Equal(http.StatusFound))
				assert.Loosely(t, rr.Header().Get("Location"), should.Equal(expectedClientURL))
				assert.Loosely(t, rr.Header().Get(cipdInstanceHeader), should.Equal(goodIID))
			})

			t.Run("No plat", func(t *ftt.Test) {
				rr := call("", goodIID)
				assert.Loosely(t, rr.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("no 'platform' specified"))
			})

			t.Run("Bad plat", func(t *ftt.Test) {
				rr := call("...", goodIID)
				assert.Loosely(t, rr.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("bad platform name"))
			})

			t.Run("No ver", func(t *ftt.Test) {
				rr := call(goodPlat, "")
				assert.Loosely(t, rr.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("no 'version' specified"))
			})

			t.Run("Bad ver", func(t *ftt.Test) {
				rr := call(goodPlat, "!!!!")
				assert.Loosely(t, rr.Code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("bad version"))
			})

			t.Run("No access", func(t *ftt.Test) {
				ctx = as("someone@example.com")
				rr := call(goodPlat, goodIID)
				assert.Loosely(t, rr.Code, should.Equal(http.StatusForbidden))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("is not allowed to see it"))
			})

			t.Run("Missing ver", func(t *ftt.Test) {
				rr := call(goodPlat, "missing")
				assert.Loosely(t, rr.Code, should.Equal(http.StatusNotFound))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("no such ref"))
			})

			t.Run("Missing instance ID", func(t *ftt.Test) {
				rr := call(goodPlat, strings.Repeat("b", 40))
				assert.Loosely(t, rr.Code, should.Equal(http.StatusNotFound))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("no such instance"))
			})

			t.Run("Not extracted yet", func(t *ftt.Test) {
				inst.ProcessorsPending = []string{proc.ProcID}
				datastore.Delete(ctx, proc)
				datastore.Put(ctx, inst)

				rr := call(goodPlat, goodIID)
				assert.Loosely(t, rr.Code, should.Equal(http.StatusNotFound))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("is not extracted yet"))
			})

			t.Run("Fatal error during extraction", func(t *ftt.Test) {
				setup(nil, "BOOM")

				rr := call(goodPlat, goodIID)
				assert.Loosely(t, rr.Code, should.Equal(http.StatusNotFound))
				assert.Loosely(t, rr.Body.String(), should.ContainSubstring("BOOM"))
			})
		})

		t.Run("DescribeClient RPC", func(t *ftt.Test) {
			call := func(pkg, sha256Digest string) (*repopb.DescribeClientResponse, error) {
				return impl.DescribeClient(ctx, &repopb.DescribeClientRequest{
					Package: pkg,
					Instance: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA256,
						HexDigest: sha256Digest,
					},
				})
			}

			t.Run("Happy path", func(t *ftt.Test) {
				resp, err := call(goodPkg, goodDigest)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(&repopb.DescribeClientResponse{
					Instance: &repopb.Instance{
						Package: goodPkg,
						Instance: &caspb.ObjectRef{
							HashAlgo:  caspb.HashAlgo_SHA256,
							HexDigest: goodDigest,
						},
					},
					ClientRef: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA256,
						HexDigest: res.ClientBinary.AllHashDigests["SHA256"],
					},
					ClientBinary: &caspb.ObjectURL{
						SignedUrl: expectedClientURL,
					},
					ClientSize: res.ClientBinary.Size,
					LegacySha1: res.ClientBinary.AllHashDigests["SHA1"],
					ClientRefAliases: []*caspb.ObjectRef{
						{
							HashAlgo:  caspb.HashAlgo_SHA1,
							HexDigest: res.ClientBinary.AllHashDigests["SHA1"],
						},
						{
							HashAlgo:  caspb.HashAlgo_SHA256,
							HexDigest: res.ClientBinary.AllHashDigests["SHA256"],
						},
					},
				}))
			})

			t.Run("Bad package name", func(t *ftt.Test) {
				_, err := call("not/a/client", goodDigest)
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("not a CIPD client package"))
			})

			t.Run("Bad instance ref", func(t *ftt.Test) {
				_, err := call(goodPkg, "not-an-id")
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("invalid SHA256 hex digest"))
			})

			t.Run("Missing instance", func(t *ftt.Test) {
				_, err := call(goodPkg, strings.Repeat("e", 64))
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such instance"))
			})

			t.Run("No access", func(t *ftt.Test) {
				ctx = as("someone@example.com")
				_, err := call(goodPkg, goodDigest)
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("not allowed to see it"))
			})

			t.Run("Not extracted yet", func(t *ftt.Test) {
				inst.ProcessorsPending = []string{proc.ProcID}
				datastore.Delete(ctx, proc)
				datastore.Put(ctx, inst)

				_, err := call(goodPkg, goodDigest)
				assert.Loosely(t, status.Code(err), should.Equal(codes.FailedPrecondition))
				assert.Loosely(t, err, should.ErrLike("the instance is not ready yet"))
			})

			t.Run("Fatal error during extraction", func(t *ftt.Test) {
				setup(nil, "BOOM")

				_, err := call(goodPkg, goodDigest)
				assert.Loosely(t, status.Code(err), should.Equal(codes.Aborted))
				assert.Loosely(t, err, should.ErrLike("some processors failed to process this instance"))
			})
		})

		t.Run("Legacy API", func(t *ftt.Test) {
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
				assert.Loosely(t, rr.Header().Get("Content-Type"), should.Equal(expCT))
				code = rr.Code
				body = strings.TrimSpace(rr.Body.String())
				return
			}

			t.Run("Happy path", func(t *ftt.Test) {
				code, body := call(goodPkg, goodIID, "json")
				assert.Loosely(t, code, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal(
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
						inst.InstanceID)))
			})

			t.Run("Bad package name", func(t *ftt.Test) {
				code, body := call("not/a/client", goodIID, "text")
				assert.Loosely(t, code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, body, should.ContainSubstring("not a CIPD client package"))
			})

			t.Run("Bad instance ID", func(t *ftt.Test) {
				code, body := call(goodPkg, "not-an-id", "text")
				assert.Loosely(t, code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, body, should.ContainSubstring("not a valid package instance ID"))
			})

			t.Run("Missing instance", func(t *ftt.Test) {
				badIID := common.ObjectRefToInstanceID(&caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA256,
					HexDigest: strings.Repeat("d", 64),
				})
				code, body := call(goodPkg, badIID, "json")
				assert.Loosely(t, code, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal(`{
  "error_message": "no such instance",
  "status": "INSTANCE_NOT_FOUND"
}`))
			})

			t.Run("No access", func(t *ftt.Test) {
				ctx = as("someone@example.com")
				code, body := call(goodPkg, goodIID, "text")
				assert.Loosely(t, code, should.Equal(http.StatusForbidden))
				assert.Loosely(t, body, should.ContainSubstring("not allowed to see it"))
			})

			t.Run("Not extracted yet", func(t *ftt.Test) {
				inst.ProcessorsPending = []string{proc.ProcID}
				datastore.Delete(ctx, proc)
				datastore.Put(ctx, inst)

				code, body := call(goodPkg, goodIID, "json")
				assert.Loosely(t, code, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal(`{
  "error_message": "the client binary is not extracted yet, try later",
  "status": "NOT_EXTRACTED_YET"
}`))
			})

			t.Run("Fatal error during extraction", func(t *ftt.Test) {
				setup(nil, "BOOM")

				code, body := call(goodPkg, goodIID, "json")
				assert.Loosely(t, code, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal(`{
  "error_message": "the client binary is not available: some processors failed to process this instance: cipd_client_binary:v1",
  "status": "ERROR"
}`))
			})
		})
	})
}

func TestLegacyHandlers(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()
		ctx = as("reader@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_READER,
					Principals: []string{"user:reader@example.com"},
				},
			},
		})

		cas := testutil.MockCAS{
			GetObjectURLImpl: func(_ context.Context, r *caspb.GetObjectURLRequest) (*caspb.ObjectURL, error) {
				return &caspb.ObjectURL{
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
		assert.Loosely(t, datastore.Put(ctx, &model.Package{Name: "a/b"}, inst1, inst2), should.BeNil)

		// Make an ambiguous tag.
		model.AttachTags(ctx, inst1, []*repopb.Tag{{Key: "k", Value: "v"}})
		model.AttachTags(ctx, inst2, []*repopb.Tag{{Key: "k", Value: "v"}})

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
			assert.Loosely(t, rr.Header().Get("Content-Type"), should.Equal(expCT))
			code = rr.Code
			body = strings.TrimSpace(rr.Body.String())
			return
		}

		t.Run("handleLegacyInstance works", func(t *ftt.Test) {
			callInstance := func(pkg, iid, ct string) (code int, body string) {
				return callHandler(adaptGrpcErr(impl.handleLegacyInstance), url.Values{
					"package_name": {pkg},
					"instance_id":  {iid},
				}, ct)
			}

			t.Run("Happy path", func(t *ftt.Test) {
				code, body := callInstance("a/b", strings.Repeat("a", 40), "json")
				assert.Loosely(t, code, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal(`{
  "fetch_url": "http://fake/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "instance": {
    "package_name": "a/b",
    "instance_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "registered_by": "user:reg@example.com",
    "registered_ts": "1454472306000000"
  },
  "status": "SUCCESS"
}`))
			})

			t.Run("Bad package", func(t *ftt.Test) {
				code, body := callInstance("///", strings.Repeat("a", 40), "plain")
				assert.Loosely(t, code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, body, should.ContainSubstring("invalid package name"))
			})

			t.Run("Bad instance ID", func(t *ftt.Test) {
				code, body := callInstance("a/b", strings.Repeat("a", 99), "plain")
				assert.Loosely(t, code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, body, should.ContainSubstring("not a valid package instance ID"))
			})

			t.Run("No access", func(t *ftt.Test) {
				code, body := callInstance("z/z/z", strings.Repeat("a", 40), "plain")
				assert.Loosely(t, code, should.Equal(http.StatusForbidden))
				assert.Loosely(t, body, should.ContainSubstring("not allowed to see"))
			})

			t.Run("Missing pkg", func(t *ftt.Test) {
				code, body := callInstance("a/z/z", strings.Repeat("a", 40), "json")
				assert.Loosely(t, code, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal(`{
  "error_message": "no such package: a/z/z",
  "status": "INSTANCE_NOT_FOUND"
}`))
			})
		})

		t.Run("handleLegacyResolve works", func(t *ftt.Test) {
			callResolve := func(pkg, ver, ct string) (code int, body string) {
				return callHandler(adaptGrpcErr(impl.handleLegacyResolve), url.Values{
					"package_name": {pkg},
					"version":      {ver},
				}, ct)
			}

			t.Run("Happy path", func(t *ftt.Test) {
				code, body := callResolve("a/b", strings.Repeat("a", 40), "json")
				assert.Loosely(t, code, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal(`{
  "instance_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "status": "SUCCESS"
}`))
			})

			t.Run("Bad request", func(t *ftt.Test) {
				code, body := callResolve("///", strings.Repeat("a", 40), "plain")
				assert.Loosely(t, code, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, body, should.ContainSubstring("invalid package name"))
			})

			t.Run("No access", func(t *ftt.Test) {
				code, body := callResolve("z/z/z", strings.Repeat("a", 40), "plain")
				assert.Loosely(t, code, should.Equal(http.StatusForbidden))
				assert.Loosely(t, body, should.ContainSubstring("not allowed to see"))
			})

			t.Run("Missing pkg", func(t *ftt.Test) {
				code, body := callResolve("a/z/z", strings.Repeat("a", 40), "json")
				assert.Loosely(t, code, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal(`{
  "error_message": "no such package: a/z/z",
  "status": "INSTANCE_NOT_FOUND"
}`))
			})

			t.Run("Ambiguous version", func(t *ftt.Test) {
				code, body := callResolve("a/b", "k:v", "json")
				assert.Loosely(t, code, should.Equal(http.StatusOK))
				assert.Loosely(t, body, should.Equal(`{
  "error_message": "ambiguity when resolving the tag, more than one instance has it",
  "status": "AMBIGUOUS_VERSION"
}`))
			})
		})
	})
}

func TestParseDownloadPath(t *testing.T) {
	t.Parallel()

	ftt.Run("OK", t, func(t *ftt.Test) {
		pkg, ver, err := parseDownloadPath("/a/b/c/+/latest")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pkg, should.Equal("a/b/c"))
		assert.Loosely(t, ver, should.Equal("latest"))

		pkg, ver, err = parseDownloadPath("/a/b/c/+/repo:https://abc")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pkg, should.Equal("a/b/c"))
		assert.Loosely(t, ver, should.Equal("repo:https://abc"))

		pkg, ver, err = parseDownloadPath("/a/b/c/+/" + strings.Repeat("a", 40))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pkg, should.Equal("a/b/c"))
		assert.Loosely(t, ver, should.Equal(strings.Repeat("a", 40)))
	})

	ftt.Run("Bad chunks", t, func(t *ftt.Test) {
		_, _, err := parseDownloadPath("/+/latest")
		assert.Loosely(t, err, should.ErrLike(`should have form`))

		_, _, err = parseDownloadPath("latest")
		assert.Loosely(t, err, should.ErrLike(`should have form`))

		_, _, err = parseDownloadPath("/a/b/c")
		assert.Loosely(t, err, should.ErrLike(`should have form`))
	})

	ftt.Run("Bad package", t, func(t *ftt.Test) {
		_, _, err := parseDownloadPath("BAD/+/latest")
		assert.Loosely(t, err, should.ErrLike(`invalid package name`))

		_, _, err = parseDownloadPath("/a//b/+/latest")
		assert.Loosely(t, err, should.ErrLike(`invalid package name`))

		_, _, err = parseDownloadPath("//+/latest")
		assert.Loosely(t, err, should.ErrLike(`invalid package name`))
	})

	ftt.Run("Bad version", t, func(t *ftt.Test) {
		_, _, err := parseDownloadPath("a/+/")
		assert.Loosely(t, err, should.ErrLike(`bad version`))

		_, _, err = parseDownloadPath("a/+/!!!!")
		assert.Loosely(t, err, should.ErrLike(`bad version`))
	})
}

type MockVSA struct {
	VSA    func() string
	status vsa.CacheStatus
}

func (m *MockVSA) Register(f *flag.FlagSet)       {}
func (m *MockVSA) Init(ctx context.Context) error { return nil }
func (m *MockVSA) VerifySoftwareArtifact(ctx context.Context, inst *model.Instance, bundle string) (vsa string) {
	return m.VSA()
}
func (m *MockVSA) NewVerifySoftwareArtifactTask(ctx context.Context, inst *model.Instance, bundle string) *tasks.CallVerifySoftwareArtifact {
	return &tasks.CallVerifySoftwareArtifact{
		Instance: inst.Proto(),
		Request: &vsapb.VerifySoftwareArtifactRequest{
			ArtifactInfo: &vsapb.ArtifactInfo{
				Attestations: []string{bundle},
			},
		},
	}
}
func (m *MockVSA) CallVerifySoftwareArtifact(ctx context.Context, t *tasks.CallVerifySoftwareArtifact) string {
	return m.VSA()
}
func (m *MockVSA) GetStatus(ctx context.Context, inst *model.Instance) (vsa.CacheStatus, error) {
	if m.status == "" {
		return vsa.CacheStatusUnknown, nil
	}
	return m.status, nil
}
func (m *MockVSA) SetStatus(ctx context.Context, inst *model.Instance, status vsa.CacheStatus) error {
	m.status = status
	return nil
}

func TestVSA(t *testing.T) {
	t.Parallel()

	ftt.Run("With fakes", t, func(t *ftt.Test) {
		ctx, _, as := testutil.TestingContext()
		ctx = as("owner@example.com")

		meta := testutil.MetadataStore{}
		meta.Populate("a", &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"user:owner@example.com"},
				},
			},
		})

		dispatcher := &tq.Dispatcher{}
		ctx, sched := tq.TestingContext(ctx, dispatcher)
		runTasks := func() {
			for _, t := range sched.Tasks() {
				sched.Executor.Execute(ctx, t, func(retry bool) {})
			}
		}

		cas := testutil.MockCAS{}
		mvsa := &MockVSA{VSA: func() string { return "vsa content" }, status: vsa.CacheStatusUnknown}

		impl := repoImpl{tq: dispatcher, meta: &meta, cas: &cas, vsa: mvsa}
		impl.registerTasks()

		inst := &model.Instance{
			InstanceID:   strings.Repeat("1", 40),
			Package:      model.PackageKey(ctx, "a/pkg"),
			RegisteredBy: "user:1@example.com",
		}

		assert.Loosely(t, datastore.Put(ctx, &model.Package{Name: "a/pkg"}, inst), should.BeNil)

		cas.GetObjectURLImpl = func(_ context.Context, r *caspb.GetObjectURLRequest) (*caspb.ObjectURL, error) {
			assert.Loosely(t, r.Object.HashAlgo, should.Equal(caspb.HashAlgo_SHA1))
			assert.Loosely(t, r.Object.HexDigest, should.Equal(inst.InstanceID))
			return &caspb.ObjectURL{
				SignedUrl: fmt.Sprintf("http://example.com/%s?d=%s", r.Object.HexDigest, r.DownloadFilename),
			}, nil
		}

		t.Run("AttachMetadata", func(t *ftt.Test) {
			t.Run("Not Ready", func(t *ftt.Test) {
				mvsa.VSA = func() string { t.Fail(); return "" }

				_, err := impl.AttachMetadata(ctx, &repopb.AttachMetadataRequest{
					Package:  "a/pkg/somethingelse",
					Instance: inst.Proto().Instance,
					Metadata: []*repopb.InstanceMetadata{
						{Key: "policy-attestations", Value: []uint8("attestation bundle"), ContentType: "application/vnd.in-toto.bundle"},
					},
				})
				assert.ErrIsLike(t, err, "no such package")
			})

			t.Run("OK", func(t *ftt.Test) {
				_, err := impl.AttachMetadata(ctx, &repopb.AttachMetadataRequest{
					Package:  inst.Package.StringID(),
					Instance: inst.Proto().Instance,
					Metadata: []*repopb.InstanceMetadata{
						{Key: "policy-attestations", Value: []uint8("attestation bundle"), ContentType: "application/vnd.in-toto.bundle"},
					},
				})
				assert.NoErr(t, err)

				m, err := model.ListMetadata(ctx, inst)
				assert.NoErr(t, err)
				assert.Loosely(t, m, should.HaveLength(2))
				assert.Loosely(t, m[0].Key, should.Equal(slsaVSAKey))
				assert.Loosely(t, m[0].Value, should.Match([]uint8("vsa content")))
				assert.Loosely(t, m[1].Key, should.Equal("policy-attestations"))
				assert.Loosely(t, m[1].Value, should.Match([]uint8("attestation bundle")))
				assert.Loosely(t, m[1].ContentType, should.Equal("application/vnd.in-toto.bundle"))

				// Should not be called again when package is requested.
				mvsa.VSA = func() string { t.Fail(); return "" }
				_, err = impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
					Package:  inst.Package.StringID(),
					Instance: inst.Proto().Instance,
				})
				runTasks()
				assert.NoErr(t, err)
			})

			t.Run("Failed", func(t *ftt.Test) {
				mvsa.VSA = func() string { return "" }

				_, err := impl.AttachMetadata(ctx, &repopb.AttachMetadataRequest{
					Package:  inst.Package.StringID(),
					Instance: inst.Proto().Instance,
					Metadata: []*repopb.InstanceMetadata{
						{Key: "policy-attestations", Value: []uint8("attestation bundle"), ContentType: "application/vnd.in-toto.bundle"},
					},
				})
				assert.NoErr(t, err)

				m, err := model.ListMetadata(ctx, inst)
				assert.NoErr(t, err)
				assert.Loosely(t, m, should.HaveLength(1))
				assert.Loosely(t, m[0].Key, should.Equal("policy-attestations"))
				assert.Loosely(t, m[0].Value, should.Match([]uint8("attestation bundle")))
				assert.Loosely(t, m[0].ContentType, should.Equal("application/vnd.in-toto.bundle"))
			})
		})

		t.Run("GetInstanceURL", func(t *ftt.Test) {
			t.Run("Without VSA", func(t *ftt.Test) {
				t.Run("With Attestation", func(t *ftt.Test) {
					assert.Loosely(t, mvsa.status, should.Equal(vsa.CacheStatusUnknown))
					model.AttachMetadata(ctx, inst, []*repopb.InstanceMetadata{{
						Key:         "policy-attestations",
						Value:       []uint8("attestation bundle"),
						ContentType: "application/vnd.in-toto.bundle"},
					})

					_, err := impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
						Package:  inst.Package.StringID(),
						Instance: inst.Proto().Instance,
					})
					assert.NoErr(t, err)
					assert.Loosely(t, mvsa.status, should.Equal(vsa.CacheStatusPending))
					assert.Loosely(t, sched.Tasks(), should.HaveLength(1))
					assert.Loosely(t, sched.Tasks()[0].Payload, should.Match(&tasks.CallVerifySoftwareArtifact{
						Instance: inst.Proto(),
						Request: &vsapb.VerifySoftwareArtifactRequest{
							ArtifactInfo: &vsapb.ArtifactInfo{
								Attestations: []string{"attestation bundle"},
							},
						},
					}))
					runTasks()

					m, err := model.ListMetadata(ctx, inst)
					assert.NoErr(t, err)
					assert.Loosely(t, m, should.HaveLength(2))
					assert.Loosely(t, m[0].Key, should.Equal(slsaVSAKey))
					assert.Loosely(t, m[0].Value, should.Match([]uint8("vsa content")))
					assert.Loosely(t, mvsa.status, should.Equal(vsa.CacheStatusCompleted))
				})

				t.Run("Without Attestation", func(t *ftt.Test) {
					assert.Loosely(t, mvsa.status, should.Equal(vsa.CacheStatusUnknown))
					_, err := impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
						Package:  inst.Package.StringID(),
						Instance: inst.Proto().Instance,
					})
					assert.NoErr(t, err)
					assert.Loosely(t, mvsa.status, should.Equal(vsa.CacheStatusPending))

					assert.Loosely(t, sched.Tasks(), should.HaveLength(1))
					assert.Loosely(t, sched.Tasks()[0].Payload, should.Match(&tasks.CallVerifySoftwareArtifact{
						Instance: inst.Proto(),
						Request: &vsapb.VerifySoftwareArtifactRequest{
							ArtifactInfo: &vsapb.ArtifactInfo{
								Attestations: []string{""},
							},
						},
					}))
					runTasks()

					m, err := model.ListMetadata(ctx, inst)
					assert.NoErr(t, err)
					assert.Loosely(t, m, should.HaveLength(1))
					assert.Loosely(t, m[0].Key, should.Equal(slsaVSAKey))
					assert.Loosely(t, m[0].Value, should.Match([]uint8("vsa content")))
					assert.Loosely(t, mvsa.status, should.Equal(vsa.CacheStatusCompleted))
				})
			})

			t.Run("With VSA", func(t *ftt.Test) {
				mvsa.VSA = func() string { t.Fail(); return "" }
				err := model.AttachMetadata(ctx, inst, []*repopb.InstanceMetadata{
					{Key: slsaVSAKey, Value: []uint8("something")},
				})
				assert.NoErr(t, err)

				resp, err := impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
					Package:  inst.Package.StringID(),
					Instance: inst.Proto().Instance,
				})
				assert.NoErr(t, err)
				assert.Loosely(t, resp, should.Match(&caspb.ObjectURL{
					SignedUrl: "http://example.com/1111111111111111111111111111111111111111?d=",
				}))

				m, err := model.ListMetadata(ctx, inst)
				assert.NoErr(t, err)
				assert.Loosely(t, m, should.HaveLength(1))
				assert.Loosely(t, m[0].Key, should.Equal(slsaVSAKey))
				assert.Loosely(t, m[0].Value, should.Match([]uint8("something")))
				assert.Loosely(t, mvsa.status, should.Equal(vsa.CacheStatusCompleted))
			})

			t.Run("With Cached Status", func(t *ftt.Test) {
				mvsa.VSA = func() string { t.Fail(); return "" }
				mvsa.status = vsa.CacheStatusPending

				_, err := impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
					Package:  inst.Package.StringID(),
					Instance: inst.Proto().Instance,
				})
				assert.NoErr(t, err)
				assert.Loosely(t, sched.Tasks(), should.HaveLength(0))

				mvsa.status = vsa.CacheStatusCompleted
				_, err = impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
					Package:  inst.Package.StringID(),
					Instance: inst.Proto().Instance,
				})
				assert.NoErr(t, err)
				assert.Loosely(t, sched.Tasks(), should.HaveLength(0))

				mvsa.status = vsa.CacheStatusUnknown
				_, err = impl.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
					Package:  inst.Package.StringID(),
					Instance: inst.Proto().Instance,
				})
				assert.NoErr(t, err)
				assert.Loosely(t, sched.Tasks(), should.HaveLength(1))
			})
		})
	})
}
