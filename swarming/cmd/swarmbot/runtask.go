// Copyright 2020 The LUCI Authors.
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
	"fmt"

	"github.com/maruel/subcommands"
)

// run_isolated.py in Go
var cmdRunTask = &subcommands.Command{
	UsageLine: "runtask <options>...",
	ShortDesc: "runs swarming task",
	CommandRun: func() subcommands.CommandRun {
		return &cmdRunTaskRun{}
	},
}

type cmdRunTaskRun struct {
	subcommands.CommandRunBase
}

func (c *cmdRunTaskRun) Run(
	a subcommands.Application, args []string, env subcommands.Env) int {
	// TODO(crbug.com/962804): implement here
	fmt.Println("running swarming task...")
	return 0
}
