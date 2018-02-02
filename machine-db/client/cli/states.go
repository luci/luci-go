// Copyright 2018 The LUCI Authors.
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

package cli

import (
	"fmt"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/machine-db/api/common/v1"
)

// GetStatesCmd is the command to get states.
type GetStatesCmd struct {
	subcommands.CommandRunBase
	prefix string
}

// Run runs the command to get state.
func (c *GetStatesCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	if c.prefix == "" {
		for _, state := range common.ValidStates() {
			fmt.Println(state.Name())
		}
		return 0
	}
	state, err := common.GetState(c.prefix)
	if err != nil {
		return 1
	}
	fmt.Println(state.Name())
	return 0
}

// getStatesCmd returns a command to get states.
func getStatesCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-states [-prefix <prefix>]",
		ShortDesc: "retrieves states",
		LongDesc:  "Retrieves the state matching the given prefix, or all states if prefix is omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetStatesCmd{}
			cmd.Flags.StringVar(&cmd.prefix, "prefix", "", "Prefix to get the matching state for.")
			return cmd
		},
	}
}
