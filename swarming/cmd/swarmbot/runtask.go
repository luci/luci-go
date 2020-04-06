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
	"context"
	"fmt"
	"time"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/cmdrunner"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
)

// run_isolated.py in Go
var cmdRunTask = &subcommands.Command{
	UsageLine: "runtask <options>...",
	ShortDesc: "runs swarming task",
	CommandRun: func() subcommands.CommandRun {
		c := cmdRunTaskRun{}
		c.Flags.BoolVar(&c.rawCmd, "raw-cmd", true, "When set, the command after -- is run on the bot. Note that this overrides any command in the .isolated file.")
		return &c
	},
}

type cmdRunTaskRun struct {
	subcommands.CommandRunBase
	rawCmd bool
}

func (c *cmdRunTaskRun) Run(
	a subcommands.Application, args []string, env subcommands.Env) int {
	// TODO(crbug.com/962804): implement here
	ctx := context.Background()

	if err := c.Parse(args); err != nil {
		printError(a, err)
	}

	return runCommand(ctx, args, env)
}

func (c *cmdRunTaskRun) Parse(args []string) error {
	if c.rawCmd && len(args) == 0 {
		return errors.Reason("arguments with -raw-cmd should be passed after -- as command delimiter").Err()
	}
	return nil
}

func runCommand(ctx context.Context, args []string, env subcommands.Env) int {
	commands := args
	cwd := "."
	cmdEnv := environ.System()
	hardTimeout := time.Minute
	gracePeriod := time.Minute
	lowerPriority := false
	containment := false

	exitCode, err := cmdrunner.Run(
		ctx, commands, cwd, cmdEnv, hardTimeout, gracePeriod, lowerPriority, containment)
	if err != nil {
		// TODO: log error
		return 1
	}
	return exitCode
}

func printError(a subcommands.Application, err error) {
	fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
}
