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
	"flag"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/output"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/swarming/client/swarming"
)

// CmdRequestShow returns an object for the `request-show` subcommand.
func CmdRequestShow(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "request-show -S <server> <task ID>",
		ShortDesc: "shows task request details",
		LongDesc:  "Shows the properties, what, when, by who, about a task request.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &requestShowImpl{}, base.Features{
				MinArgs: 1,
				MaxArgs: 1,
				OutputJSON: base.OutputJSON{
					Enabled:         true,
					DefaultToStdout: true,
				},
			})
		},
	}
}

type requestShowImpl struct {
	taskID string
}

func (cmd *requestShowImpl) RegisterFlags(fs *flag.FlagSet) {
	// Nothing.
}

func (cmd *requestShowImpl) ParseInputs(ctx context.Context, args []string, env subcommands.Env, extra base.Extra) error {
	cmd.taskID = args[0]
	return nil
}

func (cmd *requestShowImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	request, err := svc.TaskRequest(ctx, cmd.taskID)
	if err != nil {
		return errors.Annotate(err, "failed to get task request. task ID = %s", cmd.taskID).Err()
	}
	return output.Proto(sink, request)
}
