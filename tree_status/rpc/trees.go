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

// Package rpc contains the RPC handlers for the tree status service.
package rpc

import (
	"context"
	"fmt"
	"sort"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/tree_status/internal/config"
	"go.chromium.org/luci/tree_status/internal/perms"
	configpb "go.chromium.org/luci/tree_status/proto/config"
	pb "go.chromium.org/luci/tree_status/proto/v1"
)

type treeServer struct{}

var _ pb.TreesServer = &treeServer{}

// NewTreesServer creates a new server to handle Trees requests.
func NewTreesServer() *pb.DecoratedTrees {
	return &pb.DecoratedTrees{
		Service:  &treeServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// QueryTrees returns trees attached to a given LUCI project.
// If the user does not have access to a tree, that tree will not be returned.
func (*treeServer) QueryTrees(ctx context.Context, request *pb.QueryTreesRequest) (*pb.QueryTreesResponse, error) {
	config, err := config.Get(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "get config").Err()
	}
	treeNames := treeNamesForProject(config, request.Project)
	trees := []*pb.Tree{}
	for _, treeName := range treeNames {
		hasAccess, _, err := perms.HasQueryTreesPermission(ctx, treeName)
		if err != nil {
			return nil, errors.Annotate(err, "checking query tree name permission").Err()
		}
		if hasAccess {
			trees = append(trees, &pb.Tree{
				Name: fmt.Sprintf("trees/%s", treeName),
			})
		}
	}
	return &pb.QueryTreesResponse{
		Trees: trees,
	}, nil
}

func treeNamesForProject(config *configpb.Config, project string) []string {
	results := []string{}
	for _, tree := range config.Trees {
		for _, p := range tree.Projects {
			if p == project {
				results = append(results, tree.Name)
				break
			}
		}
	}
	sort.Strings(results)
	return results
}
