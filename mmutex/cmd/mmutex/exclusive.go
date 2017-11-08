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

var cmdExclusive = &subcommands.Command{
	UsageLine: "exclusive [options] -- <command>",
	ShortDesc: "acquires an exclusive lock before running the command",
	CommandRun: func() subcommands.CommandRun {
		c := &cmdExclusiveRun{}
		c.Flags.DurationVar(&c.fslockTimeout, "fslock-timeout", 2*time.Hour, "Lock acquisition timeout")
		c.Flags.DurationVar(&c.fslockPollingInterval, "fslock-polling-interval", 5*time.Second, "Lock acquisition polling interval")
		return c
	},
}

type cmdExclusiveRun struct {
	subcommands.CommandRunBase
	fslockTimeout         time.Duration
	fslockPollingInterval time.Duration
}

func (c *cmdExclusiveRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	lockFilePath, err := computeLockFilePath(env)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	drainFilePath, err := computeLockFilePath(env)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err = RunExclusive(args, lockFilePath, drainFilePath, c.fslockTimeout, c.fslockPollingInterval); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return lib.GetExitCode(exitErr)
		}

		// We encountered an error that's unrelated to the command itself.
		fmt.Fprintln(os.Stderr, err)
		return 1
	} else {
		return 0
	}
}

func RunExclusive(command []string, lockFilePath string, drainFilePath string, timeout time.Duration, pollingInterval time.Duration) error {
	runFunc := func() error {
		return runCommand(command)
	}
	return runExclusive(runFunc, lockFilePath, drainFilePath, timeout, pollingInterval)
}

func runExclusive(runFunc func() error, lockFilePath string, drainFilePath string, timeout time.Duration, pollingInterval time.Duration) error {
	if len(lockFilePath) == 0 || len(drainFilePath) == 0 {
		return runFunc()
	}

	// TODO(charliea): Wait until no drain file exists...
	// TODO(charliea): Create the drain file.

	// _, err := os.OpenFile(drainFilePath, os.O_CREATE, 0755)
	// if err != nil {
	// 	// TODO(charliea): Consider wrapping this error to add context about when it happened.
	// 	return err
	// }
	// defer os.Remove(drainFilePath)

	blocker := lib.CreateBlockerUntil(time.Now().Add(timeout), pollingInterval)
	return fslock.WithBlocking(lockFilePath, blocker, func() error {
		return runFunc()
	})
}
