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

package cipd

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/builder"
	"go.chromium.org/luci/cipd/client/cipd/digests"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/platform"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"
)

////////////////////////////////////////////////////////////////////////////////
// ACL related calls.

func TestFetchACL(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		c.Run("Works", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "GetInheritedPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				out: &api.InheritedPrefixMetadata{
					PerPrefixMetadata: []*api.PrefixMetadata{
						{
							Prefix: "a",
							Acls: []*api.PrefixMetadata_ACL{
								{Role: api.Role_READER, Principals: []string{"group:a"}},
							},
						},
					},
				},
			})

			acl, err := client.FetchACL(ctx, "a/b/c")
			assert.Loosely(c, err, should.BeNil)

			assert.Loosely(c, acl, should.Resemble([]PackageACL{
				{
					PackagePath: "a",
					Role:        "READER",
					Principals:  []string{"group:a"},
				},
			}))
		})

		c.Run("Bad prefix", func(c *ftt.Test) {
			_, err := client.FetchACL(ctx, "a/b////")
			assert.Loosely(c, err, should.ErrLike("invalid package prefix"))
		})

		c.Run("Error response", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "GetInheritedPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.FetchACL(ctx, "a/b/c")
			assert.Loosely(c, err, should.ErrLike("blah error"))
		})
	})
}

func TestModifyACL(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		c.Run("Modifies existing", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "GetPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a"},
				out: &api.PrefixMetadata{
					Prefix: "a",
					Acls: []*api.PrefixMetadata_ACL{
						{Role: api.Role_READER, Principals: []string{"group:a"}},
					},
					Fingerprint: "abc",
				},
			})
			repo.expect(rpcCall{
				method: "UpdatePrefixMetadata",
				in: &api.PrefixMetadata{
					Prefix:      "a",
					Fingerprint: "abc",
				},
				out: &api.PrefixMetadata{}, // ignored
			})

			assert.Loosely(c, client.ModifyACL(ctx, "a", []PackageACLChange{
				{Action: RevokeRole, Role: "READER", Principal: "group:a"},
			}), should.BeNil)
		})

		c.Run("Creates new", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "GetPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a"},
				err:    status.Errorf(codes.NotFound, "no metadata"),
			})
			repo.expect(rpcCall{
				method: "UpdatePrefixMetadata",
				in: &api.PrefixMetadata{
					Prefix: "a",
					Acls: []*api.PrefixMetadata_ACL{
						{Role: api.Role_READER, Principals: []string{"group:a"}},
					},
				},
				out: &api.PrefixMetadata{}, // ignored
			})

			assert.Loosely(c, client.ModifyACL(ctx, "a", []PackageACLChange{
				{Action: GrantRole, Role: "READER", Principal: "group:a"},
			}), should.BeNil)
		})

		c.Run("Noop update", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "GetPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a"},
				out: &api.PrefixMetadata{
					Prefix: "a",
					Acls: []*api.PrefixMetadata_ACL{
						{Role: api.Role_READER, Principals: []string{"group:a"}},
					},
					Fingerprint: "abc",
				},
			})

			assert.Loosely(c, client.ModifyACL(ctx, "a", []PackageACLChange{
				{Action: GrantRole, Role: "READER", Principal: "group:a"},
			}), should.BeNil)
		})

		someChanges := []PackageACLChange{
			{Action: RevokeRole, Role: "READER", Principal: "group:a"},
		}

		c.Run("Bad prefix", func(c *ftt.Test) {
			assert.Loosely(c, client.ModifyACL(ctx, "a/b////", someChanges), should.ErrLike("invalid package prefix"))
		})

		c.Run("Error response", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "GetPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			assert.Loosely(c, client.ModifyACL(ctx, "a/b/c", someChanges), should.ErrLike("blah error"))
		})
	})
}

func TestFetchRoles(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		c.Run("Works", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "GetRolesInPrefix",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				out: &api.RolesInPrefixResponse{
					Roles: []*api.RolesInPrefixResponse_RoleInPrefix{
						{Role: api.Role_OWNER},
						{Role: api.Role_WRITER},
						{Role: api.Role_READER},
					},
				},
			})

			roles, err := client.FetchRoles(ctx, "a/b/c")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, roles, should.Resemble([]string{"OWNER", "WRITER", "READER"}))
		})

		c.Run("Bad prefix", func(c *ftt.Test) {
			_, err := client.FetchRoles(ctx, "a/b////")
			assert.Loosely(c, err, should.ErrLike("invalid package prefix"))
		})

		c.Run("Error response", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "GetRolesInPrefix",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.FetchRoles(ctx, "a/b/c")
			assert.Loosely(c, err, should.ErrLike("blah error"))
		})
	})
}

