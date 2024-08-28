// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package workflow

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/system/environ"

	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/core"
)

// Generator is a general purpose generator which generates dependencies and
// computes cross-compile tuples recursively.
// This Generator implements:
//   - Add runtime dependencies to action metadata.
//   - Recursively generating all its dependencies.
//   - Add environment variables for the list of different types of dependencies.
//   - Convert generators.Platforms based on the types of the dependency for
//     cross-compiling. e.g. A static libary for host platform should be
//     DepsHostTarget dependency, while a libary for build platform should be
//     DepsBuildHost.
//
// This Generator implementation is preferred to avoid manually handling
// dependencies.
type Generator struct {
	Name         string
	Metadata     *core.Action_Metadata
	Args         []string
	Env          environ.Env
	Dependencies []generators.Dependency
}

// Generate will recursively generates the *core.Action from its content.
// The output directories of dependencies will be added to environment variables
// for build scripts to interact with.
func (g *Generator) Generate(ctx context.Context, plats generators.Platforms) (*core.Action, error) {
	metadata := &core.Action_Metadata{}
	if g.Metadata != nil {
		metadata = proto.Clone(g.Metadata).(*core.Action_Metadata)
	}

	env := g.Env.Clone()

	var deps []*core.Action
	envDeps := make(map[string][]string)
	for _, d := range g.Dependencies {
		a, err := d.Generate(ctx, plats)
		if err != nil {
			return nil, err
		}
		deps = append(deps, a)
		if d.Runtime {
			metadata.RuntimeDeps = append(metadata.RuntimeDeps, a)
		}
		envDeps[d.Type.String()] = append(envDeps[d.Type.String()], fmt.Sprintf("{{.%s}}", a.Name))
		env.Set(a.Name, fmt.Sprintf("{{.%s}}", a.Name))
	}

	// Add dependencies' environment variables.
	for dType, deps := range envDeps {
		env.Set(dType, strings.Join(deps, string(os.PathListSeparator)))
	}

	return &core.Action{
		Name:     g.Name,
		Metadata: metadata,
		Deps:     deps,
		Spec: &core.Action_Command{
			Command: &core.ActionCommand{
				Args: g.Args,
				Env:  env.Sorted(),
			},
		},
	}, nil
}
