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

package main

import (
	"errors"
	"fmt"

	"github.com/kr/pretty"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
)

func cmdRequestShow(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "request-show <task_id>",
		ShortDesc: "returns properties of a request",
		LongDesc:  "Returns the properties, what, when, by who, about a request on the Swarming server.",
		CommandRun: func() subcommands.CommandRun {
			r := &requestShowRun{}
			r.Init(defaultAuthOpts)
			return r
		},
	}
}

type requestShowRun struct {
	commonFlags
}

func (c *requestShowRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 1 {
		return errors.New("must only provide a task id")
	}
	return nil
}

func (c *requestShowRun) main(a subcommands.Application, taskid string) error {
	client, err := c.createAuthClient()
	if err != nil {
		return err
	}

	s, err := swarming.New(client)
	if err != nil {
		return err
	}
	s.BasePath = c.commonFlags.serverURL + "/api/swarming/v1/"

	call := s.Task.Request(taskid)
	result, err := call.Do()

	pretty.Println(result)

	return err
}

func (c *requestShowRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	cl, err := c.defaultFlags.StartTracing()
	if err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	defer cl.Close()
	if err := c.main(a, args[0]); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
