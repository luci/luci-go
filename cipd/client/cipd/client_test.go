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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/system/environ"

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

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

////////////////////////////////////////////////////////////////////////////////
// ACL related calls.

func TestFetchACL(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		Convey("Works", func() {
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
			So(err, ShouldBeNil)

			So(acl, ShouldResemble, []PackageACL{
				{
					PackagePath: "a",
					Role:        "READER",
					Principals:  []string{"group:a"},
				},
			})
		})

		Convey("Bad prefix", func() {
			_, err := client.FetchACL(ctx, "a/b////")
			So(err, ShouldErrLike, "invalid package prefix")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "GetInheritedPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.FetchACL(ctx, "a/b/c")
			So(err, ShouldErrLike, "blah error")
		})
	})
}

func TestModifyACL(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		Convey("Modifies existing", func() {
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

			So(client.ModifyACL(ctx, "a", []PackageACLChange{
				{Action: RevokeRole, Role: "READER", Principal: "group:a"},
			}), ShouldBeNil)
		})

		Convey("Creates new", func() {
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

			So(client.ModifyACL(ctx, "a", []PackageACLChange{
				{Action: GrantRole, Role: "READER", Principal: "group:a"},
			}), ShouldBeNil)
		})

		Convey("Noop update", func() {
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

			So(client.ModifyACL(ctx, "a", []PackageACLChange{
				{Action: GrantRole, Role: "READER", Principal: "group:a"},
			}), ShouldBeNil)
		})

		someChanges := []PackageACLChange{
			{Action: RevokeRole, Role: "READER", Principal: "group:a"},
		}

		Convey("Bad prefix", func() {
			So(client.ModifyACL(ctx, "a/b////", someChanges), ShouldErrLike, "invalid package prefix")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "GetPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			So(client.ModifyACL(ctx, "a/b/c", someChanges), ShouldErrLike, "blah error")
		})
	})
}

