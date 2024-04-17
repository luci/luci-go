// Copyright 2024 The LUCI Authors.
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

// Package bbtaskbackend contains the implementation of a Buildbucket Taskbackend.
package bbtaskbackend

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/server/auth"
)

// TaskBackend implements bbpb.TaskBackendServer.
type TaskBackend struct {
	bbpb.UnimplementedTaskBackendServer

	bbTarget string
}

// Ensure TaskBackend implements bbpb.TaskBackendServer.
var _ bbpb.TaskBackendServer = &TaskBackend{}

// NewTaskBackend returns a new bbpb.TaskBackendServer.
func NewTaskBackend(target string) bbpb.TaskBackendServer {
	return &TaskBackend{bbTarget: target}
}

// TaskBackendAuthInterceptor checks if the request caller can use Taskbackend.
func TaskBackendAuthInterceptor(isDev bool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		if strings.HasPrefix(info.FullMethod, "/buildbucket.v2.TaskBackend") {
			s := auth.GetState(ctx)
			if s == nil {
				return "", status.Errorf(codes.Internal, "the auth state is not properly configured")
			}
			// Check if requests are from Buildbucket.
			switch peer := s.PeerIdentity(); {
			case isDev && peer.Value() == "cr-buildbucket-dev@appspot.gserviceaccount.com": // on Dev
			case peer.Value() == "cr-buildbucket@appspot.gserviceaccount.com": // on Prod
			default:
				return "", status.Errorf(codes.PermissionDenied, "peer %q is not allowed to access this task backend", peer)
			}
			// In TaskBackend protocal, Buildbucket should use Project-scoped identity.
			user := s.User().Identity
			if user.Kind() != identity.Project {
				return "", status.Errorf(codes.PermissionDenied, "caller's user identity %q is not a project identity", user)
			}
		}
		return handler(ctx, req)
	}
}
