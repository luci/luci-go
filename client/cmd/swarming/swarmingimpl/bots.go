// Copyright 2018 The LUCI Authors.
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
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/flag/stringmapflag"
	"go.chromium.org/luci/swarming/client/swarming"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

// CmdBots returns an object for the `bots` subcommand.
func CmdBots(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "bots -S <server>",
		ShortDesc: "lists or counts bots matching a filter",
		LongDesc:  "Lists or counts bots matching a filter.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &botsImpl{}, base.Features{
				MinArgs: 0,
				MaxArgs: 0,
				OutputJSON: base.OutputJSON{
					Enabled:             true,
					DeprecatedAliasFlag: "json",
					DefaultToStdout:     true,
				},
			})
		},
	}
}

// TODO(crbug.com/1467263): `fields` do nothing currently. Used to be a set of
// fields to include in a partial response.

type botsImpl struct {
	dimensions stringmapflag.Value
	fields     []string
	count      bool
	botIDOnly  bool
}

func (cmd *botsImpl) RegisterFlags(fs *flag.FlagSet) {
	fs.Var(&cmd.dimensions, "dimension", "Dimension to select the right kind of bot. In the form of `key=value`.")
	fs.Var(luciflag.StringSlice(&cmd.fields), "field", "This flag currently does nothing (https://crbug.com/1467263).")
	fs.BoolVar(&cmd.count, "count", false, "Report the count of bots instead of listing them.")
	fs.BoolVar(&cmd.botIDOnly, "bare", false, "Print bot IDs to stdout as a list.")
}

func (cmd *botsImpl) ParseInputs(args []string, env subcommands.Env) error {
	if cmd.count && len(cmd.fields) > 0 {
		return errors.Reason("-field cannot be used with -count").Err()
	}
	if cmd.count && cmd.botIDOnly {
		return errors.Reason("-bare cannot be used with -count").Err()
	}
	return nil
}

func (cmd *botsImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	// TODO(vadimsh): Reuse from utils.
	dims := make([]*swarmingv2.StringPair, 0, len(cmd.dimensions))
	for k, v := range cmd.dimensions {
		dims = append(dims, &swarmingv2.StringPair{
			Key:   k,
			Value: v,
		})
	}

	if cmd.count {
		count, err := svc.CountBots(ctx, dims)
		if err != nil {
			return err
		}
		return output.Proto(sink, count)
	}

	bots, err := svc.ListBots(ctx, dims)
	if err != nil {
		return err
	}

	if cmd.botIDOnly {
		for _, bot := range bots {
			fmt.Fprintln(extra.Stdout, bot.GetBotId())
		}
		// Skip writing JSON to the stdout, since we already written to stdout.
		if extra.OutputJSON == "-" {
			return nil
		}
	}

	return output.List(sink, bots)
}
