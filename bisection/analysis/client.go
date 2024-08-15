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

// Package analysis provides a client for interacting with LUCI Analysis, and
// providing fake data for testing.
package analysis

import (
	"context"
	"net/http"

	"google.golang.org/grpc"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
)

// mockTVBClientKey is the context key indicates using LUCI Analysis test variant branches client in tests.
var mockTVBClientKey = "used in tests only for setting the mock luci analysis test variant branches client"

type client interface {
	BatchGet(ctx context.Context, req *analysispb.BatchGetTestVariantBranchRequest, opts ...grpc.CallOption) (*analysispb.BatchGetTestVariantBranchResponse, error)
}

// TestVariantBranchesClient is the client to communicate with LUCI Analysis Test Variant Branches service.
type TestVariantBranchesClient struct {
	client client
}

// NewTestVariantBranchesClient creates a client to communicate with the LUCI Analysis
// Test Variant Branches service, acting as the given project.
func NewTestVariantBranchesClient(ctx context.Context, host, project string) (*TestVariantBranchesClient, error) {
	if mockClient, ok := ctx.Value(&mockTVBClientKey).(*FakeTestVariantBranchesClient); ok {
		return &TestVariantBranchesClient{client: mockClient}, nil
	}
	tr, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(project))
	if err != nil {
		return nil, err
	}
	client := analysispb.NewTestVariantBranchesPRPCClient(
		&prpc.Client{
			C:                &http.Client{Transport: tr},
			Host:             host,
			Options:          prpc.DefaultOptions(),
			MaxContentLength: 100 * 1000 * 1000, // 100 MiB.
		})

	return &TestVariantBranchesClient{
		client: client,
	}, nil
}

// BatchGet retrieves the a list of test variant branches and their segments.
func (c *TestVariantBranchesClient) BatchGet(ctx context.Context, req *analysispb.BatchGetTestVariantBranchRequest) (*analysispb.BatchGetTestVariantBranchResponse, error) {
	return c.client.BatchGet(ctx, req)
}
