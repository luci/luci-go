package main

import (
	"fmt"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
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

	repoDir, paths, err := filesToPaths(args...)
	if err != nil {
		return r.done(err)
	}

	g, err := loadGraphFromRepo(ctx, repoDir)
	if err != nil {
		return r.done(err)
	}

	q := query{
		roots:     make([]*node, len(paths)),
		backwards: r.backwards,
	}
	for i, p := range paths {
		n := g.getNode(p)
		if n == nil {
			return r.done(errors.Reason("%q not found", paths[0]).Err())
		}
		q.roots[i] = n
	}

	return r.done(g.Query(q, func(res *queryResult) error {
		if res.n.isTreeLeaf() {
			fmt.Printf("%.4f %s\n", Distance(res.relevance), res.n.path)
		}
		return nil
	}))
}
