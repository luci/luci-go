// Copyright 2017 The LUCI Authors.
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
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/mmutex/lib"
)

var cmdExclusive = &subcommands.Command{
	UsageLine: "exclusive [options] -- <command>",
	ShortDesc: "acquires an exclusive lock before running the command",
	CommandRun: func() subcommands.CommandRun {
		return &cmdExclusiveRun{}
	},
}

type cmdExclusiveRun struct {
	subcommands.CommandRunBase
}

func (c *cmdExclusiveRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if err := RunExclusive(env, args); err != nil {
		if exitCode, exitCodePresent := exitcode.Get(err); exitCodePresent {
			return exitCode
		}

		// We encountered an error that's unrelated to the command itself.
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}

// RunExclusive runs the command with the specified environment while holding an
// exclusive mmutex lock.
func RunExclusive(env subcommands.Env, command []string) error {
	return lib.RunExclusive(env, func() error {
		return runCommand(command)
	})
}
