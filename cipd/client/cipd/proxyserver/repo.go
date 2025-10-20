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
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/grpc/prpc"

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
	cipdgrpcpb "go.chromium.org/luci/cipd/api/cipd/v1/grpcpb"
	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
	"go.chromium.org/luci/cipd/common"
)

// RemoteFactory creates a client connected to a remote CIPD backend.
type RemoteFactory func(ctx context.Context, hostname string) (grpc.ClientConnInterface, error)

// ProxyRepositoryServer implements RepositoryServer by proxying calls
// to a set of remote RepositoryServers.
type ProxyRepositoryServer struct {
	cipdgrpcpb.UnimplementedRepositoryServer

	// Policy defines what actions are allowed to be performed through the proxy.
	Policy *proxypb.Policy
	// RemoteFactory creates a client connected to a remote CIPD backend.
	RemoteFactory RemoteFactory
	// CASObfuscator does CAS object URL obfuscation.
	CASURLObfuscator *CASURLObfuscator
	// UserAgent is the proxy's user agent, appended to the client's user agent.
	UserAgent string

	m       sync.RWMutex
	remotes map[string]cipdgrpcpb.RepositoryClient
}

// DefaultRemoteFactory creates a factory that makes real pRPC clients.
func DefaultRemoteFactory(client *http.Client) RemoteFactory {
	return func(ctx context.Context, hostname string) (grpc.ClientConnInterface, error) {
		return &prpc.Client{
			C:    client,
			Host: hostname,
			Options: &prpc.Options{
				// Let the proxy client do retries itself.
				Retry: retry.None,
			},
		}, nil
	}
}

// remote returns a client connected to the given remote CIPD hostname if
// allowed by the policy.
//
// Returns gRPC errors.
func (s *ProxyRepositoryServer) remote(ctx context.Context, hostname string) (cipdgrpcpb.RepositoryClient, error) {
	if hostname == "" {
		return nil, status.Errorf(codes.Internal, "unexpectedly missing remote name in a proxied CIPD call")
	}

	s.m.RLock()
	r := s.remotes[hostname]
	s.m.RUnlock()
	if r != nil {
		return r, nil
	}

	s.m.Lock()
	defer s.m.Unlock()
	if r := s.remotes[hostname]; r != nil {
		return r, nil
	}

	if !slices.Contains(s.Policy.AllowedRemotes, hostname) {
		return nil, status.Errorf(codes.PermissionDenied, "host %q is not allowed by the CIPD proxy policy", hostname)
	}

	conn, err := s.RemoteFactory(ctx, hostname)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to initialize remote in the CIPD proxy: %s", err)
	}
	r = cipdgrpcpb.NewRepositoryClient(conn)

	if s.remotes == nil {
		s.remotes = make(map[string]cipdgrpcpb.RepositoryClient, 1)
	}
	s.remotes[hostname] = r
	return r, nil
}

// callCtx prepares a context for the call to the remote.
func (s *ProxyRepositoryServer) callCtx(ctx context.Context) context.Context {
	userAgent := metadata.ValueFromIncomingContext(ctx, "user-agent")
	if s.UserAgent != "" {
		userAgent = append(userAgent, s.UserAgent)
	}
	return metadata.AppendToOutgoingContext(ctx, "user-agent", strings.Join(userAgent, "; "))
}

// obfuscateObjectURL modifies `obj` in place by replacing SignedUrl with an
// obfuscated value.
//
// Does nothing if `obj` is nil. Returns gRPC errors.
func (s *ProxyRepositoryServer) obfuscateObjectURL(obj *cipdpb.ObjectURL) error {
	if obj != nil {
		var err error
		obj.SignedUrl, err = s.CASURLObfuscator.Obfuscate(&proxypb.ProxiedCASObject{
			SignedUrl: obj.SignedUrl,
		})
		if err != nil {
			return status.Errorf(codes.Internal, "internal CIPD proxy error: %s", err)
		}
	}
	return nil
}

// ResolveVersion implements the corresponding RPC.
func (s *ProxyRepositoryServer) ResolveVersion(ctx context.Context, req *cipdpb.ResolveVersionRequest) (*cipdpb.Instance, error) {
	policy := s.Policy.ResolveVersion
	if policy == nil {
		return nil, deniedByPolicy("ResolveVersion", "")
	}

	switch {
	case common.ValidateInstanceID(req.Version, common.AnyHash) == nil:
		// Instance IDs are always allowed. Resolving them is a noop. In fact the
		// client often doesn't even make this call. It can though, if it wants to
		// check if an instance exists.
	case common.ValidatePackageRef(req.Version) == nil:
		if !policy.AllowRefs {
			return nil, deniedByPolicy("ResolveVersion", "refs are not allowed (resolving %s@%s)", req.Package, req.Version)
		}
	case common.ValidateInstanceTag(req.Version) == nil:
		if !policy.AllowTags {
			return nil, deniedByPolicy("ResolveVersion", "tags are not allowed (resolving %s@%s)", req.Package, req.Version)
		}
	default:
		return nil, status.Errorf(codes.InvalidArgument, "bad version %q: not an instance ID, a ref or a tag", req.Version)
	}

	remote, err := s.remote(ctx, TargetHost(ctx))
	if err != nil {
		return nil, err
	}
	return remote.ResolveVersion(s.callCtx(ctx), req)
}

// GetInstanceURL implements the corresponding RPC.
func (s *ProxyRepositoryServer) GetInstanceURL(ctx context.Context, req *cipdpb.GetInstanceURLRequest) (*cipdpb.ObjectURL, error) {
	policy := s.Policy.GetInstanceUrl
	if policy == nil {
		return nil, deniedByPolicy("GetInstanceURL", "")
	}

	remote, err := s.remote(ctx, TargetHost(ctx))
	if err != nil {
		return nil, err
	}
	resp, err := remote.GetInstanceURL(s.callCtx(ctx), req)
	if err != nil {
		return nil, err
	}

	if err := s.obfuscateObjectURL(resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DescribeClient implements the corresponding RPC.
func (s *ProxyRepositoryServer) DescribeClient(ctx context.Context, req *cipdpb.DescribeClientRequest) (*cipdpb.DescribeClientResponse, error) {
	policy := s.Policy.DescribeClient
	if policy == nil {
		return nil, deniedByPolicy("DescribeClient", "")
	}

	remote, err := s.remote(ctx, TargetHost(ctx))
	if err != nil {
		return nil, err
	}
	resp, err := remote.DescribeClient(s.callCtx(ctx), req)
	if err != nil {
		return nil, err
	}

	if err := s.obfuscateObjectURL(resp.ClientBinary); err != nil {
		return nil, err
	}
	return resp, nil
}

// deniedByPolicy returns a gRPC error describing why a call was denied.
func deniedByPolicy(rpc, explainer string, args ...any) error {
	if explainer == "" {
		explainer = "this RPC is not allowed by the policy"
	} else {
		explainer = fmt.Sprintf(explainer, args...)
	}
	return status.Errorf(codes.PermissionDenied, "CIPD proxy: %s denied: %s", rpc, explainer)
}
