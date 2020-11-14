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
	"path/filepath"
	"strings"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/rts/filegraph"
	"go.chromium.org/luci/rts/filegraph/internal/git"
)

var cmdPath = &subcommands.Command{
	UsageLine: `path`,
	ShortDesc: "path",
	CommandRun: func() subcommands.CommandRun {
		r := &pathRun{}
		r.Flags.BoolVar(&r.backwards, "back", false, "Backwards")
		return r
	},
}

type pathRun struct {
	baseCommandRun
	backwards bool
}

func (r *pathRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if len(args) != 2 {
		return r.done(errors.Reason("usage: filegraph path <from> <to>").Err())
	}

	repoDir, paths, err := filesToPaths(args[0], args[1])
	if err != nil {
		return r.done(err)
	}

	g, err := filegraph.LoadGraphFromRepo(ctx, repoDir)
	if err != nil {
		return r.done(err)
	}

	from := g.Node(paths[0])
	if from == nil {
		return r.done(errors.Reason("%q not found", paths[0]).Err())
	}

	to := g.Node(paths[1])
	if from == nil {
		return r.done(errors.Reason("%q not found", paths[1]).Err())
	}

	shorest, err := g.ShortestPath(from, to, r.backwards)
	if err != nil {
		return r.done(err)
	}
	if shorest == nil {
		return r.done(errors.Reason("not reachable").Err())
	}

	for _, r := range shorest.Path() {
		fmt.Printf("%.4f (D=%.4f): %s\n", r.Relevance, filegraph.Distance(r.Relevance), r.Node.Path)
	}
	return 0
}

func filesToPaths(filePaths ...string) (repoDir string, paths []filegraph.Path, err error) {
	if _, err = ensureSameRepo(filePaths...); err != nil {
		return
	}

	repoDir, err = git.RepoRoot(filePaths[0])
	if err != nil {
		return
	}

	prefix := filepath.ToSlash(filepath.Clean(repoDir))

	paths = make([]filegraph.Path, len(filePaths))
	for i, f := range filePaths {
		f, err = filepath.Abs(f)
		if err != nil {
			return
		}
		f = filepath.Clean(f)
		f = filepath.ToSlash(f)
		rel := strings.TrimPrefix(f, prefix)
		rel = strings.Trim(rel, "/")
		paths[i] = filegraph.ParsePath(rel)
	}
	return
}
