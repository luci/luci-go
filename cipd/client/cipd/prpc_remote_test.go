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
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/proto/google"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGrantRevokeRole(t *testing.T) {
	t.Parallel()

	Convey("Grant role", t, func() {
		m := &api.PrefixMetadata{}

		So(grantRole(m, api.Role_READER, "group:a"), ShouldBeTrue)
		So(grantRole(m, api.Role_READER, "group:b"), ShouldBeTrue)
		So(grantRole(m, api.Role_READER, "group:a"), ShouldBeFalse)
		So(grantRole(m, api.Role_WRITER, "group:a"), ShouldBeTrue)

		So(m, ShouldResemble, &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{"group:a", "group:b"}},
				{Role: api.Role_WRITER, Principals: []string{"group:a"}},
			},
		})
	})

	Convey("Revoke role", t, func() {
		m := &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{"group:a", "group:b"}},
				{Role: api.Role_WRITER, Principals: []string{"group:a"}},
			},
		}

		So(revokeRole(m, api.Role_READER, "group:a"), ShouldBeTrue)
		So(revokeRole(m, api.Role_READER, "group:b"), ShouldBeTrue)
		So(revokeRole(m, api.Role_READER, "group:a"), ShouldBeFalse)
		So(revokeRole(m, api.Role_WRITER, "group:a"), ShouldBeTrue)

		So(m, ShouldResemble, &api.PrefixMetadata{})
	})
}

