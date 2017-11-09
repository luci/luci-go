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
	"os/exec"
	"time"

	"github.com/danjacques/gofslock/fslock"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/mmutex/lib"
)

var cmdShared = &subcommands.Command{
	UsageLine: "shared [options] -- <command>",
	ShortDesc: "acquires a shared lock before running the command",
	CommandRun: func() subcommands.CommandRun {
		return &cmdSharedRun{}
	},
}

type cmdSharedRun struct {
	subcommands.CommandRunBase
}

func (c *cmdSharedRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if err := RunShared(args, env); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return lib.GetExitCode(exitErr)
		}

		// The error pertains to this binary rather than the executed command.
		fmt.Fprintln(os.Stderr, err)
		return 1
	} else {
		return 0
	}
}

func RunShared(command []string, env subcommands.Env) error {
	lockFilePath, err := computeLockFilePath(env)
	if err != nil {
		return err
	}
	if len(lockFilePath) == 0 {
		return runCommand(command)
	}

	// TODO(charliea): Replace fslockTimeout with a Context.
	blocker := lib.CreateBlockerUntil(time.Now().Add(fslockTimeout), fslockPollingInterval)
	return fslock.WithSharedBlocking(lockFilePath, blocker, func() error {
		return runCommand(command)
	})
}