func TestFetchRolesOnBehalfOf(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		c.Run("Works", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "GetRolesInPrefixOnBehalfOf",
				in: &api.PrefixRequestOnBehalfOf{
					PrefixRequest: &api.PrefixRequest{Prefix: "a/b/c"},
					Identity:      "anonymous:anonymous",
				},
				out: &api.RolesInPrefixResponse{
					Roles: []*api.RolesInPrefixResponse_RoleInPrefix{
						{Role: api.Role_OWNER},
						{Role: api.Role_WRITER},
						{Role: api.Role_READER},
					},
				},
			})
			id := identity.Identity("anonymous:anonymous")
			roles, err := client.FetchRolesOnBehalfOf(ctx, "a/b/c", id)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, roles, should.Resemble([]string{"OWNER", "WRITER", "READER"}))
		})

		c.Run("Bad prefix", func(c *ftt.Test) {
			id := identity.Identity("anonymous:anonymous")
			_, err := client.FetchRolesOnBehalfOf(ctx, "a/b////", id)
			assert.Loosely(c, err, should.ErrLike("invalid package prefix"))
		})

		c.Run("Bad id", func(c *ftt.Test) {
			id := identity.Identity("chicken")
			_, err := client.FetchRolesOnBehalfOf(ctx, "a/b/c", id)
			assert.Loosely(c, err, should.ErrLike("auth: bad identity string \"chicken\""))
		})

		c.Run("Error response", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "GetRolesInPrefixOnBehalfOf",
				in: &api.PrefixRequestOnBehalfOf{
					PrefixRequest: &api.PrefixRequest{Prefix: "a/b/c"},
					Identity:      "anonymous:anonymous",
				},
				err: status.Errorf(codes.PermissionDenied, "blah error"),
			})
			id := identity.Identity("anonymous:anonymous")
			_, err := client.FetchRolesOnBehalfOf(ctx, "a/b/c", id)
			assert.Loosely(c, err, should.ErrLike("blah error"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Instance upload and registration.

func TestRegisterInstance(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			if testclock.HasTags(t, "cipd-sleeping") {
				tc.Add(d)
			}
		})

		client, cas, repo, storage := mockedCipdClient(c)
		inst := fakeInstance(t, "pkg/inst")

		registerInstanceRPC := func(s api.RegistrationStatus, op *api.UploadOperation) rpcCall {
			return rpcCall{
				method: "RegisterInstance",
				in: &api.Instance{
					Package:  "pkg/inst",
					Instance: common.InstanceIDToObjectRef(inst.Pin().InstanceID),
				},
				out: &api.RegisterInstanceResponse{
					Status: s,
					Instance: &api.Instance{
						Package:  "pkg/inst",
						Instance: common.InstanceIDToObjectRef(inst.Pin().InstanceID),
					},
					UploadOp: op,
				},
			}
		}

		finishUploadRPC := func(opID string, out *api.UploadOperation) rpcCall {
			return rpcCall{
				method: "FinishUpload",
				in:     &api.FinishUploadRequest{UploadOperationId: opID},
				out:    out,
			}
		}

		op := api.UploadOperation{
			OperationId: "zzz",
			UploadUrl:   "http://example.com/zzz_op",
		}

		c.Run("Happy path", func(c *ftt.Test) {
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			cas.expect(finishUploadRPC(op.OperationId, &api.UploadOperation{
				Status: api.UploadStatus_VERIFYING,
			}))
			cas.expect(finishUploadRPC(op.OperationId, &api.UploadOperation{
				Status: api.UploadStatus_PUBLISHED,
			}))
			repo.expect(registerInstanceRPC(api.RegistrationStatus_REGISTERED, nil))

			assert.Loosely(c, client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), should.BeNil)
			assert.Loosely(c, storage.getStored(op.UploadUrl), should.NotEqual(""))
		})

		c.Run("Already registered", func(c *ftt.Test) {
			repo.expect(registerInstanceRPC(api.RegistrationStatus_ALREADY_REGISTERED, nil))
			assert.Loosely(c, client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), should.BeNil)
		})

		c.Run("Registration error", func(c *ftt.Test) {
			rpc := registerInstanceRPC(api.RegistrationStatus_ALREADY_REGISTERED, nil)
			rpc.err = status.Errorf(codes.PermissionDenied, "denied blah")
			repo.expect(rpc)
			assert.Loosely(c, client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), should.ErrLike("denied blah"))
		})

		c.Run("Upload error", func(c *ftt.Test) {
			storage.returnErr(fmt.Errorf("upload err blah"))
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			assert.Loosely(c, client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), should.ErrLike("upload err blah"))
		})

		c.Run("Verification error", func(c *ftt.Test) {
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			cas.expect(finishUploadRPC(op.OperationId, &api.UploadOperation{
				Status:       api.UploadStatus_ERRORED,
				ErrorMessage: "baaaaad",
			}))
			assert.Loosely(c, client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), should.ErrLike("baaaaad"))
		})

		c.Run("Confused backend", func(c *ftt.Test) {
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			cas.expect(finishUploadRPC(op.OperationId, &api.UploadOperation{
				Status: api.UploadStatus_PUBLISHED,
			}))
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			assert.Loosely(c, client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), should.ErrLike("servers asks us to upload it again"))
		})

		c.Run("Verification timeout", func(c *ftt.Test) {
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			cas.expectMany(finishUploadRPC(op.OperationId, &api.UploadOperation{
				Status: api.UploadStatus_VERIFYING,
			}))
			assert.Loosely(c, client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), should.ErrLike("timeout while waiting"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Setting refs, attaching tags and metadata.

func TestAttachingStuffWhenReady(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			if testclock.HasTags(t, "cipd-sleeping") {
				tc.Add(d)
			}
		})

		client, _, repo, _ := mockedCipdClient(c)

		objRef := &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: strings.Repeat("a", 64),
		}
		pin := common.Pin{
			PackageName: "pkg/name",
			InstanceID:  common.ObjectRefToInstanceID(objRef),
		}

		createRefRPC := func() rpcCall {
			return rpcCall{
				method: "CreateRef",
				in: &api.Ref{
					Name:     "zzz",
					Package:  "pkg/name",
					Instance: objRef,
				},
				out: &emptypb.Empty{},
			}
		}

		attachTagsRPC := func() rpcCall {
			return rpcCall{
				method: "AttachTags",
				in: &api.AttachTagsRequest{
					Package:  "pkg/name",
					Instance: objRef,
					Tags: []*api.Tag{
						{Key: "k1", Value: "v1"},
						{Key: "k2", Value: "v2"},
					},
				},
				out: &emptypb.Empty{},
			}
		}

		attachMetadataRPC := func() rpcCall {
			return rpcCall{
				method: "AttachMetadata",
				in: &api.AttachMetadataRequest{
					Package:  "pkg/name",
					Instance: objRef,
					Metadata: []*api.InstanceMetadata{
						{Key: "k1", Value: []byte("v1"), ContentType: "text/1"},
						{Key: "k2", Value: []byte("v2"), ContentType: "text/2"},
					},
				},
				out: &emptypb.Empty{},
			}
		}

		cannedMD := []Metadata{
			{Key: "k1", Value: []byte("v1"), ContentType: "text/1"},
			{Key: "k2", Value: []byte("v2"), ContentType: "text/2"},
		}

		c.Run("SetRefWhenReady happy path", func(c *ftt.Test) {
			repo.expect(createRefRPC())
			assert.Loosely(c, client.SetRefWhenReady(ctx, "zzz", pin), should.BeNil)
		})

		c.Run("AttachTagsWhenReady happy path", func(c *ftt.Test) {
			repo.expect(attachTagsRPC())
			assert.Loosely(c, client.AttachTagsWhenReady(ctx, pin, []string{"k1:v1", "k2:v2"}), should.BeNil)
		})

		c.Run("AttachMetadataWhenReady happy path", func(c *ftt.Test) {
			repo.expect(attachMetadataRPC())
			assert.Loosely(c, client.AttachMetadataWhenReady(ctx, pin, cannedMD), should.BeNil)
		})

		c.Run("SetRefWhenReady timeout", func(c *ftt.Test) {
			rpc := createRefRPC()
			rpc.err = status.Errorf(codes.FailedPrecondition, "not ready")
			repo.expectMany(rpc)
			assert.Loosely(c, client.SetRefWhenReady(ctx, "zzz", pin), should.ErrLike("timeout"))
		})

		c.Run("AttachTagsWhenReady timeout", func(c *ftt.Test) {
			rpc := attachTagsRPC()
			rpc.err = status.Errorf(codes.FailedPrecondition, "not ready")
			repo.expectMany(rpc)
			assert.Loosely(c, client.AttachTagsWhenReady(ctx, pin, []string{"k1:v1", "k2:v2"}), should.ErrLike("timeout"))
		})

		c.Run("AttachMetadataWhenReady timeout", func(c *ftt.Test) {
			rpc := attachMetadataRPC()
			rpc.err = status.Errorf(codes.FailedPrecondition, "not ready")
			repo.expectMany(rpc)
			assert.Loosely(c, client.AttachMetadataWhenReady(ctx, pin, cannedMD), should.ErrLike("timeout"))
		})

		c.Run("SetRefWhenReady fatal RPC err", func(c *ftt.Test) {
			rpc := createRefRPC()
			rpc.err = status.Errorf(codes.PermissionDenied, "boo")
			repo.expect(rpc)
			assert.Loosely(c, client.SetRefWhenReady(ctx, "zzz", pin), should.ErrLike("boo"))
		})

		c.Run("AttachTagsWhenReady fatal RPC err", func(c *ftt.Test) {
			rpc := attachTagsRPC()
			rpc.err = status.Errorf(codes.PermissionDenied, "boo")
			repo.expect(rpc)
			assert.Loosely(c, client.AttachTagsWhenReady(ctx, pin, []string{"k1:v1", "k2:v2"}), should.ErrLike("boo"))
		})

		c.Run("AttachMetadataWhenReady fatal RPC err", func(c *ftt.Test) {
			rpc := attachMetadataRPC()
			rpc.err = status.Errorf(codes.PermissionDenied, "boo")
			repo.expect(rpc)
			assert.Loosely(c, client.AttachMetadataWhenReady(ctx, pin, cannedMD), should.ErrLike("boo"))
		})

		c.Run("SetRefWhenReady bad pin", func(c *ftt.Test) {
			assert.Loosely(c, client.SetRefWhenReady(ctx, "zzz", common.Pin{
				PackageName: "////",
			}), should.ErrLike("invalid package name"))
		})

		c.Run("SetRefWhenReady bad ref", func(c *ftt.Test) {
			assert.Loosely(c, client.SetRefWhenReady(ctx, "????", pin), should.ErrLike("invalid ref name"))
		})

		c.Run("AttachTagsWhenReady noop", func(c *ftt.Test) {
			assert.Loosely(c, client.AttachTagsWhenReady(ctx, pin, nil), should.BeNil)
		})

		c.Run("AttachTagsWhenReady bad pin", func(c *ftt.Test) {
			assert.Loosely(c, client.AttachTagsWhenReady(ctx, common.Pin{
				PackageName: "////",
			}, []string{"k:v"}), should.ErrLike("invalid package name"))
		})

		c.Run("AttachTagsWhenReady bad tag", func(c *ftt.Test) {
			assert.Loosely(c, client.AttachTagsWhenReady(ctx, pin, []string{"good:tag", "bad_tag"}), should.ErrLike(
				"doesn't look like a tag"))
		})

		c.Run("AttachMetadataWhenReady noop", func(c *ftt.Test) {
			assert.Loosely(c, client.AttachMetadataWhenReady(ctx, pin, nil), should.BeNil)
		})

		c.Run("AttachMetadataWhenReady bad pin", func(c *ftt.Test) {
			assert.Loosely(c, client.AttachMetadataWhenReady(ctx, common.Pin{
				PackageName: "////",
			}, cannedMD), should.ErrLike("invalid package name"))
		})

		c.Run("AttachMetadataWhenReady bad key", func(c *ftt.Test) {
			assert.Loosely(c, client.AttachMetadataWhenReady(ctx, pin, []Metadata{
				{Key: "ZZZ", Value: nil},
			}), should.ErrLike("invalid metadata key"))
		})

		c.Run("AttachMetadataWhenReady bad value", func(c *ftt.Test) {
			assert.Loosely(c, client.AttachMetadataWhenReady(ctx, pin, []Metadata{
				{Key: "k", Value: bytes.Repeat([]byte{0}, 512*1024+1)},
			}), should.ErrLike("the metadata value is too long"))
		})

		c.Run("AttachMetadataWhenReady bad content type", func(c *ftt.Test) {
			assert.Loosely(c, client.AttachMetadataWhenReady(ctx, pin, []Metadata{
				{Key: "k", ContentType: "zzz zzz"},
			}), should.ErrLike("bad content type"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Fetching info about packages and instances.

func TestListPackages(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		c.Run("Works", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "ListPrefix",
				in: &api.ListPrefixRequest{
					Prefix:        "a/b/c",
					Recursive:     true,
					IncludeHidden: true,
				},
				out: &api.ListPrefixResponse{
					Packages: []string{"a/b/c/d/pkg1", "a/b/c/d/pkg2"},
					Prefixes: []string{"a/b/c/d", "a/b/c/e", "a/b/c/e/f"},
				},
			})

			out, err := client.ListPackages(ctx, "a/b/c", true, true)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, out, should.Resemble([]string{
				"a/b/c/d/",
				"a/b/c/d/pkg1",
				"a/b/c/d/pkg2",
				"a/b/c/e/",
				"a/b/c/e/f/",
			}))
		})

		c.Run("Bad prefix", func(c *ftt.Test) {
			_, err := client.ListPackages(ctx, "a/b////", true, true)
			assert.Loosely(c, err, should.ErrLike("invalid package prefix"))
		})

		c.Run("Error response", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "ListPrefix",
				in:     &api.ListPrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.ListPackages(ctx, "a/b/c", false, false)
			assert.Loosely(c, err, should.ErrLike("blah error"))
		})
	})
}

