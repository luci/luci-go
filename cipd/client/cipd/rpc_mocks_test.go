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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"google.golang.org/grpc"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
)

// This file has no tests, but contains definition of mocks for cas.Storage and
// cipd.Repository RPC clients used by tests.

type rpcCall struct {
	method string
	in     proto.Message
	out    proto.Message
	err    error
}

type mockedRPCClient struct {
	c        C
	expected []rpcCall
	many     *rpcCall
}

func (m *mockedRPCClient) C(c C) {
	m.c = c
}

func (m *mockedRPCClient) expect(r rpcCall) {
	m.expected = append(m.expected, r)
}

func (m *mockedRPCClient) expectMany(r rpcCall) {
	m.assertAllCalled()
	m.many = &r
}

func (m *mockedRPCClient) assertAllCalled() {
	if m.many == nil {
		m.c.So(m.expected, ShouldHaveLength, 0)
	}
}

func (m *mockedRPCClient) call(method string, in proto.Message, opts []grpc.CallOption) (proto.Message, error) {
	expected := rpcCall{}
	if m.many != nil {
		expected = *m.many
	} else {
		if len(m.expected) != 0 {
			expected = m.expected[0]
			m.expected = m.expected[1:]
		}
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

func (m *mockedRepoClient) GetRolesInPrefixOnBehalfOf(ctx context.Context, in *api.PrefixRequestOnBehalfOf, opts ...grpc.CallOption) (*api.RolesInPrefixResponse, error) {
	out, err := m.call("GetRolesInPrefixOnBehalfOf", in, opts)
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

func (m *mockedRepoClient) HidePackage(ctx context.Context, in *api.PackageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("HidePackage", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) UnhidePackage(ctx context.Context, in *api.PackageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("UnhidePackage", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) DeletePackage(ctx context.Context, in *api.PackageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("DeletePackage", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
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

func (m *mockedRepoClient) CreateRef(ctx context.Context, in *api.Ref, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("CreateRef", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) DeleteRef(ctx context.Context, in *api.DeleteRefRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("DeleteRef", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) ListRefs(ctx context.Context, in *api.ListRefsRequest, opts ...grpc.CallOption) (*api.ListRefsResponse, error) {
	out, err := m.call("ListRefs", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.ListRefsResponse), nil
}

func (m *mockedRepoClient) AttachTags(ctx context.Context, in *api.AttachTagsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("AttachTags", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) DetachTags(ctx context.Context, in *api.DetachTagsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("DetachTags", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) AttachMetadata(ctx context.Context, in *api.AttachMetadataRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("AttachMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) DetachMetadata(ctx context.Context, in *api.DetachMetadataRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("DetachMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) ListMetadata(ctx context.Context, in *api.ListMetadataRequest, opts ...grpc.CallOption) (*api.ListMetadataResponse, error) {
	out, err := m.call("ListMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.ListMetadataResponse), nil
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

func (m *mockedRepoClient) DescribeBootstrapBundle(ctx context.Context, in *api.DescribeBootstrapBundleRequest, opts ...grpc.CallOption) (*api.DescribeBootstrapBundleResponse, error) {
	out, err := m.call("DescribeBootstrapBundle", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*api.DescribeBootstrapBundleResponse), nil
}