func TestPrpcRemoteImpl(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2018, time.February, 1, 2, 3, 0, 0, time.UTC)

	Convey("with mocked clients", t, func(c C) {
		ctx := context.Background()

		cas := mockedStorageClient{}
		cas.C(c)
		repo := mockedRepoClient{}
		repo.C(c)
		r := &prpcRemoteImpl{cas: &cas, repo: &repo}

		Convey("fetchACL works", func() {
			repo.expect(rpcCall{
				method: "GetInheritedPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				out: &api.InheritedPrefixMetadata{
					PerPrefixMetadata: []*api.PrefixMetadata{
						{
							Prefix: "a",
							Acls: []*api.PrefixMetadata_ACL{
								{Role: api.Role_READER, Principals: []string{"group:a"}},
								{Role: api.Role_READER, Principals: []string{"group:b"}},
								{Role: api.Role_WRITER, Principals: []string{"group:b"}},
								{Role: api.Role_OWNER, Principals: []string{"group:c"}},
							},
							UpdateUser: "user:a-updater@example.com",
							UpdateTime: google.NewTimestamp(epoch),
						},
						{
							Prefix: "a/b/c",
							Acls: []*api.PrefixMetadata_ACL{
								{Role: api.Role_OWNER, Principals: []string{"group:c"}},
							},
							UpdateUser: "user:c-updater@example.com",
							UpdateTime: google.NewTimestamp(epoch),
						},
					},
				},
			})

			acl, err := r.fetchACL(ctx, "a/b/c")
			So(err, ShouldBeNil)
			So(acl, ShouldResemble, []PackageACL{
				{
					PackagePath: "a",
					Role:        "READER",
					Principals:  []string{"group:a", "group:b"}, // merged into one PackageACL
					ModifiedBy:  "user:a-updater@example.com",
					ModifiedTs:  UnixTime(epoch),
				},
				{
					PackagePath: "a",
					Role:        "WRITER",
					Principals:  []string{"group:b"},
					ModifiedBy:  "user:a-updater@example.com",
					ModifiedTs:  UnixTime(epoch),
				},
				{
					PackagePath: "a",
					Role:        "OWNER",
					Principals:  []string{"group:c"},
					ModifiedBy:  "user:a-updater@example.com",
					ModifiedTs:  UnixTime(epoch),
				},
				{
					PackagePath: "a/b/c",
					Role:        "OWNER",
					Principals:  []string{"group:c"},
					ModifiedBy:  "user:c-updater@example.com",
					ModifiedTs:  UnixTime(epoch),
				},
			})
		})

		Convey("modifyACL works with new ACL", func() {
			repo.expect(rpcCall{
				method: "GetPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a"},
				err:    grpc.Errorf(codes.NotFound, "no metadata"),
			})
			repo.expect(rpcCall{
				method: "UpdatePrefixMetadata",
				in: &api.PrefixMetadata{
					Prefix: "a",
					Acls: []*api.PrefixMetadata_ACL{
						{Role: api.Role_READER, Principals: []string{"group:b"}},
					},
				},
				out: &api.PrefixMetadata{},
			})

			So(r.modifyACL(ctx, "a", []PackageACLChange{
				{Action: GrantRole, Role: "READER", Principal: "group:a"},
				{Action: GrantRole, Role: "READER", Principal: "group:b"},
				{Action: RevokeRole, Role: "READER", Principal: "group:a"},
				{Action: RevokeRole, Role: "UNKNOWN_ROLE", Principal: "group:a"},
			}), ShouldBeNil)

			repo.assertAllCalled()
		})

		Convey("modifyACL works with existing ACL", func() {
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
				out: &api.PrefixMetadata{},
			})

			So(r.modifyACL(ctx, "a", []PackageACLChange{
				{Action: RevokeRole, Role: "READER", Principal: "group:a"},
			}), ShouldBeNil)

			repo.assertAllCalled()
		})

		Convey("modifyACL noop call", func() {
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

			So(r.modifyACL(ctx, "a", []PackageACLChange{
				{Action: RevokeRole, Role: "READER", Principal: "group:another"},
			}), ShouldBeNil)

			repo.assertAllCalled()
		})

		Convey("fetchRoles works", func() {
			repo.expect(rpcCall{
				method: "GetRolesInPrefix",
				in:     &api.PrefixRequest{Prefix: "a"},
				out: &api.RolesInPrefixResponse{
					Roles: []*api.RolesInPrefixResponse_RoleInPrefix{
						{Role: api.Role_READER},
						{Role: api.Role_WRITER},
					},
				},
			})

			res, err := r.fetchRoles(ctx, "a")
			So(err, ShouldBeNil)
			So(res, ShouldResemble, []string{"READER", "WRITER"})

			repo.assertAllCalled()
		})

		sha1 := strings.Repeat("a", 40)

		Convey("initiateUpload works", func() {
			cas.expect(rpcCall{
				method: "BeginUpload",
				in: &api.BeginUploadRequest{
					Object: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: sha1,
					},
				},
				out: &api.UploadOperation{
					OperationId: "op_id",
					UploadUrl:   "http://upload.example.com",
				},
			})

			session, err := r.initiateUpload(ctx, sha1)
			So(err, ShouldBeNil)
			So(session, ShouldResemble, &UploadSession{
				ID:  "op_id",
				URL: "http://upload.example.com",
			})
		})

		Convey("initiateUpload already uploaded", func() {
			cas.expect(rpcCall{
				method: "BeginUpload",
				in: &api.BeginUploadRequest{
					Object: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: sha1,
					},
				},
				err: grpc.Errorf(codes.AlreadyExists, "have it"),
			})

			session, err := r.initiateUpload(ctx, sha1)
			So(err, ShouldBeNil)
			So(session, ShouldBeNil)
		})

		Convey("finalizeUpload, still verifying", func() {
			cas.expect(rpcCall{
				method: "FinishUpload",
				in: &api.FinishUploadRequest{
					UploadOperationId: "op_id",
				},
				out: &api.UploadOperation{
					OperationId: "op_id",
					Status:      api.UploadStatus_VERIFYING,
				},
			})
			verified, err := r.finalizeUpload(ctx, "op_id")
			So(err, ShouldBeNil)
			So(verified, ShouldBeFalse)
		})

		Convey("finalizeUpload, verified", func() {
			cas.expect(rpcCall{
				method: "FinishUpload",
				in: &api.FinishUploadRequest{
					UploadOperationId: "op_id",
				},
				out: &api.UploadOperation{
					OperationId: "op_id",
					Status:      api.UploadStatus_PUBLISHED,
				},
			})
			verified, err := r.finalizeUpload(ctx, "op_id")
			So(err, ShouldBeNil)
			So(verified, ShouldBeTrue)
		})

		Convey("finalizeUpload, error", func() {
			cas.expect(rpcCall{
				method: "FinishUpload",
				in: &api.FinishUploadRequest{
					UploadOperationId: "op_id",
				},
				out: &api.UploadOperation{
					OperationId:  "op_id",
					Status:       api.UploadStatus_ERRORED,
					ErrorMessage: "boo",
				},
			})
			verified, err := r.finalizeUpload(ctx, "op_id")
			So(err, ShouldErrLike, "boo")
			So(verified, ShouldBeFalse)
		})

		Convey("finalizeUpload, unknown", func() {
			cas.expect(rpcCall{
				method: "FinishUpload",
				in: &api.FinishUploadRequest{
					UploadOperationId: "op_id",
				},
				out: &api.UploadOperation{
					OperationId: "op_id",
					Status:      123,
				},
			})
			verified, err := r.finalizeUpload(ctx, "op_id")
			So(err, ShouldErrLike, "unrecognized upload operation status 123")
			So(verified, ShouldBeFalse)
		})

		Convey("registerInstance, success", func() {
			repo.expect(rpcCall{
				method: "RegisterInstance",
				in: &api.Instance{
					Package: "a",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: sha1,
					},
				},
				out: &api.RegisterInstanceResponse{
					Status: api.RegistrationStatus_REGISTERED,
					Instance: &api.Instance{
						// ... omitted fields ...
						RegisteredBy: "user:a@example.com",
						RegisteredTs: google.NewTimestamp(epoch),
					},
				},
			})

			resp, err := r.registerInstance(ctx, common.Pin{"a", sha1})
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &registerInstanceResponse{
				registeredBy: "user:a@example.com",
				registeredTs: epoch,
			})
		})

		Convey("registerInstance, not uploaded", func() {
			repo.expect(rpcCall{
				method: "RegisterInstance",
				in: &api.Instance{
					Package: "a",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: sha1,
					},
				},
				out: &api.RegisterInstanceResponse{
					Status: api.RegistrationStatus_NOT_UPLOADED,
					UploadOp: &api.UploadOperation{
						OperationId: "op_id",
						UploadUrl:   "http://upload.example.com",
					},
				},
			})

			resp, err := r.registerInstance(ctx, common.Pin{"a", sha1})
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &registerInstanceResponse{
				uploadSession: &UploadSession{"op_id", "http://upload.example.com"},
			})
		})

		Convey("resolveVersion OK", func() {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b/c",
					Version: "latest",
				},
				out: &api.Instance{
					Package: "a/b/c",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: sha1,
					},
				},
			})

			pin, err := r.resolveVersion(ctx, "a/b/c", "latest")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, common.Pin{
				PackageName: "a/b/c",
				InstanceID:  sha1,
			})

			repo.assertAllCalled()
		})

		Convey("resolveVersion NotFound", func() {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b/c",
					Version: "latest",
				},
				err: status.Errorf(codes.NotFound, "no such ref"),
			})

			_, err := r.resolveVersion(ctx, "a/b/c", "latest")
			So(err.Error(), ShouldEqual, "no such ref")

			repo.assertAllCalled()
		})

		Convey("fetchPackageRefs works", func() {
			repo.expect(rpcCall{
				method: "ListRefs",
				in:     &api.ListRefsRequest{Package: "a/b/c"},
				out: &api.ListRefsResponse{
					Refs: []*api.Ref{
						{
							Name:    "ref1",
							Package: "a/b/c",
							Instance: &api.ObjectRef{
								HashAlgo:  api.HashAlgo_SHA1,
								HexDigest: strings.Repeat("1", 40),
							},
							ModifiedBy: "user:m@example.com",
							ModifiedTs: google.NewTimestamp(epoch.Add(time.Hour)),
						},
						{
							Name:    "ref2",
							Package: "a/b/c",
							Instance: &api.ObjectRef{
								HashAlgo:  api.HashAlgo_SHA1,
								HexDigest: strings.Repeat("2", 40),
							},
							ModifiedBy: "user:m@example.com",
							ModifiedTs: google.NewTimestamp(epoch),
						},
					},
				},
			})

			refs, err := r.fetchPackageRefs(ctx, "a/b/c")
			So(err, ShouldBeNil)
			So(refs, ShouldResemble, []RefInfo{
				{
					Ref:        "ref1",
					InstanceID: strings.Repeat("1", 40),
					ModifiedBy: "user:m@example.com",
					ModifiedTs: UnixTime(epoch.Add(time.Hour)),
				},
				{
					Ref:        "ref2",
					InstanceID: strings.Repeat("2", 40),
					ModifiedBy: "user:m@example.com",
					ModifiedTs: UnixTime(epoch),
				},
			})

			repo.assertAllCalled()
		})

		Convey("fetchInstanceURL OK", func() {
			repo.expect(rpcCall{
				method: "GetInstanceURL",
				in: &api.GetInstanceURLRequest{
					Package: "a/b/c",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: sha1,
					},
				},
				out: &api.ObjectURL{
					SignedUrl: "https://example.com/signed",
				},
			})

			url, err := r.fetchInstanceURL(ctx, common.Pin{
				PackageName: "a/b/c",
				InstanceID:  sha1,
			})
			So(err, ShouldBeNil)
			So(url, ShouldEqual, "https://example.com/signed")

			repo.assertAllCalled()
		})

		Convey("fetchInstanceURL NotFound", func() {
			repo.expect(rpcCall{
				method: "GetInstanceURL",
				in: &api.GetInstanceURLRequest{
					Package: "a/b/c",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: sha1,
					},
				},
				err: status.Errorf(codes.NotFound, "no such package"),
			})

			_, err := r.fetchInstanceURL(ctx, common.Pin{
				PackageName: "a/b/c",
				InstanceID:  sha1,
			})
			So(err.Error(), ShouldEqual, "no such package")

			repo.assertAllCalled()
		})

		Convey("listPackages works", func() {
			repo.expect(rpcCall{
				method: "ListPrefix",
				in: &api.ListPrefixRequest{
					Prefix:        "a",
					Recursive:     true,
					IncludeHidden: true,
				},
				out: &api.ListPrefixResponse{
					Packages: []string{"a/b", "a/c"},
					Prefixes: []string{"a/d", "a/d/e"},
				},
			})

			pkgs, prefixes, err := r.listPackages(ctx, "a", true, true)
			So(err, ShouldBeNil)
			So(pkgs, ShouldResemble, []string{"a/b", "a/c"})
			So(prefixes, ShouldResemble, []string{"a/d", "a/d/e"})

			repo.assertAllCalled()
		})

		Convey("listInstances works", func() {
			repo.expect(rpcCall{
				method: "ListInstances",
				in: &api.ListInstancesRequest{
					Package:   "a/b/c",
					PageSize:  123,
					PageToken: "zzz",
				},
				out: &api.ListInstancesResponse{
					Instances: []*api.Instance{
						{
							Package: "a/b/c",
							Instance: &api.ObjectRef{
								HashAlgo:  api.HashAlgo_SHA1,
								HexDigest: strings.Repeat("1", 40),
							},
							RegisteredBy: "user:r@example.com",
							RegisteredTs: google.NewTimestamp(epoch.Add(time.Hour)),
						},
						{
							Package: "a/b/c",
							Instance: &api.ObjectRef{
								HashAlgo:  api.HashAlgo_SHA1,
								HexDigest: strings.Repeat("2", 40),
							},
							RegisteredBy: "user:r@example.com",
							RegisteredTs: google.NewTimestamp(epoch),
						},
					},
					NextPageToken: "xxx",
				},
			})

			resp, err := r.listInstances(ctx, "a/b/c", 123, "zzz")
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &listInstancesResponse{
				instances: []InstanceInfo{
					{
						Pin: common.Pin{
							PackageName: "a/b/c",
							InstanceID:  strings.Repeat("1", 40),
						},
						RegisteredBy: "user:r@example.com",
						RegisteredTs: UnixTime(epoch.Add(time.Hour)),
					},
					{
						Pin: common.Pin{
							PackageName: "a/b/c",
							InstanceID:  strings.Repeat("2", 40),
						},
						RegisteredBy: "user:r@example.com",
						RegisteredTs: UnixTime(epoch),
					},
				},
				cursor: "xxx",
			})

			repo.assertAllCalled()
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

type rpcCall struct {
	method string
	in     proto.Message
	out    proto.Message
	err    error
}

type mockedRPCClient struct {
	c        C
	expected []rpcCall
}

func (m *mockedRPCClient) C(c C) {
	m.c = c
}

func (m *mockedRPCClient) expect(r rpcCall) {
	m.expected = append(m.expected, r)
}

func (m *mockedRPCClient) assertAllCalled() {
	m.c.So(m.expected, ShouldHaveLength, 0)
}

func (m *mockedRPCClient) call(method string, in proto.Message, opts []grpc.CallOption) (proto.Message, error) {
	expected := rpcCall{}
	if len(m.expected) != 0 {
		expected = m.expected[0]
		m.expected = m.expected[1:]
	}
	m.c.So(rpcCall{method: method, in: in}, ShouldResemble, rpcCall{method: expected.method, in: expected.in})
	return expected.out, expected.err
}

////////////////////////////////////////////////////////////////////////////////

type mockedStorageClient struct {
	mockedRPCClient
}

func (m *mockedStorageClient) GetObjectURL(ctx context.Context, in *api.GetObjectURLRequest, opts ...grpc.CallOption) (*api.ObjectURL, error) {
	out, err := m.call("GetObjectURL", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.ObjectURL), nil
}

func (m *mockedStorageClient) BeginUpload(ctx context.Context, in *api.BeginUploadRequest, opts ...grpc.CallOption) (*api.UploadOperation, error) {
	out, err := m.call("BeginUpload", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.UploadOperation), nil
}

func (m *mockedStorageClient) FinishUpload(ctx context.Context, in *api.FinishUploadRequest, opts ...grpc.CallOption) (*api.UploadOperation, error) {
	out, err := m.call("FinishUpload", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.UploadOperation), nil
}

func (m *mockedStorageClient) CancelUpload(ctx context.Context, in *api.CancelUploadRequest, opts ...grpc.CallOption) (*api.UploadOperation, error) {
	out, err := m.call("CancelUpload", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.UploadOperation), nil
}

////////////////////////////////////////////////////////////////////////////////

type mockedRepoClient struct {
	mockedRPCClient
}

func (m *mockedRepoClient) GetPrefixMetadata(ctx context.Context, in *api.PrefixRequest, opts ...grpc.CallOption) (*api.PrefixMetadata, error) {
	out, err := m.call("GetPrefixMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.PrefixMetadata), nil
}

func (m *mockedRepoClient) GetInheritedPrefixMetadata(ctx context.Context, in *api.PrefixRequest, opts ...grpc.CallOption) (*api.InheritedPrefixMetadata, error) {
	out, err := m.call("GetInheritedPrefixMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.InheritedPrefixMetadata), nil
}

func (m *mockedRepoClient) UpdatePrefixMetadata(ctx context.Context, in *api.PrefixMetadata, opts ...grpc.CallOption) (*api.PrefixMetadata, error) {
	out, err := m.call("UpdatePrefixMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.PrefixMetadata), nil
}

func (m *mockedRepoClient) GetRolesInPrefix(ctx context.Context, in *api.PrefixRequest, opts ...grpc.CallOption) (*api.RolesInPrefixResponse, error) {
	out, err := m.call("GetRolesInPrefix", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.RolesInPrefixResponse), nil
}

func (m *mockedRepoClient) ListPrefix(ctx context.Context, in *api.ListPrefixRequest, opts ...grpc.CallOption) (*api.ListPrefixResponse, error) {
	out, err := m.call("ListPrefix", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.ListPrefixResponse), nil
}

func (m *mockedRepoClient) HidePackage(ctx context.Context, in *api.PackageRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out, err := m.call("HidePackage", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*empty.Empty), nil
}

func (m *mockedRepoClient) UnhidePackage(ctx context.Context, in *api.PackageRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out, err := m.call("UnhidePackage", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*empty.Empty), nil
}

func (m *mockedRepoClient) RegisterInstance(ctx context.Context, in *api.Instance, opts ...grpc.CallOption) (*api.RegisterInstanceResponse, error) {
	out, err := m.call("RegisterInstance", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.RegisterInstanceResponse), nil
}

func (m *mockedRepoClient) ListInstances(ctx context.Context, in *api.ListInstancesRequest, opts ...grpc.CallOption) (*api.ListInstancesResponse, error) {
	out, err := m.call("ListInstances", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.ListInstancesResponse), nil
}

func (m *mockedRepoClient) SearchInstances(ctx context.Context, in *api.SearchInstancesRequest, opts ...grpc.CallOption) (*api.SearchInstancesResponse, error) {
	out, err := m.call("SearchInstances", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.SearchInstancesResponse), nil
}

func (m *mockedRepoClient) CreateRef(ctx context.Context, in *api.Ref, opts ...grpc.CallOption) (*empty.Empty, error) {
	out, err := m.call("CreateRef", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*empty.Empty), nil
}

func (m *mockedRepoClient) DeleteRef(ctx context.Context, in *api.DeleteRefRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out, err := m.call("DeleteRef", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*empty.Empty), nil
}

func (m *mockedRepoClient) ListRefs(ctx context.Context, in *api.ListRefsRequest, opts ...grpc.CallOption) (*api.ListRefsResponse, error) {
	out, err := m.call("ListRefs", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.ListRefsResponse), nil
}

func (m *mockedRepoClient) AttachTags(ctx context.Context, in *api.AttachTagsRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out, err := m.call("AttachTags", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*empty.Empty), nil
}

func (m *mockedRepoClient) DetachTags(ctx context.Context, in *api.DetachTagsRequest, opts ...grpc.CallOption) (*empty.Empty, error) {
	out, err := m.call("DetachTags", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*empty.Empty), nil
}

func (m *mockedRepoClient) ResolveVersion(ctx context.Context, in *api.ResolveVersionRequest, opts ...grpc.CallOption) (*api.Instance, error) {
	out, err := m.call("ResolveVersion", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.Instance), nil
}

func (m *mockedRepoClient) GetInstanceURL(ctx context.Context, in *api.GetInstanceURLRequest, opts ...grpc.CallOption) (*api.ObjectURL, error) {
	out, err := m.call("GetInstanceURL", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.ObjectURL), nil
}

func (m *mockedRepoClient) DescribeInstance(ctx context.Context, in *api.DescribeInstanceRequest, opts ...grpc.CallOption) (*api.DescribeInstanceResponse, error) {
	out, err := m.call("DescribeInstance", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.DescribeInstanceResponse), nil
}

func (m *mockedRepoClient) DescribeClient(ctx context.Context, in *api.DescribeClientRequest, opts ...grpc.CallOption) (*api.DescribeClientResponse, error) {
	out, err := m.call("DescribeClient", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.DescribeClientResponse), nil
}