func TestSearchInstances(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		searchInstanceRPC := func() rpcCall {
			return rpcCall{
				method: "SearchInstances",
				in: &api.SearchInstancesRequest{
					Package: "a/b",
					Tags: []*api.Tag{
						{Key: "k1", Value: "v1"},
						{Key: "k2", Value: "v2"},
					},
					PageSize: 1000,
				},
				out: &api.SearchInstancesResponse{
					Instances: []*api.Instance{
						{
							Package:  "a/b",
							Instance: fakeObjectRef("0"),
						},
						{
							Package:  "a/b",
							Instance: fakeObjectRef("1"),
						},
					},
					NextPageToken: "blah", // ignored for now
				},
			}
		}

		c.Run("Works", func(c *ftt.Test) {
			repo.expect(searchInstanceRPC())

			out, err := client.SearchInstances(ctx, "a/b", []string{"k1:v1", "k2:v2"})
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, out, should.Resemble(common.PinSlice{
				{PackageName: "a/b", InstanceID: fakeIID("0")},
				{PackageName: "a/b", InstanceID: fakeIID("1")},
			}))
		})

		c.Run("Bad package name", func(c *ftt.Test) {
			_, err := client.SearchInstances(ctx, "a/b////", []string{"k:v"})
			assert.Loosely(c, err, should.ErrLike("invalid package name"))
		})

		c.Run("No tags", func(c *ftt.Test) {
			_, err := client.SearchInstances(ctx, "a/b", nil)
			assert.Loosely(c, err, should.ErrLike("at least one tag is required"))
		})

		c.Run("Bad tag", func(c *ftt.Test) {
			_, err := client.SearchInstances(ctx, "a/b", []string{"bad_tag"})
			assert.Loosely(c, err, should.ErrLike("doesn't look like a tag"))
		})

		c.Run("Error response", func(c *ftt.Test) {
			rpc := searchInstanceRPC()
			rpc.err = status.Errorf(codes.PermissionDenied, "blah error")
			repo.expect(rpc)

			_, err := client.SearchInstances(ctx, "a/b", []string{"k1:v1", "k2:v2"})
			assert.Loosely(c, err, should.ErrLike("blah error"))
		})

		c.Run("No package", func(c *ftt.Test) {
			rpc := searchInstanceRPC()
			rpc.err = status.Errorf(codes.NotFound, "no such package")
			repo.expect(rpc)

			out, err := client.SearchInstances(ctx, "a/b", []string{"k1:v1", "k2:v2"})
			assert.Loosely(c, out, should.HaveLength(0))
			assert.Loosely(c, err, should.BeNil)
		})
	})
}

