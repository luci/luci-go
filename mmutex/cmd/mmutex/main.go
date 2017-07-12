// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"os"

	"github.com/maruel/subcommands"
)

// TODO(charliea): Compute this path from $MMUTEX_LOCK_DIR rather than using a constant.
const lockFilePath = "/tmp/lock"

var application = &subcommands.DefaultApplication{
	Name: "mmutex",
	Title: `'Maintenance Mutex' - Global mutex to isolate maintenance tasks.

mmutex is a command line tool that helps prevent maintenance tasks from running
during user tasks. The tool does this by way of a global lock file that users
must acquire before running their tasks.

Clients can use this tool to request that their task be run with one of two
types of access to the system:

  * Exclusive access guarantees that no other callers have any access
    exclusive or shared) to the resource while the specified command is run.
  * Shared access guarantees that only other callers with shared access
    will have access to the resource while the specified command is run.

In short, exclusive access guarantees a task is run alone, while shared access
tasks may be run alongside other shared access tasks.

The source for mmutex lives at:
  https://github.com/luci/luci-go/tree/master/mmutex`,
	Commands: []*subcommands.Command{
		cmdExclusive,
		subcommands.CmdHelp,
	},
}

func main() {
	os.Exit(subcommands.Run(application, nil))
}
