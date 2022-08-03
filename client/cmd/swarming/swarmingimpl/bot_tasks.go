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
	"io/ioutil"
	"os"
	"google.golang.org/api/googleapi"

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

type botTasksRun struct {
	commonFlags
	botID   string
	limit   int64
	state   string
	outfile string
	fields  []googleapi.Field
	start   float64
}

func (b *botTasksRun) Init(authFlags AuthFlags) {
	b.commonFlags.Init(authFlags)
	b.Flags.StringVar(&b.botID, "id", "", "Bot ID to query for.")
	b.Flags.Int64Var(&b.limit, "limit", defaultLimit, "Max number of tasks to return.")
	b.Flags.StringVar(&b.state, "state", "", "Bot task state to filter to.")
	b.Flags.StringVar(&b.outfile, "json", "", "Path to output JSON results. Implies quiet.")
	b.Flags.Var(flag.FieldSlice(&b.fields), "field", "Fields to include in a partial response. May be repeated.")
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
	return nil
}

func (b *botTasksRun) main(_ subcommands.Application) error {
	ctx, cancel := context.WithCancel(b.defaultFlags.MakeLoggingContext(os.Stderr))
	defer cancel()
	defer signals.HandleInterrupt(cancel)()
	service, err := b.createSwarmingClient(ctx)
	if err != nil {
		return err
	}

	var data interface{}
	data, err = service.ListBotTasks(ctx, b.botID, b.limit, b.start, b.state, b.fields)
	if err != nil {
		return err
	}

	if !b.defaultFlags.Quiet {
		j, err := json.MarshalIndent(data, "", " ")
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", j)
	}
	if b.outfile != "" {
		j, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(b.outfile, j, 0644); err != nil {
			return err
		}
	}
	return nil
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