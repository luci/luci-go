// Copyright 2021 The LUCI Authors.
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
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/swarming/client/swarming"
)

// CmdCancelTask returns an object for the `cancel` subcommand.
func CmdCancelTask(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "cancel -S <server> <task ID>",
		ShortDesc: "cancels a task",
		LongDesc:  "Cancels a pending or running (if -kill-running is set) task.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &cancelImpl{}, base.Features{
				MinArgs: 1,
				MaxArgs: 1, // TODO(vadimsh): Support more, it is trivial.
				OutputJSON: base.OutputJSON{
					Enabled: false,
				},
			})
		},
	}
}

type cancelImpl struct {
	killRunning bool
	taskID      string
}

func (cmd *cancelImpl) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.killRunning, "kill-running", false, "Kill the task even if it's running.")
}

func (cmd *cancelImpl) ParseInputs(ctx context.Context, args []string, env subcommands.Env, extra base.Extra) error {
	cmd.taskID = args[0]
	return nil
}

func (cmd *cancelImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	res, err := svc.CancelTask(ctx, cmd.taskID, cmd.killRunning)
	if res != nil && !res.Canceled {
		err = errors.Reason("task was not canceled. running=%v\n", res.WasRunning).Err()
	}
	if err != nil {
		return errors.Annotate(err, "failed to cancel task %s\n", cmd.taskID).Err()
	}
	logging.Infof(ctx, "Canceled %s", cmd.taskID)
	return nil
}