func TestFetchRoles(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		Convey("Works", func() {
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
			So(err, ShouldBeNil)
			So(roles, ShouldResemble, []string{"OWNER", "WRITER", "READER"})
		})

		Convey("Bad prefix", func() {
			_, err := client.FetchRoles(ctx, "a/b////")
			So(err, ShouldErrLike, "invalid package prefix")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "GetRolesInPrefix",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.FetchRoles(ctx, "a/b/c")
			So(err, ShouldErrLike, "blah error")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Instance upload and registration.

func TestRegisterInstance(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			if testclock.HasTags(t, "cipd-sleeping") {
				tc.Add(d)
			}
		})

		client, cas, repo, storage := mockedCipdClient(c)
		inst := fakeInstance("pkg/inst")

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

		Convey("Happy path", func() {
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			cas.expect(finishUploadRPC(op.OperationId, &api.UploadOperation{
				Status: api.UploadStatus_VERIFYING,
			}))
			cas.expect(finishUploadRPC(op.OperationId, &api.UploadOperation{
				Status: api.UploadStatus_PUBLISHED,
			}))
			repo.expect(registerInstanceRPC(api.RegistrationStatus_REGISTERED, nil))

			So(client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), ShouldBeNil)
			So(storage.getStored(op.UploadUrl), ShouldNotEqual, "")
		})

		Convey("Already registered", func() {
			repo.expect(registerInstanceRPC(api.RegistrationStatus_ALREADY_REGISTERED, nil))
			So(client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), ShouldBeNil)
		})

		Convey("Registration error", func() {
			rpc := registerInstanceRPC(api.RegistrationStatus_ALREADY_REGISTERED, nil)
			rpc.err = status.Errorf(codes.PermissionDenied, "denied blah")
			repo.expect(rpc)
			So(client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), ShouldErrLike, "denied blah")
		})

		Convey("Upload error", func() {
			storage.returnErr(fmt.Errorf("upload err blah"))
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			So(client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), ShouldErrLike, "upload err blah")
		})

		Convey("Verification error", func() {
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			cas.expect(finishUploadRPC(op.OperationId, &api.UploadOperation{
				Status:       api.UploadStatus_ERRORED,
				ErrorMessage: "baaaaad",
			}))
			So(client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), ShouldErrLike, "baaaaad")
		})

		Convey("Confused backend", func() {
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			cas.expect(finishUploadRPC(op.OperationId, &api.UploadOperation{
				Status: api.UploadStatus_PUBLISHED,
			}))
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			So(client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), ShouldErrLike, "servers asks us to upload it again")
		})

		Convey("Verification timeout", func() {
			repo.expect(registerInstanceRPC(api.RegistrationStatus_NOT_UPLOADED, &op))
			cas.expectMany(finishUploadRPC(op.OperationId, &api.UploadOperation{
				Status: api.UploadStatus_VERIFYING,
			}))
			So(client.RegisterInstance(ctx, inst.Pin(), inst.Source(), 0), ShouldErrLike, "timeout while waiting")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Setting refs, attaching tags and metadata.

func TestAttachingStuffWhenReady(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
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

		Convey("SetRefWhenReady happy path", func() {
			repo.expect(createRefRPC())
			So(client.SetRefWhenReady(ctx, "zzz", pin), ShouldBeNil)
		})

		Convey("AttachTagsWhenReady happy path", func() {
			repo.expect(attachTagsRPC())
			So(client.AttachTagsWhenReady(ctx, pin, []string{"k1:v1", "k2:v2"}), ShouldBeNil)
		})

		Convey("AttachMetadataWhenReady happy path", func() {
			repo.expect(attachMetadataRPC())
			So(client.AttachMetadataWhenReady(ctx, pin, cannedMD), ShouldBeNil)
		})

		Convey("SetRefWhenReady timeout", func() {
			rpc := createRefRPC()
			rpc.err = status.Errorf(codes.FailedPrecondition, "not ready")
			repo.expectMany(rpc)
			So(client.SetRefWhenReady(ctx, "zzz", pin), ShouldErrLike, "timeout")
		})

		Convey("AttachTagsWhenReady timeout", func() {
			rpc := attachTagsRPC()
			rpc.err = status.Errorf(codes.FailedPrecondition, "not ready")
			repo.expectMany(rpc)
			So(client.AttachTagsWhenReady(ctx, pin, []string{"k1:v1", "k2:v2"}), ShouldErrLike, "timeout")
		})

		Convey("AttachMetadataWhenReady timeout", func() {
			rpc := attachMetadataRPC()
			rpc.err = status.Errorf(codes.FailedPrecondition, "not ready")
			repo.expectMany(rpc)
			So(client.AttachMetadataWhenReady(ctx, pin, cannedMD), ShouldErrLike, "timeout")
		})

		Convey("SetRefWhenReady fatal RPC err", func() {
			rpc := createRefRPC()
			rpc.err = status.Errorf(codes.PermissionDenied, "boo")
			repo.expect(rpc)
			So(client.SetRefWhenReady(ctx, "zzz", pin), ShouldErrLike, "boo")
		})

		Convey("AttachTagsWhenReady fatal RPC err", func() {
			rpc := attachTagsRPC()
			rpc.err = status.Errorf(codes.PermissionDenied, "boo")
			repo.expect(rpc)
			So(client.AttachTagsWhenReady(ctx, pin, []string{"k1:v1", "k2:v2"}), ShouldErrLike, "boo")
		})

		Convey("AttachMetadataWhenReady fatal RPC err", func() {
			rpc := attachMetadataRPC()
			rpc.err = status.Errorf(codes.PermissionDenied, "boo")
			repo.expect(rpc)
			So(client.AttachMetadataWhenReady(ctx, pin, cannedMD), ShouldErrLike, "boo")
		})

		Convey("SetRefWhenReady bad pin", func() {
			So(client.SetRefWhenReady(ctx, "zzz", common.Pin{
				PackageName: "////",
			}), ShouldErrLike, "invalid package name")
		})

		Convey("SetRefWhenReady bad ref", func() {
			So(client.SetRefWhenReady(ctx, "????", pin), ShouldErrLike, "invalid ref name")
		})

		Convey("AttachTagsWhenReady noop", func() {
			So(client.AttachTagsWhenReady(ctx, pin, nil), ShouldBeNil)
		})

		Convey("AttachTagsWhenReady bad pin", func() {
			So(client.AttachTagsWhenReady(ctx, common.Pin{
				PackageName: "////",
			}, []string{"k:v"}), ShouldErrLike, "invalid package name")
		})

		Convey("AttachTagsWhenReady bad tag", func() {
			So(client.AttachTagsWhenReady(ctx, pin, []string{"good:tag", "bad_tag"}), ShouldErrLike,
				"doesn't look like a tag")
		})

		Convey("AttachMetadataWhenReady noop", func() {
			So(client.AttachMetadataWhenReady(ctx, pin, nil), ShouldBeNil)
		})

		Convey("AttachMetadataWhenReady bad pin", func() {
			So(client.AttachMetadataWhenReady(ctx, common.Pin{
				PackageName: "////",
			}, cannedMD), ShouldErrLike, "invalid package name")
		})

		Convey("AttachMetadataWhenReady bad key", func() {
			So(client.AttachMetadataWhenReady(ctx, pin, []Metadata{
				{Key: "ZZZ", Value: nil},
			}), ShouldErrLike, "invalid metadata key")
		})

		Convey("AttachMetadataWhenReady bad value", func() {
			So(client.AttachMetadataWhenReady(ctx, pin, []Metadata{
				{Key: "k", Value: bytes.Repeat([]byte{0}, 512*1024+1)},
			}), ShouldErrLike, "the metadata value is too long")
		})

		Convey("AttachMetadataWhenReady bad content type", func() {
			So(client.AttachMetadataWhenReady(ctx, pin, []Metadata{
				{Key: "k", ContentType: "zzz zzz"},
			}), ShouldErrLike, "bad content type")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Fetching info about packages and instances.

func TestListPackages(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		Convey("Works", func() {
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
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []string{
				"a/b/c/d/",
				"a/b/c/d/pkg1",
				"a/b/c/d/pkg2",
				"a/b/c/e/",
				"a/b/c/e/f/",
			})
		})

		Convey("Bad prefix", func() {
			_, err := client.ListPackages(ctx, "a/b////", true, true)
			So(err, ShouldErrLike, "invalid package prefix")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "ListPrefix",
				in:     &api.ListPrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.ListPackages(ctx, "a/b/c", false, false)
			So(err, ShouldErrLike, "blah error")
		})
	})
}

func TestSearchInstances(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
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

		Convey("Works", func() {
			repo.expect(searchInstanceRPC())

			out, err := client.SearchInstances(ctx, "a/b", []string{"k1:v1", "k2:v2"})
			So(err, ShouldBeNil)
			So(out, ShouldResemble, common.PinSlice{
				{PackageName: "a/b", InstanceID: fakeIID("0")},
				{PackageName: "a/b", InstanceID: fakeIID("1")},
			})
		})

		Convey("Bad package name", func() {
			_, err := client.SearchInstances(ctx, "a/b////", []string{"k:v"})
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("No tags", func() {
			_, err := client.SearchInstances(ctx, "a/b", nil)
			So(err, ShouldErrLike, "at least one tag is required")
		})

		Convey("Bad tag", func() {
			_, err := client.SearchInstances(ctx, "a/b", []string{"bad_tag"})
			So(err, ShouldErrLike, "doesn't look like a tag")
		})

		Convey("Error response", func() {
			rpc := searchInstanceRPC()
			rpc.err = status.Errorf(codes.PermissionDenied, "blah error")
			repo.expect(rpc)

			_, err := client.SearchInstances(ctx, "a/b", []string{"k1:v1", "k2:v2"})
			So(err, ShouldErrLike, "blah error")
		})

		Convey("No package", func() {
			rpc := searchInstanceRPC()
			rpc.err = status.Errorf(codes.NotFound, "no such package")
			repo.expect(rpc)

			out, err := client.SearchInstances(ctx, "a/b", []string{"k1:v1", "k2:v2"})
			So(out, ShouldHaveLength, 0)
			So(err, ShouldBeNil)
		})
	})
}

func TestListInstances(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
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

		Convey("Works", func() {
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
			So(err, ShouldBeNil)

			all := []InstanceInfo{}
			for {
				batch, err := enum.Next(ctx, 2)
				So(err, ShouldBeNil)
				if len(batch) == 0 {
					break
				}
				all = append(all, batch...)
			}

			So(all, ShouldResemble, []InstanceInfo{
				fakeInstInfo("0"),
				fakeInstInfo("1"),
				fakeInstInfo("2"),
			})
		})

		Convey("Bad package name", func() {
			_, err := client.ListInstances(ctx, "a/b////")
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "ListInstances",
				in: &api.ListInstancesRequest{
					Package:  "a/b",
					PageSize: 100,
				},
				err: status.Errorf(codes.PermissionDenied, "blah error"),
			})
			enum, err := client.ListInstances(ctx, "a/b")
			So(err, ShouldBeNil)
			_, err = enum.Next(ctx, 100)
			So(err, ShouldErrLike, "blah error")
		})
	})
}

