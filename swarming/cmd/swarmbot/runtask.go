package main

import (
	"fmt"

	"github.com/maruel/subcommands"
)

// run_isolated.py in Go
var cmdRunTask = &subcommands.Command{
	UsageLine: "runtask <options>...",
	ShortDesc: "runs swarming task",
	CommandRun: func() subcommands.CommandRun {
		return &cmdRunTaskRun{}
	},
}

type cmdRunTaskRun struct {
	subcommands.CommandRunBase
}

func (c *cmdRunTaskRun) Run(
	a subcommands.Application, args []string, env subcommands.Env) int {
	// TODO(crbug.com/962804): implement here
	fmt.Println("running swarming task...")
	return 0
}
