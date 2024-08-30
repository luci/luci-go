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

package gitilesutil

import (
	"context"
	"net/http"

	"go.chromium.org/luci/common/api/gitiles"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/server/auth"
)

// testClientFactoryKey is the context key controls the use of gitiles client
// test doubles in tests.
var testClientFactoryKey = "used in tests only for setting the fake gitiles client factory"

// UseFakeClientFactory specifies that the given test double shall be used
// instead of making calls to gitiles.
func UseFakeClientFactory(ctx context.Context, clientFactory clientFactory) context.Context {
	return context.WithValue(ctx, &testClientFactoryKey, clientFactory)
}

// clientFactory is a function that returns a gitiles RPC client.
type clientFactory func(ctx context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (gitilespb.GitilesClient, error)

// realClientFactory creates a new Gitiles client that talks to a real Gitiles
// host.
func realClientFactory(ctx context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (gitilespb.GitilesClient, error) {
	t, err := auth.GetRPCTransport(ctx, as, opts...)
	if err != nil {
		return nil, err
	}
	return gitiles.NewRESTClient(&http.Client{Transport: t}, host, false)
}

// NewClient creates a new Gitiles client.
func NewClient(ctx context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (gitilespb.GitilesClient, error) {
	testClientFactory, ok := ctx.Value(&testClientFactoryKey).(clientFactory)
	if ok {
		return testClientFactory(ctx, host, as, opts...)
	}

	return realClientFactory(ctx, host, as, opts...)
}
