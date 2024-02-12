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

// Package accounts contains Accounts server implementation.
package accounts

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/auth_service/api/rpcpb"
)

// Server implements Accounts server.
type Server struct {
	rpcpb.UnimplementedAccountsServer
}

// GetSelf implements the corresponding RPC method.
func (*Server) GetSelf(ctx context.Context, _ *emptypb.Empty) (*rpcpb.SelfInfo, error) {
	state := auth.GetState(ctx)
	return &rpcpb.SelfInfo{
		Identity: string(state.User().Identity),
		Ip:       state.PeerIP().String(),
	}, nil
}
