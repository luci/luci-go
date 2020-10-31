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
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/rts/filegraph"
	"go.chromium.org/luci/rts/filegraph/git"
)

var cmdQuery = &subcommands.Command{
	UsageLine: `query [flags] [PATH [PATH...]]`,
	ShortDesc: "retrieve files in the descending order of distance",
	LongDesc: text.Doc(`
		Retrieve files in the descending order of distance from the given one(s).

		Each PATH must be a file in a git repository.
		All files must be in the same repository.

		Prints files in the order from closest to farthest.
		Each line has format "<distance> <filename>",
		where the filename is relative to the repo root, e.g. "foo/bar.cpp".
		Does not print unreachable files.
	`),
	CommandRun: func() subcommands.CommandRun {
		r := &queryRun{}
		r.Flags.BoolVar(&r.printDirs, "print-dirs", false, "print directories too")
		return r
	},
}

type queryRun struct {
	baseCommandRun
	printDirs bool
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

	g, err := git.Read(ctx, repoDir)
	if err != nil {
		return r.done(err)
	}

	q := filegraph.Query{
		Roots: make([]filegraph.Node, len(paths)),
	}
	for i, p := range paths {
		n := g.Node(p)
		if n == nil {
			return r.done(errors.Reason("node %q not found", p).Err())
		}
		q.Roots[i] = n
	}

	return r.done(q.Run(g, func(res *filegraph.Result) error {
		if r.printDirs || res.Node.IsFile() {
			fmt.Printf("%.2f %s\n", res.Distance, res.Node.Name())
		}
		return nil
	}))
}
