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

// Package buildbucket contains logic of interacting with Buildbucket.
package buildbucket

import (
	"context"
	"net/http"

	"google.golang.org/grpc"

	"go.chromium.org/luci/auth/scopes"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbgrpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/analysis/internal/scopedauth"
)

// testBBClientKey is the context key controls the use of buildbucket client test doubles in tests.
var testBBClientKey = "used in tests only for setting the mock buildbucket client"

type GetBuildsClient interface {
	GetBuild(ctx context.Context, in *bbpb.GetBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error)
}

// useBuildsClientForTesting specifies that the given test double shall be used
// instead of making calls to buildbucket.
func useBuildsClientForTesting(ctx context.Context, client GetBuildsClient) context.Context {
	return context.WithValue(ctx, &testBBClientKey, client)
}

func newBuildsClient(ctx context.Context, host, project string) (GetBuildsClient, error) {
	if mockClient, ok := ctx.Value(&testBBClientKey).(GetBuildsClient); ok {
		return mockClient, nil
	}

	t, err := scopedauth.GetRPCTransport(ctx, project, auth.WithScopes(scopes.BuildbucketScopeSet()...))
	if err != nil {
		return nil, err
	}
	return bbgrpcpb.NewBuildsClient(
		&prpc.Client{
			C:       &http.Client{Transport: t},
			Host:    host,
			Options: prpc.DefaultOptions(),
		}), nil
}

// Client is the client to communicate with Buildbucket.
// It wraps a bbpb.BuildsClient.
type Client struct {
	client GetBuildsClient
}

// NewClient creates a client to communicate with Buildbucket, acting as the given
// LUCI project.
func NewClient(ctx context.Context, host, project string) (*Client, error) {
	client, err := newBuildsClient(ctx, host, project)
	if err != nil {
		return nil, err
	}

	return &Client{
		client: client,
	}, nil
}

// GetBuild returns bbpb.Build for the requested build.
func (c *Client) GetBuild(ctx context.Context, req *bbpb.GetBuildRequest) (*bbpb.Build, error) {
	return c.client.GetBuild(ctx, req)
}
