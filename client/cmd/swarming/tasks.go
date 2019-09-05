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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"google.golang.org/api/googleapi"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/system/signals"
)

func cmdTasks(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "tasks <options>",
		ShortDesc: "lists tasks",
		LongDesc:  "List tasks matching the given options.",
		CommandRun: func() subcommands.CommandRun {
			r := &tasksRun{}
			r.Init(defaultAuthOpts)
			return r
		},
	}
}

type tasksRun struct {
	commonFlags
	outfile string
	limit   int64
	state   string
	tags    []string
	fields  []googleapi.Field
}

func (t *tasksRun) Init(defaultAuthOpts auth.Options) {
	t.commonFlags.Init(defaultAuthOpts)

	t.Flags.StringVar(&t.outfile, "json", "", "Path to output JSON results. Implies quiet.")
	t.Flags.Int64Var(&t.limit, "limit", 200, "Maximum number of tasks to retrieve.")
	t.Flags.StringVar(&t.state, "state", "ALL", "Only include tasks in the specified state.")
	t.Flags.Var(flag.StringSlice(&t.tags), "tag", "Tag attached to the task. May be repeated.")
	t.Flags.Var(flag.FieldSlice(&t.fields), "field", "Fields to include in a partial response. May be repeated.")
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
	return nil
}

func (t *tasksRun) main(a subcommands.Application) error {
	ctx, cancel := context.WithCancel(t.defaultFlags.MakeLoggingContext(os.Stderr))
	signals.HandleInterrupt(cancel)
	client, err := t.createAuthClient(ctx)
	if err != nil {
		return err
	}
	s, err := swarming.New(client)
	if err != nil {
		return err
	}
	s.BasePath = t.commonFlags.serverURL + "/_ah/api/swarming/v1/"
	call := s.Tasks.List().Limit(t.limit).State(t.state).Tags(t.tags...)
	// If no fields are specified, all fields will be returned. If any fields are
	// specified, ensure the cursor is specified so we can get subsequent pages.
	if len(t.fields) > 0 {
		t.fields = append(t.fields, "cursor")
	}
	call.Fields(t.fields...)
	// Keep calling as long as there's a cursor indicating more tasks to list.
	// Create an empty array, so that if saved to t.outfile, it's an empty list,
	// not null.
	var tasks []*swarming.SwarmingRpcsTaskResult
	for {
		result, err := call.Do()
		if err != nil {
			return err
		}
		tasks = append(tasks, result.Items...)
		if result.Cursor == "" || int64(len(tasks)) >= t.limit || len(result.Items) == 0 {
			break
		}
		call.Cursor(result.Cursor)
	}
	if int64(len(tasks)) > t.limit {
		tasks = tasks[0:t.limit]
	}
	if !t.defaultFlags.Quiet {
		j, err := json.MarshalIndent(tasks, "", " ")
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", j)
	}
	if t.outfile != "" {
		j, err := json.Marshal(tasks)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(t.outfile, j, 0644); err != nil {
			return err
		}
	}
	return nil
}

func (t *tasksRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := t.Parse(); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	cl, err := t.defaultFlags.StartTracing()
	if err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	defer cl.Close()
	if err := t.main(a); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
