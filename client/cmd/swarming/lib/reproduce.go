// Copyright 2021 The LUCI Authors.
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

package lib

import (
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/errors"
)

// CmdReproduce returns an object fo the `reproduce` subcommand.
func CmdReproduce(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "reproduce <task ID> -S <server>",
		ShortDesc: "reproduces a task locally",
		LongDesc:  "Fetches a TaskRequest and runs the same commands that were run on the bot.",
		CommandRun: func() subcommands.CommandRun {
			r := &reproduceRun{}
			r.Init(authFlags)
			return r
		},
	}
}

type reproduceRun struct {
	commonFlags
	work string
}

func (c *reproduceRun) Init(authFlags AuthFlags) {
	c.commonFlags.Init(authFlags)

	c.Flags.StringVar(&c.work, "work", "work", "Directory to map the task input files into.")
	// TODO(crbug.com/1027071): support cache and output directories.
}

func (c *reproduceRun) Parse(args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.Reason("must specify exactly one task id.").Err()
	}
	return nil
}

func (c *reproduceRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if err := c.Parse(args); err != nil {
		printError(a, err)
		return 1
	}
	if err := c.main(a, args, env); err != nil {
		printError(a, err)
		return 1
	}
	return 0
}

func (c *reproduceRun) main(a subcommands.Application, args []string, env subcommands.Env) error {
	// TODO(crbug.com/1027071): set up environment and execute commands.
	return nil
}
