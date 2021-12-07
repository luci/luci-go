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

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
)

// CmdCancelTask returns an object for the `cancel` subcommand.
func CmdCancelTask(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "cancel <options> <taskID>",
		ShortDesc: "cancel a task",
		LongDesc:  "Cancels the task specified by the taskID",
		CommandRun: func() subcommands.CommandRun {
			r := &cancelRun{}
			r.Init(authFlags)
			return r
		},
	}
}

type cancelRun struct {
	commonFlags
	killRunning bool
}

func (t *cancelRun) Init(authFlags AuthFlags) {
	t.commonFlags.Init(authFlags)
	t.Flags.BoolVar(&t.killRunning, "kill-running", false, "Kill the task even if it's running")
}

func (t *cancelRun) parse(taskIDs []string) error {
	if err := t.commonFlags.Parse(); err != nil {
		return err
	}

	if len(taskIDs) == 0 {
		return errors.New("must specify a swarming task ID")
	}

	if len(taskIDs) > 1 {
		return errors.New("please specify only one swarming task ID")
	}

	return nil
}

func (t *cancelRun) cancelTask(ctx context.Context, taskID string, service swarmingService) error {
	req := &swarming.SwarmingRpcsTaskCancelRequest{
		KillRunning: t.killRunning,
	}
	res, err := service.CancelTask(ctx, taskID, req)
	if res != nil && !res.Ok {
		err = errors.Reason("response was not OK. running=%v\n", res.WasRunning).Err()
	}
	if err != nil {
		return errors.Annotate(err, "failed to cancel task %s\n", taskID).Err()
	}

	fmt.Printf("Cancelled %s\n", taskID)
	return nil

}

func (t *cancelRun) main(_ subcommands.Application, taskID string) error {
	ctx, cancel := context.WithCancel(t.defaultFlags.MakeLoggingContext(os.Stderr))
	defer signals.HandleInterrupt(cancel)()
	service, err := t.createSwarmingClient(ctx)
	if err != nil {
		return err
	}
	return t.cancelTask(ctx, taskID, service)
}

func (t *cancelRun) Run(a subcommands.Application, taskIDs []string, _ subcommands.Env) int {
	if err := t.parse(taskIDs); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := t.main(a, taskIDs[0]); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
