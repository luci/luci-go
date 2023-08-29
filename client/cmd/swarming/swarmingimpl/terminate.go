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
	"fmt"
	"os"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/signals"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

// CmdTerminateBot returns an object for the `terminate` subcommand.
func CmdTerminateBot(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "terminate <options> <botID>",
		ShortDesc: "terminate a bot",
		LongDesc:  "Asks the bot specified by the botID to terminate itself gracefully",
		CommandRun: func() subcommands.CommandRun {
			r := &terminateRun{}
			r.Init(authFlags)
			return r
		},
	}
}

type terminateRun struct {
	commonFlags
	wait   bool
	reason string
}

func (t *terminateRun) Init(authFlags AuthFlags) {
	t.commonFlags.Init(authFlags)
	t.Flags.BoolVar(&t.wait, "wait", false, "Wait for the bot to terminate")
	t.Flags.StringVar(&t.reason, "reason", "", "A human defined reason given for terminating bot")
}

func (t *terminateRun) parse(botIDs []string) error {
	if err := t.commonFlags.Parse(); err != nil {
		return err
	}

	if len(botIDs) == 0 {
		return errors.New("must specify a swarming bot id")
	}

	if len(botIDs) > 1 {
		return errors.New("please specify only one swarming bot id")
	}

	return nil
}

func pollTask(ctx context.Context, taskID string, service swarmingService) (*swarmingv2.TaskResultResponse, error) {
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

func (t *terminateRun) terminateBot(ctx context.Context, botID string, service swarmingService) error {

	res, err := service.TerminateBot(ctx, botID, t.reason)

	if err != nil {
		return errors.Annotate(err, "failed to terminate bot %s\n", botID).Err()
	}

	if t.wait {
		taskres, err := pollTask(ctx, res.TaskId, service)
		if err != nil {
			return errors.Annotate(err, "failed when polling task %s\n", res.TaskId).Err()
		}
		if !TaskIsCompleted(taskres.State) {
			return errors.Reason("failed to terminate bot ID %s with task state %s", botID, taskres.State).Err()
		}
	}

	return nil

}

func (t *terminateRun) main(a subcommands.Application, botID string) error {
	ctx, cancel := context.WithCancel(t.defaultFlags.MakeLoggingContext(os.Stderr))
	defer signals.HandleInterrupt(cancel)()
	service, err := t.createSwarmingClient(ctx)
	if err != nil {
		return err
	}
	if err := t.terminateBot(ctx, botID, service); err != nil {
		return err
	}

	if !t.defaultFlags.Quiet {
		fmt.Fprintf(a.GetOut(), "Successfully terminated %s\n", botID)
	}

	return nil
}

func (t *terminateRun) Run(a subcommands.Application, botIDs []string, _ subcommands.Env) int {
	if err := t.parse(botIDs); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := t.main(a, botIDs[0]); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
