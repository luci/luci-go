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

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(rpcCall{}))
}

// This file has no tests, but contains definition of mocks for cas.Storage and
// ciintipd.Repository RPC clients used by tests.

type rpcCall struct {
	method string
	in     proto.Message
	out    proto.Message
	err    error
}

type mockedRPCClient struct {
	t        testing.TB
	expected []rpcCall
	many     *rpcCall
}

func (m *mockedRPCClient) TB(t testing.TB) {
	m.t = t
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
		assert.Loosely(m.t, m.expected, should.BeEmpty)
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
	assert.That(m.t, rpcCall{method: method, in: in},
		should.Match(rpcCall{method: expected.method, in: expected.in}))
	return expected.out, expected.err
}

////////////////////////////////////////////////////////////////////////////////

type mockedStorageClient struct {
	mockedRPCClient
}

func (m *mockedStorageClient) GetObjectURL(ctx context.Context, in *caspb.GetObjectURLRequest, opts ...grpc.CallOption) (*caspb.ObjectURL, error) {
	out, err := m.call("GetObjectURL", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*caspb.ObjectURL), nil
}

func (m *mockedStorageClient) BeginUpload(ctx context.Context, in *caspb.BeginUploadRequest, opts ...grpc.CallOption) (*caspb.UploadOperation, error) {
	out, err := m.call("BeginUpload", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*caspb.UploadOperation), nil
}

func (m *mockedStorageClient) FinishUpload(ctx context.Context, in *caspb.FinishUploadRequest, opts ...grpc.CallOption) (*caspb.UploadOperation, error) {
	out, err := m.call("FinishUpload", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*caspb.UploadOperation), nil
}

func (m *mockedStorageClient) CancelUpload(ctx context.Context, in *caspb.CancelUploadRequest, opts ...grpc.CallOption) (*caspb.UploadOperation, error) {
	out, err := m.call("CancelUpload", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*caspb.UploadOperation), nil
}

////////////////////////////////////////////////////////////////////////////////

type mockedRepoClient struct {
	mockedRPCClient
}

func (m *mockedRepoClient) GetPrefixMetadata(ctx context.Context, in *repopb.PrefixRequest, opts ...grpc.CallOption) (*repopb.PrefixMetadata, error) {
	out, err := m.call("GetPrefixMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.PrefixMetadata), nil
}

func (m *mockedRepoClient) GetInheritedPrefixMetadata(ctx context.Context, in *repopb.PrefixRequest, opts ...grpc.CallOption) (*repopb.InheritedPrefixMetadata, error) {
	out, err := m.call("GetInheritedPrefixMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.InheritedPrefixMetadata), nil
}

func (m *mockedRepoClient) UpdatePrefixMetadata(ctx context.Context, in *repopb.PrefixMetadata, opts ...grpc.CallOption) (*repopb.PrefixMetadata, error) {
	out, err := m.call("UpdatePrefixMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.PrefixMetadata), nil
}

func (m *mockedRepoClient) GetRolesInPrefix(ctx context.Context, in *repopb.PrefixRequest, opts ...grpc.CallOption) (*repopb.RolesInPrefixResponse, error) {
	out, err := m.call("GetRolesInPrefix", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.RolesInPrefixResponse), nil
}

func (m *mockedRepoClient) GetRolesInPrefixOnBehalfOf(ctx context.Context, in *repopb.PrefixRequestOnBehalfOf, opts ...grpc.CallOption) (*repopb.RolesInPrefixResponse, error) {
	out, err := m.call("GetRolesInPrefixOnBehalfOf", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.RolesInPrefixResponse), nil
}

func (m *mockedRepoClient) ListPrefix(ctx context.Context, in *repopb.ListPrefixRequest, opts ...grpc.CallOption) (*repopb.ListPrefixResponse, error) {
	out, err := m.call("ListPrefix", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.ListPrefixResponse), nil
}

func (m *mockedRepoClient) HidePackage(ctx context.Context, in *repopb.PackageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("HidePackage", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) UnhidePackage(ctx context.Context, in *repopb.PackageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("UnhidePackage", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) DeletePackage(ctx context.Context, in *repopb.PackageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("DeletePackage", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) RegisterInstance(ctx context.Context, in *repopb.Instance, opts ...grpc.CallOption) (*repopb.RegisterInstanceResponse, error) {
	out, err := m.call("RegisterInstance", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.RegisterInstanceResponse), nil
}

func (m *mockedRepoClient) ListInstances(ctx context.Context, in *repopb.ListInstancesRequest, opts ...grpc.CallOption) (*repopb.ListInstancesResponse, error) {
	out, err := m.call("ListInstances", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.ListInstancesResponse), nil
}

func (m *mockedRepoClient) SearchInstances(ctx context.Context, in *repopb.SearchInstancesRequest, opts ...grpc.CallOption) (*repopb.SearchInstancesResponse, error) {
	out, err := m.call("SearchInstances", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.SearchInstancesResponse), nil
}

func (m *mockedRepoClient) CreateRef(ctx context.Context, in *repopb.Ref, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("CreateRef", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) DeleteRef(ctx context.Context, in *repopb.DeleteRefRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("DeleteRef", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) ListRefs(ctx context.Context, in *repopb.ListRefsRequest, opts ...grpc.CallOption) (*repopb.ListRefsResponse, error) {
	out, err := m.call("ListRefs", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.ListRefsResponse), nil
}

func (m *mockedRepoClient) AttachTags(ctx context.Context, in *repopb.AttachTagsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("AttachTags", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) DetachTags(ctx context.Context, in *repopb.DetachTagsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("DetachTags", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) AttachMetadata(ctx context.Context, in *repopb.AttachMetadataRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("AttachMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) DetachMetadata(ctx context.Context, in *repopb.DetachMetadataRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out, err := m.call("DetachMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*emptypb.Empty), nil
}

func (m *mockedRepoClient) ListMetadata(ctx context.Context, in *repopb.ListMetadataRequest, opts ...grpc.CallOption) (*repopb.ListMetadataResponse, error) {
	out, err := m.call("ListMetadata", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.ListMetadataResponse), nil
}

func (m *mockedRepoClient) ResolveVersion(ctx context.Context, in *repopb.ResolveVersionRequest, opts ...grpc.CallOption) (*repopb.Instance, error) {
	out, err := m.call("ResolveVersion", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.Instance), nil
}

func (m *mockedRepoClient) GetInstanceURL(ctx context.Context, in *repopb.GetInstanceURLRequest, opts ...grpc.CallOption) (*caspb.ObjectURL, error) {
	out, err := m.call("GetInstanceURL", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*caspb.ObjectURL), nil
}

func (m *mockedRepoClient) DescribeInstance(ctx context.Context, in *repopb.DescribeInstanceRequest, opts ...grpc.CallOption) (*repopb.DescribeInstanceResponse, error) {
	out, err := m.call("DescribeInstance", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.DescribeInstanceResponse), nil
}

func (m *mockedRepoClient) DescribeClient(ctx context.Context, in *repopb.DescribeClientRequest, opts ...grpc.CallOption) (*repopb.DescribeClientResponse, error) {
	out, err := m.call("DescribeClient", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.DescribeClientResponse), nil
}

func (m *mockedRepoClient) DescribeBootstrapBundle(ctx context.Context, in *repopb.DescribeBootstrapBundleRequest, opts ...grpc.CallOption) (*repopb.DescribeBootstrapBundleResponse, error) {
	out, err := m.call("DescribeBootstrapBundle", in, opts)
	if err != nil {
		return nil, err
	}
	return out.(*repopb.DescribeBootstrapBundleResponse), nil
}