func TestListInstances(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		fakeAPIInst := func(id string) *api.Instance {
			return &api.Instance{
				Package:  "a/b",
				Instance: fakeObjectRef(id),
			}
		}

		fakeInstInfo := func(id string) InstanceInfo {
			return InstanceInfo{
				Pin: common.Pin{
					PackageName: "a/b",
					InstanceID:  fakeIID(id),
				},
			}
		}

		c.Run("Works", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "ListInstances",
				in: &api.ListInstancesRequest{
					Package:  "a/b",
					PageSize: 2,
				},
				out: &api.ListInstancesResponse{
					Instances:     []*api.Instance{fakeAPIInst("0"), fakeAPIInst("1")},
					NextPageToken: "page_tok",
				},
			})
			repo.expect(rpcCall{
				method: "ListInstances",
				in: &api.ListInstancesRequest{
					Package:   "a/b",
					PageSize:  2,
					PageToken: "page_tok",
				},
				out: &api.ListInstancesResponse{
					Instances: []*api.Instance{fakeAPIInst("2")},
				},
			})

			enum, err := client.ListInstances(ctx, "a/b")
			assert.Loosely(c, err, should.BeNil)

			all := []InstanceInfo{}
			for {
				batch, err := enum.Next(ctx, 2)
				assert.Loosely(c, err, should.BeNil)
				if len(batch) == 0 {
					break
				}
				all = append(all, batch...)
			}

			assert.Loosely(c, all, should.Resemble([]InstanceInfo{
				fakeInstInfo("0"),
				fakeInstInfo("1"),
				fakeInstInfo("2"),
			}))
		})

		c.Run("Bad package name", func(c *ftt.Test) {
			_, err := client.ListInstances(ctx, "a/b////")
			assert.Loosely(c, err, should.ErrLike("invalid package name"))
		})

		c.Run("Error response", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "ListInstances",
				in: &api.ListInstancesRequest{
					Package:  "a/b",
					PageSize: 100,
				},
				err: status.Errorf(codes.PermissionDenied, "blah error"),
			})
			enum, err := client.ListInstances(ctx, "a/b")
			assert.Loosely(c, err, should.BeNil)
			_, err = enum.Next(ctx, 100)
			assert.Loosely(c, err, should.ErrLike("blah error"))
		})
	})
}

func TestFetchPackageRefs(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		c.Run("Works", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "ListRefs",
				in:     &api.ListRefsRequest{Package: "a/b"},
				out: &api.ListRefsResponse{
					Refs: []*api.Ref{
						{
							Name:     "r1",
							Instance: fakeObjectRef("0"),
						},
						{
							Name:     "r2",
							Instance: fakeObjectRef("1"),
						},
					},
				},
			})

			out, err := client.FetchPackageRefs(ctx, "a/b")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, out, should.Resemble([]RefInfo{
				{Ref: "r1", InstanceID: fakeIID("0")},
				{Ref: "r2", InstanceID: fakeIID("1")},
			}))
		})

		c.Run("Bad package name", func(c *ftt.Test) {
			_, err := client.FetchPackageRefs(ctx, "a/b////")
			assert.Loosely(c, err, should.ErrLike("invalid package name"))
		})

		c.Run("Error response", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "ListRefs",
				in:     &api.ListRefsRequest{Package: "a/b"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.FetchPackageRefs(ctx, "a/b")
			assert.Loosely(c, err, should.ErrLike("blah error"))
		})
	})
}

