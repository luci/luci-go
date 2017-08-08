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
		c := &cmdSharedRun{}
		c.Flags.DurationVar(&c.fslockTimeout, "fslock-timeout", 2*time.Hour, "Lock acquisition timeout")
		c.Flags.DurationVar(&c.fslockPollingInterval, "fslock-polling-interval", 5*time.Second, "Lock acquisition polling interval")
		return c
	},
}

type cmdSharedRun struct {
	subcommands.CommandRunBase
	fslockTimeout         time.Duration
	fslockPollingInterval time.Duration
}

func (c *cmdSharedRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if err := RunShared(args, c.fslockTimeout, c.fslockPollingInterval); err != nil {
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

func RunShared(command []string, timeout time.Duration, pollingInterval time.Duration) error {
	blocker := lib.CreateBlockerUntil(time.Now().Add(timeout), pollingInterval)
	return fslock.WithSharedBlocking(LockFilePath, blocker, func() error {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		return cmd.Run()
	})
}
