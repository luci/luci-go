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
)

// mockResultDBClientKey is the context key indicates using mocked resultb client in tests.
var mockResultDBClientKey = "used in tests only for setting the mock resultdb client"

func newResultDBClient(ctx context.Context, host string) (rdbpb.ResultDBClient, error) {
	if mockClient, ok := ctx.Value(&mockResultDBClientKey).(*rdbpb.MockResultDBClient); ok {
		return mockClient, nil
	}

	t, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	return rdbpb.NewResultDBPRPCClient(
		&prpc.Client{
			C:                &http.Client{Transport: t},
			Host:             host,
			Options:          prpc.DefaultOptions(),
			MaxContentLength: 100 * 1000 * 1000, // 100 MiB.
		}), nil
}

// Client is the client to communicate with ResultDB.
// It wraps a rdbpb.ResultDBClient.
type Client struct {
	client rdbpb.ResultDBClient
}

// NewClient creates a client to communicate with ResultDB.
func NewClient(ctx context.Context, host string) (*Client, error) {
	client, err := newResultDBClient(ctx, host)
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

// BatchGetTestVariants retrieves the requested test variants.
func (c *Client) BatchGetTestVariants(ctx context.Context, req *rdbpb.BatchGetTestVariantsRequest) ([]*rdbpb.TestVariant, error) {
	rsp, err := c.client.BatchGetTestVariants(ctx, req)
	if err != nil {
		return nil, err
	}
	return rsp.GetTestVariants(), nil
}
