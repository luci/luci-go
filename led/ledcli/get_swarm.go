// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ledcli

import (
	"fmt"
	"net/http"

	"golang.org/x/net/context"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/ledcmd"
)

func getSwarmCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-swarm <swarm task id>",
		ShortDesc: "obtain a JobDefinition from a swarming task",
		LongDesc:  `Obtains the task definition from swarming and produce a JobDefinition.`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdGetSwarm{}
			ret.initFlags(opts)
			return ret
		},
	}
}

type cmdGetSwarm struct {
	cmdBase

	taskID       string
	swarmingHost string
	pinMachine   bool
}

func (c *cmdGetSwarm) initFlags(opts cmdBaseOptions) {
	c.Flags.StringVar(&c.swarmingHost, "S", "chromium-swarm.appspot.com",
		"the swarming `host` to get the task from.")

	c.Flags.BoolVar(&c.pinMachine, "pin-machine", false,
		"Pin the dimensions of the JobDefinition to run on the same machine.")

	c.cmdBase.initFlags(opts)
}

func (c *cmdGetSwarm) jobInput() bool                  { return false }
func (c *cmdGetSwarm) positionalRange() (min, max int) { return 1, 1 }

func (c *cmdGetSwarm) validateFlags(ctx context.Context, positionals []string, env subcommands.Env) error {
	c.taskID = positionals[0]
	return errors.Annotate(validateHost(c.swarmingHost), "swarming host").Err()
}

func (c *cmdGetSwarm) execute(ctx context.Context, authClient *http.Client, inJob *job.Definition) (out interface{}, err error) {
	return ledcmd.GetFromSwarmingTask(ctx, authClient, ledcmd.GetFromSwarmingTaskOpts{
		Name:           fmt.Sprintf("led get-swarm %s", c.taskID),
		PinBot:         c.pinMachine,
		SwarmingHost:   c.swarmingHost,
		TaskID:         c.taskID,
		KitchenSupport: c.kitchenSupport,
	})
}

func (c *cmdGetSwarm) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
