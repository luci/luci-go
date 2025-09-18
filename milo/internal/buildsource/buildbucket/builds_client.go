// Copyright 2019 The LUCI Authors.
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

	bbgrpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
)

var buildsClientContextKey = "context key for builds client"

// buildsClientFactory is a function that returns a buildbucket rpc builds
// client.
type buildsClientFactory func(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (bbgrpcpb.BuildsClient, error)

func ProdBuildsClientFactory(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (bbgrpcpb.BuildsClient, error) {
	t, err := auth.GetRPCTransport(c, as, opts...)
	if err != nil {
		return nil, err
	}
	rpcOpts := prpc.DefaultOptions()
	rpcOpts.PerRPCTimeout = bbRPCTimeout
	return bbgrpcpb.NewBuildsClient(&prpc.Client{
		C:       &http.Client{Transport: t},
		Host:    host,
		Options: rpcOpts,
	}), nil
}

// WithBuildsClientFactory installs a buildbucket rpc builds client in the
// context.
func WithBuildsClientFactory(c context.Context, factory buildsClientFactory) context.Context {
	return context.WithValue(c, &buildsClientContextKey, factory)
}

func BuildsClient(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (bbgrpcpb.BuildsClient, error) {
	factory, ok := c.Value(&buildsClientContextKey).(buildsClientFactory)
	if !ok {
		return nil, fmt.Errorf("no buildbucket builds client factory found in context")
	}
	return factory(c, host, as, opts...)
}
