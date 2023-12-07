// Copyright 2021 The LUCI Authors.
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

package redisconn

import (
	"context"

	"github.com/gomodule/redigo/redis"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/redisconn/adminpb"
)

const adminGroup = "administrators"

type adminServer struct {
	adminpb.UnimplementedAdminServer

	pool *redis.Pool
}

func (srv *adminServer) do(ctx context.Context, cmd string, args ...any) (any, error) {
	switch admin, err := auth.IsMember(ctx, adminGroup); {
	case err != nil:
		logging.Errorf(ctx, "Failed to check ACL: %s", err)
		return nil, status.Errorf(codes.Internal, "failed to check ACL")
	case !admin:
		return nil, status.Errorf(codes.PermissionDenied, "not an admin")
	}

	logging.Warningf(ctx, "Redis admin: %q is calling %q", auth.CurrentIdentity(ctx), cmd)

	conn, err := srv.pool.GetContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get redis connection: %s", err)
	}
	defer conn.Close()

	reply, err := conn.Do(cmd, args...)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%q command failed: %s", cmd, err)
	}
	return reply, nil
}

func (srv *adminServer) FlushAll(ctx context.Context, req *adminpb.FlushAllRequest) (*emptypb.Empty, error) {
	var err error
	if req.Async {
		_, err = srv.do(ctx, "FLUSHALL", "ASYNC")
	} else {
		_, err = srv.do(ctx, "FLUSHALL")
	}
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
