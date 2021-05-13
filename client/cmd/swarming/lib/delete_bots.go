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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
)

// CmdBots returns an object for the `bots` subcommand.
func CmdDeleteBots(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "delete-bots <options> <botID1 botID2 ...>",
		ShortDesc: "delete bots",
		LongDesc:  "delete bots specified by bot ID",
		CommandRun: func() subcommands.CommandRun {
			r := &deletebotsRun{}
			r.Init(authFlags)
			return r
		},
	}
}

type deletebotsRun struct {
	commonFlags
	force bool
}

func (b *deletebotsRun) Init(authFlags AuthFlags) {
	b.commonFlags.Init(authFlags)
	b.Flags.BoolVar(&b.force, "force", false, "Forcibly deletes bots")
	b.Flags.BoolVar(&b.force, "f", false, "Alias for -force")
}

func (b *deletebotsRun) Parse(botIDs []string) error {
	if err := b.commonFlags.Parse(); err != nil {
		return err
	}

	if len(botIDs) == 0 {
		return errors.Reason("must specify at least one swarming bot id").Err()
	}

	return nil
}

func (b *deletebotsRun) deleteBotsInList(ctx context.Context, botIDs []string, service swarmingService) error {
	if !b.force {
		fmt.Println("Delete the following bots?")
		for _, botID := range botIDs {
			fmt.Println(botID)
		}
		var res string
		fmt.Println("Continue? [y/N] ")
		fmt.Scan(&res)
		if res != "y" && res != "Y" {
			return errors.Reason("Canceled deleting bots, Goodbye").Err()
		}
	}

	for _, botID := range botIDs {
		_, err := service.DeleteBots(ctx, botID)
		if err != nil {
			fmt.Printf("Failed Deleting %s\n", botID)
			return err
		}
		fmt.Printf("Successfully Deleted %s\n", botID)
	}
	return nil

}

func (b *deletebotsRun) main(_ subcommands.Application, botIDs []string) error {
	ctx, cancel := context.WithCancel(b.defaultFlags.MakeLoggingContext(os.Stderr))
	signals.HandleInterrupt(cancel)
	service, err := b.createSwarmingClient(ctx)
	if err != nil {
		return err
	}
	return b.deleteBotsInList(ctx, botIDs, service)
}

func (b *deletebotsRun) Run(a subcommands.Application, botIDs []string, _ subcommands.Env) int {
	fmt.Println(a)
	if err := b.Parse(botIDs); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := b.main(a, botIDs); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