func TestFetchPackageRefs(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		Convey("Works", func() {
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
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []RefInfo{
				{Ref: "r1", InstanceID: fakeIID("0")},
				{Ref: "r2", InstanceID: fakeIID("1")},
			})
		})

		Convey("Bad package name", func() {
			_, err := client.FetchPackageRefs(ctx, "a/b////")
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "ListRefs",
				in:     &api.ListRefsRequest{Package: "a/b"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.FetchPackageRefs(ctx, "a/b")
			So(err, ShouldErrLike, "blah error")
		})
	})
}

func TestDescribeInstance(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		pin := common.Pin{
			PackageName: "a/b",
			InstanceID:  fakeIID("0"),
		}

		Convey("Works", func() {
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
			So(err, ShouldBeNil)
			So(desc, ShouldResemble, &InstanceDescription{
				InstanceInfo: InstanceInfo{Pin: pin},
			})
		})

		Convey("Bad pin", func() {
			_, err := client.DescribeInstance(ctx, common.Pin{
				PackageName: "a/b////",
			}, nil)
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "DescribeInstance",
				in: &api.DescribeInstanceRequest{
					Package:  "a/b",
					Instance: fakeObjectRef("0"),
				},
				err: status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.DescribeInstance(ctx, pin, nil)
			So(err, ShouldErrLike, "blah error")
		})
	})
}

