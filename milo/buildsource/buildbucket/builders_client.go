// Copyright 2020 The LUCI Authors.
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

package buildbucket

import (
	"context"
	"fmt"
	"net/http"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
)

var buildersClientContextKey = "context key for builders client"

// buildersClientFactory is a function that returns a buildbucket builders rpc
// client.
type buildersClientFactory func(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (buildbucketpb.BuildersClient, error)

func ProdBuildersClientFactory(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (buildbucketpb.BuildersClient, error) {
	t, err := auth.GetRPCTransport(c, as, opts...)
	if err != nil {
		return nil, err
	}
	rpcOpts := prpc.DefaultOptions()
	rpcOpts.PerRPCTimeout = bbRPCTimeout
	return buildbucketpb.NewBuildersPRPCClient(&prpc.Client{
		C:       &http.Client{Transport: t},
		Host:    host,
		Options: rpcOpts,
	}), nil
}

// WithBuildersClientFactory installs a buildbucket rpc builders client in the
// context.
func WithBuildersClientFactory(c context.Context, factory buildersClientFactory) context.Context {
	return context.WithValue(c, &buildersClientContextKey, factory)
}

func BuildersClient(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (buildbucketpb.BuildersClient, error) {
	factory, ok := c.Value(&buildersClientContextKey).(buildersClientFactory)
	if !ok {
		return nil, fmt.Errorf("no buildbucket builders client factory found in context")
	}
	return factory(c, host, as, opts...)
}
