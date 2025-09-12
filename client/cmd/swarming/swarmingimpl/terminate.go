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
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

// CmdTerminateBot returns an object for the `terminate` subcommand.
func CmdTerminateBot(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "terminate -S <server> <bot ID>",
		ShortDesc: "asks a bot to gracefully terminate",
		LongDesc:  "Asks a bot to gracefully terminate.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &terminateImpl{}, base.Features{
				MinArgs: 1,
				MaxArgs: 1, // TODO(vadimsh): Support more, it is trivial.
				OutputJSON: base.OutputJSON{
					Enabled: false,
				},
			})
		},
	}
}

type terminateImpl struct {
	wait   bool
	reason string
	botID  string
}

func (cmd *terminateImpl) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.wait, "wait", false, "Wait for the bot to terminate.")
	fs.StringVar(&cmd.reason, "reason", "", "A human defined reason given for terminating bot,")
}

func (cmd *terminateImpl) ParseInputs(ctx context.Context, args []string, env subcommands.Env, extra base.Extra) error {
	cmd.botID = args[0]
	return nil
}

func (cmd *terminateImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	res, err := svc.TerminateBot(ctx, cmd.botID, cmd.reason)
	if err != nil {
		return errors.Fmt("failed to terminate bot %s: %w", cmd.botID, err)
	}

	if cmd.wait {
		logging.Infof(ctx, "Waiting for the bot to terminate...")
		taskres, err := swarming.GetOne(ctx, svc, res.TaskId, nil, swarming.WaitAll)
		if err != nil {
			return errors.Fmt("failed when polling task %s: %w", res.TaskId, err)
		}
		if taskres.State != swarmingpb.TaskState_COMPLETED {
			return errors.Fmt("failed to terminate bot ID %s with task state %s", cmd.botID, taskres.State)
		}
	}

	logging.Infof(ctx, "Successfully terminated %s", cmd.botID)
	return nil
}