func TestDescribeInstance(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		pin := common.Pin{
			PackageName: "a/b",
			InstanceID:  fakeIID("0"),
		}

		c.Run("Works", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "DescribeInstance",
				in: &api.DescribeInstanceRequest{
					Package:      "a/b",
					Instance:     fakeObjectRef("0"),
					DescribeRefs: true,
					DescribeTags: true,
				},
				out: &api.DescribeInstanceResponse{
					Instance: &api.Instance{
						Package:  "a/b",
						Instance: fakeObjectRef("0"),
					},
				},
			})

			desc, err := client.DescribeInstance(ctx, pin, &DescribeInstanceOpts{
				DescribeRefs: true,
				DescribeTags: true,
			})
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, desc, should.Resemble(&InstanceDescription{
				InstanceInfo: InstanceInfo{Pin: pin},
			}))
		})

		c.Run("Bad pin", func(c *ftt.Test) {
			_, err := client.DescribeInstance(ctx, common.Pin{
				PackageName: "a/b////",
			}, nil)
			assert.Loosely(c, err, should.ErrLike("invalid package name"))
		})

		c.Run("Error response", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "DescribeInstance",
				in: &api.DescribeInstanceRequest{
					Package:  "a/b",
					Instance: fakeObjectRef("0"),
				},
				err: status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.DescribeInstance(ctx, pin, nil)
			assert.Loosely(c, err, should.ErrLike("blah error"))
		})
	})
}

func TestDescribeClient(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		pin := common.Pin{
			PackageName: "a/b",
			InstanceID:  fakeIID("0"),
		}

		c.Run("Works", func(c *ftt.Test) {
			sha1Ref := &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: strings.Repeat("a", 40),
			}
			sha256Ref := &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: strings.Repeat("a", 64),
			}
			futureRef := &api.ObjectRef{
				HashAlgo:  345,
				HexDigest: strings.Repeat("a", 16),
			}

			repo.expect(rpcCall{
				method: "DescribeClient",
				in: &api.DescribeClientRequest{
					Package:  "a/b",
					Instance: fakeObjectRef("0"),
				},
				out: &api.DescribeClientResponse{
					Instance: &api.Instance{
						Package:  "a/b",
						Instance: fakeObjectRef("0"),
					},
					ClientSize: 12345,
					ClientBinary: &api.ObjectURL{
						SignedUrl: "http://example.com/client_binary",
					},
					ClientRefAliases: []*api.ObjectRef{sha1Ref, sha256Ref, futureRef},
					LegacySha1:       sha1Ref.HexDigest,
				},
			})

			desc, err := client.DescribeClient(ctx, pin)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, desc, should.Resemble(&ClientDescription{
				InstanceInfo:       InstanceInfo{Pin: pin},
				Size:               12345,
				SignedURL:          "http://example.com/client_binary",
				Digest:             sha256Ref, // best supported
				AlternativeDigests: []*api.ObjectRef{sha1Ref, futureRef},
			}))
		})

		c.Run("Bad pin", func(c *ftt.Test) {
			_, err := client.DescribeClient(ctx, common.Pin{
				PackageName: "a/b////",
			})
			assert.Loosely(c, err, should.ErrLike("invalid package name"))
		})

		c.Run("Error response", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "DescribeClient",
				in: &api.DescribeClientRequest{
					Package:  "a/b",
					Instance: fakeObjectRef("0"),
				},
				err: status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.DescribeClient(ctx, pin)
			assert.Loosely(c, err, should.ErrLike("blah error"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Version resolution (including tag cache).

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(c *ftt.Test) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		expectedPin := common.Pin{
			PackageName: "a/b",
			InstanceID:  fakeIID("0"),
		}
		resolvedInst := &api.Instance{
			Package:  "a/b",
			Instance: fakeObjectRef("0"),
		}

		c.Run("Resolves ref", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b",
					Version: "latest",
				},
				out: resolvedInst,
			})
			pin, err := client.ResolveVersion(ctx, "a/b", "latest")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pin, should.Resemble(expectedPin))
		})

		c.Run("Skips resolving instance ID", func(c *ftt.Test) {
			pin, err := client.ResolveVersion(ctx, "a/b", expectedPin.InstanceID)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pin, should.Resemble(expectedPin))
		})

		c.Run("Resolves tag (no tag cache)", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b",
					Version: "k:v",
				},
				out: resolvedInst,
			})
			pin, err := client.ResolveVersion(ctx, "a/b", "k:v")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pin, should.Resemble(expectedPin))
		})

		c.Run("Resolves tag (with tag cache)", func(c *ftt.Test) {
			setupTagCache(client, c)

			// Only one RPC, even though we did two ResolveVersion calls.
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b",
					Version: "k:v",
				},
				out: resolvedInst,
			})

			// Cache miss.
			pin, err := client.ResolveVersion(ctx, "a/b", "k:v")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pin, should.Resemble(expectedPin))

			// Cache hit.
			pin, err = client.ResolveVersion(ctx, "a/b", "k:v")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pin, should.Resemble(expectedPin))
		})

		c.Run("Bad package name", func(c *ftt.Test) {
			_, err := client.ResolveVersion(ctx, "a/b////", "latest")
			assert.Loosely(c, err, should.ErrLike("invalid package name"))
		})

		c.Run("Bad version identifier", func(c *ftt.Test) {
			_, err := client.ResolveVersion(ctx, "a/b", "???")
			assert.Loosely(c, err, should.ErrLike("bad version"))
		})

		c.Run("Error response", func(c *ftt.Test) {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b",
					Version: "k:v",
				},
				err: status.Errorf(codes.FailedPrecondition, "conflict"),
			})
			_, err := client.ResolveVersion(ctx, "a/b", "k:v")
			assert.Loosely(c, err, should.ErrLike("conflict"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Instance fetching and installation (including instance cache).

func TestEnsurePackage(t *testing.T) {
	t.Parallel()

	// Generate a pretty big and random file to be put into the test package, so
	// we have enough bytes to corrupt to trigger flate errors later in the test.
	buf := strings.Builder{}
	src := rand.NewSource(42)
	for range 1000 {
		fmt.Fprintf(&buf, "%d\n", src.Int63())
	}
	testFileBody := buf.String()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := makeTestContext()
		client, _, repo, storage := mockedCipdClient(t)

		body, pin := buildTestInstance("pkg", map[string]string{"test_name": testFileBody})

		ensurePackages := func(ps common.PinSliceBySubdir) (ActionMap, error) {
			return client.EnsurePackages(ctx, ps, nil)
		}

		t.Run("EnsurePackages works", func(t *ftt.Test) {
			setupRemoteInstance(body, pin, repo, storage)

			_, err := ensurePackages(common.PinSliceBySubdir{"": {pin}})
			assert.Loosely(t, err, should.BeNil)

			body, err = os.ReadFile(filepath.Join(client.Root, "test_name"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(body), should.Equal(testFileBody))
		})

		// on windows the ONLY install mode is copy, so this is pointless.
		if platform.CurrentOS() != "windows" {
			t.Run("EnsurePackages works with OverrideInstallMode", func(t *ftt.Test) {
				setupRemoteInstance(body, pin, repo, storage)
				pins := common.PinSliceBySubdir{"": {pin}}
				eo := &EnsureOptions{
					Paranoia:            CheckPresence,
					OverrideInstallMode: pkg.InstallModeCopy,
				}
				_, err := client.EnsurePackages(ctx, pins, eo)
				assert.Loosely(t, err, should.BeNil)

				testFile := filepath.Join(client.Root, "test_name")
				fi, err := os.Lstat(testFile)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fi.Mode().IsRegular(), should.BeTrue) // it's a file

				// and can reverse the override
				// setupRemoteInstance adds another mock call to GetInstanceURL
				setupRemoteInstance(body, pin, repo, storage)
				eo.OverrideInstallMode = ""
				_, err = client.EnsurePackages(ctx, pins, eo)
				assert.Loosely(t, err, should.BeNil)

				fi, err = os.Lstat(testFile)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fi.Mode().IsRegular(), should.BeFalse) // it's a symlink
			})
		}

		t.Run("EnsurePackages uses instance cache", func(t *ftt.Test) {
			setupRemoteInstance(body, pin, repo, storage)
			cacheDir := setupInstanceCache(client, t)

			// The initial fetch.
			_, err := ensurePackages(common.PinSliceBySubdir{"1": {pin}})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, storage.downloads(), should.Equal(1))

			// The file is in the cache now.
			_, err = os.Stat(filepath.Join(cacheDir, "instances", pin.InstanceID))
			assert.Loosely(t, err, should.BeNil)

			// The second fetch should use the instance cache (and thus make no
			// additional RPCs).
			_, err = ensurePackages(common.PinSliceBySubdir{"1": {pin}, "2": {pin}})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, storage.downloads(), should.Equal(1))
		})

		t.Run("EnsurePackages handles cache corruption", func(t *ftt.Test) {
			setupRemoteInstance(body, pin, repo, storage)
			cacheDir := setupInstanceCache(client, t)

			// The initial fetch.
			_, err := ensurePackages(common.PinSliceBySubdir{"1": {pin}})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, storage.downloads(), should.Equal(1))

			// The file is in the cache now. Corrupt it by writing a bunch of zeros
			// in the middle.
			f, err := os.OpenFile(filepath.Join(cacheDir, "instances", pin.InstanceID), os.O_RDWR, 0644)
			assert.Loosely(t, err, should.BeNil)
			off, _ := f.Seek(1000, 0)
			assert.Loosely(t, off, should.Equal(1000))
			_, err = f.Write(bytes.Repeat([]byte{0}, 100))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, f.Close(), should.BeNil)

			// The second fetch should discard the cache and redownload the package.
			setupRemoteInstance(body, pin, repo, storage)
			_, err = ensurePackages(common.PinSliceBySubdir{"1": {pin}, "2": {pin}})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, storage.downloads(), should.Equal(2))
		})

		// TODO: Add more tests.
	})
}

