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
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging/gologger"
)

var application = &cli.Application{
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
	Context: gologger.StdConfig.Use,
	Commands: []*subcommands.Command{
		cmdExclusive,
		cmdShared,
		subcommands.CmdHelp,
	},
	EnvVars: map[string]subcommands.EnvVarDefinition{
		"MMUTEX_LOCK_DIR": {
			ShortDesc: "The directory containing the lock and drain files.",
			Default:   "",
		},
	},
}

func main() {
	os.Exit(subcommands.Run(application, nil))
}
