// Copyright 2020 The LUCI Authors.
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

package cli

import (
	"fmt"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/rts/filegraph"
)

var cmdQuery = &subcommands.Command{
	UsageLine: `query`,
	ShortDesc: "query",
	CommandRun: func() subcommands.CommandRun {
		r := &queryRun{}
		r.Flags.BoolVar(&r.backwards, "back", false, "Backwards")
		return r
	},
}

type queryRun struct {
	baseCommandRun
	backwards bool
}

func (r *queryRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if len(args) == 0 {
		return 0
	}

	repoDir, paths, err := filesToPaths(args...)
	if err != nil {
		return r.done(err)
	}

	g, err := filegraph.LoadGraphFromRepo(ctx, repoDir)
	if err != nil {
		return r.done(err)
	}

	q := filegraph.Query{
		Roots:     make([]*filegraph.Node, len(paths)),
		Backwards: r.backwards,
	}
	for i, p := range paths {
		n := g.Node(p)
		if n == nil {
			return r.done(errors.Reason("%q not found", paths[0]).Err())
		}
		q.Roots[i] = n
	}

	return r.done(g.Query(q, func(res *filegraph.ResultItem) error {
		if res.Node.IsTreeLeaf() {
			fmt.Printf("%.4f %s\n", filegraph.Distance(res.Relevance), res.Node.Path)
		}
		return nil
	}))
}
