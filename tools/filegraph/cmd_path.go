package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
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

	g, err := loadGraphFromRepo(ctx, repoDir)
	if err != nil {
		return r.done(err)
	}

	from := g.getNode(paths[0])
	if from == nil {
		return r.done(errors.Reason("%q not found", paths[0]).Err())
	}

	to := g.getNode(paths[1])
	if from == nil {
		return r.done(errors.Reason("%q not found", paths[1]).Err())
	}

	shorest, err := g.shortestPath(from, to, r.backwards)
	if err != nil {
		return r.done(err)
	}
	if shorest == nil {
		return r.done(errors.Reason("not reachable").Err())
	}

	for _, r := range shorest.path() {
		fmt.Printf("%.4f: %s\n", Distance(r.relevance), r.n.path)
	}
	return 0
}

func filesToPaths(filePaths ...string) (repoDir string, paths []Path, err error) {
	if _, err = ensureSameRepo(filePaths...); err != nil {
		return
	}

	repoDir, err = gitRepoRoot(filePaths[0])
	if err != nil {
		return
	}

	prefix := filepath.ToSlash(filepath.Clean(repoDir))

	paths = make([]Path, len(filePaths))
	for i, f := range filePaths {
		f, err = filepath.Abs(f)
		if err != nil {
			return
		}
		f = filepath.Clean(f)
		f = filepath.ToSlash(f)
		rel := strings.TrimPrefix(f, prefix)
		rel = strings.Trim(rel, "/")
		paths[i] = ParsePath(rel)
	}
	return
}
