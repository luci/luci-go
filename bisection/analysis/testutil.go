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

package analysis

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

type FakeTestVariantBranchesClient struct {
	TestVariantBranches []*pb.TestVariantBranch
	BatchGetErr         error
}

func (c *FakeTestVariantBranchesClient) BatchGet(ctx context.Context, req *pb.BatchGetTestVariantBranchRequest, opts ...grpc.CallOption) (*pb.BatchGetTestVariantBranchResponse, error) {
	if c.BatchGetErr != nil {
		return nil, c.BatchGetErr
	}
	response := &pb.BatchGetTestVariantBranchResponse{}

	itemsByKey := make(map[string]*pb.TestVariantBranch)
	for _, item := range c.TestVariantBranches {
		itemsByKey[item.Name] = item
	}

	for _, name := range req.Names {
		resultItem, ok := itemsByKey[name]
		if !ok {
			return nil, status.Errorf(codes.NotFound, "test variant branch with name %s not found", name)
		}
		response.TestVariantBranches = append(response.TestVariantBranches, resultItem)
	}

	return response, nil
}

func UseFakeClient(ctx context.Context, client *FakeTestVariantBranchesClient) context.Context {
	return context.WithValue(ctx, &mockTVBClientKey, client)
}
