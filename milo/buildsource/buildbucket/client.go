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
	"errors"
	"fmt"
	"net/http"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/auth"
)

var clientContextKey = "context key for builds client"

// clientFactory is a function that returns a buildbucket rpc client.
type clientFactory func(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (buildbucketpb.BuildsClient, error)

func ProdClientFactory(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (buildbucketpb.BuildsClient, error) {
	t, err := auth.GetRPCTransport(c, as, opts...)
	if err != nil {
		return nil, err
	}
	rpcOpts := prpc.DefaultOptions()
	rpcOpts.PerRPCTimeout = bbRPCTimeout
	return buildbucketpb.NewBuildsPRPCClient(&prpc.Client{
		C:       &http.Client{Transport: t},
		Host:    host,
		Options: rpcOpts,
	}), nil
}

// WithClientFactory installs a buildbucket rpc client in the context.
func WithClientFactory(c context.Context, factory clientFactory) context.Context {
	return context.WithValue(c, &clientContextKey, factory)
}

func buildbucketClient(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (buildbucketpb.BuildsClient, error) {
	factory, ok := c.Value(&clientContextKey).(clientFactory)
	if !ok {
		return nil, fmt.Errorf("no buildbucket client factory found in context")
	}
	return factory(c, host, as, opts...)
}

func getHost(c context.Context) (string, error) {
	settings := common.GetSettings(c)
	if settings.Buildbucket == nil || settings.Buildbucket.Host == "" {
		return "", errors.New("missing buildbucket host in settings")
	}
	return settings.Buildbucket.Host, nil
}
