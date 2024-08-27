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

package config

import (
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/tree_status/pbutil"
	configpb "go.chromium.org/luci/tree_status/proto/config"
)

func validateConfig(ctx *validation.Context, cfg *configpb.Config) {
	ctx.Enter("trees")
	defer ctx.Exit()

	// Make sure each tree name is unique.
	treeMap := map[string][]int{}
	// Make sure each project is mapped to at most 1 tree.
	projectMap := map[string][]string{}

	for i, tree := range cfg.Trees {
		ctx.Enter("[%d]", i)
		validateTree(ctx, tree)
		ctx.Exit()
		treeMap[tree.Name] = append(treeMap[tree.Name], i)
		for _, project := range tree.Projects {
			projectMap[project] = append(projectMap[project], tree.Name)
		}
	}

	for treeName, treeIndices := range treeMap {
		if len(treeIndices) > 1 {
			ctx.Errorf("tree name %q is reused at indices %v", treeName, treeIndices)
		}
	}

	for project, treeNames := range projectMap {
		if len(treeNames) > 1 {
			ctx.Errorf("project %q is in more than one tree: %v", project, treeNames)
		}
	}
}

func validateTree(ctx *validation.Context, tree *configpb.Tree) {
	ctx.Enter("name")
	if err := pbutil.ValidateTreeName(tree.Name); err != nil {
		ctx.Error(err)
	}
	ctx.Exit()

	ctx.Enter("projects")
	for i, project := range tree.Projects {
		ctx.Enter("[%d]", i)
		if err := pbutil.ValidateProject(project); err != nil {
			ctx.Error(err)
		}
		ctx.Exit()
	}
	ctx.Exit()

	ctx.Enter("use_default_acls")
	if !tree.UseDefaultAcls && len(tree.Projects) == 0 {
		ctx.Errorf("projects must not be empty when not using default ACL")
	}
	ctx.Exit()
}
