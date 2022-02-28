// Copyright 2022 The LUCI Authors.
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

// Package rpcs contains implementation of pRPC services.
package rpcs

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/deploy/api/rpcpb"
)

// Deployments is an implementation of deploy.service.Deployments service.
type Deployments struct {
	rpcpb.UnimplementedDeploymentsServer
}

func (*Deployments) SayHi(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	logging.Infof(ctx, "Hello %s", auth.CurrentIdentity(ctx))
	return &emptypb.Empty{}, nil
}
