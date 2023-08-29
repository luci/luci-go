// Copyright 2022 The LUCI Authors.
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
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/system/signals"
)

// CmdBotTasks returns an object for the `bot-tasks` subcommand.
func CmdBotTasks(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "bot-tasks <options>",
		ShortDesc: "List bot tasks.",
		LongDesc:  "Read bot task details.",
		CommandRun: func() subcommands.CommandRun {
			r := &botTasksRun{}
			r.Init(authFlags)
			return r
		},
	}
}

// TODO(crbug.com/1467263): `fields` do nothing currently. Used to be a set of
// fields to include in a partial response.

type botTasksRun struct {
	commonFlags
	botID   string
	limit   int
	state   string
	outfile string
	fields  []string
	start   float64
}

func (b *botTasksRun) Init(authFlags AuthFlags) {
	b.commonFlags.Init(authFlags)
	b.Flags.StringVar(&b.botID, "id", "", "Bot ID to query for.")
	b.Flags.IntVar(&b.limit, "limit", defaultLimit, "Max number of tasks to return.")
	b.Flags.StringVar(&b.state, "state", "ALL", "Bot task state to filter to.")
	b.Flags.StringVar(&b.outfile, "json", "", "Path to output JSON results. Implies quiet.")
	b.Flags.Var(flag.StringSlice(&b.fields), "field", "This flag currently does nothing (https://crbug.com/1467263).")
	b.Flags.Float64Var(&b.start, "start", 0, "Start time (in seconds since the epoch) for counting tasks.")
}

func (b *botTasksRun) Parse() error {
	if err := b.commonFlags.Parse(); err != nil {
		return err
	}
	if b.botID == "" {
		return errors.Reason("non-empty -id required").Err()
	}
	if b.defaultFlags.Quiet && b.outfile == "" {
		return errors.Reason("specify -json when using -quiet").Err()
	}
	if b.outfile != "" {
		b.defaultFlags.Quiet = true
	}
	if b.limit < 1 {
		return errors.Reason("invalid -limit %d, must be positive", b.limit).Err()
	}
	if _, err := stateMap(b.state); err != nil {
		return err
	}
	return nil
}

func (b *botTasksRun) botTasks(ctx context.Context, service swarmingService, out io.Writer) error {
	state, err := stateMap(b.state)
	if err != nil {
		return err
	}
	data, err := service.ListBotTasks(ctx, b.botID, int32(b.limit), b.start, state)
	if err != nil {
		return err
	}
	toOutput := make([]*taskResultResponse, len(data))
	for idx, tr := range data {
		toOutput[idx] = &taskResultResponse{
			Proto: tr,
		}
	}
	output, err := json.MarshalIndent(toOutput, "", DefaultIndent)
	if err != nil {
		return err
	}
	_, err = out.Write(append(output, '\n'))
	return err
}

func (b *botTasksRun) main(_ subcommands.Application) error {
	ctx, cancel := context.WithCancel(b.defaultFlags.MakeLoggingContext(os.Stderr))
	defer cancel()
	defer signals.HandleInterrupt(cancel)()
	service, err := b.createSwarmingClient(ctx)
	if err != nil {
		return err
	}
	return writeOutput(b.outfile, b.defaultFlags.Quiet, func(out io.Writer) error {
		return b.botTasks(ctx, service, out)
	})
}

func (b *botTasksRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if len(args) != 0 {
		fmt.Fprintf(a.GetErr(), "%s: unknown args: %s\n", a.GetName(), args)
		return 1
	}
	if err := b.Parse(); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := b.main(a); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
