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
	"net/http"

	"golang.org/x/net/context"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/ledcmd"
)

func launchCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "launch",
		ShortDesc: "launches a JobDefinition on swarming",
		LongDesc: `Launches a given JobDefinition on swarming.

Example:

led get-builder ... |
  led edit ... |
  led launch

If stdout is not a tty (e.g. a file), this command writes a JSON object
containing information about the launched task to stdout.
`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdLaunch{}
			ret.initFlags(opts)
			return ret
		},
	}
}

type cmdLaunch struct {
	cmdBase

	dump bool
}

func (c *cmdLaunch) initFlags(opts cmdBaseOptions) {
	c.Flags.BoolVar(&c.dump, "dump", false, "Dump swarming task to stdout instead of running it.")
	c.cmdBase.initFlags(opts)
}

func (c *cmdLaunch) jobInput() bool                  { return true }
func (c *cmdLaunch) positionalRange() (min, max int) { return 0, 0 }

func (c *cmdLaunch) validateFlags(ctx context.Context, _ []string, _ subcommands.Env) (err error) {
	return
}

func (c *cmdLaunch) execute(ctx context.Context, authClient *http.Client, inJob *job.Definition) (out interface{}, err error) {
	uid, err := ledcmd.GetUID(ctx, c.authenticator)
	if err != nil {
		return nil, err
	}

	task, meta, err := ledcmd.LaunchSwarming(ctx, authClient, inJob, ledcmd.LaunchSwarmingOpts{
		DryRun:          c.dump,
		UserID:          uid,
		FinalBuildProto: "build.proto.json",
		KitchenSupport:  c.kitchenSupport,
	})
	if err != nil {
		return nil, err
	}
	if c.dump {
		return task, nil
	}

	swarmingHostname := inJob.Info().SwarmingHostname()
	logging.Infof(ctx, "Launched swarming task: https://%s/task?id=%s",
		swarmingHostname, meta.TaskId)
	logging.Infof(ctx, "LUCI UI: https://ci.chromium.org/swarming/task/%s?server=%s",
		meta.TaskId, swarmingHostname)

	return nil, nil
}

func (c *cmdLaunch) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
