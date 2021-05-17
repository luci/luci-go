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

package lib

import (
	"context"
	"fmt"
	"os"

	"github.com/maruel/subcommands"
	"golang.org/x/sync/semaphore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
)

// CmdTerminate returns an object for the `bots` subcommand.
func CmdTerminate(authFlags AuthFlags) *subcommands.Command {
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
	wait bool
}

func (t *terminateRun) Init(authFlags AuthFlags) {
	t.commonFlags.Init(authFlags)
	t.Flags.BoolVar(&t.wait, "wait", false, "Wait for the bot to terminate")
	t.Flags.BoolVar(&t.wait, "w", false, "Alias for -wait")
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

func (t *terminateRun) terminateBot(ctx context.Context, botID string, service swarmingService) error {

	res, err := service.TerminateBot(ctx, botID)

	if err != nil {
		fmt.Printf("Failed to terminate %s\n", botID)
		return err
	}
	if res == nil {
		return errors.Reason("no response from service when asking bot %s to terminate itself", botID).Err()
	}

	if t.wait {
		c := collectRun{}
		downloadSem := semaphore.NewWeighted(int64(len(res.TaskId)))
		taskres := c.pollForTaskResult(ctx, res.TaskId, service, downloadSem)
		if taskres.err != nil {
			fmt.Printf("Failed to terminate %s\n", botID)
			return taskres.err
		}
		if taskres.result.State != "COMPLETED" {
			return errors.New("terminate couldn't complete")
		}
	}

	fmt.Printf("Successfully terminated %s\n", botID)

	return nil

}

func (t *terminateRun) main(_ subcommands.Application, botID string) error {
	ctx, cancel := context.WithCancel(t.defaultFlags.MakeLoggingContext(os.Stderr))
	defer signals.HandleInterrupt(cancel)()
	service, err := t.createSwarmingClient(ctx)
	if err != nil {
		return err
	}
	return t.terminateBot(ctx, botID, service)
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
