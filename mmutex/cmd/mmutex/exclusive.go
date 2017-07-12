// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"github.com/luci/luci-go/mmutex/lib"
)

var cmdExclusive = &subcommands.Command{
	UsageLine: "exclusive -- <command>",
	ShortDesc: "acquires an exclusive lock before running the command",
	CommandRun: func() subcommands.CommandRun {
		return &cmdExclusiveRun{}
	},
}

type cmdExclusiveRun struct {
	subcommands.CommandRunBase
}

func (c *cmdExclusiveRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	// TODO(charliea): Make sure that `args` has length greater than zero.

	if err := lib.AcquireExclusiveLock(lockFilePath); err != nil {
		fmt.Fprintf(os.Stderr, "error acquiring exclusive lock: %s\n", err)
		return 1
	}

	return 0
}
