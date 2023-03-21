// Copyright 2023 The LUCI Authors.
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

package rbe

import (
	"context"
	"sync/atomic"

	"google.golang.org/grpc"

	"go.chromium.org/luci/swarming/internal/remoteworkers"
)

type botsConnPool struct {
	next    atomic.Uint32
	clients []remoteworkers.BotsClient
}

// botsConnectionPool returns a BotsClient backed by a simple connection pool.
func botsConnectionPool(cc []grpc.ClientConnInterface) remoteworkers.BotsClient {
	clients := make([]remoteworkers.BotsClient, len(cc))
	for i, conn := range cc {
		clients[i] = remoteworkers.NewBotsClient(conn)
	}
	return &botsConnPool{clients: clients}
}

// get returns the next client to use.
func (p *botsConnPool) get() remoteworkers.BotsClient {
	return p.clients[p.next.Add(1)%uint32(len(p.clients))]
}

// CreateBotSession is a part of remoteworkers.BotsClient interface.
func (p *botsConnPool) CreateBotSession(ctx context.Context, in *remoteworkers.CreateBotSessionRequest, opts ...grpc.CallOption) (*remoteworkers.BotSession, error) {
	return p.get().CreateBotSession(ctx, in, opts...)
}

// UpdateBotSession  is a part of remoteworkers.BotsClient interface.
func (p *botsConnPool) UpdateBotSession(ctx context.Context, in *remoteworkers.UpdateBotSessionRequest, opts ...grpc.CallOption) (*remoteworkers.BotSession, error) {
	return p.get().UpdateBotSession(ctx, in, opts...)
}
