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
	"context"
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/exitcode"

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
	ctx := cli.GetContext(a, c, env)
	if err := RunShared(ctx, env, args); err != nil {
		if exitCode, exitCodePresent := exitcode.Get(err); exitCodePresent {
			return exitCode
		}

		// The error pertains to this binary rather than the executed command.
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}

// RunShared runs the command with the specified environment while holding a
// shared mmutex lock.
func RunShared(ctx context.Context, env subcommands.Env, command []string) error {
	ctx, cancel := clock.WithTimeout(ctx, lib.DefaultCommandTimeout)
	defer cancel()

	logging.Infof(ctx, "[mmutex] Running command in SHARED mode: %s", command)
	return lib.RunShared(ctx, env, func(ctx context.Context) error {
		return runCommand(ctx, command)
	})
}
