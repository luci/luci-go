// Copyright 2024 The LUCI Authors.
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
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/output"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/swarming/client/swarming"
)

// CmdCancelTasks returns an object for the `cancel-tasks` subcommand.
func CmdCancelTasks(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "cancel-tasks -S <server> -tag t:v1 -tag t:v2",
		ShortDesc: "cancels tasks matching tags",
		LongDesc:  "Mass cancels tasks. Only cancels running tasks if -kill-running is set.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &cancelTasksImpl{}, base.Features{
				MinArgs: 0,
				MaxArgs: 0,
				OutputJSON: base.OutputJSON{
					Enabled: false,
				},
			})
		},
	}
}

type cancelTasksImpl struct {
	killRunning bool
	limit       int64
	tags        []string
	start       time.Time
	end         time.Time
}

func (cmd *cancelTasksImpl) RegisterFlags(fs *flag.FlagSet) {
	// TODO(b:320541589): Add cancel-all flag to cancel all the target tasks without a limit.
	fs.BoolVar(&cmd.killRunning, "kill-running", false, "Kill the tasks even if it's running.")
	fs.Int64Var(&cmd.limit, "limit", defaultLimit, "Maximum number of tasks to cancel.")
	fs.Var(luciflag.StringSlice(&cmd.tags), "tag", "Tag attached to the task. May be repeated.")
	fs.Var(luciflag.Time(&cmd.start), "start", "Only tasks later or equal to start will be cancelled. No effect if unset. e.g:2024-01-16T12:34:56Z")
	fs.Var(luciflag.Time(&cmd.end), "end", "Only tasks earlier or equal to end will be cancelled. No effect if unset. e.g:2024-01-16T13:24:56Z")
}

func (cmd *cancelTasksImpl) ParseInputs(args []string, env subcommands.Env) error {
	if cmd.limit < 1 {
		return errors.Reason("invalid -limit %d, must be positive", cmd.limit).Err()
	}
	if cmd.limit > defaultLimit && len(cmd.tags) == 0 {
		return errors.Reason("invalid -limit %d, cannot be larger than %d when no tags is specified", cmd.limit, defaultLimit).Err()
	}

	// check start and end time.
	if !cmd.start.IsZero() && !cmd.end.IsZero() && cmd.start.After(cmd.end) {
		return errors.Reason("invalid -start -end, start time %v cannot be after end time %v", cmd.start, cmd.end).Err()
	}
	return nil
}

func (cmd *cancelTasksImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	_, err := svc.CancelTasks(ctx, int32(cmd.limit), cmd.tags, cmd.killRunning, cmd.start, cmd.end)
	if err != nil {
		return errors.Annotate(err, "failed to cancel all tasks\n").Err()
	}
	logging.Infof(ctx, "Cancel tasks request submitted without error.")
	return nil
}
