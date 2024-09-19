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
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/tree_status/internal/config"
	"go.chromium.org/luci/tree_status/internal/perms"
	"go.chromium.org/luci/tree_status/pbutil"
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

// GetTree returns a tree given a tree name.
func (*treeServer) GetTree(ctx context.Context, request *pb.GetTreeRequest) (*pb.Tree, error) {
	treeID, err := pbutil.ParseTreeName(request.Name)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "name").Err())
	}
	ctx = logging.SetField(ctx, "tree_id", treeID)
	config, err := config.Get(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "get config").Err()
	}
	for _, tree := range config.Trees {
		if tree.Name == treeID {
			hasAccess, msg, err := perms.HasGetTreePermission(ctx, treeID)
			if err != nil {
				return nil, errors.Annotate(err, "has get tree permission").Err()
			}
			if !hasAccess {
				return nil, permissionDeniedError(errors.New(msg))
			}
			return &pb.Tree{
				Name:     request.Name,
				Projects: tree.Projects,
			}, nil
		}
	}
	return nil, notFoundError(errors.Reason("tree %q not found", treeID).Err())
}

// QueryTrees returns trees attached to a given LUCI project.
// If the user does not have access to a tree, that tree will not be returned.
func (*treeServer) QueryTrees(ctx context.Context, request *pb.QueryTreesRequest) (*pb.QueryTreesResponse, error) {
	config, err := config.Get(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "get config").Err()
	}
	configTrees := treesForProject(config, request.Project)
	trees := []*pb.Tree{}
	for _, configTree := range configTrees {
		treeName := configTree.Name
		hasAccess, _, err := perms.HasQueryTreesPermission(ctx, treeName)
		if err != nil {
			return nil, errors.Annotate(err, "checking query tree name permission").Err()
		}
		if hasAccess {
			trees = append(trees, &pb.Tree{
				Name:     fmt.Sprintf("trees/%s", treeName),
				Projects: configTree.Projects,
			})
		}
	}
	return &pb.QueryTreesResponse{
		Trees: trees,
	}, nil
}

func treesForProject(config *configpb.Config, project string) []*configpb.Tree {
	results := []*configpb.Tree{}
	for _, tree := range config.Trees {
		for _, p := range tree.Projects {
			if p == project {
				results = append(results, tree)
				break
			}
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results
}
