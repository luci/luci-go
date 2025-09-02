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

// Package resultdb contains logic of interacting with resultdb.
package resultdb

import (
	"context"
	"net/http"

	"go.chromium.org/luci/grpc/prpc"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/analysis/internal/scopedauth"
)

// mockResultDBClientKey is the context key indicates using mocked resultb client in tests.
var mockResultDBClientKey = "used in tests only for setting the mock resultdb client"

func newResultDBClient(ctx context.Context, host string, createTransport func() (http.RoundTripper, error)) (rdbpb.ResultDBClient, error) {
	if mockClient, ok := ctx.Value(&mockResultDBClientKey).(rdbpb.ResultDBClient); ok {
		return mockClient, nil
	}

	t, err := createTransport()
	if err != nil {
		return nil, err
	}

	return rdbpb.NewResultDBPRPCClient(
		&prpc.Client{
			C:               &http.Client{Transport: t},
			Host:            host,
			Options:         prpc.DefaultOptions(),
			MaxResponseSize: 100 * 1000 * 1000, // 100 MiB.
		}), nil
}

// Client is the client to communicate with ResultDB.
// It wraps a rdbpb.ResultDBClient.
type Client struct {
	client rdbpb.ResultDBClient
}

// NewClient creates a client to communicate with ResultDB, acting as
// the given project. Recommended way to construct a ResultDB client.
func NewClient(ctx context.Context, host, project string) (*Client, error) {
	createTransport := func() (http.RoundTripper, error) {
		return scopedauth.GetRPCTransport(ctx, project)
	}

	client, err := newResultDBClient(ctx, host, createTransport)
	if err != nil {
		return nil, err
	}

	return &Client{
		client: client,
	}, nil
}

// NewCredentialForwardingClient creates a client to communicate with ResultDB,
// forwarding the credentials of the caller who made the current
// request.
func NewCredentialForwardingClient(ctx context.Context, host string) (*Client, error) {
	createTransport := func() (http.RoundTripper, error) {
		return auth.GetRPCTransport(ctx, auth.AsCredentialsForwarder)
	}

	client, err := newResultDBClient(ctx, host, createTransport)
	if err != nil {
		return nil, err
	}

	return &Client{
		client: client,
	}, nil
}

// NewPrivilegedClient creates a client to communicate with ResultDB,
// acting as LUCI Analysis to access data from any project.
//
// Caution: Callers must take special care to avoid "confused deputy"
// issues when using this client, e.g. being tricked by one project
// to access the resources of another. ResultDB will not check the
// accessed resource is in the project that was intended.
func NewPrivilegedClient(ctx context.Context, host string) (*Client, error) {
	createTransport := func() (http.RoundTripper, error) {
		return auth.GetRPCTransport(ctx, auth.AsSelf)
	}

	client, err := newResultDBClient(ctx, host, createTransport)
	if err != nil {
		return nil, err
	}

	return &Client{
		client: client,
	}, nil
}

// QueryTestVariants queries a single page of test variants.
func (c *Client) QueryTestVariants(ctx context.Context, req *rdbpb.QueryTestVariantsRequest) (*rdbpb.QueryTestVariantsResponse, error) {
	return c.client.QueryTestVariants(ctx, req)
}

// QueryRunTestVerdicts queries a single page of test variants from a test run.
func (c *Client) QueryRunTestVerdicts(ctx context.Context, req *rdbpb.QueryRunTestVerdictsRequest) (*rdbpb.QueryRunTestVerdictsResponse, error) {
	return c.client.QueryRunTestVerdicts(ctx, req)
}

// QueryArtifacts queries a single page of test artifacts.
func (c *Client) QueryArtifacts(ctx context.Context, req *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
	return c.client.QueryArtifacts(ctx, req)
}

// GetInvocation retrieves the invocation.
func (c *Client) GetInvocation(ctx context.Context, invName string) (*rdbpb.Invocation, error) {
	inv, err := c.client.GetInvocation(ctx, &rdbpb.GetInvocationRequest{
		Name: invName,
	})
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// QueryTestMetadata queries a single page of test metadata.
func (c *Client) QueryTestMetadata(ctx context.Context, req *rdbpb.QueryTestMetadataRequest) (*rdbpb.QueryTestMetadataResponse, error) {
	return c.client.QueryTestMetadata(ctx, req)
}
