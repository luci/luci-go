// Copyright 2025 The LUCI Authors.
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

package proxyserver

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/prpc"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	repogrpcpb "go.chromium.org/luci/cipd/api/cipd/v1/repopb/grpcpb"
	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
)

func TestProxyRepositoryServer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	repoImpl := &repoImpl{}

	remote := &prpctest.Server{}
	repogrpcpb.RegisterRepositoryServer(remote, repoImpl)
	remote.Start(ctx)
	defer remote.Close()

	obfuscator := NewCASURLObfuscator()

	local := &prpctest.Server{}
	repogrpcpb.RegisterRepositoryServer(local, &ProxyRepositoryServer{
		Policy: &proxypb.Policy{
			AllowedRemotes: []string{"allowed"},
			ResolveVersion: &proxypb.Policy_ResolveVersionPolicy{AllowTags: true},
			GetInstanceUrl: &proxypb.Policy_GetInstanceURLPolicy{},
			DescribeClient: &proxypb.Policy_DescribeClientPolicy{},
		},
		RemoteFactory: func(ctx context.Context, hostname string) (grpc.ClientConnInterface, error) {
			assert.That(t, hostname, should.Equal("allowed"))
			return remote.NewClient()
		},
		CASURLObfuscator: obfuscator,
		UserAgent:        "proxy-user-agent",
	})
	// Mock ":authority" header to make TargetHost return what we want. Without
	// this we'd need to replace http.DefaultTransport in the pRPC client with
	// something more complicated.
	local.UnaryServerInterceptor = func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		if val := md.Get("fake-authority"); len(val) != 0 {
			md.Set(":authority", val...)
		}
		return handler(metadata.NewIncomingContext(ctx, md), req)
	}
	local.Start(ctx)
	defer local.Close()

	callCtx := func(target string) context.Context {
		return metadata.NewOutgoingContext(ctx, metadata.MD{
			"fake-authority": {target},
		})
	}

	prpcC, err := local.NewClientWithOptions(&prpc.Options{
		UserAgent: "client-user-agent",
	})
	assert.NoErr(t, err)
	repoC := repogrpcpb.NewRepositoryClient(prpcC)

	t.Run("ResolveVersion: OK", func(t *testing.T) {
		resp, err := repoC.ResolveVersion(callCtx("allowed"), &repopb.ResolveVersionRequest{
			Package: "some/pkg",
			Version: "some:tag",
		})
		assert.NoErr(t, err)
		assert.That(t, resp, should.Match(&repopb.Instance{
			Package: "some/pkg",
			Instance: &caspb.ObjectRef{
				HashAlgo:  caspb.HashAlgo_SHA256,
				HexDigest: "fake-digest",
			},
		}))
		assert.That(t, repoImpl.lastUserAgent, should.Equal("client-user-agent; proxy-user-agent"))
	})

	t.Run("ResolveVersion: wrong host", func(t *testing.T) {
		_, err := repoC.ResolveVersion(callCtx("unknown"), &repopb.ResolveVersionRequest{
			Package: "some/pkg",
			Version: "some:tag",
		})
		assert.That(t, status.Code(err), should.Equal(codes.PermissionDenied))
		assert.That(t, err, should.ErrLike(`host "unknown" is not allowed by the CIPD proxy policy`))
	})

	t.Run("ResolveVersion: forbidden ref", func(t *testing.T) {
		_, err := repoC.ResolveVersion(callCtx("allowed"), &repopb.ResolveVersionRequest{
			Package: "some/pkg",
			Version: "some-ref",
		})
		assert.That(t, status.Code(err), should.Equal(codes.PermissionDenied))
		assert.That(t, err, should.ErrLike(`CIPD proxy: ResolveVersion denied: refs are not allowed (resolving some/pkg@some-ref)`))
	})

	t.Run("GetInstanceURL", func(t *testing.T) {
		resp, err := repoC.GetInstanceURL(callCtx("allowed"), &repopb.GetInstanceURLRequest{
			Package: "some/pkg",
			Instance: &caspb.ObjectRef{
				HashAlgo:  caspb.HashAlgo_SHA256,
				HexDigest: "fake-digest",
			},
		})
		assert.NoErr(t, err)

		original, err := obfuscator.Unobfuscate(resp.SignedUrl)
		assert.NoErr(t, err)
		assert.That(t, original.SignedUrl, should.Equal("http://cas.example.com/some/pkg"))
	})

	t.Run("DescribeClient", func(t *testing.T) {
		resp, err := repoC.DescribeClient(callCtx("allowed"), &repopb.DescribeClientRequest{})
		assert.NoErr(t, err)

		original, err := obfuscator.Unobfuscate(resp.ClientBinary.SignedUrl)
		assert.NoErr(t, err)
		assert.That(t, original.SignedUrl, should.Equal("http://cas.example.com/client"))
	})
}

type repoImpl struct {
	repogrpcpb.UnimplementedRepositoryServer

	lastUserAgent string
}

func (r *repoImpl) ResolveVersion(ctx context.Context, req *repopb.ResolveVersionRequest) (*repopb.Instance, error) {
	r.lastUserAgent = ""
	if val := metadata.ValueFromIncomingContext(ctx, "user-agent"); len(val) > 0 {
		r.lastUserAgent = val[0]
	}
	return &repopb.Instance{
		Package: req.Package,
		Instance: &caspb.ObjectRef{
			HashAlgo:  caspb.HashAlgo_SHA256,
			HexDigest: "fake-digest",
		},
	}, nil
}

func (r *repoImpl) GetInstanceURL(ctx context.Context, req *repopb.GetInstanceURLRequest) (*caspb.ObjectURL, error) {
	return &caspb.ObjectURL{
		SignedUrl: "http://cas.example.com/" + req.Package,
	}, nil
}

func (r *repoImpl) DescribeClient(ctx context.Context, req *repopb.DescribeClientRequest) (*repopb.DescribeClientResponse, error) {
	return &repopb.DescribeClientResponse{
		ClientBinary: &caspb.ObjectURL{
			SignedUrl: "http://cas.example.com/client",
		},
	}, nil
}
