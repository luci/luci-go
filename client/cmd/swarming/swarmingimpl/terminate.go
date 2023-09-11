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
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/swarming/client/swarming"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
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

func (cmd *terminateImpl) ParseInputs(args []string, env subcommands.Env) error {
	cmd.botID = args[0]
	return nil
}

func pollTask(ctx context.Context, taskID string, service swarming.Client) (*swarmingv2.TaskResultResponse, error) {
	for {
		res, err := service.TaskResult(ctx, taskID, false)
		if err != nil {
			return res, errors.Annotate(err, "failed to get task result").Err()
		}

		if err != nil {
			return res, errors.Annotate(err, "failed to parse task state").Err()
		}
		if !TaskIsAlive(res.State) {
			return res, nil
		}

		delay := 5 * time.Second

		logging.Debugf(ctx, "Waiting %s for task: %s", delay, taskID)
		timerResult := <-clock.After(ctx, delay)

		if timerResult.Err != nil {
			return res, errors.Annotate(err, "failed to wait for task").Err()
		}
	}
}

func (cmd *terminateImpl) Execute(ctx context.Context, svc swarming.Client, extra base.Extra) (any, error) {
	res, err := svc.TerminateBot(ctx, cmd.botID, cmd.reason)
	if err != nil {
		return nil, errors.Annotate(err, "failed to terminate bot %s", cmd.botID).Err()
	}

	if cmd.wait {
		taskres, err := pollTask(ctx, res.TaskId, svc)
		if err != nil {
			return nil, errors.Annotate(err, "failed when polling task %s", res.TaskId).Err()
		}
		if !TaskIsCompleted(taskres.State) {
			return nil, errors.Reason("failed to terminate bot ID %s with task state %s", cmd.botID, taskres.State).Err()
		}
	}

	logging.Infof(ctx, "Successfully terminated %s", cmd.botID)
	return nil, nil
}
