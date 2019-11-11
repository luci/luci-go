package main

import (
	"fmt"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/cli"
)

var cmdStats = &subcommands.Command{
	UsageLine: `stats`,
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

	g, err := loadGraphFromRepo(ctx, repoDir)
	if err != nil {
		return r.done(err)
	}

	files := 0
	g.visit(func(n *node) error {
		if n.isTreeLeaf() {
			files++
		}

		for _, a := range n.adjacent
		return nil
	})

	fmt.Printf("files: %d\n", files)
	fmt.Printf("edges: %d\n", len(g.weight))
	return 0
}
