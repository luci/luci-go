// Copyright 2022 The LUCI Authors.
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

package swarmingimpl

import (
	"context"
	"flag"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/output"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/swarming/client/swarming"
)

// CmdBotTasks returns an object for the `bot-tasks` subcommand.
func CmdBotTasks(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "bot-tasks -S <server> -id <bot ID>",
		ShortDesc: "lists tasks executed by a bot",
		LongDesc:  "List details of tasks executed by a particular bot.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &botTasksImpl{}, base.Features{
				MinArgs: 0,
				MaxArgs: 0,
				OutputJSON: base.OutputJSON{
					Enabled:             true,
					DeprecatedAliasFlag: "json",
					DefaultToStdout:     true,
				},
			})
		},
	}
}

// TODO(crbug.com/1467263): `fields` do nothing currently. Used to be a set of
// fields to include in a partial response.

type botTasksImpl struct {
	botID  string
	limit  int
	state  string
	fields []string
	start  float64
}

func (cmd *botTasksImpl) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&cmd.botID, "id", "", "Bot ID to query tasks from.")
	fs.IntVar(&cmd.limit, "limit", defaultLimit, "Max number of tasks to return.")
	fs.StringVar(&cmd.state, "state", "ALL", "Task state to filter on.")
	fs.Var(luciflag.StringSlice(&cmd.fields), "field", "This flag currently does nothing (https://crbug.com/1467263).")
	fs.Float64Var(&cmd.start, "start", 0, "Start time (in seconds since the epoch) for counting tasks.")
}

func (cmd *botTasksImpl) ParseInputs(args []string, env subcommands.Env) error {
	if cmd.botID == "" {
		return errors.Reason("non-empty -id required").Err()
	}
	if cmd.limit < 1 {
		return errors.Reason("invalid -limit %d, must be positive", cmd.limit).Err()
	}
	if _, err := stateMap(cmd.state); err != nil {
		return err
	}
	return nil
}

func (cmd *botTasksImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	state, _ := stateMap(cmd.state)
	list, err := svc.ListBotTasks(ctx, cmd.botID, int32(cmd.limit), cmd.start, state)
	if err != nil {
		return err
	}
	return output.List(sink, list)
}
