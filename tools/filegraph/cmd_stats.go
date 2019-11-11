// Copyright 2019 The LUCI Authors.
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

package main

import (
	"fmt"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/cli"
)

var cmdStats = &subcommands.Command{
	UsageLine: `stats`,
	ShortDesc: `prints basic graph stats.`,
	CommandRun: func() subcommands.CommandRun {
		r := &statsRun{}
		r.Flags.StringVar(&r.dir, "dir", ".", "path to the git repository")
		return r
	},
}

type statsRun struct {
	baseCommandRun
	dir string
}

func (r *statsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	repoDir, err := gitRepoRoot(r.dir)
	if err != nil {
		return r.done(err)
	}

	g, err := loadGraph(ctx, repoDir)
	if err != nil {
		return r.done(err)
	}

	files := 0
	g.visit(func(n *node) error {
		if n.isTreeLeaf() {
			files++
		}
		return nil
	})

	fmt.Printf("files: %d\n", files)
	fmt.Printf("edges: %d\n", len(g.edgeWeights))
	return 0
}
