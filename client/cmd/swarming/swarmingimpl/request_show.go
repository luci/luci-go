// Copyright 2015 The LUCI Authors.
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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
)

// CmdRequestShow returns an object for the `request-show` subcommand.
func CmdRequestShow(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "request-show <task_id>",
		ShortDesc: "returns properties of a request",
		LongDesc:  "Returns the properties, what, when, by who, about a request on the Swarming server.",
		CommandRun: func() subcommands.CommandRun {
			r := &requestShowRun{}
			r.commonFlags.Init(authFlags)
			return r
		},
	}
}

type requestShowRun struct {
	commonFlags
}

func (c *requestShowRun) Parse(_ subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 1 {
		return errors.Reason("must only provide a task id").Err()
	}
	return nil
}

func (c *requestShowRun) main(_ subcommands.Application, taskID string) error {
	ctx, cancel := context.WithCancel(c.defaultFlags.MakeLoggingContext(os.Stderr))
	defer cancel()
	defer signals.HandleInterrupt(cancel)()

	service, err := c.createSwarmingClient(ctx)
	if err != nil {
		return err
	}

	request, err := service.TaskRequest(ctx, taskID)
	if err != nil {
		return errors.Annotate(err, fmt.Sprintf("failed to get task request. task ID = %s", taskID)).Err()
	}
	b, err := DefaultProtoMarshalOpts.Marshal(request)
	if err != nil {
		return errors.Annotate(err, "faled to marshal task request").Err()
	}

	fmt.Println(string(b))

	return nil
}

func (c *requestShowRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := c.main(a, args[0]); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
