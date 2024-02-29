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
	"fmt"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/output"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/swarming/client/swarming"
)

// CmdDeleteBots returns an object for the `delete-bots` subcommand.
func CmdDeleteBots(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "delete-bots -S <server> <bot ID> <bot ID> ...",
		ShortDesc: "deletes bots given their IDs",
		LongDesc:  "Deletes bots given their IDs.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &deleteBotsImpl{}, base.Features{
				MinArgs: 1,
				MaxArgs: base.Unlimited,
				OutputJSON: base.OutputJSON{
					Enabled: false,
				},
			})
		},
	}
}

type deleteBotsImpl struct {
	force  bool
	botIDs []string
}

func (cmd *deleteBotsImpl) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.force, "force", false, "If set, skip confirmation prompt.")
	fs.BoolVar(&cmd.force, "f", false, "Alias for -force.")
}

func (cmd *deleteBotsImpl) ParseInputs(args []string, env subcommands.Env) error {
	cmd.botIDs = args
	return nil
}

func (cmd *deleteBotsImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	if !cmd.force {
		fmt.Println("Delete the following bots?")
		for _, botID := range cmd.botIDs {
			fmt.Println(botID)
		}
		var res string
		fmt.Println("Continue? [y/N] ")
		_, err := fmt.Scan(&res)
		if err != nil {
			return errors.Annotate(err, "error receiving your response").Err()
		}
		if res != "y" && res != "Y" {
			fmt.Println("canceled deleting bots, Goodbye")
			return nil
		}
	}

	// TODO(vadimsh): Run in parallel.
	for _, botID := range cmd.botIDs {
		res, err := svc.DeleteBot(ctx, botID)
		if err != nil {
			logging.Errorf(ctx, "Failed deleting %s", botID)
			return err
		}
		if !res.Deleted {
			return errors.Reason("bot %s was not deleted", botID).Err()
		}
		logging.Infof(ctx, "Successfully deleted %s", botID)
	}
	return nil
}
