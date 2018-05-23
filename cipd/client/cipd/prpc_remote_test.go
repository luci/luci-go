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
	"context"
	"testing"
	"time"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/common/proto/google"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
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

		repo := mockedRepoClient{}
		repo.C(c)

		r := &prpcRemoteImpl{repo: &repo}

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

func (m *mockedRepoClient) RegisterInstance(ctx context.Context, in *api.Instance, opts ...grpc.CallOption) (*api.RegisterInstanceResponse, error) {
	out, err := m.call("RegisterInstance", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.RegisterInstanceResponse), nil
}