func TestDescribeClient(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		pin := common.Pin{
			PackageName: "a/b",
			InstanceID:  fakeIID("0"),
		}

		Convey("Works", func() {
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
			So(err, ShouldBeNil)
			So(desc, ShouldResemble, &ClientDescription{
				InstanceInfo:       InstanceInfo{Pin: pin},
				Size:               12345,
				SignedURL:          "http://example.com/client_binary",
				Digest:             sha256Ref, // best supported
				AlternativeDigests: []*api.ObjectRef{sha1Ref, futureRef},
			})
		})

		Convey("Bad pin", func() {
			_, err := client.DescribeClient(ctx, common.Pin{
				PackageName: "a/b////",
			})
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "DescribeClient",
				in: &api.DescribeClientRequest{
					Package:  "a/b",
					Instance: fakeObjectRef("0"),
				},
				err: status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.DescribeClient(ctx, pin)
			So(err, ShouldErrLike, "blah error")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Version resolution (including tag cache).

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
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

		Convey("Resolves ref", func() {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b",
					Version: "latest",
				},
				out: resolvedInst,
			})
			pin, err := client.ResolveVersion(ctx, "a/b", "latest")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, expectedPin)
		})

		Convey("Skips resolving instance ID", func() {
			pin, err := client.ResolveVersion(ctx, "a/b", expectedPin.InstanceID)
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, expectedPin)
		})

		Convey("Resolves tag (no tag cache)", func() {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b",
					Version: "k:v",
				},
				out: resolvedInst,
			})
			pin, err := client.ResolveVersion(ctx, "a/b", "k:v")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, expectedPin)
		})

		Convey("Resolves tag (with tag cache)", func() {
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
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, expectedPin)

			// Cache hit.
			pin, err = client.ResolveVersion(ctx, "a/b", "k:v")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, expectedPin)
		})

		Convey("Bad package name", func() {
			_, err := client.ResolveVersion(ctx, "a/b////", "latest")
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Bad version identifier", func() {
			_, err := client.ResolveVersion(ctx, "a/b", "???")
			So(err, ShouldErrLike, "bad version")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b",
					Version: "k:v",
				},
				err: status.Errorf(codes.FailedPrecondition, "conflict"),
			})
			_, err := client.ResolveVersion(ctx, "a/b", "k:v")
			So(err, ShouldErrLike, "conflict")
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
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&buf, "%d\n", src.Int63())
	}
	testFileBody := buf.String()

	Convey("With mocks", t, func(c C) {
		ctx := makeTestContext()
		client, _, repo, storage := mockedCipdClient(c)

		body, pin := buildTestInstance("pkg", map[string]string{"test_name": testFileBody})
		setupRemoteInstance(body, pin, repo, storage)

		ensurePackages := func(ps common.PinSliceBySubdir) (ActionMap, error) {
			return client.EnsurePackages(ctx, ps, nil)
		}

		Convey("EnsurePackages works", func() {
			_, err := ensurePackages(common.PinSliceBySubdir{"": {pin}})
			So(err, ShouldBeNil)

			body, err = os.ReadFile(filepath.Join(client.Root, "test_name"))
			So(err, ShouldBeNil)
			So(string(body), ShouldEqual, testFileBody)
		})

		// on windows the ONLY install mode is copy, so this is pointless.
		if platform.CurrentOS() != "windows" {
			Convey("EnsurePackages works with OverrideInstallMode", func() {
				pins := common.PinSliceBySubdir{"": {pin}}
				eo := &EnsureOptions{
					Paranoia:            CheckPresence,
					OverrideInstallMode: pkg.InstallModeCopy,
				}
				_, err := client.EnsurePackages(ctx, pins, eo)
				So(err, ShouldBeNil)

				testFile := filepath.Join(client.Root, "test_name")
				fi, err := os.Lstat(testFile)
				So(err, ShouldBeNil)
				So(fi.Mode().IsRegular(), ShouldBeTrue) // it's a file

				// and can reverse the override
				// setupRemoteInstance adds another mock call to GetInstanceURL
				setupRemoteInstance(body, pin, repo, storage)
				eo.OverrideInstallMode = ""
				_, err = client.EnsurePackages(ctx, pins, eo)
				So(err, ShouldBeNil)

				fi, err = os.Lstat(testFile)
				So(err, ShouldBeNil)
				So(fi.Mode().IsRegular(), ShouldBeFalse) // it's a symlink
			})
		}

		Convey("EnsurePackages uses instance cache", func() {
			cacheDir := setupInstanceCache(client, c)

			// The initial fetch.
			_, err := ensurePackages(common.PinSliceBySubdir{"1": {pin}})
			So(err, ShouldBeNil)
			So(storage.downloads(), ShouldEqual, 1)

			// The file is in the cache now.
			_, err = os.Stat(filepath.Join(cacheDir, "instances", pin.InstanceID))
			So(err, ShouldBeNil)

			// The second fetch should use the instance cache (and thus make no
			// additional RPCs).
			_, err = ensurePackages(common.PinSliceBySubdir{"1": {pin}, "2": {pin}})
			So(err, ShouldBeNil)
			So(storage.downloads(), ShouldEqual, 1)
		})

		Convey("EnsurePackages handles cache corruption", func() {
			cacheDir := setupInstanceCache(client, c)

			// The initial fetch.
			_, err := ensurePackages(common.PinSliceBySubdir{"1": {pin}})
			So(err, ShouldBeNil)
			So(storage.downloads(), ShouldEqual, 1)

			// The file is in the cache now. Corrupt it by writing a bunch of zeros
			// in the middle.
			f, err := os.OpenFile(filepath.Join(cacheDir, "instances", pin.InstanceID), os.O_RDWR, 0644)
			So(err, ShouldBeNil)
			off, _ := f.Seek(1000, 0)
			So(off, ShouldEqual, 1000)
			_, err = f.Write(bytes.Repeat([]byte{0}, 100))
			So(err, ShouldBeNil)
			So(f.Close(), ShouldBeNil)

			// The second fetch should discard the cache and redownload the package.
			setupRemoteInstance(body, pin, repo, storage)
			_, err = ensurePackages(common.PinSliceBySubdir{"1": {pin}, "2": {pin}})
			So(err, ShouldBeNil)
			So(storage.downloads(), ShouldEqual, 2)
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

	Convey("MaybeUpdateClient", t, func(c C) {
		tempDir, err := os.MkdirTemp("", "cipd_tag_cache")
		c.So(err, ShouldBeNil)
		c.Reset(func() {
			os.RemoveAll(tempDir)
		})

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
			So(os.WriteFile(path, []byte(body), 0777), ShouldBeNil)
		}

		readFile := func(path string) string {
			body, err := os.ReadFile(path)
			So(err, ShouldBeNil)
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

		Convey("Updates outdated client, warming cold caches", func() {
			// Should update 'outdated' to 'up-to-date' and warm up the tag cache.
			expectRPCs()
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, nil)
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, clientPin)

			// Yep, updated.
			So(readFile(clientBin), ShouldEqual, "up-to-date")

			// Also drops .cipd_version file.
			verFile := filepath.Join(tempDir, ".versions", clientFileName+".cipd_version")
			So(readFile(verFile), ShouldEqual,
				fmt.Sprintf(`{"package_name":"%s","instance_id":"%s"}`, clientPin.PackageName, clientPin.InstanceID))

			// And the second update call does nothing and hits no RPCs, since the
			// client is up-to-date and the tag cache is warm.
			pin, err = MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, nil)
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, clientPin)

			Convey("Updates outdated client using warm cache", func() {
				writeFile(clientBin, "outdated")

				// Should update 'outdated' to 'up-to-date'. It skips ResolveVersion,
				// since the tag cache is warm. Still needs DescribeClient to grab
				// the download URL.
				expectDescribeClient()
				pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, nil)
				So(err, ShouldBeNil)
				So(pin, ShouldResemble, clientPin)

				// Yep, updated.
				So(readFile(clientBin), ShouldEqual, "up-to-date")
			})
		})

		Convey("Skips updating up-to-date client, warming the cache", func() {
			writeFile(clientBin, "up-to-date")

			// Should just warm the tag cache.
			expectRPCs()
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, nil)
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, clientPin)

			// Also drops .cipd_version file.
			verFile := filepath.Join(tempDir, ".versions", clientFileName+".cipd_version")
			So(readFile(verFile), ShouldEqual,
				fmt.Sprintf(`{"package_name":"%s","instance_id":"%s"}`, clientPin.PackageName, clientPin.InstanceID))

			// And the second update call does nothing and hits no RPCs, since the
			// client is up-to-date and the tag cache is warm.
			pin, err = MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, nil)
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, clientPin)
		})

		Convey("Updates outdated client using digests file", func() {
			dig := digests.ClientDigestsFile{}
			dig.AddClientRef(platform, upToDateSHA256Ref)

			// Should update 'outdated' to 'up-to-date' and warm up the tag cache.
			expectRPCs()
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, &dig)
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, clientPin)

			// Yep, updated.
			So(readFile(clientBin), ShouldEqual, "up-to-date")

			// And the second update call does nothing and hits no RPCs, since the
			// client is up-to-date already.
			pin, err = MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, &dig)
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, clientPin)
		})

		Convey("Refuses to update if *.digests doesn't match what backend says", func() {
			dig := digests.ClientDigestsFile{}
			dig.AddClientRef(platform, caclObjRef("something-else", api.HashAlgo_SHA256))

			// Should refuse the update.
			expectRPCs()
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, &dig)
			So(err, ShouldErrLike, "is not in *.digests file")
			So(pin, ShouldResemble, common.Pin{})

			// The client file wasn't replaced.
			So(readFile(clientBin), ShouldEqual, "outdated")
		})

		Convey("Refuses to update on hash mismatch", func() {
			expectedRef := caclObjRef("something-else", api.HashAlgo_SHA256)

			dig := digests.ClientDigestsFile{}
			dig.AddClientRef(platform, expectedRef)

			// Should refuse the update.
			expectRPCs(expectedRef)
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, &dig)
			So(err, ShouldErrLike, "file hash mismatch")
			So(pin, ShouldResemble, common.Pin{})

			// The client file wasn't replaced.
			So(readFile(clientBin), ShouldEqual, "outdated")
		})

		Convey("Refuses to fetch unknown unpinned platform", func() {
			dig := digests.ClientDigestsFile{}

			// Should refuse the update.
			expectRPCs()
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin, &dig)
			So(err, ShouldErrLike, "there's no supported hash for")
			So(pin, ShouldResemble, common.Pin{})

			// The client file wasn't replaced.
			So(readFile(clientBin), ShouldEqual, "outdated")
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

	Convey("Defaults", t, func() {
		cl, err := NewClientFromEnv(ctx, ClientOptions{
			mockedConfigFile: "something-definitely-missing-and-thats-ok.cfg",
		})
		So(err, ShouldBeNil)

		opts := cl.Options()
		So(opts.ServiceURL, ShouldStartWith, "https://")
		So(opts.Root, ShouldEqual, "")
		So(opts.CacheDir, ShouldEqual, "")
		So(opts.Versions, ShouldBeNil)
		So(opts.AnonymousClient, ShouldEqual, http.DefaultClient)
		So(opts.AuthenticatedClient, ShouldNotBeNil)
		So(opts.MaxThreads, ShouldEqual, runtime.NumCPU())
		So(opts.ParallelDownloads, ShouldEqual, 0) // replaced with DefaultParallelDownloads later
		So(opts.UserAgent, ShouldEqual, UserAgent)
		So(opts.LoginInstructions, ShouldNotBeEmpty)
		So(opts.PluginsContext, ShouldEqual, ctx)
		So(opts.AdmissionPlugin, ShouldBeNil)
	})

	Convey("With default config file", t, func() {
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
		So(err, ShouldBeNil)

		Convey("Without CIPD_CONFIG_FILE", func() {
			cl, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			So(err, ShouldBeNil)
			So(cl.Options().AdmissionPlugin, ShouldResemble, []string{"something", "arg 1", "arg 2"})
		})

		Convey("With CIPD_CONFIG_FILE", func() {
			cfg2 := filepath.Join(t.TempDir(), "cipd_2.cfg")
			err := os.WriteFile(cfg2, []byte(`
			plugins: {
				admission: {
					cmd: "override"
				}
			}`), 0600)
			So(err, ShouldBeNil)

			env := environ.FromCtx(ctx)
			env.Set(EnvConfigFile, cfg2)
			ctx := env.SetInCtx(ctx)

			cl, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			So(err, ShouldBeNil)
			So(cl.Options().AdmissionPlugin, ShouldResemble, []string{"override"})
		})

		Convey("CIPD_CONFIG_FILE is empty", func() {
			env := environ.FromCtx(ctx)
			env.Set(EnvConfigFile, "")
			ctx := env.SetInCtx(ctx)

			cl, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			So(err, ShouldBeNil)
			So(cl.Options().AdmissionPlugin, ShouldResemble, []string{"something", "arg 1", "arg 2"})
		})

		Convey("CIPD_CONFIG_FILE is -", func() {
			env := environ.FromCtx(ctx)
			env.Set(EnvConfigFile, "-")
			ctx := env.SetInCtx(ctx)

			cl, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			So(err, ShouldBeNil)
			So(cl.Options().AdmissionPlugin, ShouldBeNil)
		})

		Convey("CIPD_CONFIG_FILE missing", func() {
			missingAbs, _ := filepath.Abs("something-definitely-missing.cfg")

			env := environ.FromCtx(ctx)
			env.Set(EnvConfigFile, missingAbs)
			ctx := env.SetInCtx(ctx)

			_, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			So(err, ShouldErrLike, "loading CIPD config")
		})

		Convey("CIPD_CONFIG_FILE is not absolute", func() {
			env := environ.FromCtx(ctx)
			env.Set(EnvConfigFile, "relative-not-allowed")
			ctx := env.SetInCtx(ctx)

			_, err := NewClientFromEnv(ctx, ClientOptions{mockedConfigFile: cfg})
			So(err, ShouldErrLike, "must be an absolute path")
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

func fakeInstance(name string) pkg.Instance {
	ctx := context.Background()
	out := bytes.Buffer{}
	_, err := builder.BuildInstance(ctx, builder.Options{
		Output:      &out,
		PackageName: name,
	})
	So(err, ShouldBeNil)
	inst, err := reader.OpenInstance(ctx, bytesInstance(out.Bytes()), reader.OpenInstanceOpts{
		VerificationMode: reader.CalculateHash,
		HashAlgo:         api.HashAlgo_SHA256,
	})
	So(err, ShouldBeNil)
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

func mockedClientOpts(c C) (ClientOptions, *mockedStorageClient, *mockedRepoClient, *mockedStorage) {
	cas := &mockedStorageClient{}
	cas.C(c)
	repo := &mockedRepoClient{}
	repo.C(c)

	// When the test case ends, make sure all expected RPCs were called.
	c.Reset(func() {
		cas.assertAllCalled()
		repo.assertAllCalled()
	})

	storage := &mockedStorage{}

	siteRoot, err := os.MkdirTemp("", "cipd_site_root")
	c.So(err, ShouldBeNil)
	c.Reset(func() { os.RemoveAll(siteRoot) })

	return ClientOptions{
		ServiceURL:  "https://service.example.com",
		Root:        siteRoot,
		casMock:     cas,
		repoMock:    repo,
		storageMock: storage,
	}, cas, repo, storage
}

func mockedCipdClient(c C) (*clientImpl, *mockedStorageClient, *mockedRepoClient, *mockedStorage) {
	opts, cas, repo, storage := mockedClientOpts(c)
	client, err := NewClient(opts)
	c.So(err, ShouldBeNil)
	c.Reset(func() { client.Close(context.Background()) })
	impl := client.(*clientImpl)
	return impl, cas, repo, storage
}

func setupTagCache(cl *clientImpl, c C) string {
	c.So(cl.tagCache, ShouldBeNil)
	tempDir, err := os.MkdirTemp("", "cipd_tag_cache")
	c.So(err, ShouldBeNil)
	c.Reset(func() { os.RemoveAll(tempDir) })
	cl.tagCache = internal.NewTagCache(fs.NewFileSystem(tempDir, ""), "service.example.com")
	return tempDir
}

func setupInstanceCache(cl *clientImpl, c C) string {
	tempDir, err := os.MkdirTemp("", "cipd_instance_cache")
	c.So(err, ShouldBeNil)
	c.Reset(func() { os.RemoveAll(tempDir) })
	cl.CacheDir = tempDir
	return tempDir
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
