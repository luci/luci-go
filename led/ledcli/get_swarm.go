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

package ledcli

import (
	"context"
	"fmt"
	"net/http"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
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
	pinBotID     bool
	priorityDiff int
}

func (c *cmdGetSwarm) initFlags(opts cmdBaseOptions) {
	c.Flags.StringVar(&c.swarmingHost, "S", "chromium-swarm.appspot.com",
		"the swarming `host` to get the task from.")

	c.Flags.BoolVar(&c.pinBotID, "pin-bot-id", false,
		"Pin the bot id in the generated job Definition's dimensions.")

	c.Flags.IntVar(&c.priorityDiff, "adjust-priority", 10,
		"Increase or decrease the priority of the generated job. Note: priority works like Unix 'niceness'; Higher values indicate lower priority.")

	c.cmdBase.initFlags(opts)
}

func (c *cmdGetSwarm) jobInput() bool                  { return false }
func (c *cmdGetSwarm) positionalRange() (min, max int) { return 1, 1 }

func (c *cmdGetSwarm) validateFlags(ctx context.Context, positionals []string, env subcommands.Env) error {
	c.taskID = positionals[0]
	return errors.WrapIf(pingHost(c.swarmingHost), "swarming host")
}

func (c *cmdGetSwarm) execute(ctx context.Context, authClient *http.Client, _ auth.Options, inJob *job.Definition) (out any, err error) {
	return ledcmd.GetFromSwarmingTask(ctx, authClient, nil, ledcmd.GetFromSwarmingTaskOpts{
		Name:         fmt.Sprintf("led get-swarm %s", c.taskID),
		PinBotID:     c.pinBotID,
		SwarmingHost: c.swarmingHost,
		TaskID:       c.taskID,
		PriorityDiff: c.priorityDiff,

		KitchenSupport: c.kitchenSupport,
	})
}

func (c *cmdGetSwarm) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
