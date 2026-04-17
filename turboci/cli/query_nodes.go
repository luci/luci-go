// Copyright 2026 The LUCI Authors.
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

import "github.com/maruel/subcommands"

func cmdQueryNodes(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `query-nodes [flags]`,
		ShortDesc: "query TurboCI nodes",
		LongDesc:  "Query TurboCI nodes.",
		CommandRun: func() subcommands.CommandRun {
			r := &queryNodesRun{}
			return r
		},
	}
}

type queryNodesRun struct {
	baseCommandRun
}

func (r *queryNodesRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	panic("not implemented")
}
