// Copyright 2019 The LUCI Authors.
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
	"os"

	"google.golang.org/api/googleapi"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/system/signals"
)

// CmdTasks returns an object for the `tasks` subcommand.
func CmdTasks(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "tasks <options>",
		ShortDesc: "lists tasks",
		LongDesc:  "List tasks matching the given options.",
		CommandRun: func() subcommands.CommandRun {
			r := &tasksRun{}
			r.Init(authFlags)
			return r
		},
	}
}

const defaultLimit int64 = 200

type tasksRun struct {
	commonFlags
	outfile string
	limit   int64
	state   string
	tags    []string
	fields  []googleapi.Field
	count   bool
	start   float64
}

func (t *tasksRun) Init(authFlags AuthFlags) {
	t.commonFlags.Init(authFlags)
	t.Flags.StringVar(&t.outfile, "json", "", "Path to output JSON results. Implies quiet.")
	t.Flags.Int64Var(&t.limit, "limit", defaultLimit, "Maximum number of tasks to retrieve.")
	t.Flags.StringVar(&t.state, "state", "ALL", "Only include tasks in the specified state.")
	t.Flags.Var(flag.StringSlice(&t.tags), "tag", "Tag attached to the task. May be repeated.")
	t.Flags.Var(flag.FieldSlice(&t.fields), "field", "Fields to include in a partial response. May be repeated.")
	t.Flags.BoolVar(&t.count, "count", false, "Report the count of tasks instead of listing them.")
	t.Flags.Float64Var(&t.start, "start", 0, "Start time (in seconds since the epoch) for counting tasks.")
}

func (t *tasksRun) Parse() error {
	if err := t.commonFlags.Parse(); err != nil {
		return err
	}
	if t.defaultFlags.Quiet && t.outfile == "" {
		return errors.Reason("specify -json when using -quiet").Err()
	}
	if t.limit < 1 {
		return errors.Reason("invalid -limit %d, must be positive", t.limit).Err()
	}
	if t.outfile != "" {
		t.defaultFlags.Quiet = true
	}
	if t.count {
		if len(t.fields) > 0 {
			return errors.Reason("-field cannot be used with -count").Err()
		}
		if t.limit != defaultLimit {
			return errors.Reason("-limit cannot be used with -count").Err()
		}
		if t.start <= 0 {
			return errors.Reason("with -count, must provide -start >0").Err()
		}
	}
	return nil
}

func (t *tasksRun) main(_ subcommands.Application) error {
	ctx, cancel := context.WithCancel(t.defaultFlags.MakeLoggingContext(os.Stderr))
	signals.HandleInterrupt(cancel)
	service, err := t.createSwarmingClient(ctx)
	if err != nil {
		return err
	}
	var data interface{}
	if t.count {
		if data, err = service.CountTasks(ctx, t.start, t.state, t.tags...); err != nil {
			return err
		}
	} else {
		if data, err = service.ListTasks(ctx, t.limit, t.start, t.state, t.tags, t.fields); err != nil {
			return err
		}
	}
	if !t.defaultFlags.Quiet {
		j, err := json.MarshalIndent(data, "", " ")
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", j)
	}
	if t.outfile != "" {
		j, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if err = os.WriteFile(t.outfile, j, 0644); err != nil {
			return err
		}
	}
	return nil
}

func (t *tasksRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if len(args) != 0 {
		fmt.Fprintf(a.GetErr(), "%s: unknown args: %s\n", a.GetName(), args)
		return 1
	}
	if err := t.Parse(); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := t.main(a); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