////////////////////////////////////////////////////////////////////////////////
// Client self-update.

func TestMaybeUpdateClient(t *testing.T) {
	t.Parallel()

	clientPackage, err := template.DefaultExpander().Expand(ClientPackage)
	if err != nil {
		panic(err)
	}
	clientFileName := "cipd"
	if platform.CurrentOS() == "windows" {
		clientFileName = "cipd.exe"
	}
	platform := template.DefaultExpander()["platform"]

	ctx := context.Background()

	ftt.Run("MaybeUpdateClient", t, func(c *ftt.Test) {
		tempDir := t.TempDir()

		clientOpts, _, repo, storage := mockedClientOpts(c)

		clientPkgRef := &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: strings.Repeat("a", 64),
		}
		clientPin := common.Pin{
			PackageName: clientPackage,
			InstanceID:  common.ObjectRefToInstanceID(clientPkgRef),
		}

		clientBin := filepath.Join(tempDir, clientFileName)

		caclObjRef := func(body string, algo api.HashAlgo) *api.ObjectRef {
			h := common.MustNewHash(algo)
			h.Write([]byte(body))
			return &api.ObjectRef{
				HashAlgo:  algo,
				HexDigest: common.HexDigest(h),
			}
		}

		writeFile := func(path, body string) {
			assert.Loosely(c, os.WriteFile(path, []byte(body), 0777), should.BeNil)
		}

		readFile := func(path string) string {
			body, err := os.ReadFile(path)
			assert.Loosely(c, err, should.BeNil)
			return string(body)
		}

		storage.putStored("http://example.com/client_bin", "up-to-date")
		upToDateSHA256Ref := caclObjRef("up-to-date", api.HashAlgo_SHA256)
		upToDateSHA1Ref := caclObjRef("up-to-date", api.HashAlgo_SHA256)

		writeFile(clientBin, "outdated")

		expectResolveVersion := func() {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: clientPackage,
					Version: "git:deadbeef",
				},
				out: &api.Instance{
					Package:  clientPackage,
					Instance: clientPkgRef,
				},
			})
		}

		expectDescribeClient := func(refs ...*api.ObjectRef) {
			if len(refs) == 0 {
				refs = []*api.ObjectRef{
					upToDateSHA256Ref,
					upToDateSHA1Ref,
					{HashAlgo: 555, HexDigest: strings.Repeat("f", 66)},
				}
			}
			repo.expect(rpcCall{
				method: "DescribeClient",
				in: &api.DescribeClientRequest{
					Package:  clientPackage,
					Instance: clientPkgRef,
				},
				out: &api.DescribeClientResponse{
					Instance: &api.Instance{
						Package:  clientPackage,
						Instance: clientPkgRef,
					},
					ClientRef: refs[0],
					ClientBinary: &api.ObjectURL{
						SignedUrl: "http://example.com/client_bin",
					},
					ClientSize:       10000,
					LegacySha1:       upToDateSHA1Ref.HexDigest,
					ClientRefAliases: refs,
				},
			})
		}

		expectRPCs := func(refs ...*api.ObjectRef) {
			expectResolveVersion()
			expectDescribeClient(refs...)
		}

		c.Run("Updates outdated client, warming cold caches", func(c *ftt.Test) {
			// Should update 'outdated' to 'up-to-date' and warm up the tag cache.
			expectRPCs()
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, nil)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pin, should.Resemble(clientPin))

			// Yep, updated.
			assert.Loosely(c, readFile(clientBin), should.Equal("up-to-date"))

			// Also drops .cipd_version file.
			verFile := filepath.Join(tempDir, ".versions", clientFileName+".cipd_version")
			assert.Loosely(c, readFile(verFile), should.Equal(
				fmt.Sprintf(`{"package_name":"%s","instance_id":"%s"}`, clientPin.PackageName, clientPin.InstanceID)))

			// And the second update call does nothing and hits no RPCs, since the
			// client is up-to-date and the tag cache is warm.
			pin, err = MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, nil)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pin, should.Resemble(clientPin))

			c.Run("Updates outdated client using warm cache", func(c *ftt.Test) {
				writeFile(clientBin, "outdated")

				// Should update 'outdated' to 'up-to-date'. It skips ResolveVersion,
				// since the tag cache is warm. Still needs DescribeClient to grab
				// the download URL.
				expectDescribeClient()
				pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, nil)
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, pin, should.Resemble(clientPin))

				// Yep, updated.
				assert.Loosely(c, readFile(clientBin), should.Equal("up-to-date"))
			})
		})

		c.Run("Skips updating up-to-date client, warming the cache", func(c *ftt.Test) {
			writeFile(clientBin, "up-to-date")

			// Should just warm the tag cache.
			expectRPCs()
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, nil)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pin, should.Resemble(clientPin))

			// Also drops .cipd_version file.
			verFile := filepath.Join(tempDir, ".versions", clientFileName+".cipd_version")
			assert.Loosely(c, readFile(verFile), should.Equal(
				fmt.Sprintf(`{"package_name":"%s","instance_id":"%s"}`, clientPin.PackageName, clientPin.InstanceID)))

			// And the second update call does nothing and hits no RPCs, since the
			// client is up-to-date and the tag cache is warm.
			pin, err = MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, nil)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pin, should.Resemble(clientPin))
		})

		c.Run("Updates outdated client using digests file", func(c *ftt.Test) {
			dig := digests.ClientDigestsFile{}
			dig.AddClientRef(platform, upToDateSHA256Ref)

			// Should update 'outdated' to 'up-to-date' and warm up the tag cache.
			expectRPCs()
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, &dig)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pin, should.Resemble(clientPin))

			// Yep, updated.
			assert.Loosely(c, readFile(clientBin), should.Equal("up-to-date"))

			// And the second update call does nothing and hits no RPCs, since the
			// client is up-to-date already.
			pin, err = MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, &dig)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, pin, should.Resemble(clientPin))
		})

		c.Run("Refuses to update if *.digests doesn't match what backend says", func(c *ftt.Test) {
			dig := digests.ClientDigestsFile{}
			dig.AddClientRef(platform, caclObjRef("something-else", api.HashAlgo_SHA256))

			// Should refuse the update.
			expectRPCs()
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, &dig)
			assert.Loosely(c, err, should.ErrLike("is not in *.digests file"))
			assert.Loosely(c, pin, should.Resemble(common.Pin{}))

			// The client file wasn't replaced.
			assert.Loosely(c, readFile(clientBin), should.Equal("outdated"))
		})

		c.Run("Refuses to update on hash mismatch", func(c *ftt.Test) {
			expectedRef := caclObjRef("something-else", api.HashAlgo_SHA256)

			dig := digests.ClientDigestsFile{}
			dig.AddClientRef(platform, expectedRef)

			// Should refuse the update.
			expectRPCs(expectedRef)
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, &dig)
			assert.Loosely(c, err, should.ErrLike("file hash mismatch"))
			assert.Loosely(c, pin, should.Resemble(common.Pin{}))

			// The client file wasn't replaced.
			assert.Loosely(c, readFile(clientBin), should.Equal("outdated"))
		})

		c.Run("Refuses to fetch unknown unpinned platform", func(c *ftt.Test) {
			dig := digests.ClientDigestsFile{}

			// Should refuse the update.
			expectRPCs()
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, &dig)
			assert.Loosely(c, err, should.ErrLike("there's no supported hash for"))
			assert.Loosely(c, pin, should.Resemble(common.Pin{}))

			// The client file wasn't replaced.
			assert.Loosely(c, readFile(clientBin), should.Equal("outdated"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Batch ops.

// TODO

////////////////////////////////////////////////////////////////////////////////
// Constructing the client.

func TestNewClientFromEnv(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = environ.New(nil).SetInCtx(ctx)

	ftt.Run("Defaults", t, func(t *ftt.Test) {
		cl, err := NewClientFromEnv(ctx, ClientOptions{
			mockedConfigFile: "something-definitely-missing-and-thats-ok.cfg",
		})
		assert.Loosely(t, err, should.BeNil)

		opts := cl.Options()
		assert.Loosely(t, opts.ServiceURL, should.HavePrefix("https://"))
		assert.Loosely(t, opts.Root, should.BeEmpty)
		assert.Loosely(t, opts.CacheDir, should.BeEmpty)
		assert.Loosely(t, opts.Versions, should.BeNil)
		assert.Loosely(t, opts.AnonymousClient, should.Equal(http.DefaultClient))
		assert.Loosely(t, opts.AuthenticatedClient, should.NotBeNil)
		assert.Loosely(t, opts.MaxThreads, should.Equal(runtime.NumCPU()))
		assert.Loosely(t, opts.ParallelDownloads, should.BeZero) // replaced with DefaultParallelDownloads later
		assert.Loosely(t, opts.UserAgent, should.Equal(UserAgent))
		assert.Loosely(t, opts.LoginInstructions, should.NotBeEmpty)
		assert.Loosely(t, opts.PluginsContext, should.Equal(ctx))
		assert.Loosely(t, opts.AdmissionPlugin, should.BeNil)
	})

	ftt.Run("With default config file", t, func(t *ftt.Test) {
		cfg := filepath.Join(t.TempDir(), "cipd.cfg")
		err := os.WriteFile(cfg, []byte(`
		plugins: {
			admission: {
				cmd: "something"
				args: "arg 1"
				args: "arg 2"
				unrecognized_extra: "zzz"
				more_extra: {
					blarg: 123
				}
			}
		}`), 0600)
		assert.Loosely(t, err, should.BeNil)

		t.Run("Without CIPD_CONFIG_FILE", func(t *ftt.Test) {
			cl, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cl.Options().AdmissionPlugin, should.Resemble([]string{"something", "arg 1", "arg 2"}))
		})

		t.Run("With CIPD_CONFIG_FILE", func(t *ftt.Test) {
			cfg2 := filepath.Join(t.TempDir(), "cipd_2.cfg")
			err := os.WriteFile(cfg2, []byte(`
			plugins: {
				admission: {
					cmd: "override"
				}
			}`), 0600)
			assert.Loosely(t, err, should.BeNil)

			env := environ.FromCtx(ctx)
			env.Set(EnvConfigFile, cfg2)
			ctx := env.SetInCtx(ctx)

			cl, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cl.Options().AdmissionPlugin, should.Resemble([]string{"override"}))
		})

		t.Run("CIPD_CONFIG_FILE is empty", func(t *ftt.Test) {
			env := environ.FromCtx(ctx)
			env.Set(EnvConfigFile, "")
			ctx := env.SetInCtx(ctx)

			cl, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cl.Options().AdmissionPlugin, should.Resemble([]string{"something", "arg 1", "arg 2"}))
		})

		t.Run("CIPD_CONFIG_FILE is -", func(t *ftt.Test) {
			env := environ.FromCtx(ctx)
			env.Set(EnvConfigFile, "-")
			ctx := env.SetInCtx(ctx)

			cl, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cl.Options().AdmissionPlugin, should.BeNil)
		})

		t.Run("CIPD_CONFIG_FILE missing", func(t *ftt.Test) {
			missingAbs, _ := filepath.Abs("something-definitely-missing.cfg")

			env := environ.FromCtx(ctx)
			env.Set(EnvConfigFile, missingAbs)
			ctx := env.SetInCtx(ctx)

			_, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			assert.Loosely(t, err, should.ErrLike("loading CIPD config"))
		})

		t.Run("CIPD_CONFIG_FILE is not absolute", func(t *ftt.Test) {
			env := environ.FromCtx(ctx)
			env.Set(EnvConfigFile, "relative-not-allowed")
			ctx := env.SetInCtx(ctx)

			_, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			assert.Loosely(t, err, should.ErrLike("must be an absolute path"))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

type bytesInstanceFile struct {
	*bytes.Reader
}

func (bytesInstanceFile) Close(context.Context, bool) error { return nil }

func bytesInstance(data []byte) pkg.Source {
	return bytesInstanceFile{bytes.NewReader(data)}
}

func fakeInstance(t testing.TB, name string) pkg.Instance {
	ctx := context.Background()
	out := bytes.Buffer{}
	_, err := builder.BuildInstance(ctx, builder.Options{
		Output:      &out,
		PackageName: name,
	})
	assert.Loosely(t, err, should.BeNil)
	inst, err := reader.OpenInstance(ctx, bytesInstance(out.Bytes()), reader.OpenInstanceOpts{
		VerificationMode: reader.CalculateHash,
		HashAlgo:         api.HashAlgo_SHA256,
	})
	assert.Loosely(t, err, should.BeNil)
	return inst
}

func fakeObjectRef(letter string) *api.ObjectRef {
	return &api.ObjectRef{
		HashAlgo:  api.HashAlgo_SHA256,
		HexDigest: strings.Repeat(letter, 64),
	}
}

func fakeIID(letter string) string {
	return common.ObjectRefToInstanceID(fakeObjectRef(letter))
}

func buildTestInstance(pkg string, blobs map[string]string) ([]byte, common.Pin) {
	keys := make([]string, 0, len(blobs))
	for k := range blobs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	files := make([]fs.File, 0, len(keys))
	for _, k := range keys {
		files = append(files, fs.NewTestFile(k, blobs[k], fs.TestFileOpts{}))
	}

	out := bytes.Buffer{}
	pin, err := builder.BuildInstance(context.Background(), builder.Options{
		Input:            files,
		Output:           &out,
		PackageName:      pkg,
		CompressionLevel: 5,
	})
	if err != nil {
		panic(err)
	}
	return out.Bytes(), pin
}

////////////////////////////////////////////////////////////////////////////////

func mockedClientOpts(t testing.TB) (ClientOptions, *mockedStorageClient, *mockedRepoClient, *mockedStorage) {
	cas := &mockedStorageClient{}
	cas.TB(t)
	repo := &mockedRepoClient{}
	repo.TB(t)

	// When the test case ends, make sure all expected RPCs were called.
	t.Cleanup(func() {
		cas.assertAllCalled()
		repo.assertAllCalled()
	})

	storage := &mockedStorage{}

	siteRoot := t.TempDir()

	return ClientOptions{
		ServiceURL:  "https://service.example.com",
		Root:        siteRoot,
		casMock:     cas,
		repoMock:    repo,
		storageMock: storage,
	}, cas, repo, storage
}

func mockedCipdClient(t testing.TB) (*clientImpl, *mockedStorageClient, *mockedRepoClient, *mockedStorage) {
	opts, cas, repo, storage := mockedClientOpts(t)
	client, err := NewClient(opts)
	assert.Loosely(t, err, should.BeNil)
	t.Cleanup(func() { client.Close(context.Background()) })
	impl := client.(*clientImpl)
	return impl, cas, repo, storage
}

func setupTagCache(cl *clientImpl, t testing.TB) string {
	assert.Loosely(t, cl.tagCache, should.BeNil)
	tempDir := t.TempDir()
	cl.tagCache = internal.NewTagCache(fs.NewFileSystem(tempDir, ""), "service.example.com")
	return tempDir
}

func setupInstanceCache(cl *clientImpl, t testing.TB) string {
	cl.CacheDir = t.TempDir()
	return cl.CacheDir
}

func setupRemoteInstance(body []byte, pin common.Pin, repo *mockedRepoClient, storage *mockedStorage) {
	// "Plant" the package into the storage mock.
	pkgURL := fmt.Sprintf("https://example.com/fake/%s/%s", pin.PackageName, pin.InstanceID)
	storage.putStored(pkgURL, string(body))

	// Make the planted package discoverable.
	repo.expect(rpcCall{
		method: "GetInstanceURL",
		in: &api.GetInstanceURLRequest{
			Package:  pin.PackageName,
			Instance: common.InstanceIDToObjectRef(pin.InstanceID),
		},
		out: &api.ObjectURL{SignedUrl: pkgURL},
	})
}
