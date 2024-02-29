// Copyright 2019 The LUCI Authors.
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

// CmdTasks returns an object for the `tasks` subcommand.
func CmdTasks(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "tasks -S <server>",
		ShortDesc: "lists or counts tasks matching a filter",
		LongDesc:  "Lists or counts tasks matching a filter.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &tasksImpl{}, base.Features{
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

const defaultLimit = 200

// TODO(crbug.com/1467263): `fields` do nothing currently. Used to be a set of
// fields to include in a partial response.

type tasksImpl struct {
	limit  int64
	state  string
	tags   []string
	fields []string
	count  bool
	start  float64
}

func (cmd *tasksImpl) RegisterFlags(fs *flag.FlagSet) {
	fs.Int64Var(&cmd.limit, "limit", defaultLimit, "Maximum number of tasks to retrieve.")
	fs.StringVar(&cmd.state, "state", "ALL", "Only include tasks in the specified state.")
	fs.Var(luciflag.StringSlice(&cmd.tags), "tag", "Tag attached to the task. May be repeated.")
	fs.Var(luciflag.StringSlice(&cmd.fields), "field", "This flag currently does nothing (https://crbug.com/1467263).")
	fs.BoolVar(&cmd.count, "count", false, "Report the count of tasks instead of listing them.")
	fs.Float64Var(&cmd.start, "start", 0, "Start time (in seconds since the epoch) for counting tasks.")
}

func (cmd *tasksImpl) ParseInputs(args []string, env subcommands.Env) error {
	if cmd.limit < 1 {
		return errors.Reason("invalid -limit %d, must be positive", cmd.limit).Err()
	}
	if cmd.count {
		if len(cmd.fields) > 0 {
			return errors.Reason("-field cannot be used with -count").Err()
		}
		if cmd.limit != defaultLimit {
			return errors.Reason("-limit cannot be used with -count").Err()
		}
		if cmd.start <= 0 {
			return errors.Reason("with -count, must provide -start >0").Err()
		}
	}
	if _, err := stateMap(cmd.state); err != nil {
		return errors.Annotate(err, "bad -state").Err()
	}
	return nil
}

func (cmd *tasksImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	state, _ := stateMap(cmd.state)

	if cmd.count {
		count, err := svc.CountTasks(ctx, cmd.start, state, cmd.tags)
		if err != nil {
			return err
		}
		return output.Proto(sink, count)
	}

	list, err := svc.ListTasks(ctx, int32(cmd.limit), cmd.start, state, cmd.tags)
	if err != nil {
		return err
	}
	return output.List(sink, list)
}
